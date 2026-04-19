---
type: plan
step: "A.5"
title: "Flow E2E harness — Plan A.5: auth coverage + 18 REST ops + harness README"
status: approved
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "A.5"
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - "2026-04-18-flow-e2e-harness-01-foundation.md"
  - "2026-04-18-flow-orchestration-01-foundation.md"
---

# Flow E2E Harness — Plan A.5: Auth Coverage + 18 REST Ops + Harness README

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal.** Three deliverables on top of the now-landed Plan A foundation
and Foundation orchestration impl:

1. **Auth tests** for the existing REST surface — Bearer-JWT happy
   path, ApiKey-v1 happy path, no-auth rejection, malformed
   `Authorization` header, expired JWT, scheme-dispatch enforcement
   (Bearer with API-key body → 401, ApiKey-v1 with JWT body → 401).
   Mirrors the Cluster-3b regression coverage Hive, Sharkfin, Combine,
   and Pylon landed during the Passport scheme-split.
2. **All 18 production REST endpoints covered by e2e**, exercising
   every path through the real spawned daemon. The 18 ops live in
   `internal/daemon/rest_huma.go` (verified 2026-04-19 by grepping
   `OperationID:` — exact match to the count in
   `AGENT-POOL-REMAINING-WORK.md`). The work-item subset (8 ops)
   requires a small REST surface extension to seed steps and
   transitions; see Task 4.
3. **`tests/e2e/README.md`** documenting how to add an endpoint test,
   the per-test backend pattern, the FakeHive/FakeSharkfin wire-format
   approach, every env var the harness reads, and how to run against
   both backends.

**Why this plan exists, scoped to these three.** Plan A
(`2026-04-18-flow-e2e-harness-01-foundation.md`) shipped the harness
skeleton (daemon spawn, four fakes, JWKS stub, `Client` wrappers, two
health tests, dual-backend wiring). The Foundation plan
(`2026-04-18-flow-orchestration-01-foundation.md`) added e2e coverage
for the new diag endpoints (`scheduler/_diag/*`, `audit/_diag/*`,
`runtime/_diag/*`). Plan A.5 is the next bite-sized layer: auth and the
production REST surface. It deliberately does NOT include MCP (12
tools), the Sharkfin webhook receiver, or bidirectional round-trip
flows — those land in Plan B.

**Verification of "18".** `grep "OperationID:" internal/daemon/rest_huma.go`
returns 18 entries (templates × 5, instances × 4, work items × 5,
transitions × 1, approvals × 3). The diag endpoints in
`runtime_diag.go` (2) and `scheduler_diag.go` (3) are NOT in the 18 —
those are internal `_diag` endpoints already exercised by Foundation.
The 4 raw mux handlers (`/v1/health`, `/ui/health`,
`/v1/webhooks/sharkfin`, `/mcp`) are also outside the 18: health is
exercised by Plan A; the webhook and MCP endpoint are Plan B's scope.

**Authoritative endpoint list (the 18).**

| # | Method | Path | OperationID | Tag |
|---|--------|------|-------------|-----|
| 1 | GET | `/v1/templates` | `list-templates` | Templates |
| 2 | POST | `/v1/templates` | `create-template` | Templates |
| 3 | GET | `/v1/templates/{id}` | `get-template` | Templates |
| 4 | PATCH | `/v1/templates/{id}` | `update-template` | Templates |
| 5 | DELETE | `/v1/templates/{id}` | `delete-template` | Templates |
| 6 | GET | `/v1/instances` | `list-instances` | Instances |
| 7 | POST | `/v1/instances` | `create-instance` | Instances |
| 8 | GET | `/v1/instances/{id}` | `get-instance` | Instances |
| 9 | PATCH | `/v1/instances/{id}` | `update-instance` | Instances |
| 10 | POST | `/v1/instances/{id}/items` | `create-work-item` | WorkItems |
| 11 | GET | `/v1/instances/{id}/items` | `list-work-items` | WorkItems |
| 12 | GET | `/v1/items/{id}` | `get-work-item` | WorkItems |
| 13 | PATCH | `/v1/items/{id}` | `update-work-item` | WorkItems |
| 14 | GET | `/v1/items/{id}/history` | `get-work-item-history` | WorkItems |
| 15 | POST | `/v1/items/{id}/transition` | `transition-work-item` | Transitions |
| 16 | POST | `/v1/items/{id}/approve` | `approve-work-item` | Approvals |
| 17 | POST | `/v1/items/{id}/reject` | `reject-work-item` | Approvals |
| 18 | GET | `/v1/items/{id}/approvals` | `list-approvals` | Approvals |

**Step/transition seeding gap and the resolution.** Plan A noted (lines
77–78) that 8 of these 18 ops cannot be exercised end-to-end today
because no REST path writes Steps or Transitions:
`PatchTemplateInput` (`internal/daemon/rest_types.go:80-86`) only
accepts `name` and `description`; `CreateTemplateInput` (lines 73-78)
likewise. The store *does* support steps/transitions —
`(*sqlite.Store).UpdateTemplate`
(`internal/infra/sqlite/templates.go:246-330`) and
`(*postgres.Store).UpdateTemplate`
(`internal/infra/postgres/templates.go`) already replace Steps and
Transitions from the input struct. The handler is the only thing
discarding them.

The minimum-viable resolution, taken in Task 4 of this plan: extend
`PatchTemplateInput.Body` to optionally accept `steps` and
`transitions` arrays. When present, the handler maps them onto the
existing template and calls `store.UpdateTemplate`. This is a
~40-line surgical change (the wiring, plus a guard on the
Position-uniqueness invariant the SQL already requires) that unlocks
the entire work-item REST surface for E2E coverage. It is
intentionally a `PATCH`-shaped extension — the operation already
implies "replace what you give me" semantics for non-zero fields, and
`UpdateTemplate`'s store impl already replaces these collections
on every call. No new endpoint, no new domain port.

This avoids the alternatives:
- Adding `POST /v1/templates/{id}/steps` + transitions endpoints
  (5+ new endpoints, larger surface, scope blowup).
- Test-only DB seed helpers in the harness (would need direct DB
  access, breaking the wire-only constraint Plan A established).
- Skipping the 8 work-item ops (violates this plan's deliverable
  and `feedback_no_test_failures.md`).

**Non-goals (deferred).**
- MCP tool coverage (12 tools, Plan B).
- Sharkfin webhook receiver coverage (Plan B).
- Bidirectional Flow→Sharkfin→Flow round-trip (Plan B).
- Step/transition CRUD endpoints as separate REST resources (Flow
  doesn't need them yet; the Patch extension covers the test gap).
- `/mcp`, `/openapi`, `/docs`, public-path skip exhaustive coverage
  (the auth tests in Task 2 cover the public-path branch via
  `/v1/health`; Plan B covers `/mcp`).

**Hard constraints (non-negotiable, carried from Plan A).**
- Harness imports zero WorkFort Go client packages. No
  `sharkfinclient`, no `hiveclient`, no `pylonclient`, no
  Flow-internal packages. Plan A's docstring on
  `tests/e2e/harness/client.go:13-23` is stale (claims Bearer-only)
  and gets corrected in Task 1; the harness Client gains an
  ApiKey-v1 constructor in the same task.
- Fakes do not reuse real services' handler code.
- Every commit message uses the multi-line conventional-commits
  HEREDOC format with body + `Co-Authored-By: Claude Sonnet 4.6
  <noreply@anthropic.com>` trailer.
- Commit messages contain **no `!` markers** and **no `BREAKING
  CHANGE:` footers** (pre-1.0 enforcement, per
  `AGENT-POOL-REMAINING-WORK.md` "Process discipline").
- E2E suites must run both SQLite and Postgres. Each new test gets
  added to `mise run e2e --backend=sqlite` and
  `mise run e2e --backend=postgres`. No silent skips.
- Tests do NOT call `t.Parallel()` — daemon spawn cost is ~200ms per
  test; the harness already documents this convention
  (`tests/e2e/harness/env.go:60-64`).

**Tech stack.** Unchanged from Plan A: Go 1.26, `net/http`,
`net/http/httptest`, `encoding/json`, `database/sql`,
`github.com/lestrrat-go/jwx/v2`, `github.com/jackc/pgx/v5/stdlib`.
**No new dependencies introduced by this plan.**

---

## Prerequisites

Before starting:
- [ ] Plan A landed (`status: complete`) — verified by
      `tests/e2e/harness/{client,daemon,env,fake_hive,fake_sharkfin,jwks_stub,pylon_stub}.go`
      existing.
- [ ] Foundation plan landed (`status: complete`) — verified by
      `tests/e2e/{agent_pool_test,audit_events_test,runtime_diag_test}.go`
      existing.
- [ ] `mise run e2e --backend=sqlite` and
      `mise run e2e --backend=postgres` both green on master tip.
- [ ] Local Postgres reachable at
      `postgres://postgres@127.0.0.1/flow_test?sslmode=disable`
      (peer-trust auth as `postgres` user).

---

## Task breakdown

### Task 1: Harness — fix stale docstring + add ApiKey-v1 client constructor

**Why first.** Plan A's Client wires the JWT-only Bearer convention
into a docstring claiming "JWTs and API keys ride the same wire". The
Passport scheme-split landed; this is no longer true. Every auth test
in Task 2, every REST test in Tasks 3 + 5, and every reader of the
harness assumes the docstring is correct. Fix it once, here, before
adding consumers.

**Files:**
- Modify: `tests/e2e/harness/client.go:14-47`

**Step 1: Update docstring on `Client` and add `NewClientAPIKey`**

> **Field-rename semantic shift — read carefully.** The OLD code stored
> the bare credential in `Client.token` and synthesised the scheme at
> request time (`req.Header.Set("Authorization", "Bearer "+c.token)` at
> `client.go:67`). The NEW code stores the FULL header value
> (`"Bearer <jwt>"` or `"ApiKey-v1 <key>"`) in `Client.authHeader` and
> writes it verbatim. Each constructor is responsible for prepending
> its own scheme. Verify all four constructors below include their
> respective scheme prefix; do NOT also synthesise the prefix in
> `authedRequest`. Mis-applying this patch (e.g., leaving the old
> `"Bearer "+c.token` and renaming `token` to `authHeader`) would emit
> `"Bearer Bearer <jwt>"` and break every authenticated test.

Replace `tests/e2e/harness/client.go:14-47` with:

```go
// Client is a raw http.Client wrapper for the test side of the harness.
// It explicitly does NOT depend on any WorkFort SDK — every contract
// is wire-only. If you reach for sharkfinclient or hiveclient here,
// stop and add the request inline instead.
//
// Auth model (post-Passport-scheme-split). Passport's middleware now
// dispatches by Authorization scheme:
//   - "Bearer <jwt>"      → JWT validator only
//   - "ApiKey-v1 <key>"   → API-key validator only
//   - any other scheme    → 401 (no fallthrough)
//
// Pick the constructor that documents intent:
//   - NewClient(baseURL, jwt)        → Authorization: Bearer <jwt>
//   - NewClientAPIKey(baseURL, key)  → Authorization: ApiKey-v1 <key>
//   - NewClientNoAuth(baseURL)       → no Authorization header
//   - NewClientRawAuth(baseURL, raw) → Authorization: <raw>, exact bytes
//     (use for negative tests that need malformed/garbage headers)
type Client struct {
	baseURL    string
	authHeader string // empty means: send no Authorization header
	http       *http.Client
}

// NewClient returns a Client that sends `Authorization: Bearer <token>`.
// Use it only for JWTs — API keys must use NewClientAPIKey or the
// daemon's scheme dispatch will reject them with 401.
func NewClient(baseURL, jwt string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Bearer " + jwt,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientAPIKey returns a Client that sends `Authorization: ApiKey-v1 <key>`.
// Required for API-key auth after the Passport scheme-split — sending
// an API key under "Bearer" is rejected as a Cluster-3b regression.
func NewClientAPIKey(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "ApiKey-v1 " + apiKey,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientNoAuth returns a Client that sends no auth headers.
func NewClientNoAuth(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientRawAuth returns a Client that sends the given string verbatim
// as the Authorization header value. Use only for negative tests
// (malformed scheme, missing space, garbage). Production-style auth
// MUST use NewClient or NewClientAPIKey.
func NewClientRawAuth(baseURL, rawAuth string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: rawAuth,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}
```

Then in `authedRequest` (currently lines 49-70), replace the single
`if c.token != ""` line with:

```go
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
```

(Field renamed from `token` to `authHeader` to reflect it's the full
header value, not just the credential.)

**Step 2: Update existing call sites to compile**

Search/replace verification — these files reference `harness.NewClient`
or the old `token` field:

- `tests/e2e/agent_pool_test.go:20` — uses `NewClient(..., tok)` where
  `tok` is a JWT. No change needed; constructor signature unchanged.
- `tests/e2e/audit_events_test.go:19` — same. No change.
- `tests/e2e/runtime_diag_test.go:17,61` — same. No change.
- `tests/e2e/daemon_auth_test.go` — uses `http.NewRequest` directly,
  not the harness Client. No change.

**Step 3: Run e2e to verify Task 1 didn't break anything**

Run: `mise run e2e --backend=sqlite -run TestHealth`
Expected: PASS (health tests use `NewClientNoAuth`, untouched).

Run: `mise run e2e --backend=sqlite -run TestAgentPool|TestAudit|TestRuntime`
Expected: PASS (all three use `NewClient(baseURL, jwt)`, signature
unchanged).

**Step 4: Commit**

```
test(e2e): split harness Client by Authorization scheme

The docstring claimed "JWTs and API keys ride the same wire" — true
before the Passport scheme-split, false after. Add NewClientAPIKey for
ApiKey-v1 callers and NewClientRawAuth for negative tests, rename the
internal field from token to authHeader so the role of each
constructor is unambiguous. No call-site changes; the existing
NewClient signature is preserved.
```

---

### Task 2: Auth e2e suite

**Why.** Plan A landed two of the auth scenarios in
`daemon_auth_test.go` (Bearer-with-API-key → 401; ApiKey-v1 happy
path). The remaining six are: Bearer-with-JWT happy path, no-auth
rejection, malformed Authorization, expired JWT, ApiKey-v1 with JWT
→ 401, public-path skip. Plus a JWKS-stub helper for "expired" tokens
the existing stub does not yet expose.

**Depends on:** Task 1 (uses `NewClientAPIKey`, `NewClientRawAuth`).

**Files:**
- Modify: `tests/e2e/harness/jwks_stub.go` (add `SignExpiredJWT`)
- Create: `tests/e2e/auth_test.go`
- Modify: `tests/e2e/daemon_auth_test.go` (move to `auth_test.go` —
  keep the file name singular for the suite)

**Step 1: Add `SignExpiredJWT` to the JWKS stub**

In `tests/e2e/harness/jwks_stub.go`, add this method to `JWKSStub`
(after `SignJWT`, around line 47):

```go
// SignExpiredJWT mints a JWT whose `exp` claim is one hour in the
// past — used by negative-auth tests to confirm the JWT validator
// honours expiration. Same signature/issuer/audience as SignJWT
// otherwise, so any failure is attributable to the exp claim alone.
func (s *JWKSStub) SignExpiredJWT(id, username, displayName, userType string) string {
	return s.signExpiredJWT(id, username, displayName, userType)
}
```

Add a `signExpiredJWT` field to the `JWKSStub` struct (after `signJWT`):

```go
	signExpiredJWT func(id, username, displayName, userType string) string
```

In `StartJWKSStub`, after the existing `stub.signJWT = func(...)` block
(currently lines 139-160), add:

```go
	stub.signExpiredJWT = func(id, username, displayName, userType string) string {
		past := time.Now().Add(-2 * time.Hour)
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("passport-stub").
			Audience([]string{"flow"}).
			IssuedAt(past).
			Expiration(past.Add(1 * time.Hour)). // exp = 1h ago
			Claim("username", username).
			Claim("name", displayName).
			Claim("display_name", displayName).
			Claim("type", userType).
			Build()
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: build expired JWT: %v", err))
		}
		signedBytes, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: sign expired JWT: %v", err))
		}
		return string(signedBytes)
	}
```

**Step 2: Write the failing auth test file**

Create `tests/e2e/auth_test.go`:

```go
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
//
// Wait — read the stub. The current /v1/verify-api-key handler (line
// 107-128) accepts any non-empty key except the literal "INVALID".
// That makes JWTs-as-api-keys "valid" by the stub's lights, which is
// wrong for this test. Fix in Task 2 Step 3: tighten the stub to
// reject anything that isn't the canned service token.
func TestAuth_ApiKeyV1WithJWTReturns401(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	jwt := env.Daemon.SignJWT("usr-003", "user-with-jwt", "User", "user")
	c := harness.NewClientAPIKey(env.Daemon.BaseURL(), jwt) // wrong: JWT under ApiKey-v1

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
// branch in server.go:160 must still apply post-scheme-split.
func TestAuth_PublicHealthSkipsAuth(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())
	for _, p := range []string{"/v1/health", "/ui/health"} {
		status, body, err := c.GetJSON(p, nil)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if status != http.StatusOK {
			t.Errorf("%s status=%d body=%s, want 200", p, status, body)
		}
	}
}
```

**Step 3: Tighten the JWKS stub's `/v1/verify-api-key` handler**

The current stub accepts any non-empty key except literal "INVALID"
(`tests/e2e/harness/jwks_stub.go:107-128`). That over-accepts: tests
in Step 2 send JWTs under ApiKey-v1 and expect 401. Replace the
permissive check with a registry-based check that mirrors Hive's
stub pattern (`hive/lead/tests/e2e/jwks_stub_test.go:73-109`).

In `tests/e2e/harness/jwks_stub.go`, add a registry above
`StartJWKSStub`:

```go
// apiKeyEntry records the identity claims to return for a registered
// API key. Mirrors the structure Hive's e2e stub uses.
type apiKeyEntry struct {
	id, username, displayName, userType string
}
```

Add a registry field to `JWKSStub`:

```go
	apiKeys   map[string]apiKeyEntry
	apiKeysMu sync.Mutex
```

Initialise it in `StartJWKSStub` (alongside the `stub := &JWKSStub{}`
line, around line 100):

```go
	stub := &JWKSStub{apiKeys: make(map[string]apiKeyEntry)}
```

**API-key call-site audit (verified 2026-04-19 before tightening
the stub).** Tightening the verify-api-key handler from "accept any
non-empty key except INVALID" to "registry-only" risks 401-ing every
unregistered call site. The full audit:

| Call site | Sender | Header sent | Reaches JWKS stub? |
|---|---|---|---|
| `daemon_auth_test.go:54` (`TestDaemon_ApiKeyV1RoutesToVerify`) | test | `ApiKey-v1 harness-service-token` | YES — pre-register required |
| Task 2 `TestAuth_ApiKeyV1WithJWTReturns401` | test | `ApiKey-v1 <jwt-string>` | YES — should return 401 (jwt-string not in registry, by design) |
| `internal/infra/hive/adapter.go` (Flow → FakeHive) | daemon | `ApiKey-v1 harness-service-token` | NO — FakeHive doesn't validate (see `tests/e2e/harness/fake_hive.go:114-268` — no auth check on any handler) |
| `internal/infra/sharkfin/adapter.go` (Flow → FakeSharkfin) | daemon | `ApiKey-v1 harness-service-token` | NO — FakeSharkfin doesn't validate |
| `pylonclient.New` in `server.go:67` (Flow → PylonStub) | daemon | `ApiKey-v1 harness-service-token` | NO — PylonStub serves discovery without auth |

The JWKS stub is only reached when Flow's INBOUND middleware dispatches
an ApiKey-v1 header to the api-key validator, which then calls
`POST /v1/verify-api-key` against the JWKS stub. Outbound calls Flow
makes (to fake Hive/Sharkfin/Pylon) carry the same token but never
hit the JWKS stub.

Conclusion: pre-registering ONLY `harness-service-token` is sufficient.
No daemon-internal flow sends an unrelated API key to its own
validator. Add the pre-registration in the same block as
`stub := &JWKSStub{...}`:

```go
	stub.apiKeys["harness-service-token"] = apiKeyEntry{
		id: "00000000-0000-0000-0000-000000000099",
		username: "flow-e2e-apikey", displayName: "Flow E2E API Key",
		userType: "service",
	}
```

If a future test introduces a NEW API-key call site, that test must
register its key via `JWKSStub.MintAPIKey(...)` (added below) before
the call is made — the audit above is the contract; deviating from it
without re-running the audit is a Plan-A.5 regression.

Replace the `POST /v1/verify-api-key` handler body (lines 107-128) with:

```go
	mux.HandleFunc("POST /v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		stub.apiKeyHits.Add(1)
		var req struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "invalid request"})
			return
		}
		stub.apiKeysMu.Lock()
		ent, ok := stub.apiKeys[req.Key]
		stub.apiKeysMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"valid": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"key": map[string]any{
				"userId": ent.id,
				"metadata": map[string]any{
					"username":     ent.username,
					"name":         ent.displayName,
					"display_name": ent.displayName,
					"type":         ent.userType,
				},
			},
		})
	})
```

Add a public registration helper:

```go
// MintAPIKey registers a new API key and returns the key string.
// Tests that need a non-default api-key identity call this; tests
// content with the canned service token use it directly.
func (s *JWKSStub) MintAPIKey(key, id, username, displayName, userType string) {
	s.apiKeysMu.Lock()
	defer s.apiKeysMu.Unlock()
	s.apiKeys[key] = apiKeyEntry{id: id, username: username, displayName: displayName, userType: userType}
}
```

Add `"sync"` to the import block. The current file imports
`"sync/atomic"` (for `apiKeyHits atomic.Int64`), which is a different
package — `sync` and `sync/atomic` are NOT the same import. The new
`sync.Mutex` field requires the parent `sync` package.

**Step 4: Run the failing tests to confirm they fail for the right reason**

Run: `mise run e2e --backend=sqlite -run TestAuth_`
Expected: All six `TestAuth_*` tests PASS now (the implementation in
the daemon already supports them; Task 2 added test coverage and the
stub adjustments needed to express the expectations).

If `TestAuth_ApiKeyV1WithJWTReturns401` fails on a 200, the stub's
permissive branch wasn't tightened correctly — re-check Step 3.

**Pre-verified.** Reading
`passport/lead/go/service-auth/middleware.go:22-35` confirms
`parseAuthScheme` splits at the FIRST space and the dispatcher does
exact case-sensitive scheme equality (`switch scheme { case
SchemeBearer: ... case SchemeApiKeyV1: ... default: 401 }`). Therefore
`"ApiKey ABC"` parses to scheme=`"ApiKey"` (not `"ApiKey-v1"`), falls
through to `default`, returns 401. Same for every other case in the
table. `TestAuth_MalformedAuthorizationReturns401` MUST pass on every
subtest — no `t.Skip` is permitted.

**Step 5: Commit**

```
test(e2e): cover six remaining auth scenarios

Add Bearer-JWT happy-path, no-auth rejection, malformed-Authorization
(table-driven), expired-JWT, ApiKey-v1-with-JWT rejection, and
public-health skip. Tighten the JWKS stub's /v1/verify-api-key
handler to a registry-based check (mirrors hive's e2e stub) so JWTs
sent under ApiKey-v1 cannot pass the verify call. Adds JWKS stub
helper SignExpiredJWT and MintAPIKey.

Two existing scenarios already live in daemon_auth_test.go (Bearer
with API-key body → 401, ApiKey-v1 happy path); leave them in place.
```

---

### Task 3: Templates + Instances REST coverage (9 ops)

**Why before work-items.** These 9 ops (template CRUD × 5, instance
CRUD × 4) need no schema extension. They unblock Task 4's seed
extension because seeded templates are the input to the work-item
tests in Task 5.

**Depends on:** Task 1.

**Files:**
- Create: `tests/e2e/templates_test.go`
- Create: `tests/e2e/instances_test.go`

> **JSON-tag convention for every test struct in this plan.** The
> daemon emits snake_case JSON (e.g., `template_id`, `current_step_id`,
> `from_step_id`). Go's `encoding/json` does case-insensitive matching
> only over identical strings — `TemplateID` does NOT match
> `template_id` (the underscore is a difference, not a case). EVERY
> anonymous struct that decodes a daemon response MUST carry explicit
> `json:"snake_case"` tags on each multi-word field. Single-word
> fields (`Name`, `Title`, `Status`, `Decision`, `Comment`, `Priority`,
> `Version`, `ID`) match by case-folding and may omit the tag, but
> for consistency the code blocks below tag every field.

**Step 1: Write `templates_test.go`**

```go
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

	// Get after delete → 404
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
```

**Step 2: Write `instances_test.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

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

	// Update (status → paused)
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
```

**Step 3: Run tests, expect PASS**

Run: `mise run e2e --backend=sqlite -run TestTemplates_|TestInstances_`
Expected: All five tests PASS.

Run: `mise run e2e --backend=postgres -run TestTemplates_|TestInstances_`
Expected: All five tests PASS.

**Step 4: Commit**

```
test(e2e): cover all template + instance REST endpoints

Five tests cover the 9 production REST ops on /v1/templates and
/v1/instances: list (empty + filtered), create, get, update, delete,
plus a 404 lookup. Tests run on both SQLite and Postgres backends.
Adds an authedClient helper and a createBareTemplate helper used by
later work-item tests.
```

---

### Task 4: Extend `PatchTemplateInput` to accept steps + transitions

**Why.** Without this, 8 work-item ops cannot be exercised E2E because
no REST path can write Steps to a template (see "Step/transition
seeding gap" in the Overview). The store layer already supports
replacing Steps/Transitions through `UpdateTemplate`; only the handler
discards them. This task is the smallest change that unblocks Task 5.

**Depends on:** Task 3 (the template tests are the regression net for
this change).

**Files:**
- Modify: `internal/daemon/rest_types.go:80-86` (extend
  `PatchTemplateInput`)
- Modify: `internal/daemon/rest_huma.go:178-202` (the `update-template`
  handler)
- Modify: `internal/infra/sqlite/templates.go` (Step 0 — add the
  missing DELETEs for transitions/role_mappings/integration_hooks)
- Modify: `internal/infra/postgres/templates.go` (Step 0 — same)
- Modify: `tests/e2e/templates_test.go` (add a test asserting the
  steps/transitions PATCH path round-trips through GET, plus a
  multi-PATCH test that proves the new DELETEs are correct)

**Step 0: Fix latent UpdateTemplate UPSERT bug (both stores)**

The plan's correctness rests on `UpdateTemplate` having
"replace-on-write semantics" for steps and transitions. Verified
behaviour:

| Collection | sqlite | postgres |
|---|---|---|
| `steps` | `DELETE FROM steps WHERE template_id = ?` precedes INSERT loop (`internal/infra/sqlite/templates.go:265`) | Same — `internal/infra/postgres/templates.go:265` |
| `transitions` | INSERT only — no DELETE | INSERT only — no DELETE |
| `role_mappings` | INSERT only — no DELETE | INSERT only — no DELETE |
| `integration_hooks` | INSERT only — no DELETE | INSERT only — no DELETE |

A second PATCH that includes `transitions[]` would trip a UNIQUE
constraint on the transitions PK (or duplicate, depending on schema).
The plan's single-PATCH happy-path test in Step 4 below would not
surface this, but the public REST contract this plan introduces
("PATCH /v1/templates/{id} with `transitions[]` replaces the
collection") would silently fail.

In `internal/infra/sqlite/templates.go`, immediately after the
existing `DELETE FROM steps` on line ~265 (and the subsequent step
INSERT loop), insert THREE additional DELETE statements before the
respective INSERT loops:

```go
	if _, err := tx.ExecContext(ctx, "DELETE FROM transitions WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete transitions: %w", err)
	}
	// (existing transitions INSERT loop follows)
```

```go
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_mappings WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete role_mappings: %w", err)
	}
	// (existing role_mappings INSERT loop follows)
```

```go
	if _, err := tx.ExecContext(ctx, "DELETE FROM integration_hooks WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete integration_hooks: %w", err)
	}
	// (existing integration_hooks INSERT loop follows)
```

Apply the same three DELETEs in `internal/infra/postgres/templates.go`'s
`UpdateTemplate`, using `$1` as the placeholder instead of `?`:

```go
	if _, err := tx.ExecContext(ctx, "DELETE FROM transitions WHERE template_id = $1", t.ID); err != nil {
		return fmt.Errorf("delete transitions: %w", err)
	}
	// ...
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_mappings WHERE template_id = $1", t.ID); err != nil {
		return fmt.Errorf("delete role_mappings: %w", err)
	}
	// ...
	if _, err := tx.ExecContext(ctx, "DELETE FROM integration_hooks WHERE template_id = $1", t.ID); err != nil {
		return fmt.Errorf("delete integration_hooks: %w", err)
	}
```

Both DELETEs run inside the same transaction `UpdateTemplate` already
opens, so atomicity is preserved.

Run unit tests for the store layer to confirm no regression:

Run: `mise run test`
Expected: PASS (the existing UpdateTemplate tests in
`internal/infra/sqlite/store_test.go` and
`internal/infra/postgres/store_test.go` should keep passing — the new
DELETEs are no-ops on the first call.)

**Commit Step 0 separately so the latent-bug fix is bisectable:**

```
fix(stores): UpdateTemplate must delete transitions/role_mappings/hooks

UpdateTemplate already DELETEs the steps collection before INSERTing
the new set, but transitions, role_mappings, and integration_hooks
were going straight to INSERT. A second update on a template that
already had any of these would either trip a UNIQUE PRIMARY KEY
violation or silently duplicate rows. Apply the same DELETE-then-
INSERT pattern uniformly for all four collections, in both sqlite
and postgres stores. No behaviour change on first UpdateTemplate call.

Surfaces as a precondition for plan A.5's PATCH-seed extension —
without this, the new public REST contract ("PATCH replaces
transitions") would silently fail on the second call.
```

**Step 1: Extend `PatchTemplateInput.Body`**

Replace `internal/daemon/rest_types.go:80-86` with:

```go
type PatchTemplateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name        string `json:"name,omitempty" doc:"Template name"`
		Description string `json:"description,omitempty" doc:"Template description"`
		// Steps, when non-nil, REPLACES the template's step set.
		// Each Step's TemplateID is overwritten with the URL path's id.
		// Mirrors store.UpdateTemplate's replace-on-write semantics
		// (internal/infra/sqlite/templates.go:265 deletes-then-inserts;
		// the other three collections are deleted-then-inserted by
		// the Step 0 fix above).
		Steps []stepInput `json:"steps,omitempty"`
		// Transitions, when non-nil, REPLACES the template's transition
		// set. Each Transition's TemplateID is overwritten with the
		// URL path's id.
		Transitions []transitionInput `json:"transitions,omitempty"`
	}
}

// stepInput is the request-shape of Step. Mirrors the response-shape
// stepResponse but flips client-supplied fields to required and drops
// server-only fields.
type stepInput struct {
	ID       string             `json:"id" doc:"Step ID (client-supplied)"`
	Key      string             `json:"key" doc:"Step key (unique within template)"`
	Name     string             `json:"name" doc:"Step display name"`
	Type     string             `json:"type" enum:"task,gate" doc:"Step type"`
	Position int                `json:"position" doc:"Display order"`
	Approval *stepApprovalInput `json:"approval,omitempty"`
}

type stepApprovalInput struct {
	Mode              string `json:"mode" enum:"any,unanimous"`
	RequiredApprovers int    `json:"required_approvers"`
	ApproverRoleID    string `json:"approver_role_id"`
	RejectionStepID   string `json:"rejection_step_id,omitempty"`
}

type transitionInput struct {
	ID             string `json:"id" doc:"Transition ID (client-supplied)"`
	Key            string `json:"key" doc:"Transition key"`
	Name           string `json:"name" doc:"Display name"`
	FromStepID     string `json:"from_step_id" doc:"Source step ID"`
	ToStepID       string `json:"to_step_id" doc:"Destination step ID"`
	Guard          string `json:"guard,omitempty" doc:"CEL guard expression"`
	RequiredRoleID string `json:"required_role_id,omitempty"`
}
```

**Step 2: Update the `update-template` handler**

Replace the handler body in `internal/daemon/rest_huma.go:178-202`
(the closure passed to `huma.Register` for `update-template`) with:

```go
	}, func(ctx context.Context, input *PatchTemplateInput) (*TemplateOutput, error) {
		existing, err := store.GetTemplate(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Name != "" {
			existing.Name = input.Body.Name
		}
		if input.Body.Description != "" {
			existing.Description = input.Body.Description
		}
		if input.Body.Steps != nil {
			existing.Steps = make([]domain.Step, len(input.Body.Steps))
			for i, s := range input.Body.Steps {
				step := domain.Step{
					ID: s.ID, TemplateID: input.ID, Key: s.Key,
					Name: s.Name, Type: domain.StepType(s.Type),
					Position: s.Position,
				}
				if s.Approval != nil {
					step.Approval = &domain.ApprovalConfig{
						Mode:              domain.ApprovalMode(s.Approval.Mode),
						RequiredApprovers: s.Approval.RequiredApprovers,
						ApproverRoleID:    s.Approval.ApproverRoleID,
						RejectionStepID:   s.Approval.RejectionStepID,
					}
				}
				existing.Steps[i] = step
			}
		}
		if input.Body.Transitions != nil {
			existing.Transitions = make([]domain.Transition, len(input.Body.Transitions))
			for i, tr := range input.Body.Transitions {
				existing.Transitions[i] = domain.Transition{
					ID: tr.ID, TemplateID: input.ID, Key: tr.Key,
					Name: tr.Name, FromStepID: tr.FromStepID,
					ToStepID: tr.ToStepID, Guard: tr.Guard,
					RequiredRoleID: tr.RequiredRoleID,
				}
			}
		}
		if err := store.UpdateTemplate(ctx, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetTemplate(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TemplateOutput{Body: templateToResponse(updated)}, nil
	})
```

**Step 3: Add a regression test in `templates_test.go`**

Append to `tests/e2e/templates_test.go`:

```go
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
// constraint violation or surfaces 4 transitions instead of 1.
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
```

**Step 4: Run the new test, expect FAIL on master**

Run (after Step 1 + Step 2 are still un-applied):
`mise run e2e --backend=sqlite -run TestTemplates_PatchSeedsStepsAndTransitions`
Expected: FAIL — old handler ignores `steps`, GET returns empty.

After applying Step 1 + Step 2:
Run: `mise run e2e --backend=sqlite -run TestTemplates_PatchSeedsStepsAndTransitions`
Expected: PASS.

Run: `mise run e2e --backend=postgres -run TestTemplates_PatchSeedsStepsAndTransitions`
Expected: PASS.

Re-run all template tests to confirm no regression:
Run: `mise run e2e --backend=sqlite -run TestTemplates_`
Expected: All four tests PASS.

**Step 5: Commit (one commit, after Step 0's fix is already on disk)**

```
feat(rest): allow PATCH /v1/templates/{id} to seed steps + transitions

PatchTemplateInput now accepts optional steps[] and transitions[]
arrays. When present, the handler replaces the template's collections
and delegates to store.UpdateTemplate. Without this, no public REST
path could write the step graph a work item needs to exist.

Combined with the prior commit fixing the missing DELETEs in
UpdateTemplate, the public contract ("PATCH replaces steps and
transitions") now matches store behaviour.

Adds two e2e tests: a round-trip seed asserting the shape survives a
subsequent GET, and a multi-PATCH test asserting the second PATCH
replaces (not appends to) the first.
```

---

### Task 5: Work-item, transition, and approval REST coverage (8 ops)

**Why.** This is the second half of the 18-op coverage. Every op
exercises the spawned daemon end-to-end, with the template seeded
through the Task 4 PATCH extension.

**Depends on:** Task 4 (uses the new step/transition seed path).

**Files:**
- Create: `tests/e2e/items_test.go`
- Create: `tests/e2e/transitions_test.go`
- Create: `tests/e2e/approvals_test.go`

**Step 1: Add a `seedThreeStepTemplate` helper**

Append to `tests/e2e/templates_test.go` (the helper sits with the
other template helpers):

```go
// seedThreeStepTemplate seeds the canonical 3-step / 2-transition
// shape items_test.go and transitions_test.go expect:
//
//   stp_todo  --trn_submit-->  stp_review (gate)  --trn_approve-->  stp_done
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
```

**Step 2: Write `items_test.go` (covers ops 10-14)**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestWorkItems_CreateGetListUpdate(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-W")

	// Create work item.
	var wi struct {
		ID            string `json:"id"`
		InstanceID    string `json:"instance_id"`
		Title         string `json:"title"`
		CurrentStepID string `json:"current_step_id"`
		Priority      string `json:"priority"`
	}
	createReq := map[string]any{
		"title": "First task", "description": "Body",
		"assigned_agent_id": "a_001", "priority": "high",
	}
	status, body, err := c.PostJSON("/v1/instances/"+instID+"/items", createReq, &wi)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create: status=%d err=%v body=%s", status, err, body)
	}
	if wi.InstanceID != instID || wi.Title != "First task" || wi.Priority != "high" {
		t.Errorf("create response: %+v", wi)
	}
	if wi.CurrentStepID != "stp_todo" {
		t.Errorf("first step id: got %q, want stp_todo", wi.CurrentStepID)
	}

	// List by instance.
	var list []struct {
		ID              string `json:"id"`
		Title           string `json:"title"`
		AssignedAgentID string `json:"assigned_agent_id"`
		Priority        string `json:"priority"`
	}
	status, body, err = c.GetJSON("/v1/instances/"+instID+"/items", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list: status=%d err=%v body=%s", status, err, body)
	}
	if len(list) != 1 || list[0].ID != wi.ID {
		t.Errorf("list: %+v", list)
	}

	// List filtered by agent_id.
	var byAgent []struct {
		ID string `json:"id"`
	}
	status, _, _ = c.GetJSON("/v1/instances/"+instID+"/items?agent_id=a_001", &byAgent)
	if status != http.StatusOK || len(byAgent) != 1 {
		t.Errorf("filter agent_id: status=%d len=%d", status, len(byAgent))
	}
	var byOther []struct {
		ID string `json:"id"`
	}
	status, _, _ = c.GetJSON("/v1/instances/"+instID+"/items?agent_id=a_999", &byOther)
	if status != http.StatusOK || len(byOther) != 0 {
		t.Errorf("filter agent_id no-match: status=%d len=%d", status, len(byOther))
	}

	// Get single.
	var got struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Priority string `json:"priority"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wi.ID, &got)
	if err != nil || status != http.StatusOK {
		t.Fatalf("get: status=%d err=%v", status, err)
	}
	if got.Title != "First task" || got.Priority != "high" {
		t.Errorf("get: %+v", got)
	}

	// Patch.
	patchReq := map[string]any{"title": "Renamed task"}
	var patched struct {
		Title string `json:"title"`
	}
	status, body, err = c.PatchJSON("/v1/items/"+wi.ID, patchReq, &patched)
	if err != nil || status != http.StatusOK {
		t.Fatalf("patch: status=%d err=%v body=%s", status, err, body)
	}
	if patched.Title != "Renamed task" {
		t.Errorf("patch title: %q", patched.Title)
	}
}

func TestWorkItems_HistoryEmptyBeforeTransition(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-H")

	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "T"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}

	var hist []map[string]any
	status, _, err := c.GetJSON("/v1/items/"+wi.ID+"/history", &hist)
	if err != nil || status != http.StatusOK {
		t.Fatalf("history: status=%d err=%v", status, err)
	}
	if len(hist) != 0 {
		t.Errorf("history before any transition: %+v", hist)
	}
}
```

**Step 3: Write `transitions_test.go` (covers op 15)**

```go
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

	// Submit (todo → review).
	transReq := map[string]any{
		"transition_id":  "trn_submit",
		"actor_agent_id": "a_actor", "actor_role_id": "role_dev",
		"reason":         "ready for review",
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
```

**Step 4: Write `approvals_test.go` (covers ops 16, 17, 18)**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestApprovals_ApprovePath(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-AP")

	wiID := createAndAdvance(t, c, instID, "stp_review")

	approveReq := map[string]any{"agent_id": "a_reviewer", "comment": "LGTM"}
	var approved struct {
		CurrentStepID string `json:"current_step_id"`
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/approve", approveReq, &approved)
	if err != nil || status != http.StatusOK {
		t.Fatalf("approve: status=%d err=%v body=%s", status, err, body)
	}
	if approved.CurrentStepID != "stp_done" {
		t.Errorf("after approve: %q, want stp_done", approved.CurrentStepID)
	}

	// List approvals — should have one approved entry.
	var list []struct {
		WorkItemID string `json:"work_item_id"`
		AgentID    string `json:"agent_id"`
		Decision   string `json:"decision"`
		Comment    string `json:"comment"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wiID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 1 {
		t.Fatalf("approvals len: %d", len(list))
	}
	got := list[0]
	if got.AgentID != "a_reviewer" || got.Decision != "approved" || got.Comment != "LGTM" {
		t.Errorf("approval entry: %+v", got)
	}
}

func TestApprovals_RejectPath(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-RJ")

	wiID := createAndAdvance(t, c, instID, "stp_review")

	rejectReq := map[string]any{"agent_id": "a_reviewer", "comment": "needs work"}
	var rejected struct {
		CurrentStepID string `json:"current_step_id"`
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/reject", rejectReq, &rejected)
	if err != nil || status != http.StatusOK {
		t.Fatalf("reject: status=%d err=%v body=%s", status, err, body)
	}
	// Verified by reading internal/workflow/service.go:335-397 — RejectItem
	// with empty RejectionStepID succeeds, records the Approval, and
	// leaves CurrentStepID unchanged. Assert only that the call
	// succeeded and the approval was recorded; do not assert the
	// destination step, which is the workflow service's contract,
	// not the REST surface's.
	var list []struct {
		AgentID  string `json:"agent_id"`
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	status, _, err = c.GetJSON("/v1/items/"+wiID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 1 {
		t.Fatalf("approvals len: %d", len(list))
	}
	if list[0].Decision != "rejected" || list[0].Comment != "needs work" {
		t.Errorf("approval: %+v", list[0])
	}
}

func TestApprovals_ListEmptyBeforeAnyDecision(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	c := authedClient(t, env)

	tplID := seedThreeStepTemplate(t, c)
	instID := seedActiveInstance(t, c, tplID, "team-EM")

	var wi struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "T"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}

	var list []map[string]any
	status, _, err := c.GetJSON("/v1/items/"+wi.ID+"/approvals", &list)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list approvals: status=%d err=%v", status, err)
	}
	if len(list) != 0 {
		t.Errorf("approvals empty: %+v", list)
	}
}

// createAndAdvance creates a work item and submits it to the named
// step via trn_submit. Used by approval tests that need a work item
// already at the gate.
func createAndAdvance(t *testing.T, c *harness.Client, instID, wantStep string) string {
	t.Helper()
	var wi struct {
		ID            string `json:"id"`
		CurrentStepID string `json:"current_step_id"`
	}
	if status, body, err := c.PostJSON("/v1/instances/"+instID+"/items",
		map[string]any{"title": "to-be-advanced"}, &wi); err != nil || status != http.StatusCreated {
		t.Fatalf("create wi: status=%d err=%v body=%s", status, err, body)
	}
	transReq := map[string]any{
		"transition_id": "trn_submit", "actor_agent_id": "a_dev", "actor_role_id": "role_dev",
	}
	var moved struct {
		CurrentStepID string `json:"current_step_id"`
	}
	if status, body, err := c.PostJSON("/v1/items/"+wi.ID+"/transition", transReq, &moved); err != nil || status != http.StatusOK {
		t.Fatalf("submit: status=%d err=%v body=%s", status, err, body)
	}
	if moved.CurrentStepID != wantStep {
		t.Fatalf("advance: at %q, want %q", moved.CurrentStepID, wantStep)
	}
	return wi.ID
}
```

**Step 5: Run all REST tests on both backends**

Run: `mise run e2e --backend=sqlite -run TestWorkItems_|TestTransitions_|TestApprovals_`
Expected: All seven tests PASS.

Run: `mise run e2e --backend=postgres -run TestWorkItems_|TestTransitions_|TestApprovals_`
Expected: All seven tests PASS.

**Pre-verified (no Skip allowed).** `internal/workflow/service.go:335-397`
(`RejectItem`) tolerates `RejectionStepID == ""`: it records the
Approval, leaves `CurrentStepID` unchanged, and returns 200. The
seeded gate step uses an empty `ApproverRoleID` so no role check
fires. `TestApprovals_RejectPath` MUST pass on both backends; if it
returns non-2xx, the failure is real and must be fixed in this plan
(do not `t.Skip`, do not file as a follow-up). Per
`feedback_no_test_failures.md`.

**Step 6: Commit (in two commits to keep diffs reviewable)**

First commit:

```
test(e2e): cover work-item REST endpoints (ops 10-14)

Five ops on /v1/instances/{id}/items and /v1/items/{id}: create,
list (with filters), get, patch, history. Adds the
seedThreeStepTemplate and seedActiveInstance helpers used by the
transition and approval suites.
```

Second commit:

```
test(e2e): cover transition + approval REST endpoints (ops 15-18)

Three suites: transitions (happy path + invalid-transition rejection),
approvals (approve advances to next step, reject records the decision,
list empty before any decision). Adds the createAndAdvance helper
that gets a work item to the gate step.
```

---

### Task 6: Run the full suite, both backends

**Why.** Per `feedback_no_test_failures.md` and `feedback_monitor_every_push.md`,
red CI is unacceptable. Confirm the entire suite is green before the
README task.

**Depends on:** Tasks 1-5.

**Step 1: Run full e2e suite, SQLite**

Run: `mise run e2e --backend=sqlite`
Expected: ALL tests PASS — Plan A's 2 health, Foundation's 4
(agent_pool, audit, runtime ×2), the original 2 daemon_auth tests,
plus this plan's additions (~17 new tests).

**Step 2: Run full e2e suite, Postgres**

Run: `mise run e2e --backend=postgres`
Expected: ALL tests PASS.

**Step 3: Run unit + e2e together via the ci task**

Run: `mise run ci`
Expected: lint + test + e2e all green.

If any pre-existing test fails (i.e., not added by this plan), STOP
and report — per `feedback_no_test_failures.md`, do not file
"pre-existing" issues, fix the failure inline. Likely culprits if
this surfaces: the JWKS-stub tightening in Task 2 Step 3 may have
broken a non-auth test that relied on the over-permissive verify-api-
key path. The harness service token registration in the same step is
the safety net — verify it's correct.

---

### Task 7: Harness README

**Why.** Plan A's "out of scope" list explicitly deferred the README
to Plan A.5. With auth + REST coverage now broad enough to be
representative, the README captures the patterns once.

**Depends on:** Tasks 1-6.

**Files:**
- Create: `tests/e2e/README.md`

**Step 1: Write the README**

Create `tests/e2e/README.md`:

````markdown
<!-- SPDX-License-Identifier: GPL-2.0-only -->
# Flow E2E test harness

End-to-end tests in this directory spawn a real `flow daemon` subprocess
wired to in-process fakes, then drive it over raw HTTP. Every contract
is wire-verified — the harness imports zero WorkFort Go client
packages, so a wire-format drift between Flow and any consumer
surfaces as a test failure.

## How it fits together

```
┌──────── tests/e2e (nested go module) ────────┐
│                                              │
│  ┌──────────────┐    ┌──────────────────┐    │
│  │ harness pkg  │    │ test packages    │    │
│  │              │    │ (auth, items, …) │    │
│  │ ▸ Daemon     │    │  uses raw HTTP   │    │
│  │ ▸ JWKS stub  │    │  via harness.Client │
│  │ ▸ Pylon stub │    │                  │    │
│  │ ▸ FakeHive   │    │                  │    │
│  │ ▸ FakeShark  │    │                  │    │
│  └──────┬───────┘    └────────┬─────────┘    │
└─────────┼─────────────────────┼──────────────┘
          ▼                     ▼
   ┌────────────────────┐  ┌─────────────────┐
   │ flow daemon (real) │◀─│ raw net/http    │
   └────────┬───────────┘  └─────────────────┘
            │ outbound HTTP
            ▼
   ┌─────────────────────────────────────────┐
   │ fakes (httptest.Server) — plain JSON    │
   │ passport, pylon, hive, sharkfin         │
   └─────────────────────────────────────────┘
```

## Running

| Command | What it does |
|---|---|
| `mise run e2e` | Default: SQLite backend |
| `mise run e2e --backend=sqlite` | Same as default |
| `mise run e2e --backend=postgres` | PG backend (DSN from `FLOW_E2E_PG_DSN` or default) |
| `mise run e2e --backend=sqlite -run TestAuth_` | Filter by test name |
| `mise run ci` | lint + unit tests + e2e (sqlite) |

CI runs both backends as parallel jobs.

## Per-test backend pattern

Every test gets a fresh daemon and a fresh DB. Postgres reset uses
`DROP SCHEMA public CASCADE` before the daemon starts; SQLite uses a
new file per spawn. There is no test pooling — each `harness.NewEnv`
call costs ~200ms (subprocess fork + readiness poll), but daemon
isolation is non-negotiable to avoid cross-test state bleed.

Tests do NOT call `t.Parallel()`. Sequential is the contract.

## Adding a new endpoint test

1. Decide which test file the endpoint belongs to:
   - templates / instances → `templates_test.go` / `instances_test.go`
   - work items / transitions / approvals → `items_test.go` /
     `transitions_test.go` / `approvals_test.go`
   - new auth scenario → `auth_test.go`
   - new diag endpoint (`/v1/.../_diag/...`) → name it after the
     subsystem (`runtime_diag_test.go`, `audit_events_test.go`)
2. Stand up the env in the first lines:

   ```go
   env := harness.NewEnv(t)
   defer env.Cleanup(t)
   c := authedClient(t, env) // or harness.NewClientNoAuth, etc.
   ```

3. Use one of the seed helpers in `templates_test.go` if you need a
   fully-formed template:
   - `createBareTemplate(t, c, name) string` — empty template
   - `seedThreeStepTemplate(t, c) string` — todo→review (gate)→done
   - `seedActiveInstance(t, c, tplID, teamID) string`
   - `createAndAdvance(t, c, instID, wantStep) string` — work item
     pre-positioned at the named step
4. Drive the endpoint with `c.GetJSON / PostJSON / PatchJSON / DeleteJSON`.
5. Assert the status code AND a couple of fields on the decoded body.
   The `body []byte` return value is for failure diagnostics — log it
   in `t.Fatalf` so a regression shows the wire payload.

## The FakeHive / FakeSharkfin wire-format approach

Both fakes serve hand-rolled JSON via `net/http`. They do NOT import
the real Hive/Sharkfin server-side handlers. If a real-service handler
bug were the only thing keeping a contract test green, this harness
should catch it — that is the load-bearing reason for not reusing the
real handlers.

Field tags on the wire types (`HiveAgent`, `PoolAgent`,
`SharkfinMessage`) match what the real services emit (capitalised
JSON names where Hive uses them, snake_case where Sharkfin uses them).
When extending a fake, copy the wire shape from the real service's
type, do not import it.

Rationale: see `feedback_e2e_harness_independence.md` in the project
memory.

## Auth model (post-Passport-scheme-split)

| Header | Routes to | Use |
|---|---|---|
| `Authorization: Bearer <jwt>` | JWT validator only | `harness.NewClient(url, jwt)` |
| `Authorization: ApiKey-v1 <key>` | API-key validator only | `harness.NewClientAPIKey(url, key)` |
| (none) | public-path skip → through to mux for `/v1/health`, `/ui/health`; 401 elsewhere | `harness.NewClientNoAuth(url)` |
| Anything else | 401 | `harness.NewClientRawAuth(url, raw)` (negative tests only) |

The JWKS stub mints the per-test JWT via `env.Daemon.SignJWT` (audience
`flow`, 1-hour expiry). For an expired token use
`env.JWKS.SignExpiredJWT(...)`. For a non-default API-key identity,
register it with `env.JWKS.MintAPIKey(...)` and pass the returned key
into `NewClientAPIKey`.

The harness service token (`harness-service-token`, baked into
`StartDaemon`) is pre-registered with the JWKS stub, so tests that
exercise the daemon's outbound path through Pylon/Hive/Sharkfin work
without per-test API-key minting.

## Environment variables the harness reads

| Variable | Used by | Default | Notes |
|---|---|---|---|
| `FLOW_BINARY` | `harness.NewEnv` (`harness/env.go:73`) | `../../build/flow` | `mise run e2e` sets this to a fresh `e2e`-tagged build |
| `FLOW_DB` | `harness.StartDaemon` (`harness/daemon.go:102`) | (unset; SQLite default) | Used as `--db <dsn>` arg when set; takes precedence over `WithDB(...)` |
| `FLOW_E2E_PG_DSN` | `harness.NewEnv` PG backend (`harness/env.go:97`) | `postgres://postgres@127.0.0.1/flow_test?sslmode=disable` | Only consulted when backend is `postgres` |
| `FLOW_E2E_RUNTIME_STUB` | `harness.WithStubRuntime` (`harness/daemon.go:130`) | (unset) | When `1`, the daemon binds the stub `RuntimeDriver` so `/v1/runtime/_diag/*` works |
| `FLOW_LEASE_RENEWER_INTERVAL` | daemon, set by harness (`harness/daemon.go:126`) | `100ms` | Override only if a test needs a different cadence |
| `FLOW_LEASE_TTL` | daemon, set by harness (`harness/daemon.go:127`) | `2s` | Same |

CI sets `FLOW_E2E_PG_DSN` to its service-container address; locally,
the default points at the host's `postgres` peer-trust user.

## Extending the harness

If you need a new wire fixture (e.g., a Hive endpoint that doesn't
exist in `fake_hive.go` yet), add the handler to the appropriate
`fake_*.go` file:

1. Copy the wire shape from the real service. Add fields exactly as
   the real service emits them (don't shorten, don't add fields the
   real service doesn't emit).
2. Add a recording field on the fake struct so tests can assert
   on what Flow sent.
3. Register the route with `mux.HandleFunc("METHOD /path", ...)`.
4. If the route requires per-call state (claim/release pairing, etc.),
   protect it with the existing mutex.

## Orphan-process hardening

The harness uses Setpgid + negative-pid SIGTERM/SIGKILL + `*os.File`
stdio + `WaitDelay` so daemon descendants cannot leak past `Cleanup`.
The leak-detection test in `harness/daemon_leak_test.go` enforces
this. If you add a new fake that spawns goroutines that outlive the
daemon, ensure your `stop` function joins them — the leak test will
not catch goroutine leaks inside the test process.

See `skills/lead/go-service-architecture/` for the canonical pattern.
````

**Step 2: Verify the README renders cleanly**

Run: `cat tests/e2e/README.md | head`
Expected: SPDX header on line 1, no rendering glitches.

**Step 3: Commit**

```
docs(e2e): add tests/e2e/README explaining the harness

Documents how to add a new endpoint test, the per-test backend
pattern, the fake wire-format approach, every env var the harness
reads, and how to run the suite against both backends. Captures the
auth-scheme dispatch model and the orphan-process hardening rules
in one place so future contributors don't have to reverse-engineer
either from the harness source.
```

---

## Verification checklist

After all tasks land:

- [ ] `mise run e2e --backend=sqlite` is green
- [ ] `mise run e2e --backend=postgres` is green
- [ ] `mise run ci` is green
- [ ] `grep -c "OperationID:" internal/daemon/rest_huma.go` returns 18
- [ ] Every endpoint in the table at the top of this plan has at least
      one test in `tests/e2e/`. Verify by name: list-templates →
      `TestTemplates_ListEmpty`, create-template →
      `TestTemplates_CreateGetUpdateDelete`, get-template → same,
      update-template → same + `TestTemplates_PatchSeedsStepsAndTransitions`,
      delete-template → `TestTemplates_CreateGetUpdateDelete`,
      list-instances → `TestInstances_ListEmpty` +
      `TestInstances_CreateGetUpdateList`, create-instance / get-instance
      / update-instance → `TestInstances_CreateGetUpdateList`,
      create-work-item / list-work-items / get-work-item /
      update-work-item / get-work-item-history → `TestWorkItems_*`,
      transition-work-item → `TestTransitions_*`, approve-work-item /
      reject-work-item / list-approvals → `TestApprovals_*`.
- [ ] Auth scenarios from the requirements all have a test:
      Bearer-JWT happy → `TestAuth_BearerJWTHappyPath`; ApiKey-v1
      happy → `TestDaemon_ApiKeyV1RoutesToVerify` (existing); no-auth
      → `TestAuth_NoAuthHeaderReturns401`; malformed →
      `TestAuth_MalformedAuthorizationReturns401` (table-driven);
      expired → `TestAuth_ExpiredJWTReturns401`; Bearer-with-API-key
      → `TestDaemon_BearerForAPIKeyReturns401` (existing); ApiKey-v1-
      with-JWT → `TestAuth_ApiKeyV1WithJWTReturns401`; public-path
      → `TestAuth_PublicHealthSkipsAuth`.
- [ ] No `t.Skip` in this plan's added tests — neither in the
      reject-path test (RejectItem with empty RejectionStepID is
      verified to succeed at `internal/workflow/service.go:335-397`)
      nor in the malformed-Authorization table-driven test (verified
      at `passport/lead/go/service-auth/middleware.go:22-35`).
- [ ] Multi-PATCH regression test (`TestTemplates_PatchSeedsTwiceReplaces`)
      passes — confirms the Step 0 fix to UpdateTemplate's missing
      DELETE for transitions/role_mappings/integration_hooks.
- [ ] Every anonymous struct receiver in this plan's tests carries
      explicit `json:"snake_case"` tags on multi-word fields. Single-
      word fields may rely on case-folding but are tagged for
      consistency.
- [ ] `tests/e2e/README.md` exists and lists every env var the harness
      currently reads.
- [ ] No new dependencies in `tests/e2e/go.mod` (this plan uses only
      packages Plan A already pulled in).
- [ ] No commit message in this plan's series carries `!` markers or
      `BREAKING CHANGE:` footers.
- [ ] All commits authored with multi-line conventional format and
      `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`.
