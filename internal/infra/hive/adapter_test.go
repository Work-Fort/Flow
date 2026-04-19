// SPDX-License-Identifier: GPL-2.0-only
package hive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHiveAdapter_OutboundIsApiKeyV1 asserts that the Hive adapter sends the
// service token under the ApiKey-v1 Authorization scheme. This is the
// construction-side wire-format guard for the Cluster 3b scheme-split:
// Hive's client now sends ApiKey-v1 (parameter renamed token → apiKey,
// signature unchanged). Any future drift in the upstream wire format is
// caught here at Flow's boundary rather than at deploy time.
func TestHiveAdapter_OutboundIsApiKeyV1(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		// Return a minimal AgentWithRoles response so hiveclient doesn't error.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "agent-1",
			"name":    "Test Agent",
			"team_id": "team-1",
			"roles":   []any{},
		})
	}))
	defer srv.Close()

	a := New(srv.URL, "wf-svc_secret")
	// ResolveAgent calls hiveclient.GetAgent which hits GET /v1/agents/:id.
	_, _ = a.ResolveAgent(context.Background(), "agent-1")

	want := "ApiKey-v1 wf-svc_secret"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}
