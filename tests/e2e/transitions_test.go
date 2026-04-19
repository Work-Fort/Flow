// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestTransitions_MoveTodoToReview(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-T")

	var wi struct {
		ID            string `json:"id"`
		CurrentStepID string `json:"current_step_id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "Transition test"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}
	if wi.CurrentStepID != "stp_todo" {
		t.Fatalf("initial step: %q", wi.CurrentStepID)
	}

	// Submit (todo -> review).
	transReq := map[string]any{
		"transition_id":  "trn_submit",
		"actor_agent_id": "a_actor", "actor_role_id": "role_dev",
		"reason": "ready for review",
	}
	var moved struct {
		CurrentStepID string `json:"current_step_id"`
	}
	status, body, err := c.PostJSON("/v1/items/"+wi.ID+"/transition", transReq, &moved)
	if err != nil || status != http.StatusOK {
		t.Fatalf("transition: status=%d err=%v body=%s", status, err, body)
	}
	if moved.CurrentStepID != "stp_review" {
		t.Errorf("after submit: %q, want stp_review", moved.CurrentStepID)
	}

	// History should now have one entry.
	var hist []struct {
		FromStepID   string `json:"from_step_id"`
		ToStepID     string `json:"to_step_id"`
		TransitionID string `json:"transition_id"`
		TriggeredBy  string `json:"triggered_by"`
		Reason       string `json:"reason"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wi.ID+"/history", &hist)
	if err != nil || status != http.StatusOK {
		t.Fatalf("history: status=%d err=%v", status, err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len: %d", len(hist))
	}
	h := hist[0]
	if h.FromStepID != "stp_todo" || h.ToStepID != "stp_review" {
		t.Errorf("history step ids: from=%q to=%q", h.FromStepID, h.ToStepID)
	}
	if h.TransitionID != "trn_submit" || h.Reason != "ready for review" {
		t.Errorf("history meta: %+v", h)
	}
}

func TestTransitions_InvalidTransitionRejected(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-INV")

	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "X"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}

	// trn_approve only fires from stp_review; firing it from stp_todo
	// must be rejected.
	transReq := map[string]any{
		"transition_id":  "trn_approve",
		"actor_agent_id": "a_actor", "actor_role_id": "role_dev",
	}
	status, _, _ := c.PostJSON("/v1/items/"+wi.ID+"/transition", transReq, nil)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("invalid transition: status=%d, want 422", status)
	}
}
