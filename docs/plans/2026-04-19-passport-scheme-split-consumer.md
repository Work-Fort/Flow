---
type: plan
step: "1"
title: "Passport scheme split — Flow consumer migration"
status: approved
assessment_status: complete
provenance:
  source: cross-repo-coordination
  issue_id: "Cluster 3b (Passport, 2026-04-19)"
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - passport/lead/docs/plans/2026-04-19-auth-scheme-dispatch.md
  - sharkfin/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - hive/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - pylon/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - combine/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
---

# Flow — Passport Scheme Split Consumer Migration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update Flow's three downstream-service adapters (Hive, Sharkfin, Pylon) to send the daemon's service token under the `ApiKey-v1` scheme. The service token is a Passport API key (`wf-svc_*`), not a JWT, so all three call sites are migrating from misuse-of-Bearer to correct-API-key-scheme. The daemon's inbound auth middleware also flips from `NewFromValidators` to `NewSchemeDispatch`.

**Background / Why:** Per TPM clarification 2026-04-19: only web browser clients use JWT; agents and services use API keys. Flow is a service caller — every outbound call carries a `wf-svc_*` API key. The latent bug at `internal/infra/sharkfin/adapter.go:31` (Flow passing a `wf-svc_*` API key into sharkfin's `WithToken`, which is the JWT option) became visible when planning the scheme dispatch — the call only worked because passport's middleware fell through from JWT to API-key validation. Per the upstream simplifications, sharkfin renames `WithToken` → `WithAPIKey`, hive renames its constructor parameter, and pylon does the same; Flow's adapter changes become pure mechanical renames. Flow's inbound middleware still needs both schemes (browser-routed JWT via Scope; ApiKey-v1 from agents) — that's the only place JWT-side complexity remains.

**Architecture:** Flow today builds three downstream clients with `cfg.ServiceToken` (a `wf-svc_*` API key from `--service-token`):

1. `pylonclient.New(cfg.PylonURL, cfg.ServiceToken)` — Pylon's client renames its parameter to `apiKey`; the call signature is unchanged but the wire format flips to `ApiKey-v1`. Pure dependency bump on Flow's side.
2. `hiveclient.New(hiveSvc.BaseURL, cfg.ServiceToken)` — same as Pylon; pure dependency bump.
3. `sharkfininfra.New(...)` calls `sharkfinclient.NewRESTClient(baseURL, sharkfinclient.WithToken(token))` at `internal/infra/sharkfin/adapter.go:31` — `WithToken` is removed; switch to `sharkfinclient.WithAPIKey(token)`. This is the load-bearing edit on Flow's side.

The inbound `NewFromValidators(...)` in `internal/daemon/server.go:129` (verified at planning time — see also lines 118-130 for the surrounding init) becomes `NewSchemeDispatch(jwtV, apiKeyV)`. Both validators are required; if `jwt.New` failed at startup (line 118-122 logs a warning and leaves `jwtV == nil`), substitute `auth.AlwaysFail(...)` (exported by passport's `service-auth`) so the constructor's non-nil precondition holds. **Do NOT reimplement the always-fail stub locally** — the helper exists upstream specifically so consumers don't diverge.

**Tech Stack:** Go 1.x. Depends on per-tag bumps of `pylonclient`, `hiveclient`, `sharkfinclient`, and `service-auth`.

---

## Conventions

- Conventional Commits multi-line + `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` per commit.
- `mise run test`, `mise run e2e`, `mise run lint` for verification.
- Pin all four service-auth-related deps to local branches via `replace` until each upstream is published.

---

## Pre-flight: pin all four upstream packages locally

Add to root `go.mod`. **Module paths are case-sensitive** — verified at planning time against each upstream's `go.mod`:

- `github.com/Work-Fort/Passport/go/service-auth` (capital `P` per passport's `go.mod`)
- `github.com/Work-Fort/Hive/client` (capital `H` per hive's `client/go.mod`)
- `github.com/Work-Fort/Pylon/client/go` (capital `P` per pylon's `client/go/go.mod`)
- `github.com/Work-Fort/sharkfin/client/go` (lowercase `s` per sharkfin's `client/go/go.mod`)

```
replace (
	github.com/Work-Fort/Passport/go/service-auth => /home/kazw/Work/WorkFort/passport/lead/go/service-auth
	github.com/Work-Fort/Hive/client => /home/kazw/Work/WorkFort/hive/lead/client
	github.com/Work-Fort/Pylon/client/go => /home/kazw/Work/WorkFort/pylon/lead/client/go
	github.com/Work-Fort/sharkfin/client/go => /home/kazw/Work/WorkFort/sharkfin/lead/client/go
)
```

`go mod tidy`, commit. Each upstream branch must be checked out locally to the scheme-split branch first.

---

### Task 1: Switch the inbound middleware to `NewSchemeDispatch`

**Files:**
- Modify: `internal/daemon/server.go` (lines 118-130 — JWKS init through middleware chain construction)
- Add: `tests/e2e/daemon_auth_test.go` (or wherever the canonical e2e harness lives — verify with `ls tests/e2e/`)

**Step 1: Audit existing auth coverage**

Pre-flight grep at planning time confirmed: `tests/e2e/harness/jwks_stub.go` defines `JWKSStub.APIKeyVerifyCount()` (returns int64 atomic counter) but **no current Flow test asserts a specific count** — `APIKeyVerifyCount` only appears in the harness itself, not in test bodies (`grep -rn 'APIKeyVerifyCount' tests/` finds it only in `jwks_stub.go:50,52` and the doc comment at `env.go:18`). That means there is no existing assertion to flip. We are ADDING two new daemon-level tests in this task, not editing existing ones.

Re-run the grep before editing in case new tests landed:

```bash
cd /home/kazw/Work/WorkFort/flow/lead && grep -rn 'APIKeyVerifyCount' tests/ internal/
```

Any new caller that asserts `>= 1` on a Bearer-only flow would be a regression victim of the original bug — convert it to assert exactly 0 on Bearer flows and `>= 1` only on `ApiKey-v1` flows.

**Step 2: Write the failing daemon-level tests**

Add to `tests/e2e/daemon_auth_test.go`:

```go
func TestDaemon_BearerForAPIKeyReturns401(t *testing.T) {
	env := harness.Start(t) // canonical Flow daemon harness — pulls in JWKSStub
	defer env.Close()

	req, _ := http.NewRequest("GET", env.DaemonURL+"/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+env.ServiceAPIKey) // wf-svc_* sent under wrong scheme
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

func TestDaemon_ApiKeyV1RoutesToVerify(t *testing.T) {
	env := harness.Start(t)
	defer env.Close()

	beforeCount := env.JWKS.APIKeyVerifyCount()

	req, _ := http.NewRequest("GET", env.DaemonURL+"/v1/runs", nil)
	req.Header.Set("Authorization", "ApiKey-v1 "+env.ServiceAPIKey)
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
```

(Adapt to the harness's actual entry point — whatever the canonical `harness.Start` / `env.DaemonURL` / `env.ServiceAPIKey` accessors are. The assertion shape — 401 with zero count for Bearer-misuse, 200 with advanced count for `ApiKey-v1` — is load-bearing.)

**Step 3: Update the middleware construction**

In `internal/daemon/server.go`, replace the lines around 122-129:

```go
var validators []auth.Validator
if jwtV != nil {
	validators = append(validators, jwtV)
}
validators = append(validators, apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL))

passportMW := auth.NewFromValidators(validators...)
handler = publicPathSkip(passportMW(mux), mux)
```

with:

```go
apiKeyV := apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL)

// NewSchemeDispatch requires both validators non-nil. If JWKS init
// failed at startup (logged a warning above and left jwtV == nil),
// substitute the fail-closed stub exported by service-auth so the
// API-key path keeps working. Use the upstream helper rather than
// reimplementing it locally — single source of truth.
jwtForDispatch := jwtV
if jwtForDispatch == nil {
	jwtForDispatch = auth.AlwaysFail(fmt.Errorf("jwt validator unavailable (jwks init failed)"))
}

passportMW := auth.NewSchemeDispatch(jwtForDispatch, apiKeyV)
handler = publicPathSkip(passportMW(mux), mux)
```

(Note: `auth.AlwaysFail` is exported by `github.com/Work-Fort/Passport/go/service-auth` — see passport plan Task 3 Step 2. Do NOT define a local `alwaysFailValidator` type; the upstream helper exists specifically to prevent divergence across consumers.)

**Step 4: Run tests**

```
mise run test && mise run e2e -- -run "TestDaemon_BearerForAPIKey|TestDaemon_ApiKeyV1Routes" -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(daemon)!: dispatch inbound auth by Authorization scheme

BREAKING CHANGE: Flow's HTTP daemon now requires JWTs under "Bearer"
and API keys under "ApiKey-v1". The legacy try-each-validator chain
is removed. If JWKS init fails at startup an always-fail JWT stub is
substituted so the API-key path keeps working — the dispatcher
contract requires both validators non-nil.

Closes the local exposure to passport's Cluster 3b validator
fallthrough (a malformed JWT amplified into a verify-api-key call).
Inbound middleware still accepts both schemes (browser-routed JWT
via Scope; ApiKey-v1 from agents/services).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Add Pylon + Hive adapter wire-format regression tests

**Files:**
- Add: `internal/infra/pylon/adapter_test.go` (or extend if it exists)
- Add: `internal/infra/hive/adapter_test.go` (or extend if it exists)

Pylon's `New(pylonURL, apiKey)` and Hive's `New(baseURL, apiKey)` keep the same Go signature — only the parameter name and the on-the-wire scheme change. Flow has no compile-time edit at the call sites; the actual dependency bump happens in Task 5 alongside the other three.

This task is purely defensive: add construction-side tests so Flow's adapters catch any future drift in the upstream wire format.

**Step 1: Verify build against the locally-replaced clients**

```
cd /home/kazw/Work/WorkFort/flow/lead && go build ./...
```

Expected: build OK against the locally-replaced pylonclient + hiveclient.

**Step 2: Add the adapter-side wire-format assertions**

For each adapter, the test pattern is the same: spin up an `httptest.Server` that records the inbound `Authorization` header, build the adapter through Flow's normal construction, invoke any zero-arg method, and assert the recorded header is `ApiKey-v1 <key>`.

```go
func TestPylonAdapter_OutboundIsApiKeyV1(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"services":[]}`))
	}))
	defer srv.Close()

	a := pylonadapter.New(srv.URL, "wf-svc_secret")
	_, _ = a.Services(context.Background())

	if gotAuth != "ApiKey-v1 wf-svc_secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "ApiKey-v1 wf-svc_secret")
	}
}
```

(Hive's adapter test is the same shape — call any zero-arg adapter method and assert the recorded header.)

**Step 3: Run and commit**

```bash
go test ./internal/infra/pylon/... ./internal/infra/hive/...
git add internal/infra/pylon/adapter_test.go internal/infra/hive/adapter_test.go
git commit -m "$(cat <<'EOF'
test(adapters): assert pylon + hive outbound Authorization is ApiKey-v1

Both upstream clients now send API keys under ApiKey-v1 (parameter
renamed token → apiKey, signature unchanged). Flow's adapters wrap
those constructors directly — add construction-side wire-format tests
so any future drift in the upstream is caught at Flow's boundary
rather than discovered at deploy time.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Update the Sharkfin adapter call site (`WithToken` → `WithAPIKey`)

**Files:**
- Modify: `internal/infra/sharkfin/adapter.go:31`

**Step 1: Switch the option (pure mechanical rename)**

Replace:

```go
client: sharkfinclient.NewRESTClient(baseURL, sharkfinclient.WithToken(token)),
```

with:

```go
client: sharkfinclient.NewRESTClient(baseURL, sharkfinclient.WithAPIKey(token)),
```

This is the load-bearing edit. The original wiring was the latent-bug enabler — Flow was passing a `wf-svc_*` API key into the JWT option, and passport's validator fallthrough silently accepted it. With sharkfin's `WithToken` removed, the rename to `WithAPIKey` makes the call type-honest; the wire format also flips to `ApiKey-v1` (handled inside sharkfinclient).

The local variable name `serviceToken` describes its origin (a `--service-token` flag); leaving the local name alone is fine — the type-narrowing happens at the option call.

**Step 2: Test**

```
mise run e2e
```

Expected: PASS.

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
fix(adapter)!: sharkfin client uses WithAPIKey for service-token auth

Latent bug surfaced by passport's scheme dispatch: the service token
(a wf-svc_* API key) was being sent via WithToken (the JWT option),
which previously worked only because passport's middleware fell
through from JWT to API-key validation. Sharkfin removed WithToken
in its scheme-split work; switch to WithAPIKey, which sends the
correct ApiKey-v1 wire format.

Per TPM clarification 2026-04-19: agents and services only ever
send API keys outbound. This adapter is now type-honest about that.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Update the e2e harness JWKSStub apikey hits semantics

**Files:**
- Verify: `tests/e2e/harness/jwks_stub.go` — confirm `APIKeyVerifyCount()` is incremented only when an actual `verify-api-key` request arrives. After scheme-dispatch, this counter MUST stay at 0 when only Bearer (JWT) traffic is sent.
- Adjust: any e2e test that asserted a positive count from a JWT-only flow.

**Step 1: Enumerate every `APIKeyVerifyCount` call site**

```bash
cd /home/kazw/Work/WorkFort/flow/lead
grep -rn 'APIKeyVerifyCount' tests/ internal/
```

Snapshot at planning time:

| File | Line | Kind |
| --- | --- | --- |
| `tests/e2e/harness/jwks_stub.go` | 23, 33, 50, 52 | helper definition / godoc — leave |
| `tests/e2e/harness/env.go` | 18 | godoc on the harness env type — leave |
| (no test bodies assert the counter today) | — | — |

So at planning time there is no test assertion to flip. Re-run the grep before this task — if a new test landed that asserts `>= 1` on a JWT-only flow, convert it to assert exactly 0 on Bearer flows and `>= 1` only on `ApiKey-v1` flows.

**Step 2: Run the full e2e and inspect**

```
mise run e2e -v
```

If any test fails because `APIKeyVerifyCount()` is now 0 where it expected positive, that test was a regression victim of the original bug; convert the assertion to "API-key path is reached only when `ApiKey-v1` is sent." Note: `signJWT` and inbound JWT-acceptance tests stay — they exercise the inbound JWT validator, which is still needed for browser-routed traffic.

**Step 3: Commit any required test updates (no commit if no test changes)**

```bash
git commit -m "$(cat <<'EOF'
test(e2e): tighten APIKeyVerifyCount assertions for scheme dispatch

After passport's scheme-dispatch landing, the verify-api-key endpoint
is hit only when the client sends ApiKey-v1. Update assertions that
previously expected fallthrough hits from misformatted Bearer JWTs.

Inbound JWT acceptance tests (signJWT-driven) stay — those exercise
the still-needed JWT validator path for browser-routed traffic.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Drop the four `replace` directives, bump deps, push

(Same shape as the sharkfin plan's Task 6.) Sequence: only after all four upstream tags exist.

```bash
go get github.com/Work-Fort/Passport/go/service-auth@<tag>
go get github.com/Work-Fort/Hive/client@<tag>
go get github.com/Work-Fort/Pylon/client/go@<tag>
go get github.com/Work-Fort/sharkfin/client/go@<tag>
go mod tidy
mise run lint && mise run test && mise run e2e
```

```bash
git commit -m "$(cat <<'EOF'
chore(deps): bump passport / hive / pylon / sharkfin clients (scheme dispatch)

Drops the local replace directives; pins to released tags that ship
the API-key-only outbound clients and dispatcher middleware. Flow's
sharkfin adapter switched WithToken → WithAPIKey; pylon and hive
adapters were no-op renames (parameter name changed, signature
unchanged).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Verification checklist

- [ ] `mise run lint` clean
- [ ] `mise run test` PASS
- [ ] `mise run e2e` PASS
- [ ] No `sharkfinclient.WithToken` anywhere in Flow
- [ ] No `replace` in `go.mod`
- [ ] Daemon's middleware uses `NewSchemeDispatch`, not `NewFromValidators`
