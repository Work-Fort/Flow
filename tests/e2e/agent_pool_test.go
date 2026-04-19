// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestAgentPool_AcquireRenewRelease(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	env.Hive.SeedPoolAgent("a_e2e_001", "agent-e2e-1", "team-e2e")

	tok := env.Daemon.SignJWT("svc-001", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	claimReq := map[string]any{
		"role": "developer", "project": "flow",
		"workflow_id": "wf-e2e-1", "lease_ttl_seconds": 60,
	}
	status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "a_e2e_001") {
		t.Fatalf("claim response should contain agent ID: %s", body)
	}

	// Wait for ≥1 renew. Daemon runs renewer at 100ms (harness env var).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.Hive.RenewCalls() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if env.Hive.RenewCalls() < 1 {
		t.Errorf("renewer never called Hive: claim=%d release=%d renew=%d",
			env.Hive.ClaimCalls(), env.Hive.ReleaseCalls(), env.Hive.RenewCalls())
	}

	releaseReq := map[string]any{
		"agent_id": "a_e2e_001", "workflow_id": "wf-e2e-1",
	}
	status, body, err = c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("release status=%d body=%s", status, body)
	}
	if env.Hive.ReleaseCalls() != 1 {
		t.Errorf("release calls: got %d, want 1", env.Hive.ReleaseCalls())
	}
}
