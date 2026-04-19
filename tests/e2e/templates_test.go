// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// authedClient is a small helper to keep tests focused on assertions
// rather than handshake plumbing. It mints a service-token API-key
// client because every protected route is tested with the same
// identity; the auth test suite separately covers per-scheme branches.
func authedClient(t *testing.T, env *harness.Env) *harness.Client {
	t.Helper()
	tok := env.Daemon.SignJWT("usr-rest", "rest-user", "REST User", "user")
	return harness.NewClient(env.Daemon.BaseURL(), tok)
}

func TestTemplates_ListEmpty(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := authedClient(t, env)
	var out []map[string]any
	status, body, err := c.GetJSON("/v1/templates", &out)
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v body=%s", status, err, body)
	}
	if len(out) != 0 {
		t.Errorf("want empty list, got %d entries: %s", len(out), body)
	}
}

func TestTemplates_CreateGetUpdateDelete(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	// Create
	var created struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version int    `json:"version"`
	}
	createReq := map[string]any{"name": "Triage", "description": "Initial triage flow"}
	status, body, err := c.PostJSON("/v1/templates", createReq, &created)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create: status=%d err=%v body=%s", status, err, body)
	}
	if created.ID == "" || created.Name != "Triage" || created.Version != 1 {
		t.Fatalf("create response: %+v body=%s", created, body)
	}

	// Get (single)
	var got struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     int    `json:"version"`
	}
	status, body, err = c.GetJSON("/v1/templates/"+created.ID, &got)
	if err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v body=%s", status, err, body)
	}
	if got.Name != "Triage" || got.Description != "Initial triage flow" {
		t.Errorf("get: %+v", got)
	}

	// List (now non-empty)
	var list []map[string]any
	status, _, err = c.GetJSON("/v1/templates", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list after create: status=%d err=%v", status, err)
	}
	if len(list) != 1 {
		t.Errorf("list len: got %d, want 1", len(list))
	}

	// Update
	patchReq := map[string]any{"description": "Updated description"}
	var updated struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	status, body, err = c.PatchJSON("/v1/templates/"+created.ID, patchReq, &updated)
	if err != nil || status != http.StatusOK {
		t.Fatalf("update: status=%d err=%v body=%s", status, err, body)
	}
	if updated.Description != "Updated description" {
		t.Errorf("update description: %q", updated.Description)
	}

	// Delete
	status, body, err = c.DeleteJSON("/v1/templates/" + created.ID)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("delete: status=%d err=%v body=%s", status, err, body)
	}

	// Get after delete -> 404
	status, _, err = c.GetJSON("/v1/templates/"+created.ID, nil)
	if err != nil {
		t.Fatalf("get-after-delete: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("get-after-delete: status=%d, want 404", status)
	}
}

func TestTemplates_GetNotFound(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)
	status, _, _ := c.GetJSON("/v1/templates/tpl_does_not_exist", nil)
	if status != http.StatusNotFound {
		t.Errorf("status=%d, want 404", status)
	}
}

// createBareTemplate creates a template with no steps/transitions and
// returns its ID. Used by instance tests that do not need work items.
func createBareTemplate(t *testing.T, c *harness.Client, name string) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	status, body, err := c.PostJSON("/v1/templates", map[string]any{"name": name}, &out)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create template: status=%d err=%v body=%s", status, err, body)
	}
	return out.ID
}

// TestTemplates_PatchSeedsStepsAndTransitions verifies that a PATCH
// with `steps` and `transitions` arrays replaces the template's
// step/transition collections. This is the seeding path the work-item
// tests in items_test.go and transitions_test.go depend on.
func TestTemplates_PatchSeedsStepsAndTransitions(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := createBareTemplate(t, c, "Seeded")

	patchReq := map[string]any{
		"steps": []map[string]any{
			{"id": "stp_a", "key": "todo", "name": "To Do", "type": "task", "position": 0},
			{"id": "stp_b", "key": "review", "name": "Review", "type": "gate", "position": 1,
				"approval": map[string]any{
					"mode":               "any",
					"required_approvers": 1,
					"approver_role_id":   "role_reviewer",
				},
			},
			{"id": "stp_c", "key": "done", "name": "Done", "type": "task", "position": 2},
		},
		"transitions": []map[string]any{
			{"id": "trn_1", "key": "submit", "name": "Submit",
				"from_step_id": "stp_a", "to_step_id": "stp_b"},
			{"id": "trn_2", "key": "approve", "name": "Approve",
				"from_step_id": "stp_b", "to_step_id": "stp_c"},
		},
	}
	var updated struct {
		ID    string `json:"id"`
		Steps []struct {
			ID       string `json:"id"`
			Key      string `json:"key"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Position int    `json:"position"`
		} `json:"steps"`
		Transitions []struct {
			ID         string `json:"id"`
			Key        string `json:"key"`
			FromStepID string `json:"from_step_id"`
			ToStepID   string `json:"to_step_id"`
		} `json:"transitions"`
	}
	status, body, err := c.PatchJSON("/v1/templates/"+tplID, patchReq, &updated)
	if err != nil || status != http.StatusOK {
		t.Fatalf("patch: status=%d err=%v body=%s", status, err, body)
	}
	if len(updated.Steps) != 3 {
		t.Errorf("want 3 steps, got %d (%s)", len(updated.Steps), body)
	}
	if len(updated.Transitions) != 2 {
		t.Errorf("want 2 transitions, got %d (%s)", len(updated.Transitions), body)
	}

	// Round-trip via GET.
	var fetched struct {
		Steps []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"steps"`
		Transitions []struct {
			ID         string `json:"id"`
			FromStepID string `json:"from_step_id"`
			ToStepID   string `json:"to_step_id"`
		} `json:"transitions"`
	}
	status, _, err = c.GetJSON("/v1/templates/"+tplID, &fetched)
	if err != nil || status != http.StatusOK {
		t.Fatalf("get after patch: status=%d err=%v", status, err)
	}
	if len(fetched.Steps) != 3 || fetched.Steps[0].Key != "todo" {
		t.Errorf("steps: %+v", fetched.Steps)
	}
	if len(fetched.Transitions) != 2 || fetched.Transitions[0].FromStepID != "stp_a" {
		t.Errorf("transitions: %+v", fetched.Transitions)
	}
}

// TestTemplates_PatchSeedsTwiceReplaces is the regression test for
// the Step 0 fix to UpdateTemplate. A second PATCH with a smaller
// transitions[] array must REPLACE (not append-and-conflict) the
// previous set. Without the fix, this either fails on a UNIQUE
// PRIMARY KEY violation or surfaces 4 transitions instead of 1.
func TestTemplates_PatchSeedsTwiceReplaces(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := createBareTemplate(t, c, "Twice")

	first := map[string]any{
		"steps": []map[string]any{
			{"id": "s1", "key": "k1", "name": "N1", "type": "task", "position": 0},
			{"id": "s2", "key": "k2", "name": "N2", "type": "task", "position": 1},
		},
		"transitions": []map[string]any{
			{"id": "t1", "key": "k1", "name": "N1", "from_step_id": "s1", "to_step_id": "s2"},
			{"id": "t2", "key": "k2", "name": "N2", "from_step_id": "s2", "to_step_id": "s1"},
		},
	}
	if status, body, err := c.PatchJSON("/v1/templates/"+tplID, first, nil); err != nil || status != http.StatusOK {
		t.Fatalf("first patch: status=%d err=%v body=%s", status, err, body)
	}

	// Second PATCH with a smaller transition set MUST replace, not append.
	second := map[string]any{
		"steps": []map[string]any{
			{"id": "s1", "key": "k1", "name": "N1", "type": "task", "position": 0},
			{"id": "s2", "key": "k2", "name": "N2", "type": "task", "position": 1},
		},
		"transitions": []map[string]any{
			{"id": "t9", "key": "only", "name": "Only",
				"from_step_id": "s1", "to_step_id": "s2"},
		},
	}
	if status, body, err := c.PatchJSON("/v1/templates/"+tplID, second, nil); err != nil || status != http.StatusOK {
		t.Fatalf("second patch: status=%d err=%v body=%s", status, err, body)
	}

	var fetched struct {
		Transitions []struct {
			ID string `json:"id"`
		} `json:"transitions"`
	}
	if status, body, err := c.GetJSON("/v1/templates/"+tplID, &fetched); err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v body=%s", status, err, body)
	}
	if len(fetched.Transitions) != 1 || fetched.Transitions[0].ID != "t9" {
		t.Errorf("after second patch: want exactly [t9], got %+v", fetched.Transitions)
	}
}

// seedThreeStepTemplate seeds the canonical 3-step / 2-transition
// shape items_test.go and transitions_test.go expect:
//
//	stp_todo  --trn_submit-->  stp_review (gate)  --trn_approve-->  stp_done
//
// Returns the template ID. The gate step at stp_review has an "any" /
// 1-required approval config so approve/reject endpoints can be
// exercised without role-resolution wiring.
func seedThreeStepTemplate(t *testing.T, c *harness.Client) string {
	t.Helper()
	var tpl struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/templates",
		map[string]any{"name": "Three-Step"}, &tpl); err != nil || status != http.StatusCreated {
		t.Fatalf("create template: status=%d err=%v body=%s", status, err, body)
	}
	patch := map[string]any{
		"steps": []map[string]any{
			{"id": "stp_todo", "key": "todo", "name": "To Do", "type": "task", "position": 0},
			{"id": "stp_review", "key": "review", "name": "Review", "type": "gate", "position": 1,
				"approval": map[string]any{
					"mode": "any", "required_approvers": 1, "approver_role_id": "",
				},
			},
			{"id": "stp_done", "key": "done", "name": "Done", "type": "task", "position": 2},
		},
		"transitions": []map[string]any{
			{"id": "trn_submit", "key": "submit", "name": "Submit",
				"from_step_id": "stp_todo", "to_step_id": "stp_review"},
			{"id": "trn_approve", "key": "approve", "name": "Approve",
				"from_step_id": "stp_review", "to_step_id": "stp_done"},
		},
	}
	if status, body, err := c.PatchJSON("/v1/templates/"+tpl.ID, patch, nil); err != nil || status != http.StatusOK {
		t.Fatalf("seed template: status=%d err=%v body=%s", status, err, body)
	}
	return tpl.ID
}

// seedActiveInstance creates an instance against a seeded template
// and returns the instance ID.
func seedActiveInstance(t *testing.T, c *harness.Client, tplID, teamID string) string {
	t.Helper()
	var inst struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances",
		map[string]any{"template_id": tplID, "team_id": teamID, "name": "test-inst"},
		&inst); err != nil || status != http.StatusCreated {
		t.Fatalf("create instance: status=%d err=%v body=%s", status, err, body)
	}
	return inst.ID
}
