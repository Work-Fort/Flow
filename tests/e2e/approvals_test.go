// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestApprovals_ApprovePath(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-AP")

	wiID := createAndAdvance(t, c, instID, "stp_review")

	approveReq := map[string]any{"agent_id": "a_reviewer", "comment": "LGTM"}
	var approved struct {
		CurrentStepID string `json:"current_step_id"`
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/approve", approveReq, &approved)
	if err != nil || status != http.StatusOK {
		t.Fatalf("approve: status=%d err=%v body=%s", status, err, body)
	}
	if approved.CurrentStepID != "stp_done" {
		t.Errorf("after approve: %q, want stp_done", approved.CurrentStepID)
	}

	// List approvals -- should have one approved entry.
	var list []struct {
		WorkItemID string `json:"work_item_id"`
		AgentID    string `json:"agent_id"`
		Decision   string `json:"decision"`
		Comment    string `json:"comment"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wiID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 1 {
		t.Fatalf("approvals len: %d", len(list))
	}
	got := list[0]
	if got.AgentID != "a_reviewer" || got.Decision != "approved" || got.Comment != "LGTM" {
		t.Errorf("approval entry: %+v", got)
	}
}

func TestApprovals_RejectPath(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-RJ")

	wiID := createAndAdvance(t, c, instID, "stp_review")

	rejectReq := map[string]any{"agent_id": "a_reviewer", "comment": "needs work"}
	var rejected struct {
		CurrentStepID string `json:"current_step_id"`
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/reject", rejectReq, &rejected)
	if err != nil || status != http.StatusOK {
		t.Fatalf("reject: status=%d err=%v body=%s", status, err, body)
	}
	// Verified by reading internal/workflow/service.go:335-397 -- RejectItem
	// with empty RejectionStepID succeeds, records the Approval, and
	// leaves CurrentStepID unchanged. Assert only that the call
	// succeeded and the approval was recorded.
	var list []struct {
		AgentID  string `json:"agent_id"`
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wiID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 1 {
		t.Fatalf("approvals len: %d", len(list))
	}
	if list[0].Decision != "rejected" || list[0].Comment != "needs work" {
		t.Errorf("approval: %+v", list[0])
	}
}

func TestApprovals_ListEmptyBeforeAnyDecision(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-EM")

	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "T"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}

	var list []map[string]any
	status, _, err := c.GetJSON("/v1/items/"+wi.ID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 0 {
		t.Errorf("approvals empty: %+v", list)
	}
}

// createAndAdvance creates a work item and submits it to the named
// step via trn_submit. Used by approval tests that need a work item
// already at the gate.
func createAndAdvance(t *testing.T, c *harness.Client, instID, wantStep string) string {
	t.Helper()
	var wi struct {
		ID            string `json:"id"`
		CurrentStepID string `json:"current_step_id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "to-be-advanced"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}
	transReq := map[string]any{
		"transition_id": "trn_submit", "actor_agent_id": "a_dev", "actor_role_id": "role_dev",
	}
	var moved struct {
		CurrentStepID string `json:"current_step_id"`
	}
	if status, body, err := c.PostJSON("/v1/items/"+wi.ID+"/transition", transReq, &moved); err != nil || status != http.StatusOK {
		t.Fatalf("submit: status=%d err=%v body=%s", status, err, body)
	}
	if moved.CurrentStepID != wantStep {
		t.Fatalf("advance: at %q, want %q", moved.CurrentStepID, wantStep)
	}
	return wi.ID
}
