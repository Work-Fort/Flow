// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestInstances_ListEmpty(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)
	var list []map[string]any
	status, _, err := c.GetJSON("/v1/instances", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if len(list) != 0 {
		t.Errorf("want 0, got %d", len(list))
	}
}

func TestInstances_CreateGetUpdateList(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := createBareTemplate(t, c, "Inst Tmpl")

	// Create
	var inst struct {
		ID         string `json:"id"`
		TemplateID string `json:"template_id"`
		TeamID     string `json:"team_id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
	}
	createReq := map[string]any{
		"template_id": tplID, "team_id": "team-A", "name": "Q1 Triage",
	}
	status, body, err := c.PostJSON("/v1/instances", createReq, &inst)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create: status=%d err=%v body=%s", status, err, body)
	}
	if inst.TemplateID != tplID || inst.TeamID != "team-A" || inst.Name != "Q1 Triage" {
		t.Errorf("create: %+v", inst)
	}
	if inst.Status != "active" {
		t.Errorf("default status = %q, want active", inst.Status)
	}

	// Get
	var got struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	status, _, err = c.GetJSON("/v1/instances/"+inst.ID, &got)
	if err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v", status, err)
	}
	if got.Name != "Q1 Triage" {
		t.Errorf("get name: %q", got.Name)
	}

	// Update (status -> paused)
	var updated struct {
		Status string `json:"status"`
	}
	patchReq := map[string]any{"status": "paused"}
	status, body, err = c.PatchJSON("/v1/instances/"+inst.ID, patchReq, &updated)
	if err != nil || status != http.StatusOK {
		t.Fatalf("update: status=%d err=%v body=%s", status, err, body)
	}
	if updated.Status != "paused" {
		t.Errorf("status: %q, want paused", updated.Status)
	}

	// List filtered by team_id
	var list []struct {
		ID     string `json:"id"`
		TeamID string `json:"team_id"`
	}
	status, _, err = c.GetJSON("/v1/instances?team_id=team-A", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list: status=%d err=%v", status, err)
	}
	if len(list) != 1 || list[0].TeamID != "team-A" {
		t.Errorf("list: %+v", list)
	}

	// List with non-matching filter
	var empty []struct {
		ID string `json:"id"`
	}
	status, _, _ = c.GetJSON("/v1/instances?team_id=team-Z", &empty)
	if status != http.StatusOK || len(empty) != 0 {
		t.Errorf("list filtered: status=%d len=%d", status, len(empty))
	}
}
