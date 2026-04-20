// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestAuditFiltered_ByWorkflow(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	env.Hive.SeedPoolAgent("a_filt_001", "agent-filt-1", "team-filt")

	tok := env.Daemon.SignJWT("svc-filt", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Produce two events for wi-filt-1 and one for wi-filt-2.
	claim := func(wf string) {
		req := map[string]any{
			"role": "developer", "project": "flow",
			"workflow_id": wf, "lease_ttl_seconds": 60,
		}
		if status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", req, nil); err != nil || status != http.StatusOK {
			t.Fatalf("claim %s: status=%d err=%v body=%s", wf, status, err, body)
		}
		rel := map[string]any{"agent_id": "a_filt_001", "workflow_id": wf}
		if status, body, err := c.PostJSON("/v1/scheduler/_diag/release", rel, nil); err != nil {
			t.Fatalf("release %s: %v", wf, err)
		} else if status != http.StatusOK && status != http.StatusNoContent {
			t.Fatalf("release %s status=%d body=%s", wf, status, body)
		}
	}
	claim("wi-filt-1")
	claim("wi-filt-1")
	claim("wi-filt-2")

	status, body, err := c.GetJSON("/v1/audit?workflow_id=wi-filt-1", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("GET /v1/audit: status=%d err=%v body=%s", status, err, body)
	}
	var resp struct {
		Events []struct {
			WorkflowID string `json:"workflow_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Events) < 2 {
		t.Fatalf("want >=2 events for wi-filt-1, got %d", len(resp.Events))
	}
	for _, e := range resp.Events {
		if e.WorkflowID != "wi-filt-1" {
			t.Errorf("got event for wrong workflow %q", e.WorkflowID)
		}
	}
}
