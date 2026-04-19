// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// TestBotLifecycle_RoundTrip simulates the canonical agent-pool
// proof-of-life loop entirely in-process via fakes:
//
//  1. Operator creates a work item in a Flow project (via REST).
//  2. The project bot posts the work item to its channel (simulated
//     here as a Sharkfin webhook into Flow with a flow_command body).
//  3. The bot claims a pool agent from FakeHive (via the scheduler
//     diag claim endpoint).
//  4. The agent runs (simulated by REST transitions through the
//     workflow steps).
//  5. Combine fires a pull_request_merged webhook into Flow's new
//     /v1/webhooks/combine endpoint.
//  6. The bot reports completion (final transition + a flow_command
//     "mark_done" via the Sharkfin webhook).
//  7. The bot releases the pool agent.
//  8. Audit log contains the expected sequence of events.
//
// All client interactions go through the harness Client + raw
// http.Client; no real bot process runs and no real Hive / Sharkfin
// / Combine clients are imported.
func TestBotLifecycle_RoundTrip(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	env.Hive.SeedPoolAgent("a_b_001", "agent-b-1", "team-b")

	tok := env.Daemon.SignJWT("svc-b", "flow-b", "Flow B", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)
	httpc := &http.Client{Timeout: 5 * time.Second}

	const workflowID = "wf_round_trip_001"
	const repoName = "flow"

	// 1. Seed template (POST then PATCH; CreateTemplateInput accepts
	// only name+description per rest_types.go:73-78). Step and
	// transition IDs are client-supplied. The gate step at `rev`
	// supplies an empty approver_role_id, matching the convention
	// in tests/e2e/templates_test.go:293 (the existing approve/reject
	// tests rely on this exact shape).
	const (
		stpDev    = "stp_rt_dev"
		stpRev    = "stp_rt_rev"
		stpQA     = "stp_rt_qa"
		stpMerged = "stp_rt_merged"
		trnD2R    = "trn_rt_d2r"
		trnR2Q    = "trn_rt_r2q"
		trnQ2M    = "trn_rt_q2m"
	)
	var tmpl struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/templates",
		map[string]any{"name": "round-trip", "description": "round trip e2e"},
		&tmpl); err != nil || status != http.StatusCreated {
		t.Fatalf("create template: status=%d body=%s err=%v", status, body, err)
	}
	patch := map[string]any{
		"steps": []map[string]any{
			{"id": stpDev, "key": "dev", "name": "Dev", "type": "task", "position": 0},
			{"id": stpRev, "key": "rev", "name": "Review", "type": "gate", "position": 1,
				"approval": map[string]any{
					"mode": "any", "required_approvers": 1, "approver_role_id": "",
				}},
			{"id": stpQA, "key": "qa", "name": "QA", "type": "task", "position": 2},
			{"id": stpMerged, "key": "merged", "name": "Merged", "type": "task", "position": 3},
		},
		"transitions": []map[string]any{
			{"id": trnD2R, "key": "dev_to_rev", "name": "to review",
				"from_step_id": stpDev, "to_step_id": stpRev},
			{"id": trnR2Q, "key": "rev_to_qa", "name": "to qa",
				"from_step_id": stpRev, "to_step_id": stpQA},
			{"id": trnQ2M, "key": "qa_to_merged", "name": "to merged",
				"from_step_id": stpQA, "to_step_id": stpMerged},
		},
	}
	if status, body, err := c.PatchJSON("/v1/templates/"+tmpl.ID, patch, nil); err != nil ||
		status != http.StatusOK {
		t.Fatalf("seed template: status=%d body=%s err=%v", status, body, err)
	}

	var inst struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances", map[string]any{
		"template_id": tmpl.ID, "team_id": "team-b", "name": "round-trip",
	}, &inst); err != nil || status != http.StatusCreated {
		t.Fatalf("create instance: status=%d body=%s err=%v", status, body, err)
	}
	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+inst.ID+"/items", map[string]any{
		"title": "round-trip wi",
	}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create work item: status=%d body=%s err=%v", status, body, err)
	}

	// 2. Bot posts work-item announce to its channel — simulated as a
	// Sharkfin webhook into Flow.
	postSharkfinCommand(t, c, "claim_for_dev", wi.ID, "agent-bot")

	// 3. Bot claims an agent from the pool.
	claimReq := map[string]any{
		"role": "developer", "project": "flow",
		"workflow_id": workflowID, "lease_ttl_seconds": 60,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil); err != nil ||
		status != http.StatusOK {
		t.Fatalf("claim: status=%d body=%s err=%v", status, body, err)
	}
	if env.Hive.ClaimCalls() != 1 {
		t.Errorf("ClaimCalls = %d, want 1", env.Hive.ClaimCalls())
	}

	// 4. Agent runs: dev -> rev -> approve -> qa -> merged.
	transitionWorkItem(t, c, wi.ID, trnD2R, "agent-bot", "role-dev")

	// Approve auto-transitions from rev (gate) to qa via the configured
	// transition. We do NOT call transitionWorkItem for trnR2Q — approve
	// performs the transition internally.
	approveReq := map[string]any{
		"agent_id": "agent-bot", "comment": "lgtm",
	}
	if status, body, err := c.PostJSON("/v1/items/"+wi.ID+"/approve", approveReq, nil); err != nil ||
		status != http.StatusOK {
		t.Fatalf("approve: status=%d body=%s err=%v", status, body, err)
	}

	// 5. Combine merge webhook fires.
	postCombineMerge(t, env, tok, httpc, repoName, 42, "main", "agent-merge")

	// 6. Final transition + flow_command "mark_done".
	transitionWorkItem(t, c, wi.ID, trnQ2M, "agent-bot", "role-qa")
	postSharkfinCommand(t, c, "mark_done", wi.ID, "agent-bot")

	// 7. Release the agent.
	releaseReq := map[string]any{
		"agent_id": "a_b_001", "workflow_id": workflowID,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil); err != nil ||
		(status != http.StatusOK && status != http.StatusNoContent) {
		t.Fatalf("release: status=%d body=%s err=%v", status, body, err)
	}

	// 8. Audit log assertion.
	var got struct {
		Events []struct {
			Type     string `json:"type"`
			AgentID  string `json:"agent_id"`
			Workflow string `json:"workflow_id"`
		} `json:"events"`
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if status, _, err := c.GetJSON("/v1/audit/_diag/by-workflow/"+workflowID, &got); err == nil && status == 200 {
			if hasType(got.Events, "agent_claimed") &&
				hasType(got.Events, "agent_released") {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	want := []string{"agent_claimed", "agent_released"}
	for _, w := range want {
		if !hasType(got.Events, w) {
			t.Errorf("audit missing type %q (got %+v)", w, got.Events)
		}
	}
}

func hasType(events []struct {
	Type     string `json:"type"`
	AgentID  string `json:"agent_id"`
	Workflow string `json:"workflow_id"`
}, ty string) bool {
	for _, e := range events {
		if e.Type == ty {
			return true
		}
	}
	return false
}

func postSharkfinCommand(t *testing.T, c *harness.Client, action, workItemID, fromAgent string) {
	t.Helper()
	meta := map[string]any{
		"event_type": "flow_command",
		"event_payload": map[string]any{
			"action": action, "work_item_id": workItemID,
		},
	}
	mb, _ := json.Marshal(meta)
	ms := string(mb)
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   time.Now().UnixNano(),
		"channel_id":   1,
		"channel_name": "flow",
		"channel_type": "public",
		"from":         fromAgent,
		"from_type":    "service",
		"body":         "@flow " + action + " " + workItemID,
		"metadata":     ms,
		"sent_at":      time.Now().UTC().Format(time.RFC3339),
	}
	status, body, err := c.PostJSON("/v1/webhooks/sharkfin", payload, nil)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("sharkfin webhook %s: status=%d body=%s err=%v", action, status, body, err)
	}
}

func postCombineMerge(t *testing.T, env *harness.Env, tok string, httpc *http.Client,
	repoName string, prNumber int64, targetBranch, mergedBy string) {
	t.Helper()
	payload := map[string]any{
		"repository":   map[string]any{"name": repoName},
		"pull_request": map[string]any{"number": prNumber, "target_branch": targetBranch},
		"sender":       map[string]any{"username": mergedBy},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, env.Daemon.BaseURL()+"/v1/webhooks/combine", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", "pull_request_merged")
	req.Header.Set("X-SoftServe-Delivery", "del-rt")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := httpc.Do(req)
	if err != nil {
		t.Fatalf("combine merge webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("combine merge webhook status = %d, want 204", resp.StatusCode)
	}
}

func transitionWorkItem(t *testing.T, c *harness.Client, wiID, transitionID, actor, roleID string) {
	t.Helper()
	req := map[string]any{
		"transition_id":  transitionID,
		"actor_agent_id": actor,
		"actor_role_id":  roleID,
		"reason":         "round-trip",
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/transition", req, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("transition %s: status=%d body=%s err=%v", transitionID, status, body, err)
	}
}
