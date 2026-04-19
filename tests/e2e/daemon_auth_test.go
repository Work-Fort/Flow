// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// serviceToken is the --service-token value baked into StartDaemon in
// harness/daemon.go. Tests that exercise outbound/inbound scheme routing
// use this directly rather than a Env.ServiceAPIKey accessor (the harness
// doesn't expose one — the token is an implementation constant).
const serviceToken = "harness-service-token"

// TestDaemon_BearerForAPIKeyReturns401 exercises the Cluster 3b regression:
// sending a wf-svc_* API key under the "Bearer" scheme must be rejected.
// Before the scheme-dispatch fix, passport's middleware fell through from the
// JWT validator to the API-key validator, silently accepting the misformatted
// request. After the fix, Bearer dispatches only to the JWT validator — an
// API key fails JWT parse/validate and returns 401 without touching the
// API-key validator (APIKeyVerifyCount must stay zero).
func TestDaemon_BearerForAPIKeyReturns401(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	req, _ := http.NewRequest("GET", env.Daemon.BaseURL()+"/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer "+serviceToken) // API key under wrong scheme
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (API key under Bearer must not be accepted)", resp.StatusCode)
	}
	if got := env.JWKS.APIKeyVerifyCount(); got != 0 {
		t.Errorf("APIKeyVerifyCount = %d, want 0 (Bearer must not fall through to verify-api-key)", got)
	}
}

// TestDaemon_ApiKeyV1RoutesToVerify confirms that a request carrying an API key
// under the correct "ApiKey-v1" scheme is accepted (200) and that the
// API-key verifier was actually called (counter advances).
func TestDaemon_ApiKeyV1RoutesToVerify(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	beforeCount := env.JWKS.APIKeyVerifyCount()

	req, _ := http.NewRequest("GET", env.Daemon.BaseURL()+"/v1/templates", nil)
	req.Header.Set("Authorization", "ApiKey-v1 "+serviceToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := env.JWKS.APIKeyVerifyCount(); got <= beforeCount {
		t.Errorf("APIKeyVerifyCount did not advance (was %d, now %d) — verify-api-key was not called", beforeCount, got)
	}
}
