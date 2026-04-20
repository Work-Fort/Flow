// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/Work-Fort/Flow/internal/domain"
)

type fakeHive struct {
	got domain.HiveAgentFilter
	out []domain.HiveAgentRecord
}

func (f *fakeHive) ListAgents(_ context.Context, filter domain.HiveAgentFilter) ([]domain.HiveAgentRecord, error) {
	f.got = filter
	return f.out, nil
}

func (f *fakeHive) ClaimAgent(context.Context, string, string, string, int) (*domain.HiveAgent, error) {
	return nil, nil
}
func (f *fakeHive) ReleaseAgent(context.Context, string, string) error         { return nil }
func (f *fakeHive) RenewAgentLease(context.Context, string, string, int) error { return nil }

func TestListAgents_PassesFiltersThrough(t *testing.T) {
	fh := &fakeHive{out: []domain.HiveAgentRecord{
		{ID: "a1", Name: "agent-1", TeamID: "t1", CurrentWorkflowID: "wf-1"},
		{ID: "a2", Name: "agent-2", TeamID: "t1"},
	}}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	registerAgentRoutes(api, fh)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agents?team_id=t1&assigned=true&role=developer", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	if fh.got.TeamID != "t1" || fh.got.Assigned != "true" || fh.got.Role != "developer" {
		t.Errorf("filter = %+v", fh.got)
	}

	var body struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Agents) != 2 {
		t.Fatalf("agents = %d, want 2", len(body.Agents))
	}
}
