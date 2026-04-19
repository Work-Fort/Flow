// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestAuditEvents_RecordedThroughDaemon(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	env.Hive.SeedPoolAgent("a_audit_001", "agent-audit-1", "team-audit")

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())

	claimReq := map[string]any{
		"role": "reviewer", "project": "flow",
		"workflow_id": "wf-audit-1", "lease_ttl_seconds": 60,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil); err != nil || status != http.StatusOK {
		t.Fatalf("claim: status=%d err=%v body=%s", status, err, body)
	}

	releaseReq := map[string]any{
		"agent_id": "a_audit_001", "workflow_id": "wf-audit-1",
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil); err != nil {
		t.Fatalf("release: %v", err)
	} else if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("release status=%d body=%s", status, body)
	}

	status, body, err := c.GetJSON("/v1/audit/_diag/by-workflow/wf-audit-1", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("audit list: status=%d err=%v body=%s", status, err, body)
	}
	var resp struct {
		Events []struct {
			Type     string `json:"type"`
			AgentID  string `json:"agent_id"`
			Workflow string `json:"workflow_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(resp.Events) < 2 {
		t.Fatalf("want >= 2 events, got %d (%s)", len(resp.Events), body)
	}
	first, second := resp.Events[0], resp.Events[1]
	if first.Type != "agent_claimed" {
		t.Errorf("event[0].type = %q, want agent_claimed", first.Type)
	}
	if second.Type != "agent_released" {
		t.Errorf("event[1].type = %q, want agent_released", second.Type)
	}
	if first.AgentID != "a_audit_001" {
		t.Errorf("event[0].agent_id = %q, want a_audit_001", first.AgentID)
	}
}
