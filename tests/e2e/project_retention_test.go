// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestProjects_RetentionDays(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	tok := env.Daemon.SignJWT("svc-ret", "flow-ret", "Flow Ret", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Create project with retention_days set.
	var created struct {
		ID            string `json:"id"`
		RetentionDays *int   `json:"retention_days"`
	}
	if status, _, err := c.PostJSON("/v1/projects", map[string]any{
		"name": "ret-prj", "channel_name": "#ret-prj", "retention_days": 30,
	}, &created); err != nil || status != http.StatusCreated {
		t.Fatalf("create: status=%d err=%v", status, err)
	}
	if created.RetentionDays == nil || *created.RetentionDays != 30 {
		t.Errorf("create: want retention=30, got %v", created.RetentionDays)
	}

	// GET round-trip: value persists.
	var got struct {
		RetentionDays *int `json:"retention_days"`
	}
	if status, _, err := c.GetJSON("/v1/projects/"+created.ID, &got); err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v", status, err)
	}
	if got.RetentionDays == nil || *got.RetentionDays != 30 {
		t.Errorf("get: want retention=30, got %v", got.RetentionDays)
	}

	// PATCH: change to 90.
	var patched struct {
		RetentionDays *int `json:"retention_days"`
	}
	if status, _, err := c.PatchJSON("/v1/projects/"+created.ID, map[string]any{
		"retention_days": 90,
	}, &patched); err != nil || status != http.StatusOK {
		t.Fatalf("patch to 90: status=%d err=%v", status, err)
	}
	if patched.RetentionDays == nil || *patched.RetentionDays != 90 {
		t.Errorf("patch: want 90, got %v", patched.RetentionDays)
	}

	// PATCH: clear to permanent (nil).
	var cleared struct {
		RetentionDays *int `json:"retention_days"`
	}
	if status, _, err := c.PatchJSON("/v1/projects/"+created.ID, map[string]any{
		"clear_retention_days": true,
	}, &cleared); err != nil || status != http.StatusOK {
		t.Fatalf("patch clear: status=%d err=%v", status, err)
	}
	if cleared.RetentionDays != nil {
		t.Errorf("clear: want nil retention, got %v", cleared.RetentionDays)
	}
}
