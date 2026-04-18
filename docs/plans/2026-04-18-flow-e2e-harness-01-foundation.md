---
type: plan
step: "1"
title: "Flow E2E harness — Step 1: foundation + adapter switch + first proof"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "1"
dates:
  created: "2026-04-18"
  approved: null
  completed: null
related_plans: []
---

# Flow E2E Harness — Step 1: Foundation + Adapter Switch + First Proof

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stand up Flow's first end-to-end test harness scaffolding and prove it works. After this step, `mise run e2e` spawns a real `flow daemon` subprocess wired to in-process fakes for Passport (JWKS), Pylon, Hive, and Sharkfin, then drives it over raw HTTP and asserts both health endpoints respond. The Sharkfin chat adapter is also switched from a WebSocket dial to a stateless HTTP client (Sharkfin v0.2.0's `RESTClient`) so the harness's fake Sharkfin can stay pure HTTP. Auth coverage, REST coverage, MCP tools, the Sharkfin webhook receiver, and bidirectional Flow→Sharkfin→Flow round-trips are deferred to follow-up plans.

**Why split this narrowly?** Per `planner.md` ("Plans over ~1500 lines should be split into sub-steps"; "Split by capability — what the user gets, not by layer"), the full E2E harness is estimated at 2,400–3,800 LOC. The natural seam is the moment the harness becomes verifiable end-to-end — once the daemon-spawn helper, the four fakes, and the first health test exist, every later capability (auth coverage, REST coverage, MCP, webhooks) layers on without changing the foundation. This plan stops at that seam. Each follow-up plan adds a new capability against the same foundation.

The follow-up plans, drafted as separate planner-spawns once this plan is in flight:

- **Plan A.5 (auth + REST coverage + README).** Auth tests (valid/missing/malformed/expired JWT, API key valid/invalid, public-path skip) and the 18 huma REST operations, plus the README e2e section. The work-item subset of those 18 operations depends on Flow growing step/transition CRUD endpoints — see "Out of scope" below.
- **Plan B (MCP + webhook + bidirectional).** The 12 MCP tools, the Sharkfin webhook receiver, and the bidirectional Flow→fake-Sharkfin→Flow round-trip.

**Why bundle the adapter switch in this plan?** Flow's current `internal/infra/sharkfin/adapter.go` calls `sharkfinclient.Dial`, which opens a WebSocket. Sharkfin v0.2.0 (already tagged at `client/go/v0.2.0`) ships `NewRESTClient(baseURL, ...)` — a stateless HTTP-only client with the same nine-method surface (`Register`, `CreateChannel`, `JoinChannel`, `SendMessage`, `RegisterWebhook`, etc.). Switching the adapter to `RESTClient` first means the harness's fake Sharkfin (here, and in Plan B) can be a plain `httptest.Server` (~150 LOC) instead of a `gorilla/websocket` server (~350 LOC, plus an upgrade handshake, write-pump, ping pong). It also keeps `gorilla/websocket` out of the harness module entirely. The adapter change is ~30 lines of mechanical work and lands in this plan (Task 2) so the WS fake is never written.

**Architecture:**

```
┌────────────────── tests/e2e (nested go module) ──────────────────┐
│                                                                  │
│  ┌──────────────────┐    ┌─────────────────────────────────────┐ │
│  │ harness package  │    │ test packages (rest_test, auth_test)│ │
│  │                  │    │                                     │ │
│  │ ▸ Daemon spawn   │    │  uses raw http.Client wrappers      │ │
│  │ ▸ JWKS stub      │    │  ─ NO sharkfinclient import         │ │
│  │ ▸ Pylon stub     │    │  ─ NO hiveclient import             │ │
│  │ ▸ Fake Hive      │    │                                     │ │
│  │ ▸ Fake Sharkfin  │    │                                     │ │
│  │ ▸ Postgres reset │    │                                     │ │
│  └────────┬─────────┘    └────────────┬────────────────────────┘ │
└───────────┼───────────────────────────┼──────────────────────────┘
            │                           │
            ▼                           ▼
   ┌────────────────────┐      ┌─────────────────────┐
   │ flow daemon (real) │◀─────│ raw net/http client │
   │   build/flow       │ JSON │   in test           │
   └────────┬───────────┘      └─────────────────────┘
            │ outbound HTTP
            ▼
   ┌──────────────────────────────────────────────────┐
   │  fakes (httptest.Server) — plain JSON, no SDKs    │
   │                                                   │
   │  passport://v1/jwks, /v1/verify-api-key           │
   │  pylon://api/services                             │
   │  hive://v1/agents/{id}, /v1/roles/{id}, …         │
   │  sharkfin://api/v1/auth/register, …               │
   └──────────────────────────────────────────────────┘
```

**Hard constraints (non-negotiable):**

- The harness module imports **zero** WorkFort Go client packages: no `sharkfinclient`, no `hiveclient`, no `pylonclient`, no Flow-internal packages. It only imports `net/http`, `net/http/httptest`, `encoding/json`, `database/sql`, `crypto/rsa`, `github.com/lestrrat-go/jwx/v2/...` (for signing test JWTs against the JWKS stub), and `github.com/jackc/pgx/v5` (for Postgres reset). Importing any client SDK would mean tests pass because the SDK and the server agree on a wire format that nobody is verifying.
- The fake Sharkfin and fake Hive **do not reuse the real services' server-side handler code**. They are hand-rolled JSON responders that mimic the wire format. If a real-service handler bug is the only thing keeping a contract test green, the harness must catch it.
- Every commit message uses the multi-line conventional-commits HEREDOC format with body + `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer (release tooling depends on this).

**Tech Stack:** Go 1.26, `net/http`, `net/http/httptest`, `encoding/json`, `database/sql`, `github.com/lestrrat-go/jwx/v2`, `github.com/jackc/pgx/v5/stdlib` (pulled in only for the optional Postgres reset path), `mise run` for all commands.

**Out of scope for this plan:**

- **Auth middleware tests** (valid/missing/malformed/expired JWT, API key valid/invalid, public-path skip). Deferred to Plan A.5. The harness primitives this plan ships (the JWKS stub's `signJWT` closure, the `Daemon.SignJWT` helper, the no-auth `Client` flavour) are designed so Plan A.5 can add tests without changing the harness.
- **REST operation tests** (the 18 huma operations in `internal/daemon/rest_huma.go`). Deferred to Plan A.5. Flow does not yet expose REST endpoints to add steps or transitions to a template (`rest_huma.go:120-217` covers template CRUD only — no `POST /v1/templates/{id}/steps`). Without those endpoints, work items cannot be created via the public REST surface, which means 8 of the 18 operations (everything under `/v1/items/...` plus `transition`/`approve`/`reject`) cannot be exercised end-to-end. Plan A.5 should scope its REST coverage to the 9 operations testable today (templates × 5, instances × 4) and call out the gap; the remaining 8 wait on a separate plan that adds the step/transition CRUD endpoints first. Listing tests as `t.Skip` rots — absent tests are honest.
- **MCP tool coverage** (12 tools — needs JSON-RPC client + session header handling). Deferred to Plan B.
- **Sharkfin webhook receiver coverage** (POST `/v1/webhooks/sharkfin`, command parsing). Deferred to Plan B.
- **Bidirectional Flow→fake-Sharkfin→Flow round-trip** (relies on fake Sharkfin's webhook-sender capability and on the receiver tests). Deferred to Plan B.

---

## Prerequisites

- Flow's daemon already builds via `mise run build:dev` and produces `build/flow`.
- The two health endpoints exist: `/v1/health` (raw handler with conditional 200/218/503) and `/ui/health` (raw handler).
- Sharkfin v0.2.0 is on the remote and exposes `*RESTClient`. Tag: `client/go/v0.2.0`.
- Flow has no `openspec/` directory; spec sections are skipped.
- The repo has no `tests/e2e/` directory yet.

---

## Conventions (apply to every task)

- Run all build/lint/test commands via `mise run <task>` from `flow/lead/`.
  - `mise run build:dev` — produces `build/flow`.
  - `mise run lint` — gofmt + go vet for the main module.
  - `mise run test` — race-enabled unit tests for the main module.
  - `mise run e2e` (added in Task 4) — builds `build/flow`, then `cd tests/e2e && go test ./...`.
- Targeted TDD test runs from inside the nested module are permitted (planner.md exception): `cd tests/e2e && go test -run TestX ./...`.
- Commit after each task with conventional-commits prefix (`feat`, `fix`, `test`, `chore`, `refactor`, `docs`) and the HEREDOC pattern below.

```bash
git add <files>
git commit -m "$(cat <<'EOF'
<type>(<scope>): <description>

<body explaining why, not what>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task Breakdown

### Task 1: Bump Sharkfin client to v0.2.0

**Files:**
- Modify: `go.mod` (line 30 — `github.com/Work-Fort/sharkfin/client/go v0.1.0` → `v0.2.0`)
- Modify: `go.sum` (regenerated)

**Step 1: Update go.mod**

Edit `go.mod` line 30:

```
	github.com/Work-Fort/sharkfin/client/go v0.2.0
```

**Step 2: Resolve module graph**

Run: `go mod tidy`
Expected: no errors, `go.sum` updated.

**Step 3: Verify the existing build still passes**

Run: `mise run build:dev`
Expected: `Built build/flow`. The current adapter still calls `Dial` and that API is preserved in v0.2.0; this is purely an additive bump.

**Step 4: Verify unit tests still pass**

Run: `mise run test`
Expected: all green. No source changes yet, so behaviour is identical.

**Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore(deps): bump sharkfin client to v0.2.0

Adds the new RESTClient surface that the next task switches the
Sharkfin adapter to. Keeping the bump in its own commit makes the
behavioural change in Task 2 reviewable in isolation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Switch Sharkfin adapter from WS Dial to RESTClient

**Files:**
- Modify: `internal/infra/sharkfin/adapter.go`
- Modify: `internal/daemon/server.go:60-91` (the `sharkfininfra.New` call site — signature changes from `(ctx, baseURL, token) (*Adapter, error)` to `(baseURL, token) *Adapter`)
- Create: `internal/infra/sharkfin/adapter_test.go`

There is no existing adapter unit test. This task adds one alongside the rewrite so the behavioural change has a unit-level safety net — `httptest.Server` standing in for Sharkfin, exercising every adapter method end-to-end through the new `RESTClient`. The e2e harness in Tasks 9-11 layers on top; the unit test covers the adapter in isolation.

**Step 1: Inspect the existing `sharkfin/adapter.go`**

Read: `internal/infra/sharkfin/adapter.go`. Note that:
- `New(ctx, baseURL, token)` calls `sharkfinclient.Dial` and returns `(*Adapter, error)`.
- All five method bodies forward to a `*sharkfinclient.Client`.
- The `httpToWS` helper exists solely to convert the base URL for the WS dial.
- `Close()` calls `c.client.Close()`, which on `*Client` shuts the WS goroutine; on `*RESTClient` it's a documented no-op.

**Step 2: Rewrite the adapter**

Replace the file contents:

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package sharkfin provides a Flow ChatProvider backed by the Sharkfin chat service.
package sharkfin

import (
	"context"
	"encoding/json"
	"fmt"

	sharkfinclient "github.com/Work-Fort/sharkfin/client/go"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.ChatProvider using the Sharkfin REST API.
//
// Flow receives chat events through the Sharkfin webhook receiver
// (internal/daemon/webhook_sharkfin.go), so it has no need for the
// WebSocket event stream. A REST-only client avoids the WS goroutine,
// reconnection state, and pending-request machinery.
type Adapter struct {
	client *sharkfinclient.RESTClient
}

// New constructs an Adapter. baseURL is the HTTP base URL returned by
// Pylon (e.g., "http://sharkfin:16000"). token is a Passport JWT or
// API key. No network I/O happens at construction time.
func New(baseURL, token string) *Adapter {
	return &Adapter{
		client: sharkfinclient.NewRESTClient(baseURL, sharkfinclient.WithToken(token)),
	}
}

// PostMessage sends content to channel and returns the message ID.
// metadata is attached as a JSON sidecar on the message. If metadata
// is nil or empty, no metadata is attached.
func (a *Adapter) PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error) {
	var opts *sharkfinclient.SendOpts
	if len(metadata) > 0 {
		s := string(metadata)
		opts = &sharkfinclient.SendOpts{Metadata: &s}
	}
	id, err := a.client.SendMessage(ctx, channel, content, opts)
	if err != nil {
		return 0, fmt.Errorf("sharkfin post message to %s: %w", channel, err)
	}
	return id, nil
}

// CreateChannel creates a channel in Sharkfin.
func (a *Adapter) CreateChannel(ctx context.Context, name string, public bool) error {
	if err := a.client.CreateChannel(ctx, name, public); err != nil {
		return fmt.Errorf("sharkfin create channel %s: %w", name, err)
	}
	return nil
}

// JoinChannel joins the named channel.
func (a *Adapter) JoinChannel(ctx context.Context, channel string) error {
	if err := a.client.JoinChannel(ctx, channel); err != nil {
		return fmt.Errorf("sharkfin join channel %s: %w", channel, err)
	}
	return nil
}

// Register registers Flow's identity as a service bot with Sharkfin.
func (a *Adapter) Register(ctx context.Context) error {
	if err := a.client.Register(ctx); err != nil {
		return fmt.Errorf("sharkfin register: %w", err)
	}
	return nil
}

// RegisterWebhook registers a webhook callback URL. Returns the webhook ID.
func (a *Adapter) RegisterWebhook(ctx context.Context, callbackURL string) (string, error) {
	id, err := a.client.RegisterWebhook(ctx, callbackURL)
	if err != nil {
		return "", fmt.Errorf("sharkfin register webhook: %w", err)
	}
	return id, nil
}

// ListWebhooks returns all registered webhooks for this identity.
func (a *Adapter) ListWebhooks(ctx context.Context) ([]sharkfinclient.Webhook, error) {
	whs, err := a.client.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("sharkfin list webhooks: %w", err)
	}
	return whs, nil
}

// Close releases the underlying HTTP client. No-op for the REST
// client today; provided so callers can write symmetric setup/teardown.
func (a *Adapter) Close() error {
	return a.client.Close()
}

// Ensure Adapter satisfies domain.ChatProvider at compile time.
var _ domain.ChatProvider = (*Adapter)(nil)
```

**Step 3: Update the call site in `server.go`**

Modify `internal/daemon/server.go:73-87`. Replace this block:

```go
		if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Sharkfin); err == nil {
			if a, err := sharkfininfra.New(startupCtx, sharkfinSvc.BaseURL, cfg.ServiceToken); err == nil {
				chatAdapter = a
				if err := a.Register(startupCtx); err != nil {
					log.Warn("sharkfin register failed", "err", err)
				}
				if cfg.WebhookBaseURL != "" {
					callbackURL := strings.TrimRight(cfg.WebhookBaseURL, "/") + "/v1/webhooks/sharkfin"
					if _, err := a.RegisterWebhook(startupCtx, callbackURL); err != nil {
						log.Warn("sharkfin register webhook failed", "err", err)
					}
				}
			} else {
				log.Warn("sharkfin dial failed, chat disabled", "err", err)
			}
		} else {
			log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
		}
```

with:

```go
		if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Sharkfin); err == nil {
			a := sharkfininfra.New(sharkfinSvc.BaseURL, cfg.ServiceToken)
			chatAdapter = a
			if err := a.Register(startupCtx); err != nil {
				log.Warn("sharkfin register failed", "err", err)
			}
			if cfg.WebhookBaseURL != "" {
				callbackURL := strings.TrimRight(cfg.WebhookBaseURL, "/") + "/v1/webhooks/sharkfin"
				if _, err := a.RegisterWebhook(startupCtx, callbackURL); err != nil {
					log.Warn("sharkfin register webhook failed", "err", err)
				}
			}
		} else {
			log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
		}
```

The error-on-construction branch disappears because `NewRESTClient` cannot fail.

**Step 4: Write the adapter unit test**

Create `internal/infra/sharkfin/adapter_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package sharkfin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestAdapter returns an Adapter pointing at the given httptest.Server.
func newTestAdapter(t *testing.T, srv *httptest.Server) *Adapter {
	t.Helper()
	return New(srv.URL, "test-token")
}

// readJSON decodes the request body into out, or fails the test.
func readJSON(t *testing.T, r *http.Request, out any) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("unmarshal body %q: %v", string(body), err)
	}
}

func TestAdapter_Register_PostsAuthRegister(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/register" {
			hits.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 register call, got %d", hits.Load())
	}
}

func TestAdapter_CreateChannel_SendsNameAndPublic(t *testing.T) {
	var got struct {
		Name   string `json:"name"`
		Public bool   `json:"public"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/channels" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		readJSON(t, r, &got)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": got.Name, "public": got.Public})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.CreateChannel(context.Background(), "general", true); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if got.Name != "general" || got.Public != true {
		t.Fatalf("got name=%q public=%v", got.Name, got.Public)
	}
}

func TestAdapter_JoinChannel_PostsToJoin(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.JoinChannel(context.Background(), "general"); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if want := "/api/v1/channels/general/join"; path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
}

func TestAdapter_PostMessage_NoMetadata(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages") {
			http.NotFound(w, r)
			return
		}
		readJSON(t, r, &body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"id": 42})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	id, err := a.PostMessage(context.Background(), "general", "hello", nil)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if id != 42 {
		t.Fatalf("id=%d want 42", id)
	}
	if body["body"] != "hello" {
		t.Fatalf("body[body]=%v", body["body"])
	}
	if _, present := body["metadata"]; present {
		t.Fatalf("metadata should be absent, got %v", body["metadata"])
	}
}

func TestAdapter_PostMessage_WithMetadata(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readJSON(t, r, &body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"id": 7})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	meta := json.RawMessage(`{"event_type":"flow_command","event_payload":{"action":"approve"}}`)
	id, err := a.PostMessage(context.Background(), "ops", "approve please", meta)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if id != 7 {
		t.Fatalf("id=%d want 7", id)
	}
	// Server-side, the RESTClient parses metadata into a map.
	mm, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata is not a map: %T %v", body["metadata"], body["metadata"])
	}
	if mm["event_type"] != "flow_command" {
		t.Fatalf("metadata.event_type=%v", mm["event_type"])
	}
}

func TestAdapter_RegisterWebhook_ReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/webhooks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "wh-abc"})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	id, err := a.RegisterWebhook(context.Background(), "http://flow/v1/webhooks/sharkfin")
	if err != nil {
		t.Fatalf("RegisterWebhook: %v", err)
	}
	if id != "wh-abc" {
		t.Fatalf("id=%q want wh-abc", id)
	}
}

func TestAdapter_ListWebhooks_ReturnsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/webhooks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "wh-1", "url": "http://a", "active": true},
			{"id": "wh-2", "url": "http://b", "active": false},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	whs, err := a.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(whs) != 2 {
		t.Fatalf("len=%d want 2", len(whs))
	}
}

func TestAdapter_Close_NoOp(t *testing.T) {
	a := New("http://unused", "")
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
```

**Step 5: Run the new tests fail-then-pass**

After Step 2's adapter rewrite the new tests should compile and pass on first run (the `RESTClient` already exists from Task 1's bump). If you wrote the test file before Step 2, the test compiles but fails because `New` still returns `(*Adapter, error)`. Either order is fine; the spec is "test passes only after the rewrite is in place".

Run: `mise run test -- -run TestAdapter_ ./internal/infra/sharkfin/...`
Expected: PASS for all 8 tests.

**Step 6: Run unit tests for the rest of the module**

Run: `mise run test`
Expected: all green. The `internal/daemon` test packages do not exercise the adapter directly; they construct fakes against `domain.ChatProvider`.

**Step 7: Run lint**

Run: `mise run lint`
Expected: no diagnostics.

**Step 8: Commit**

```bash
git add internal/infra/sharkfin/adapter.go internal/infra/sharkfin/adapter_test.go internal/daemon/server.go
git commit -m "$(cat <<'EOF'
refactor(sharkfin): switch adapter from WS Dial to RESTClient

Flow receives Sharkfin events via the webhook receiver, never via
the WS event stream. Using NewRESTClient drops the WS dial,
read-pump goroutine, and reconnection state at zero behavioural
cost. The adapter surface (PostMessage, CreateChannel,
JoinChannel, Register, RegisterWebhook, ListWebhooks, Close) is
unchanged.

Adds the first unit-test coverage for this adapter — eight
httptest-server-backed tests verifying every method against the
real RESTClient transport.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Create the `tests/e2e/` nested Go module

**Files:**
- Create: `tests/e2e/go.mod`
- Create: `tests/e2e/go.sum` (generated)
- Create: `tests/e2e/.gitignore` (one line: `/build/`)

The nested module isolates JWT signing libs, the Postgres driver pull-through, and the harness fakes from Flow's main `go.mod`. Sharkfin and Combine both follow this pattern; Hive does not, and Hive's harness imports its own client package as a result — exactly the contamination this plan exists to avoid.

**Step 1: Create the nested module**

Run from `flow/lead/`:

```bash
mkdir -p tests/e2e
cd tests/e2e
go mod init github.com/Work-Fort/Flow/tests/e2e
```

Confirm `tests/e2e/go.mod` reads:

```
module github.com/Work-Fort/Flow/tests/e2e

go 1.26
```

**Step 2: Add the only allowed dependencies**

Edit `tests/e2e/go.mod` to add the explicit `require` block (versions match what the main module already pins, so `go mod tidy` resolves cleanly without dragging in surprise transitive bumps):

```
require (
	github.com/jackc/pgx/v5 v5.8.0
	github.com/lestrrat-go/jwx/v2 v2.1.6
)
```

Run: `cd tests/e2e && go mod tidy`
Expected: `go.sum` populated; no `Work-Fort/Flow`, `Work-Fort/sharkfin`, `Work-Fort/Hive`, `Work-Fort/Pylon`, or `Work-Fort/Passport` modules anywhere in `go.sum`.

**Step 3: Verify the constraint**

Run: `cd tests/e2e && grep -E "Work-Fort/(Flow|sharkfin|Hive|Pylon|Passport)" go.sum`
Expected: no matches (exit code 1). If anything matches, stop and audit imports — a contamination check is the whole point of the nested module.

**Step 4: Add `.gitignore`**

Write `tests/e2e/.gitignore`:

```
/build/
```

**Step 5: Commit**

```bash
git add tests/e2e/go.mod tests/e2e/go.sum tests/e2e/.gitignore
git commit -m "$(cat <<'EOF'
chore(e2e): scaffold nested tests/e2e go module

Isolates JWT signing libs and the Postgres driver from Flow's main
go.mod. Pinned to the same versions Flow already uses to avoid
surprise transitive bumps. Imports of any WorkFort client SDK are
forbidden in this module.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Add the `mise run e2e` task

**Files:**
- Create: `.mise/tasks/e2e`

**Step 1: Write the task script**

Write `.mise/tasks/e2e`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run end-to-end tests against a real flow daemon (rebuilds via build:dev)"
#MISE depends=["build:dev"]
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
export FLOW_BINARY="$REPO_ROOT/build/flow"

if [[ ! -x "$FLOW_BINARY" ]]; then
  echo "FLOW_BINARY not found at $FLOW_BINARY — run 'mise run build:dev' first" >&2
  exit 1
fi

cd "$REPO_ROOT/tests/e2e"
exec go test -race -count=1 ./...
```

`chmod +x .mise/tasks/e2e`.

**Step 2: Verify the task is discovered**

Run: `mise tasks ls | grep e2e`
Expected: line beginning with `e2e ...Run end-to-end tests against a real flow daemon (rebuilds via build:dev)`.

**Step 3: Run it (no test files yet, so the package set is empty — should succeed with `no test files`)**

Run: `mise run e2e`
Expected: `build:dev` runs first; `go test ./...` reports `?  github.com/Work-Fort/Flow/tests/e2e ...    [no test files]`. Exit 0.

**Step 4: Decide on CI integration**

Do **not** add `e2e` to `.mise/tasks/ci`. Rationale: `ci` is the fast inner-loop gate (lint + unit tests, ~seconds). E2E spawns a subprocess per test and is an order of magnitude slower; it belongs to a separate CI job that runs on the same triggers but does not block fast feedback. The README addition in Task 14 documents this.

**Step 5: Commit**

```bash
git add .mise/tasks/e2e
git commit -m "$(cat <<'EOF'
chore(mise): add e2e task that builds flow then runs tests/e2e

Depends on build:dev so the harness always tests the freshly-built
binary. Kept out of the ci task — e2e is a separate gate so the
fast inner loop (lint + unit) stays under a few seconds.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: JWKS stub (copied from Sharkfin, audited)

**Files:**
- Create: `tests/e2e/harness/jwks_stub.go`

The Passport JWT validator in Flow (`internal/daemon/server.go:120-130`) calls `jwt.New(ctx, opts.JWKSURL, opts.JWKSRefreshInterval)` from the Passport SDK. The validator fetches a JWKS document from `<passport-url>/v1/jwks` and verifies token signatures against it. The same SDK is used in Sharkfin and Hive, so we copy Sharkfin's stub almost verbatim. The only divergence: Flow's audience claim is `"flow"`, not `"sharkfin"`.

**Step 1: Write the stub file**

Create `tests/e2e/harness/jwks_stub.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// StartJWKSStub starts an in-process Passport stub serving:
//   - GET  /v1/jwks            — public key in JWKS format
//   - POST /v1/verify-api-key  — accepts any non-empty key (except the literal
//     "INVALID"), returns a canned identity. Each call increments an internal
//     counter exposed via JWKSStub.APIKeyVerifyCount().
//
// The stub is hand-rolled — it does NOT import or reuse Passport's
// real handler code. If Passport's wire format drifts, this stub
// must drift in lockstep so the harness keeps catching mismatches.
//
// The returned struct exposes:
//   - Addr        — host:port the stub listens on.
//   - Stop()      — shuts the stub down (idempotent).
//   - SignJWT(...) — produces a 1-hour token signed by the JWKS key, audience "flow".
//   - APIKeyVerifyCount() — total /v1/verify-api-key requests since start.
//     Used by future API-key tests to prove the apikey path was actually
//     exercised (the JWT validator runs first in the chain; without this
//     counter, an apikey-validator regression could be masked by the JWT
//     validator returning the same 401).
type JWKSStub struct {
	Addr    string
	srv     *http.Server
	signJWT func(id, username, displayName, userType string) string
	apiKeyHits atomic.Int64
}

// SignJWT produces a token signed by the JWKS stub's key, audience "flow".
func (s *JWKSStub) SignJWT(id, username, displayName, userType string) string {
	return s.signJWT(id, username, displayName, userType)
}

// APIKeyVerifyCount returns the number of /v1/verify-api-key requests
// served since the stub started.
func (s *JWKSStub) APIKeyVerifyCount() int64 {
	return s.apiKeyHits.Load()
}

// Stop shuts the stub's HTTP server down.
func (s *JWKSStub) Stop() {
	s.srv.Close()
}

// StartJWKSStub starts the Passport stub and returns it ready for use.
func StartJWKSStub() *JWKSStub {
	rawKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: generate RSA key: %v", err))
	}

	privJWK, err := jwk.FromRaw(rawKey)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: create JWK from private key: %v", err))
	}
	_ = privJWK.Set(jwk.KeyIDKey, "test-key-1")
	_ = privJWK.Set(jwk.AlgorithmKey, jwa.RS256)

	privSet := jwk.NewSet()
	_ = privSet.AddKey(privJWK)

	pubSet, err := jwk.PublicSetOf(privSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: derive public key set: %v", err))
	}

	jwksBytes, err := json.Marshal(pubSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: marshal JWKS: %v", err))
	}

	apiKeyIdentity := map[string]any{
		"valid": true,
		"key": map[string]any{
			"userId": "00000000-0000-0000-0000-000000000099",
			"metadata": map[string]any{
				"username":     "flow-e2e-apikey",
				"name":         "Flow E2E API Key",
				"display_name": "Flow E2E API Key",
				"type":         "service",
			},
		},
	}

	stub := &JWKSStub{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksBytes)
	})
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
		// Distinguish a "bad" key. The literal "INVALID" is rejected;
		// anything else (including JWT-shaped strings the JWT validator
		// already rejected) returns the canned identity.
		if req.Key == "INVALID" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"valid": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiKeyIdentity)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: listen: %v", err))
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	stub.Addr = ln.Addr().String()
	stub.srv = srv
	stub.signJWT = func(id, username, displayName, userType string) string {
		now := time.Now()
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("passport-stub").
			Audience([]string{"flow"}).
			IssuedAt(now).
			Expiration(now.Add(1 * time.Hour)).
			Claim("username", username).
			Claim("name", displayName).
			Claim("display_name", displayName).
			Claim("type", userType).
			Build()
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: build JWT: %v", err))
		}
		signedBytes, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: sign JWT: %v", err))
		}
		return string(signedBytes)
	}

	return stub
}
```

**Step 2: Run-fail check (no test yet, but compile must pass)**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 3: Commit**

```bash
git add tests/e2e/harness/jwks_stub.go
git commit -m "$(cat <<'EOF'
test(e2e): add JWKS stub for Passport JWT validation

Hand-rolled HTTP stub that serves /v1/jwks and /v1/verify-api-key
in the wire format Passport's SDK expects. Audience is "flow".
The SignJWT method shares the private key with the JWKS endpoint so
signed tokens validate against the public set the daemon fetches at
startup. Tracks /v1/verify-api-key request count so future tests can
assert the apikey path was actually exercised.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Pylon stub

**Files:**
- Create: `tests/e2e/harness/pylon_stub.go`

The Pylon client (`pylon/lead/client/go/client.go`) calls `GET /api/services` and decodes `{"services": [...]}`. Each service includes `name`, `base_url`, plus a handful of presentation fields. Flow's startup looks up two by name: the Hive name (configurable) and the Sharkfin name (configurable). The stub returns whatever services the harness was constructed with.

**Step 1: Write the stub**

Create `tests/e2e/harness/pylon_stub.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
)

// PylonService is the JSON shape the Pylon client expects per service.
// Field names match pylon/lead/client/go/types.go on the wire.
type PylonService struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Route     string   `json:"route"`
	BaseURL   string   `json:"base_url"`
	UI        bool     `json:"ui"`
	Connected bool     `json:"connected"`
	WSPaths   []string `json:"ws_paths"`
}

// StartPylonStub serves GET /api/services with the provided service list.
// The list is captured at start time; tests that need to vary the
// registry must restart the stub.
func StartPylonStub(services []PylonService) (addr string, stop func()) {
	body, _ := json.Marshal(map[string]any{"services": services})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("pylon_stub: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return ln.Addr().String(), func() { srv.Close() }
}
```

**Step 2: Compile check**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 3: Commit**

```bash
git add tests/e2e/harness/pylon_stub.go
git commit -m "$(cat <<'EOF'
test(e2e): add Pylon services stub

Serves GET /api/services in the JSON format Flow's Pylon client
decodes. Tests pass in the service list at construction time.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Fake Hive (HTTP, stateless)

**Files:**
- Create: `tests/e2e/harness/fake_hive.go`

Flow's Hive adapter calls three endpoints: `GET /v1/agents/{id}` (with `roles` array), `GET /v1/roles/{id}`, `GET /v1/agents?team_id=...`. Step 1's REST coverage doesn't actually exercise Hive (the 18 huma operations don't traverse `IdentityProvider`), but the daemon's startup probes Pylon → Hive and the harness must wire something serving the Hive base URL or startup logs noise. The stub also unblocks Step 2.

The fake is **stateless and seeded** — each test constructs a fake with a small map of agents/roles and the fake reads from that map. It does not embed Hive's real handler logic.

**Step 1: Write the fake**

Create `tests/e2e/harness/fake_hive.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HiveAgent is the canonical JSON shape Flow's Hive adapter decodes.
// Field names mirror hive/lead/client/types.go (capitalised, matching
// the real Hive server's quirky JSON tags).
type HiveAgent struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	TeamID    string    `json:"TeamID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

type HiveRoleAssignment struct {
	AgentID  string `json:"AgentID"`
	RoleID   string `json:"RoleID"`
	Priority int    `json:"Priority"`
}

type HiveAgentWithRoles struct {
	HiveAgent
	Roles []HiveRoleAssignment `json:"roles"`
}

type HiveRole struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	ParentID  string    `json:"ParentID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// FakeHive holds the seeded agents and roles. Tests mutate the maps
// before starting the server. The fake serves wire-format JSON only;
// it does not run any business logic.
type FakeHive struct {
	mu     sync.RWMutex
	agents map[string]HiveAgentWithRoles // by agent ID
	roles  map[string]HiveRole           // by role ID
}

// NewFakeHive returns an empty FakeHive. Use AddAgent / AddRole to seed.
func NewFakeHive() *FakeHive {
	return &FakeHive{
		agents: make(map[string]HiveAgentWithRoles),
		roles:  make(map[string]HiveRole),
	}
}

func (h *FakeHive) AddAgent(a HiveAgentWithRoles) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[a.ID] = a
}

func (h *FakeHive) AddRole(r HiveRole) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roles[r.ID] = r
}

// Start begins serving on a random port. Returns the base URL and a
// stop function.
func (h *FakeHive) Start() (baseURL string, stop func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.mu.RLock()
		a, ok := h.agents[id]
		h.mu.RUnlock()
		if !ok {
			writeHiveError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, a)
	})

	mux.HandleFunc("GET /v1/roles/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/roles/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.mu.RLock()
		role, ok := h.roles[id]
		h.mu.RUnlock()
		if !ok {
			writeHiveError(w, http.StatusNotFound, "role not found")
			return
		}
		writeJSON(w, http.StatusOK, role)
	})

	mux.HandleFunc("GET /v1/agents", func(w http.ResponseWriter, r *http.Request) {
		teamID := r.URL.Query().Get("team_id")
		h.mu.RLock()
		out := make([]HiveAgent, 0, len(h.agents))
		for _, a := range h.agents {
			if teamID == "" || a.TeamID == teamID {
				out = append(out, a.HiveAgent)
			}
		}
		h.mu.RUnlock()
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "checks": []any{}})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("fake_hive: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return "http://" + ln.Addr().String(), func() { srv.Close() }
}

func writeHiveError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSON is shared with fake_sharkfin.go.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
```

**Step 2: Compile check**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 3: Commit**

```bash
git add tests/e2e/harness/fake_hive.go
git commit -m "$(cat <<'EOF'
test(e2e): add stateless fake Hive for the harness

Serves GET /v1/agents/{id}, /v1/roles/{id}, /v1/agents in the wire
format Flow's Hive adapter decodes. Seeded by tests; no business
logic. Hand-rolled — not a thin wrapper over Hive's real handlers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Fake Sharkfin REST (no WS)

**Files:**
- Create: `tests/e2e/harness/fake_sharkfin.go`

With Task 2's adapter switch in place, the daemon only ever speaks HTTP to Sharkfin. The fake covers the four endpoints Flow exercises at startup and runtime: `POST /api/v1/auth/register`, `POST /api/v1/webhooks` (returns webhook ID), `POST /api/v1/channels`, `POST /api/v1/channels/{name}/messages`. (The webhook-sender capability — fake Sharkfin POSTing to Flow's webhook receiver — is added in Step 2, where it's actually exercised by tests.)

**Step 1: Write the fake**

Create `tests/e2e/harness/fake_sharkfin.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// SharkfinMessage records a message Flow posted to a channel via the
// adapter. Tests assert against Messages() to verify outbound chat
// behaviour.
type SharkfinMessage struct {
	ID       int64
	Channel  string
	Body     string
	Metadata map[string]any
}

// SharkfinChannelCreate records a CreateChannel call.
type SharkfinChannelCreate struct {
	Name   string
	Public bool
}

// FakeSharkfin is a hand-rolled HTTP fake of the Sharkfin REST surface
// Flow uses. It intentionally does NOT import sharkfinclient or reuse
// Sharkfin's real handlers — drift between this fake and Sharkfin's
// wire format must surface in tests.
type FakeSharkfin struct {
	mu              sync.Mutex
	registered      bool
	channels        []SharkfinChannelCreate
	messages        []SharkfinMessage
	webhooks        []string
	nextMessageID   int64
	nextWebhookSeq  atomic.Int64
}

func NewFakeSharkfin() *FakeSharkfin { return &FakeSharkfin{} }

// Registered reports whether Flow's startup register call was made.
func (s *FakeSharkfin) Registered() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registered
}

// Channels returns CreateChannel calls in order.
func (s *FakeSharkfin) Channels() []SharkfinChannelCreate {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SharkfinChannelCreate, len(s.channels))
	copy(out, s.channels)
	return out
}

// Messages returns SendMessage calls in order.
func (s *FakeSharkfin) Messages() []SharkfinMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SharkfinMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

// Webhooks returns the URLs Flow registered.
func (s *FakeSharkfin) Webhooks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.webhooks))
	copy(out, s.webhooks)
	return out
}

// Start begins serving on a random port. Returns the base URL and a stop fn.
func (s *FakeSharkfin) Start() (baseURL string, stop func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.registered = true
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/channels", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string `json:"name"`
			Public bool   `json:"public"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.channels = append(s.channels, SharkfinChannelCreate{Name: req.Name, Public: req.Public})
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"name": req.Name, "public": req.Public})
	})

	mux.HandleFunc("POST /api/v1/channels/", func(w http.ResponseWriter, r *http.Request) {
		// Path is /api/v1/channels/{name}/{action}.
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/channels/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		channel := parts[0]
		action := parts[1]
		switch action {
		case "join":
			w.WriteHeader(http.StatusNoContent)
		case "messages":
			var req struct {
				Body     string         `json:"body"`
				Metadata map[string]any `json:"metadata,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.nextMessageID++
			id := s.nextMessageID
			s.messages = append(s.messages, SharkfinMessage{
				ID: id, Channel: channel, Body: req.Body, Metadata: req.Metadata,
			})
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"id": id})
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("POST /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.webhooks = append(s.webhooks, req.URL)
		s.mu.Unlock()
		seq := s.nextWebhookSeq.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "wh-" + itoaSeq(seq)})
	})

	mux.HandleFunc("GET /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		urls := make([]string, len(s.webhooks))
		copy(urls, s.webhooks)
		s.mu.Unlock()
		out := make([]map[string]any, len(urls))
		for i, u := range urls {
			out[i] = map[string]any{"id": "wh-" + itoaSeq(int64(i+1)), "url": u, "active": true}
		}
		writeJSON(w, http.StatusOK, out)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("fake_sharkfin: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return "http://" + ln.Addr().String(), func() { srv.Close() }
}

func itoaSeq(n int64) string {
	// avoid strconv import here just to keep dependencies minimal
	return formatInt(n)
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
```

**Step 2: Compile check**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 3: Commit**

```bash
git add tests/e2e/harness/fake_sharkfin.go
git commit -m "$(cat <<'EOF'
test(e2e): add fake Sharkfin REST server for the harness

Serves the four REST endpoints Flow's adapter touches at startup
and runtime: auth/register, channels, channels/{name}/{join,messages},
webhooks. Records calls so tests can assert Flow's outbound
behaviour. Hand-rolled, no sharkfinclient or sharkfin-server import.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Daemon spawn helper + Postgres reset

**Files:**
- Create: `tests/e2e/harness/daemon.go`

This is the heart of the harness. Mirror Sharkfin's `harness.go` `StartDaemon`/`StopDaemon`/cleanup pattern, adapted for Flow's CLI flags:

| Sharkfin flag    | Flow flag (verified against `cmd/daemon/daemon.go`) |
|------------------|--------------------------------------------------------|
| `--daemon`       | `--bind` + `--port` (Flow takes them split)            |
| `--passport-url` | `--passport-url`                                       |
| `--db`           | `--db`                                                 |
| `--webhook-url`  | `--webhook-base-url`                                   |
| (n/a)            | `--pylon-url`, `--service-token`                       |

Pylon service names default to `"hive"` and `"sharkfin"`; Flow reads them from viper keys `pylon.services.hive` / `pylon.services.sharkfin`. The harness sets neither; defaults are fine.

Flow's main module pulls in `modernc.org/sqlite` and `pgx`. The harness does NOT need to open Flow's database — Flow's own daemon does. The Postgres reset path is a side-channel: when `FLOW_DB=postgres://...` is set, the harness drops + recreates the `public` schema before each daemon spawn so goose migrations re-run cleanly.

**Step 1: Write the daemon helper**

Create `tests/e2e/harness/daemon.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// daemonCfg captures the per-spawn configuration. Tests build it via
// DaemonOption helpers.
type daemonCfg struct {
	pylonAddr      string // host:port of the Pylon stub (required)
	passportAddr   string // host:port of the JWKS stub (required)
	webhookBaseURL string // optional — passed to --webhook-base-url
	dbDSN          string // explicit DB DSN override
}

type DaemonOption func(*daemonCfg)

func WithWebhookBaseURL(u string) DaemonOption {
	return func(c *daemonCfg) { c.webhookBaseURL = u }
}

func WithDB(dsn string) DaemonOption {
	return func(c *daemonCfg) { c.dbDSN = dsn }
}

// Daemon represents a spawned flow daemon subprocess.
type Daemon struct {
	cmd     *exec.Cmd
	addr    string // host:port the daemon listens on
	xdgDir  string // tempdir backing XDG_STATE_HOME / XDG_CONFIG_HOME
	stderr  *bytes.Buffer
	signJWT func(id, username, displayName, userType string) string
	stops   []func()
}

// StartDaemon spawns a flow daemon subprocess wired to in-process fakes.
// pylonAddr and passportAddr are host:port pairs returned by the stubs.
// signJWT is the closure returned by StartJWKSStub — re-used so tests can
// mint JWTs that validate against the JWKS the daemon fetched at startup.
func StartDaemon(
	t testing.TB,
	binary, pylonAddr, passportAddr string,
	signJWT func(id, username, displayName, userType string) string,
	opts ...DaemonOption,
) (*Daemon, error) {
	t.Helper()

	cfg := &daemonCfg{
		pylonAddr:    pylonAddr,
		passportAddr: passportAddr,
	}
	for _, o := range opts {
		o(cfg)
	}

	xdgDir, err := os.MkdirTemp("", "flow-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	addr, err := freePort()
	if err != nil {
		os.RemoveAll(xdgDir)
		return nil, err
	}
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	args := []string{
		"daemon",
		"--bind", host,
		"--port", strconv.Itoa(port),
		"--passport-url", "http://" + passportAddr,
		"--pylon-url", "http://" + pylonAddr,
		"--service-token", "harness-service-token",
	}
	if cfg.webhookBaseURL != "" {
		args = append(args, "--webhook-base-url", cfg.webhookBaseURL)
	}

	dsn := cfg.dbDSN
	if dsn == "" {
		dsn = os.Getenv("FLOW_DB")
	}
	if dsn != "" {
		args = append(args, "--db", dsn)
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			if err := resetPostgres(dsn); err != nil {
				os.RemoveAll(xdgDir)
				return nil, fmt.Errorf("reset postgres: %w", err)
			}
		}
	}

	var stderrBuf bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+xdgDir+"/config",
		"XDG_STATE_HOME="+xdgDir+"/state",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Start(); err != nil {
		os.RemoveAll(xdgDir)
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	d := &Daemon{
		cmd: cmd, addr: addr, xdgDir: xdgDir,
		stderr: &stderrBuf, signJWT: signJWT,
	}

	if err := waitReady(addr, 5*time.Second); err != nil {
		d.kill()
		return nil, err
	}
	return d, nil
}

func (d *Daemon) Addr() string   { return d.addr }
func (d *Daemon) BaseURL() string { return "http://" + d.addr }

// SignJWT mints a 1-hour token signed by the JWKS stub's key.
func (d *Daemon) SignJWT(id, username, displayName, userType string) string {
	return d.signJWT(id, username, displayName, userType)
}

// Stop sends SIGTERM, waits up to 5s, then SIGKILLs. Cleans tempdir.
// Fails the test if the daemon emitted a DATA RACE marker on stderr.
// On test failure, dumps the captured stderr buffer to t.Logf so a
// daemon panic, fatal log line, or context-cancel chain explains the
// failure even after stderr scrolled off the live tty.
func (d *Daemon) Stop(t testing.TB) {
	t.Helper()
	if d.cmd.Process != nil {
		d.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { d.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("daemon did not exit after SIGTERM, killing")
			d.cmd.Process.Kill()
			<-done
		}
	}
	if t.Failed() && d.stderr.Len() > 0 {
		t.Logf("daemon stderr:\n%s", d.stderr.String())
	}
	os.RemoveAll(d.xdgDir)
	if strings.Contains(d.stderr.String(), "DATA RACE") {
		t.Fatal("data race detected in daemon (see stderr above)")
	}
}

func (d *Daemon) kill() {
	if d.cmd.Process != nil {
		d.cmd.Process.Kill()
		d.cmd.Wait()
	}
	os.RemoveAll(d.xdgDir)
}

// freePort returns 127.0.0.1:N for a currently-free N.
func freePort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr, nil
}

// waitReady polls /v1/health until it returns 200, 218 or 503 (any health
// reply means the listener is up), or until deadline.
func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := "http://" + addr + "/v1/health"
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready on %s within %s", addr, timeout)
}

// resetPostgres drops and recreates the public schema. Goose migrations
// re-run on next daemon startup. Mirrors sharkfin/lead/tests/e2e harness.
func resetPostgres(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP SCHEMA public CASCADE"); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	if _, err := db.Exec("CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}
```

**Step 2: Compile check**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 3: Commit**

```bash
git add tests/e2e/harness/daemon.go
git commit -m "$(cat <<'EOF'
test(e2e): add daemon spawn helper with Postgres reset

Spawns build/flow as a subprocess wired to the JWKS, Pylon, Hive
and Sharkfin stubs. Polls /v1/health to detect readiness. Forwards
the signJWT closure so tests can mint tokens that validate against
the daemon's fetched JWKS. Postgres reset is env-gated on FLOW_DB.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Test environment helper + raw HTTP client wrappers

**Files:**
- Create: `tests/e2e/harness/env.go`
- Create: `tests/e2e/harness/client.go`

`Env` is the per-test convenience: it stands up all four stubs, spawns the daemon, and returns a struct with `Daemon`, `Hive`, `Sharkfin` plus a `Cleanup(t)` that tears everything down in reverse order. `Client` is a raw `http.Client` wrapper with `GetJSON` / `PostJSON` / `PatchJSON` / `DeleteJSON` and a Bearer-token field — no SDK imports.

**Step 1: Write env.go**

Create `tests/e2e/harness/env.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"os"
	"testing"
)

// Env is the all-in-one harness construction. Each test's setup looks like:
//
//	env := harness.NewEnv(t)
//	defer env.Cleanup(t)
//	tok := env.Daemon.SignJWT(...)
//	c := harness.NewClient(env.Daemon.BaseURL(), tok)
//	... assertions ...
//
// Env exposes the underlying JWKS stub so tests can read its
// APIKeyVerifyCount() to assert the apikey path was actually traversed.
type Env struct {
	Daemon       *Daemon
	JWKS         *JWKSStub
	Hive         *FakeHive
	Sharkfin     *FakeSharkfin
	stopPylon    func()
	stopHive     func()
	stopSharkfin func()
}

// NewEnv stands up the JWKS stub, Pylon stub, fake Hive, fake Sharkfin,
// then spawns the daemon pointed at all of them. Calls t.Fatal on failure.
//
// The flow binary is read from FLOW_BINARY (set by `mise run e2e`). When
// FLOW_BINARY is unset, falls back to "../../build/flow" so tests can be
// run directly from inside tests/e2e during TDD.
//
// Tests using NewEnv are intentionally NOT t.Parallel — each spawn costs
// ~200ms (subprocess fork + readiness poll) and there is no shared state
// to keep multiple daemons consistent against. If suite latency becomes a
// problem, batch related assertions into a single test rather than
// parallelizing.
func NewEnv(t testing.TB) *Env {
	t.Helper()

	binary := os.Getenv("FLOW_BINARY")
	if binary == "" {
		binary = "../../build/flow"
	}

	jwks := StartJWKSStub()

	hive := NewFakeHive()
	hiveBase, stopHive := hive.Start()

	sharkfin := NewFakeSharkfin()
	sharkfinBase, stopSharkfin := sharkfin.Start()

	pylonServices := []PylonService{
		{Name: "hive", BaseURL: hiveBase, Label: "Hive", Route: "/hive"},
		{Name: "sharkfin", BaseURL: sharkfinBase, Label: "Sharkfin", Route: "/sharkfin"},
	}
	pylonAddr, stopPylon := StartPylonStub(pylonServices)

	d, err := StartDaemon(t, binary, pylonAddr, jwks.Addr, jwks.SignJWT)
	if err != nil {
		stopSharkfin()
		stopHive()
		stopPylon()
		jwks.Stop()
		t.Fatalf("start daemon: %v", err)
	}

	return &Env{
		Daemon: d, JWKS: jwks, Hive: hive, Sharkfin: sharkfin,
		stopPylon: stopPylon,
		stopHive: stopHive, stopSharkfin: stopSharkfin,
	}
}

// Cleanup stops everything in reverse order. Idempotent.
func (e *Env) Cleanup(t testing.TB) {
	t.Helper()
	if e.Daemon != nil {
		e.Daemon.Stop(t)
		e.Daemon = nil
	}
	if e.stopSharkfin != nil {
		e.stopSharkfin()
		e.stopSharkfin = nil
	}
	if e.stopHive != nil {
		e.stopHive()
		e.stopHive = nil
	}
	if e.stopPylon != nil {
		e.stopPylon()
		e.stopPylon = nil
	}
	if e.JWKS != nil {
		e.JWKS.Stop()
		e.JWKS = nil
	}
}
```

**Step 2: Write client.go**

Create `tests/e2e/harness/client.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a raw http.Client wrapper for the test side of the harness.
// It explicitly does NOT depend on any WorkFort SDK — every contract
// is wire-only. If you reach for sharkfinclient or hiveclient here,
// stop and add the request inline instead.
//
// Auth model: Passport's middleware reads `Authorization: Bearer <token>`
// only — there is no separate API-key header. The validator chain
// internally tries JWT first and falls back to API-key validation
// against the same Bearer token. So both JWTs and API keys ride the
// same wire; tests pick the constructor that documents intent.
type Client struct {
	baseURL string
	token   string // empty means: send no Authorization header
	http    *http.Client
}

// NewClient returns a Client that sends `Authorization: Bearer <token>`.
// Use it for both JWTs and API keys — Passport's validator chain
// distinguishes them server-side.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientNoAuth returns a Client that sends no auth headers.
func NewClientNoAuth(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) authedRequest(method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// Do executes the request and returns (statusCode, responseBody).
// out, if non-nil and the response is 2xx, is JSON-decoded into.
// On non-2xx the response body is returned raw.
func (c *Client) Do(method, path string, body, out any) (int, []byte, error) {
	req, err := c.authedRequest(method, path, body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, respBody, fmt.Errorf("decode response: %w (body: %s)", err, respBody)
		}
	}
	return resp.StatusCode, respBody, nil
}

// GetJSON / PostJSON / PatchJSON / DeleteJSON are thin sugar.
func (c *Client) GetJSON(path string, out any) (int, []byte, error) {
	return c.Do(http.MethodGet, path, nil, out)
}
func (c *Client) PostJSON(path string, body, out any) (int, []byte, error) {
	return c.Do(http.MethodPost, path, body, out)
}
func (c *Client) PatchJSON(path string, body, out any) (int, []byte, error) {
	return c.Do(http.MethodPatch, path, body, out)
}
func (c *Client) DeleteJSON(path string) (int, []byte, error) {
	return c.Do(http.MethodDelete, path, nil, nil)
}
```

**Step 3: Compile check**

Run: `cd tests/e2e && go build ./...`
Expected: success.

**Step 4: Commit**

```bash
git add tests/e2e/harness/env.go tests/e2e/harness/client.go
git commit -m "$(cat <<'EOF'
test(e2e): add Env all-in-one setup and raw HTTP test client

Env stitches the JWKS, Pylon, Hive and Sharkfin stubs together,
spawns the daemon, and returns a single Cleanup(t) for symmetry.
Client wraps stdlib net/http with Bearer-token and no-auth
constructors — Passport reads `Authorization: Bearer <token>`
for both JWTs and API keys, so a single transport covers both.
Neither imports any WorkFort SDK.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: First end-to-end test — health endpoints

**Files:**
- Create: `tests/e2e/health_test.go`

This is the first test that actually spawns the daemon. It validates the harness end-to-end before any larger surface tests pile on. Health is unauthenticated (`publicPathSkip` in `internal/daemon/middleware.go:14-17`) so it doesn't need a JWT.

**Step 1: Write the failing test**

Create `tests/e2e/health_test.go`:

```go
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
```

**Step 2: Run failing test (no flow binary yet built? It exists from Task 4 but in case e2e is run first the e2e mise task depends on build:dev)**

Run: `mise run e2e -- -run TestHealth ./...`
Expected: PASS. (This task is structurally the "implementation" — the harness work in Tasks 5-10 is what makes the test pass. If the test fails, debug the harness, not the test.)

If the test fails because of a JSON decoding issue on `/ui/health` — note that the file `internal/daemon/health.go` (referenced by `HandleUIHealth()`) returns whatever shape it returns; the test doesn't decode it. Status code only.

**Step 3: Confirm both health tests pass**

Run: `mise run e2e -- -run TestHealth ./...`
Expected: `PASS    TestHealth_LivenessReturns200` and `PASS    TestHealth_UIHealthReturns200`.

**Step 4: Commit**

```bash
git add tests/e2e/health_test.go
git commit -m "$(cat <<'EOF'
test(e2e): cover /v1/health and /ui/health

First end-to-end test exercising the full harness: spawn daemon,
hit unauthenticated health endpoints, assert 200. Validates the
Env/Daemon/Client wiring before larger surface tests layer on.
The liveness test additionally asserts the fake Sharkfin saw the
adapter's Register call during daemon init, proving the bundled
fakes carry real traffic — a Pylon-discovery, adapter, or fake
regression that would otherwise be masked by Sharkfin failures
being non-fatal surfaces here immediately.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Verification Checklist

After all 11 tasks complete, the following must hold:

- [ ] `mise run build:dev` succeeds.
- [ ] `mise run lint` reports no diagnostics.
- [ ] `mise run test` (unit + race) passes — including the 8 new
      `TestAdapter_*` cases on `internal/infra/sharkfin`.
- [ ] `mise run e2e` succeeds end-to-end. Specifically:
  - [ ] `TestHealth_LivenessReturns200` passes (real daemon spawned, real
        `/v1/health` reply 200, fake Sharkfin saw the Register call).
  - [ ] `TestHealth_UIHealthReturns200` passes.
- [ ] `cd tests/e2e && grep -E "Work-Fort/(Flow|sharkfin|Hive|Pylon|Passport)" go.sum`
      reports nothing — no WorkFort SDK leaked into the harness module.
- [ ] `tests/e2e/harness/` source files do not import `sharkfinclient`,
      `hiveclient`, `pylonclient`, `Flow/internal/...`, or any other WorkFort
      module.
- [ ] `JWKSStub.APIKeyVerifyCount()` is callable from the harness package
      (Plan A.5 will read it).
- [ ] `Daemon.Stop` dumps captured stderr when `t.Failed()` is true.
- [ ] No goroutine leaks: each test's `Env.Cleanup(t)` returns within 5s.
- [ ] No `DATA RACE` markers in daemon stderr across the full suite.
- [ ] Switching the Sharkfin adapter from WS to REST does not break daemon
      startup against a real Sharkfin instance (manual smoke test —
      `mise run build:dev` then `./build/flow daemon --pylon-url=...
      --passport-url=...` with a known-good Pylon record for Sharkfin).

## Hand-off (planner workflow)

This plan is `status: pending, assessment_status: needed`. Hand to the assessor before any implementation begins; do **not** commit during this planner phase.
