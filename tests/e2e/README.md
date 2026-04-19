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
