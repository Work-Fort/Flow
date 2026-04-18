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

// FakeHive holds the seeded agents and roles. Tests mutate the maps
// before starting the server. The fake serves wire-format JSON only;
// it does not run any business logic.
type FakeHive struct {
	mu     sync.RWMutex
	agents map[string]HiveAgentWithRoles // by agent ID
	roles  map[string]HiveRole           // by role ID
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

// Start begins serving on a random port. Returns the base URL and a
// stop function.
func (h *FakeHive) Start() (baseURL string, stop func()) {
	mux := http.NewServeMux()

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

// writeJSON is shared with fake_sharkfin.go.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
