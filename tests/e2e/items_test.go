// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestWorkItems_CreateGetListUpdate(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-W")

	// Create work item.
	var wi struct {
		ID            string `json:"id"`
		InstanceID    string `json:"instance_id"`
		Title         string `json:"title"`
		CurrentStepID string `json:"current_step_id"`
		Priority      string `json:"priority"`
	}
	createReq := map[string]any{
		"title": "First task", "description": "Body",
		"assigned_agent_id": "a_001", "priority": "high",
	}
	status, body, err := c.PostJSON("/v1/instances/"+instID+"/items", createReq, &wi)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create: status=%d err=%v body=%s", status, err, body)
	}
	if wi.InstanceID != instID || wi.Title != "First task" || wi.Priority != "high" {
		t.Errorf("create response: %+v", wi)
	}
	if wi.CurrentStepID != "stp_todo" {
		t.Errorf("first step id: got %q, want stp_todo", wi.CurrentStepID)
	}

	// List by instance.
	var list []struct {
		ID              string `json:"id"`
		Title           string `json:"title"`
		AssignedAgentID string `json:"assigned_agent_id"`
		Priority        string `json:"priority"`
	}
	status, body, err = c.GetJSON("/v1/instances/"+instID+"/items", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list: status=%d err=%v body=%s", status, err, body)
	}
	if len(list) != 1 || list[0].ID != wi.ID {
		t.Errorf("list: %+v", list)
	}

	// List filtered by agent_id.
	var byAgent []struct {
		ID string `json:"id"`
	}
	status, _, _ = c.GetJSON("/v1/instances/"+instID+"/items?agent_id=a_001", &byAgent)
	if status != http.StatusOK || len(byAgent) != 1 {
		t.Errorf("filter agent_id: status=%d len=%d", status, len(byAgent))
	}
	var byOther []struct {
		ID string `json:"id"`
	}
	status, _, _ = c.GetJSON("/v1/instances/"+instID+"/items?agent_id=a_999", &byOther)
	if status != http.StatusOK || len(byOther) != 0 {
		t.Errorf("filter agent_id no-match: status=%d len=%d", status, len(byOther))
	}

	// Get single.
	var got struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Priority string `json:"priority"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wi.ID, &got)
	if err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v", status, err)
	}
	if got.Title != "First task" || got.Priority != "high" {
		t.Errorf("get: %+v", got)
	}

	// Patch.
	patchReq := map[string]any{"title": "Renamed task"}
	var patched struct {
		Title string `json:"title"`
	}
	status, body, err = c.PatchJSON("/v1/items/"+wi.ID, patchReq, &patched)
	if err != nil || status != http.StatusOK {
		t.Fatalf("patch: status=%d err=%v body=%s", status, err, body)
	}
	if patched.Title != "Renamed task" {
		t.Errorf("patch title: %q", patched.Title)
	}
}

func TestWorkItems_HistoryEmptyBeforeTransition(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-H")

	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "T"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}

	var hist []map[string]any
	status, _, err := c.GetJSON("/v1/items/"+wi.ID+"/history", &hist)
	if err != nil || status != http.StatusOK {
		t.Fatalf("history: status=%d err=%v", status, err)
	}
	if len(hist) != 0 {
		t.Errorf("history before any transition: %+v", hist)
	}
}
