// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestHealth_LivenessReturns200(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())
	status, body, err := c.GetJSON("/v1/health", nil)
	if err != nil {
		t.Fatalf("get /v1/health: %v", err)
	}
	// Flow's HandleHealth returns 200 (healthy), 218 (degraded), or 503
	// (unhealthy). On a fresh daemon with the SQLite check passing, expect 200.
	if status != http.StatusOK {
		t.Fatalf("/v1/health status=%d body=%s", status, body)
	}
	// Cross-check that the bundled fakes are actually carrying traffic:
	// during daemon init the Sharkfin adapter calls Register, which the
	// fake records. A regression in Pylon discovery, the adapter switch,
	// or the fake's wire format would leave Registered() false even
	// though /v1/health succeeds (Sharkfin failures are logged, not
	// fatal). Assert here so a silent break surfaces immediately.
	if !env.Sharkfin.Registered() {
		t.Fatal("fake Sharkfin never received Register during daemon init — adapter or wiring broken")
	}
}

func TestHealth_UIHealthReturns200(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())
	status, body, err := c.GetJSON("/ui/health", nil)
	if err != nil {
		t.Fatalf("get /ui/health: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("/ui/health status=%d body=%s", status, body)
	}
}
