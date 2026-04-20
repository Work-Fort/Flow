// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestInstances_ProjectBinding(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	tok := env.Daemon.SignJWT("svc-ip", "flow-ip", "Flow IP", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Create project.
	var prj struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/projects", map[string]any{
		"name": "ip-prj", "channel_name": "#ip-prj",
	}, &prj); err != nil || status != http.StatusCreated {
		t.Fatalf("create project: status=%d err=%v", status, err)
	}

	// Create template with a step so work items can be created.
	var tpl struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/templates", map[string]any{
		"name": "ip-tpl",
	}, &tpl); err != nil || status != http.StatusCreated {
		t.Fatalf("create template: status=%d err=%v", status, err)
	}
	// PATCH in a step so CreateWorkItem has a step to anchor to.
	if status, _, err := c.PatchJSON("/v1/templates/"+tpl.ID, map[string]any{
		"steps": []map[string]any{
			{"id": "step-1", "key": "todo", "name": "To Do", "type": "task", "position": 1},
		},
	}, nil); err != nil || status != http.StatusOK {
		t.Fatalf("patch template steps: status=%d err=%v", status, err)
	}

	// Create instance bound to project.
	var inst struct {
		ID        string `json:"id"`
		ProjectID string `json:"project_id"`
	}
	if status, _, err := c.PostJSON("/v1/instances", map[string]any{
		"template_id": tpl.ID, "team_id": "team-1", "name": "inst-1",
		"project_id": prj.ID,
	}, &inst); err != nil || status != http.StatusCreated {
		t.Fatalf("create instance: status=%d err=%v", status, err)
	}
	if inst.ProjectID != prj.ID {
		t.Errorf("instance.project_id = %q, want %q", inst.ProjectID, prj.ID)
	}

	// GET /v1/projects/{id}/instances returns the bound instance.
	var instanceList []struct {
		ID        string `json:"id"`
		ProjectID string `json:"project_id"`
	}
	if status, _, err := c.GetJSON("/v1/projects/"+prj.ID+"/instances", &instanceList); err != nil || status != http.StatusOK {
		t.Fatalf("list instances by project: status=%d err=%v", status, err)
	}
	if len(instanceList) != 1 || instanceList[0].ID != inst.ID {
		t.Errorf("list instances: want 1 result with id=%s, got %+v", inst.ID, instanceList)
	}

	// Create work item in the bound instance.
	var wi struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/instances/"+inst.ID+"/items", map[string]any{
		"title": "task-1",
	}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create work item: status=%d err=%v", status, err)
	}

	// GET /v1/projects/{id}/work-items returns the work item.
	var workItemList []struct {
		ID string `json:"id"`
	}
	if status, _, err := c.GetJSON("/v1/projects/"+prj.ID+"/work-items", &workItemList); err != nil || status != http.StatusOK {
		t.Fatalf("list work items by project: status=%d err=%v", status, err)
	}
	if len(workItemList) != 1 || workItemList[0].ID != wi.ID {
		t.Errorf("list work items: want 1 result with id=%s, got %+v", wi.ID, workItemList)
	}

	// Unbound instance's work items do NOT appear.
	var inst2 struct {
		ID string `json:"id"`
	}
	_, _, _ = c.PostJSON("/v1/instances", map[string]any{
		"template_id": tpl.ID, "team_id": "team-1", "name": "inst-unbound",
	}, &inst2)
	_, _, _ = c.PostJSON("/v1/instances/"+inst2.ID+"/items", map[string]any{"title": "other"}, nil)

	var workItemList2 []struct{ ID string }
	if _, _, err := c.GetJSON("/v1/projects/"+prj.ID+"/work-items", &workItemList2); err != nil {
		t.Fatalf("second list: %v", err)
	}
	if len(workItemList2) != 1 {
		t.Errorf("expected 1 work item for project, got %d", len(workItemList2))
	}
}
