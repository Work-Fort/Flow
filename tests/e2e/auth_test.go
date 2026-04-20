// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// TestAuth_BearerJWTHappyPath confirms a valid signed JWT under
// "Bearer" is accepted on a protected route. Complements
// TestDaemon_ApiKeyV1RoutesToVerify (in daemon_auth_test.go) which
// covers the API-key path.
func TestAuth_BearerJWTHappyPath(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("usr-001", "alice", "Alice Tester", "user")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	status, body, err := c.GetJSON("/v1/templates", nil)
	if err != nil {
		t.Fatalf("get /v1/templates: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	// Bearer-only test: the API-key validator MUST NOT be called.
	if got := env.JWKS.APIKeyVerifyCount(); got != 0 {
		t.Errorf("APIKeyVerifyCount = %d, want 0 (Bearer must not invoke api-key validator)", got)
	}
}

// TestAuth_NoAuthHeaderReturns401 confirms a protected route with no
// Authorization header is rejected.
func TestAuth_NoAuthHeaderReturns401(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())
	status, body, err := c.GetJSON("/v1/templates", nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", status, body)
	}
}

// TestAuth_MalformedAuthorizationReturns401 confirms an Authorization
// header with no recognised scheme is rejected. The exact wire shape
// here exercises the dispatcher's "unknown scheme" branch — neither
// "Bearer" nor "ApiKey-v1" prefix.
func TestAuth_MalformedAuthorizationReturns401(t *testing.T) {
	cases := []struct {
		name, raw string
	}{
		{"empty value", ""},
		{"no scheme", "garbage-without-space"},
		{"unknown scheme", "Basic dXNlcjpwYXNz"}, // base64('user:pass')
		{"old api-key alias", "ApiKey ABC"},      // pre-v1 scheme name
		{"bearer no token", "Bearer"},
		{"bearer empty token", "Bearer "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := harness.NewEnv(t)
			defer env.Cleanup(t)

			c := harness.NewClientRawAuth(env.Daemon.BaseURL(), tc.raw)
			status, body, err := c.GetJSON("/v1/templates", nil)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if status != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s, want 401", status, body)
			}
		})
	}
}

// TestAuth_ExpiredJWTReturns401 confirms a JWT past its exp claim is
// rejected — the JWT validator must honour expiration. Like the
// happy-path test, the api-key validator MUST stay untouched.
func TestAuth_ExpiredJWTReturns401(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	expired := env.JWKS.SignExpiredJWT("usr-002", "expired-user", "Expired", "user")
	c := harness.NewClient(env.Daemon.BaseURL(), expired)

	status, body, err := c.GetJSON("/v1/templates", nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", status, body)
	}
	if got := env.JWKS.APIKeyVerifyCount(); got != 0 {
		t.Errorf("APIKeyVerifyCount = %d, want 0 (expired Bearer must not fall through to api-key)", got)
	}
}

// TestAuth_ApiKeyV1WithJWTReturns401 is the inverse of
// TestDaemon_BearerForAPIKeyReturns401: a real signed JWT sent under
// "ApiKey-v1" must be rejected. The api-key validator should be
// invoked (the scheme dispatch sends it there) and reject the JWT
// because the JWKS stub's /v1/verify-api-key only honours the literal
// service-token string, not arbitrary JWT-shaped strings.
func TestAuth_ApiKeyV1WithJWTReturns401(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	jwtTok := env.Daemon.SignJWT("usr-003", "user-with-jwt", "User", "user")
	c := harness.NewClientAPIKey(env.Daemon.BaseURL(), jwtTok) // wrong: JWT under ApiKey-v1

	status, body, err := c.GetJSON("/v1/templates", nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401 (JWT under ApiKey-v1 must be rejected by stub)", status, body)
	}
	// The api-key validator IS invoked (by design: scheme dispatch
	// routes "ApiKey-v1 ..." there). The stub must reject it.
	if got := env.JWKS.APIKeyVerifyCount(); got < 1 {
		t.Errorf("APIKeyVerifyCount = %d, want >= 1 (api-key validator should be reached)", got)
	}
}

// TestAuth_PublicHealthSkipsAuth confirms /v1/health and /ui/health
// remain reachable without any Authorization header. The publicPathSkip
// branch in server.go must still apply post-scheme-split.
// /ui/health returns 503 when no UI is embedded and 200 when embedded;
// the auth test only asserts the endpoint is reachable (not 401/403).
func TestAuth_PublicHealthSkipsAuth(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())

	// /v1/health must return 200.
	if status, body, err := c.GetJSON("/v1/health", nil); err != nil {
		t.Fatalf("/v1/health: %v", err)
	} else if status != http.StatusOK {
		t.Errorf("/v1/health status=%d body=%s, want 200", status, body)
	}

	// /ui/health must be reachable (not 401/403) — 503 is valid pre-embed.
	if status, body, err := c.GetJSON("/ui/health", nil); err != nil {
		t.Fatalf("/ui/health: %v", err)
	} else if status == http.StatusUnauthorized || status == http.StatusForbidden {
		t.Errorf("/ui/health status=%d body=%s, want not 401/403", status, body)
	}
}
