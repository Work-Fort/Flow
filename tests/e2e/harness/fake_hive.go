// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HiveAgent is the canonical JSON shape Flow's Hive adapter decodes.
// Field names mirror hive/lead/client/types.go (capitalised, matching
// the real Hive server's quirky JSON tags).
type HiveAgent struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	TeamID    string    `json:"TeamID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

type HiveRoleAssignment struct {
	AgentID  string `json:"AgentID"`
	RoleID   string `json:"RoleID"`
	Priority int    `json:"Priority"`
}

type HiveAgentWithRoles struct {
	HiveAgent
	Roles []HiveRoleAssignment `json:"roles"`
}

type HiveRole struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	ParentID  string    `json:"ParentID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// PoolAgent is the wire-shape Hive's claim/release/renew endpoints
// emit and consume. Field tags match hive/lead/client/types.go.
type PoolAgent struct {
	ID                string    `json:"ID"`
	Name              string    `json:"Name"`
	TeamID            string    `json:"TeamID"`
	AssignedRole      string    `json:"AssignedRole,omitempty"`
	CurrentProject    string    `json:"CurrentProject,omitempty"`
	CurrentWorkflowID string    `json:"CurrentWorkflowID,omitempty"`
	LeaseExpiresAt    time.Time `json:"LeaseExpiresAt,omitempty"`
}

// poolState tracks per-agent claim state inside the FakeHive.
type poolState struct {
	role, project, workflowID string
	leaseExpiresAt            time.Time
}

// FakeHive holds the seeded agents and roles. Tests mutate the maps
// before starting the server. The fake serves wire-format JSON only;
// it does not run any business logic.
type FakeHive struct {
	mu           sync.RWMutex
	agents       map[string]HiveAgentWithRoles // by agent ID
	roles        map[string]HiveRole           // by role ID
	pool         map[string]*poolState
	poolMeta     []PoolAgent
	claimCalls   int
	releaseCalls int
	renewCalls   int
}

// NewFakeHive returns an empty FakeHive. Use AddAgent / AddRole to seed.
func NewFakeHive() *FakeHive {
	return &FakeHive{
		agents: make(map[string]HiveAgentWithRoles),
		roles:  make(map[string]HiveRole),
	}
}

func (h *FakeHive) AddAgent(a HiveAgentWithRoles) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[a.ID] = a
}

func (h *FakeHive) AddRole(r HiveRole) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roles[r.ID] = r
}

// SeedPoolAgent registers a free agent the fake will hand out via
// /v1/agents/claim.
func (h *FakeHive) SeedPoolAgent(id, name, teamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pool == nil {
		h.pool = make(map[string]*poolState)
	}
	h.pool[id] = nil // nil = unclaimed
	h.poolMeta = append(h.poolMeta, PoolAgent{ID: id, Name: name, TeamID: teamID})
}

// ClaimCalls / ReleaseCalls / RenewCalls return per-method call counts.
func (h *FakeHive) ClaimCalls() int   { h.mu.RLock(); defer h.mu.RUnlock(); return h.claimCalls }
func (h *FakeHive) ReleaseCalls() int { h.mu.RLock(); defer h.mu.RUnlock(); return h.releaseCalls }
func (h *FakeHive) RenewCalls() int   { h.mu.RLock(); defer h.mu.RUnlock(); return h.renewCalls }

// Start begins serving on a random port. Returns the base URL and a
// stop function.
func (h *FakeHive) Start() (baseURL string, stop func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/agents/claim", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Role            string `json:"role"`
			Project         string `json:"project"`
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeHiveError(w, http.StatusBadRequest, "bad json")
			return
		}
		h.mu.Lock()
		h.claimCalls++
		var picked string
		for _, meta := range h.poolMeta {
			if h.pool[meta.ID] == nil {
				picked = meta.ID
				break
			}
		}
		if picked == "" {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "agent pool exhausted")
			return
		}
		expiry := time.Now().UTC().Add(time.Duration(body.LeaseTTLSeconds) * time.Second)
		h.pool[picked] = &poolState{
			role: body.Role, project: body.Project,
			workflowID: body.WorkflowID, leaseExpiresAt: expiry,
		}
		var meta PoolAgent
		for _, m := range h.poolMeta {
			if m.ID == picked {
				meta = m
				break
			}
		}
		meta.AssignedRole = body.Role
		meta.CurrentProject = body.Project
		meta.CurrentWorkflowID = body.WorkflowID
		meta.LeaseExpiresAt = expiry
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, meta)
	})

	mux.HandleFunc("POST /v1/agents/{id}/release", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			WorkflowID string `json:"workflow_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		h.mu.Lock()
		h.releaseCalls++
		st, ok := h.pool[id]
		if !ok {
			h.mu.Unlock()
			writeHumaError(w, http.StatusNotFound, "agent not found")
			return
		}
		if st == nil || st.workflowID != body.WorkflowID {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "workflow id mismatch")
			return
		}
		h.pool[id] = nil
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /v1/agents/{id}/renew", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		h.mu.Lock()
		h.renewCalls++
		st, ok := h.pool[id]
		if !ok {
			h.mu.Unlock()
			writeHumaError(w, http.StatusNotFound, "agent not found")
			return
		}
		if st == nil || st.workflowID != body.WorkflowID {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "workflow id mismatch")
			return
		}
		st.leaseExpiresAt = time.Now().UTC().Add(time.Duration(body.LeaseTTLSeconds) * time.Second)
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.mu.RLock()
		a, ok := h.agents[id]
		h.mu.RUnlock()
		if !ok {
			writeHiveError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, a)
	})

	mux.HandleFunc("GET /v1/roles/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/roles/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.mu.RLock()
		role, ok := h.roles[id]
		h.mu.RUnlock()
		if !ok {
			writeHiveError(w, http.StatusNotFound, "role not found")
			return
		}
		writeJSON(w, http.StatusOK, role)
	})

	mux.HandleFunc("GET /v1/agents", func(w http.ResponseWriter, r *http.Request) {
		teamID := r.URL.Query().Get("team_id")
		h.mu.RLock()
		out := make([]HiveAgent, 0, len(h.agents))
		for _, a := range h.agents {
			if teamID == "" || a.TeamID == teamID {
				out = append(out, a.HiveAgent)
			}
		}
		h.mu.RUnlock()
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "checks": []any{}})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("fake_hive: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return "http://" + ln.Addr().String(), func() { srv.Close() }
}

func writeHiveError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeHumaError(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

// writeJSON is shared with fake_sharkfin.go.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
