// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestRuntime_DiagDrivesStubDriver(t *testing.T) {
	env := harness.NewEnv(t, harness.WithStubRuntimeEnv())
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-001", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	startReq := map[string]any{
		"project_id":   "flow",
		"work_item_id": "wi-rt-1",
		"agent_id":     "a_rt_001",
		"git_ref":      "main",
	}
	status, body, err := c.PostJSON("/v1/runtime/_diag/start", startReq, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("start: status=%d err=%v body=%s", status, err, body)
	}
	var startResp struct {
		Master struct{ Kind, ID string } `json:"master"`
		Work   struct{ Kind, ID string } `json:"work"`
		Handle struct{ Kind, ID string } `json:"handle"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if startResp.Handle.Kind != "stub" || startResp.Handle.ID == "" {
		t.Errorf("handle: got %+v", startResp.Handle)
	}
	if startResp.Master.Kind != "stub" || startResp.Work.Kind != "stub" {
		t.Errorf("volume kinds: master=%+v work=%+v", startResp.Master, startResp.Work)
	}

	stopReq := map[string]any{
		"handle": startResp.Handle,
		"volume": startResp.Work,
	}
	status, body, err = c.PostJSON("/v1/runtime/_diag/stop", stopReq, nil)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("stop status=%d body=%s", status, body)
	}
}

func TestRuntime_DiagReturns503WithoutStub(t *testing.T) {
	env := harness.NewEnv(t) // no WithStubRuntimeEnv()
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-001", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)
	status, _, err := c.PostJSON("/v1/runtime/_diag/start", map[string]any{
		"project_id": "flow", "work_item_id": "wi", "agent_id": "a", "git_ref": "main",
	}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no stub bound, got %d", status)
	}
}
