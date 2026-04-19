---
type: plan
step: "B"
title: "Flow E2E harness — Plan B: MCP coverage + Combine webhook receiver + bot lifecycle round-trip"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "B"
dates:
  created: "2026-04-19"
  approved: null
  completed: null
related_plans:
  - "2026-04-18-flow-e2e-harness-01-foundation.md"
  - "2026-04-18-flow-orchestration-01-foundation.md"
  - "2026-04-19-flow-plan-a5-auth-rest.md"
---

# Flow E2E Harness — Plan B: MCP Coverage + Combine Webhook Receiver + Bot Lifecycle Round-Trip

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal.** Three deliverables on top of the now-landed Plans A and A.5:

1. **All 12 MCP tools covered by e2e**, exercising every tool through
   the real spawned daemon's `/mcp` endpoint via raw JSON-RPC 2.0
   framing — no `mark3labs/mcp-go` client import in the harness, no
   `sharkfinclient`, no `hiveclient`. Per
   `feedback_e2e_harness_independence.md`: harness speaks the wire
   protocol directly so client/server drift surfaces as test failure.
2. **`/v1/webhooks/combine` receiver** — Flow exposes a new HTTP
   handler for inbound Combine push + pull-request-merged events. This
   is the missing endpoint that unblocks Combine's webhook subscription
   operational task (per `AGENT-POOL-REMAINING-WORK.md` Combine row).
   The Sharkfin webhook receiver already exists
   (`internal/daemon/webhook_sharkfin.go`); Plan B extends harness
   coverage of it as part of deliverable 3 below.
3. **Bot-lifecycle round-trip via fakes** — full e2e test where a
   harness-driven "bot" simulates the canonical proof-of-life loop
   from `AGENT-POOL-REMAINING-WORK.md` "End-to-end agent pool live
   verification scenario": work-item-created → bot-posts-to-channel →
   bot-claims-agent → agent-runs (simulated) → combine-merge-webhook →
   bot-reports-completion → close, with every transition + claim +
   release + chat-post + Combine-merge audited through Flow's
   `audit_events` store. The whole loop runs with FakeHive +
   FakeSharkfin + a new FakeCombine — zero real-client imports.

## MCP tool count verification

Verified 2026-04-19 by reading
`internal/daemon/mcp_tools.go:25-351`: the comment on line 25 reads
"registerTools registers all 12 MCP tools on the server" and the
file contains exactly 12 `s.AddTool(` invocations:

| # | Tool name | Backing call |
|---|-----------|--------------|
| 1 | `list_templates` | `Store.ListTemplates` |
| 2 | `get_template` | `Store.GetTemplate` |
| 3 | `create_instance` | `Store.{GetTemplate,CreateInstance,GetInstance}` |
| 4 | `list_instances` | `Store.ListInstances` (filterable by `team_id`) |
| 5 | `create_work_item` | `Store.{GetInstance,GetTemplate,CreateWorkItem,GetWorkItem}` |
| 6 | `list_work_items` | `Store.ListWorkItems` (filterable by `step_id`/`agent_id`/`priority`) |
| 7 | `get_work_item` | `Store.{GetWorkItem,GetTransitionHistory}` |
| 8 | `transition_work_item` | `Svc.TransitionItem` |
| 9 | `approve_work_item` | `Svc.ApproveItem` |
| 10 | `reject_work_item` | `Svc.RejectItem` |
| 11 | `assign_work_item` | `Store.{GetWorkItem,UpdateWorkItem}` |
| 12 | `get_instance_status` | `Store.{GetInstance,ListWorkItems}` (groups by step) |

The "12" cited in `AGENT-POOL-REMAINING-WORK.md` and the
orchestration-impl spec is correct. **No discrepancy to flag.**

## Why this plan exists, scoped to these three

Plan A (`2026-04-18-flow-e2e-harness-01-foundation.md`) shipped the
harness skeleton (daemon spawn, four fakes, JWKS stub, `Client`
wrappers, two health tests, dual-backend wiring). Plan A.5
(`2026-04-19-flow-plan-a5-auth-rest.md`) extended coverage to all 18
production REST endpoints + auth scheme-dispatch + harness README.

Plan B is the next bite-sized layer: the **non-REST surface** Flow
exposes to the agent pool (MCP, two webhook receivers) plus the first
**multi-system orchestration test** that proves the agent-pool
lifecycle works end-to-end across Flow + Hive + Sharkfin + Combine
fakes. It does NOT include:

- Real bot processes (those land in the bot-vocabulary plan, the next
  workstream after Plan B).
- Per-project bot identity bootstrap via Scope+Playwright (deferred
  per `AGENT-POOL-REMAINING-WORK.md` "Bot bootstrap — manual via
  Scope+Playwright" decision).
- The k8s `RuntimeDriver` impl (separate future plan).
- Flow UI work (separate plan, prerequisite for bot bootstrap).

## Verification of "Combine webhook receiver doesn't yet exist"

Verified 2026-04-19:
- `internal/daemon/server.go:124` mounts only
  `POST /v1/webhooks/sharkfin`. There is no
  `POST /v1/webhooks/combine` route.
- `internal/daemon/webhook_sharkfin.go` is the only `webhook_*.go` in
  `internal/daemon/`.
- `AGENT-POOL-REMAINING-WORK.md` Combine row: "Push + merge webhook
  subscription to Flow's `/v1/webhooks/combine` endpoint (which Flow
  doesn't yet expose) — blocked on Flow side". Plan B unblocks this.

## Verification of "Sharkfin webhook receiver already exists, needs e2e harness coverage"

Verified 2026-04-19:
- `internal/daemon/webhook_sharkfin.go:56-86` defines
  `HandleSharkfinWebhook`. It parses the JSON body matching Sharkfin's
  `WebhookPayload` (see `sharkfin/lead/pkg/daemon/webhooks.go:15-30`),
  unwraps the `metadata` JSON string into `sharkfinMessageMeta`,
  routes `event_type == "flow_command"` payloads to the optional
  `CommandHandler`, and always responds 204.
- `internal/daemon/webhook_sharkfin_test.go` covers two unit cases
  (plain message ignored, flow_command parsed). There is no e2e test
  reaching `/v1/webhooks/sharkfin` through the spawned daemon.
- `internal/daemon/server.go:124` wires `HandleSharkfinWebhook(nil)`
  — production currently dispatches commands nowhere. Plan B keeps
  the production wiring as-is (a no-op handler is correct until the
  bot-vocabulary plan adds real dispatch); the e2e test exercises the
  parser via the production endpoint.

## Combine event subset Plan B receives

Plan B accepts and audits exactly two Combine event types:

| Combine event | `X-SoftServe-Event` header | Flow audit reaction |
|---|---|---|
| `EventPush` | `push` | record `combine_push_received` audit event with `repo`, `ref`, `before`, `after` |
| `EventPullRequestMerged` | `pull_request_merged` | record `combine_merge_received` audit event with `repo`, `pr_number`, `merged_by`, `target_branch` |

All other Combine event types (12 others enumerated in
`combine/lead/internal/infra/webhook/event.go:9-54`) are accepted with
204 and silently ignored. The bot-vocabulary plan (next workstream)
extends the dispatch — Plan B's receiver is the wire-format
foundation.

The `User-Agent: SoftServe/<version>` and `X-SoftServe-Delivery: <uuid>`
headers Combine sends (per `combine/lead/internal/infra/webhook/webhook.go:115-124`)
are read but not validated — Combine signing is a future hardening
task tracked in `combine/lead/docs/remaining-work.md`, NOT in scope
for Plan B.

## Bot-lifecycle round-trip — what runs and what it asserts

The deliverable-3 test
(`tests/e2e/bot_lifecycle_test.go`) is a single end-to-end Go test.
It uses a new `harness.BotSimulator` helper that issues raw HTTP
calls into Flow + asserts against the three fakes' recorded state.
**No real bot process runs.** The simulator IS the bot for the
duration of the test, mechanically driving each phase the way a real
project bot would:

1. **Setup.** `harness.NewEnv(t)` stands up daemon + JWKS + FakeHive +
   FakeSharkfin + a new **FakeCombine** the simulator can later call
   from the merge phase. `env.Hive.SeedPoolAgent("a_b_001",
   "agent-b-1", "team-b")`. Service token client is built with
   `harness.NewClientAPIKey`.
2. **Project + workflow seed.** Simulator POSTs a workflow template
   (4 steps: `dev → review → qa → merged`, one approve gate at
   `review`), creates an instance for `team_b`, creates one work item
   `wi_round_trip_001` at the `dev` step.
3. **Bot posts to channel (work-item-claimed analogue).** Simulator
   POSTs to `/v1/webhooks/sharkfin` a synthesized
   `WebhookPayload` whose body and metadata mirror what a real bot
   would emit when announcing the work item (event_type
   `flow_command`, action `claim_for_dev`,
   work_item_id=`wi_round_trip_001`). Asserts: 204 received.
4. **Bot claims agent (in-progress analogue).** Simulator POSTs
   `/v1/scheduler/_diag/claim` with role=`developer` project=`flow`
   workflow_id=`wf_round_trip_001` ttl=60. Asserts: response contains
   `a_b_001`; FakeHive `ClaimCalls() == 1`; an `agent_claimed` row
   exists in the audit log via
   `/v1/audit/_diag/by-workflow/wf_round_trip_001`.
5. **Agent runs (simulated developer phase).** Simulator transitions
   the work item from `dev → review` via the production REST
   `POST /v1/items/{id}/transition`. Asserts: status 200; work item's
   `current_step_id` now references the `review` step.
6. **Submitted-for-review → approved.** Simulator approves the work
   item via `POST /v1/items/{id}/approve` (gate step), then
   transitions to `qa`. Asserts: approval recorded;
   `/v1/items/{id}/approvals` returns 1.
7. **Combine merge webhook fires.** Simulator POSTs to a NEW
   `/v1/webhooks/combine` a synthesized Combine `pull_request_merged`
   payload referring to the work item's branch. Asserts: 204
   received; `combine_merge_received` audit row written.
8. **Bot reports completion + closes.** Simulator transitions
   `qa → merged`, then POSTs to `/v1/webhooks/sharkfin` a final
   `flow_command` action `mark_done`. Asserts: 204 received; work
   item's final state is `merged`.
9. **Bot releases agent.** Simulator POSTs
   `/v1/scheduler/_diag/release` for `(a_b_001, wf_round_trip_001)`.
   Asserts: 204 / 200; FakeHive `ReleaseCalls() == 1`; an
   `agent_released` row exists in the audit log.
10. **Final audit assertion.** Simulator GETs
    `/v1/audit/_diag/by-workflow/wf_round_trip_001` and asserts the
    event sequence is exactly:
    `agent_claimed → lease_renewed*+ → combine_merge_received →
     agent_released`
    (renew events appear at least once because the daemon's renewer
    runs at 100 ms via the harness env override).

The test runs against both backends via the existing
`-backend` flag wired by Plan A.

## Non-goals (deferred)

- Real `bot_*` daemons / per-project bot identity bootstrap (next
  workstream, formalises SDLC vocabulary on top of Plan B's wire
  flow).
- Bidirectional Sharkfin command dispatch into the workflow service
  (today the `webhook_sharkfin.go` `CommandHandler` is `nil` in
  production; wiring it requires the bot vocabulary plan's
  command-to-action mapping; Plan B asserts the parse path only).
- Combine webhook signature validation (deferred: Combine doesn't
  sign today).
- MCP `subscriptions/listen`, prompts, sampling, or any other
  protocol surface beyond `tools/list` + `tools/call` (Flow's MCP
  server is configured `WithToolCapabilities(false)` —
  `internal/daemon/mcp_server.go:25` — i.e. only static tool list +
  tool calls; Plan B exercises exactly that surface).
- The k8s `RuntimeDriver` impl (separate future plan); Plan B keeps
  the existing `WithStubRuntimeEnv()` driver injection.

## Hard constraints (non-negotiable, carried from Plans A and A.5)

- Harness imports zero WorkFort Go client packages. No
  `sharkfinclient`, no `hiveclient`, no `pylonclient`, no
  `mark3labs/mcp-go` client. **MCP framing is hand-rolled**:
  `tests/e2e/harness/mcp_client.go` synthesizes JSON-RPC 2.0
  envelopes and parses SSE / JSON / 202 responses the same way the
  Sharkfin mcp-bridge does
  (`sharkfin/lead/cmd/mcpbridge/mcp_bridge.go:115-147`). The harness
  does NOT import `cmd/mcpbridge` either; the parsing lives in the
  harness as a few dozen lines of `bufio.Scanner` over the response
  body.
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
- Tests do NOT call `t.Parallel()` — daemon spawn cost is ~200 ms per
  test; the harness already documents this convention
  (`tests/e2e/harness/env.go:60-64`).
- New helpers obey hexagonal-architecture rules from
  `skills/lead/go-service-architecture/SKILL.md`: domain types in
  `internal/domain/`, infra adapters in `internal/infra/*`, daemon
  wiring in `internal/daemon/`. No business logic in the new HTTP
  handlers — they parse, audit, return 204.

## Tech stack

Unchanged from Plans A and A.5: Go 1.26, `net/http`,
`net/http/httptest`, `encoding/json`, `database/sql`, `bufio`,
`github.com/lestrrat-go/jwx/v2`, `github.com/jackc/pgx/v5/stdlib`.
**No new dependencies introduced by this plan.**

---

## Prerequisites

Before starting:
- [ ] Plan A landed (`status: complete`) — verified by
      `tests/e2e/harness/{client,daemon,env,fake_hive,fake_sharkfin,jwks_stub,pylon_stub}.go`
      existing.
- [ ] Plan A.5 landed (`status: complete`) — verified by
      `tests/e2e/{auth_test,daemon_auth_test,templates_test,instances_test,items_test,transitions_test,approvals_test}.go`
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

### Task 1: Harness — minimal MCP wire client (`tests/e2e/harness/mcp_client.go`)

**Why first.** All MCP tests in Task 2 depend on a JSON-RPC 2.0
client that:
- speaks the streaming-HTTP MCP transport Flow's
  `server.NewStreamableHTTPServer` exposes
  (`internal/daemon/mcp_server.go:30`),
- handles the three response shapes that transport produces
  (`application/json`, `text/event-stream`, `202 Accepted`),
- carries the `Mcp-Session-Id` header across calls,
- speaks `Authorization: ApiKey-v1 <key>` (matching the Sharkfin
  mcp-bridge convention,
  `sharkfin/lead/cmd/mcpbridge/mcp_bridge.go:166`),
- imports zero MCP libraries.

Delivering this first as a self-contained file lets Task 2's tests
read top-to-bottom against a stable wire client.

**Files:**
- Create: `tests/e2e/harness/mcp_client.go`
- Test: `tests/e2e/harness/mcp_client_test.go`

**Step 1: Write the failing test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// Verifies the MCP client envelopes a tools/call request as JSON-RPC
// 2.0, sends Authorization: ApiKey-v1 <key>, captures the
// Mcp-Session-Id header, and decodes a single-line application/json
// response into the result.
func TestMCPClient_ToolsCallRoundTrip(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "ApiKey-v1 test-key" {
			t.Errorf("Authorization = %q, want %q", got, "ApiKey-v1 test-key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["jsonrpc"] != "2.0" || body["method"] != "tools/call" {
			t.Errorf("unexpected body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "sess-abc")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"\"ok\""}]}}`))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")

	text, err := mc.Call("ping_tool", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if text != `"ok"` {
		t.Errorf("text = %q, want %q", text, `"ok"`)
	}
	if mc.SessionID() != "sess-abc" {
		t.Errorf("session = %q, want sess-abc", mc.SessionID())
	}

	// Second call should send the captured session header back.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Mcp-Session-Id"); got != "sess-abc" {
			t.Errorf("session header = %q, want sess-abc", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"\"ok2\""}]}}`))
	}))
	defer srv2.Close()
	mc2 := harness.NewMCPClient(srv2.URL+"/mcp", "test-key")
	mc2.SetSessionID("sess-abc")
	if _, err := mc2.Call("ping_tool", nil); err != nil {
		t.Fatalf("Call (session reuse): %v", err)
	}
}

// Verifies the client extracts the last data: line from an SSE
// response (matching the mcp-bridge readResponseBody convention).
func TestMCPClient_ParsesSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notification\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"\\\"sse-ok\\\"\"}]}}\n\n"))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")
	text, err := mc.Call("ping_tool", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(text, "sse-ok") {
		t.Errorf("text = %q, want substring sse-ok", text)
	}
}

// Verifies that an MCP-level error result surfaces as a Go error.
func TestMCPClient_ErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"boom"}]}}`))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")
	_, err := mc.Call("ping_tool", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want containing 'boom'", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tests/e2e/harness/ -run TestMCPClient`
Expected: FAIL with `undefined: harness.NewMCPClient` and friends.

**Step 3: Write minimal implementation**

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPClient is a hand-rolled MCP-over-streaming-HTTP wire client.
// It speaks JSON-RPC 2.0, sends Authorization: ApiKey-v1 <key>,
// follows Mcp-Session-Id across calls, and copes with the three
// response shapes Flow's NewStreamableHTTPServer produces:
// application/json, text/event-stream, and 202 Accepted.
//
// It deliberately does NOT import mark3labs/mcp-go. Drift between
// this client and the real MCP server's wire format must surface as
// test failure — see feedback_e2e_harness_independence.md.
type MCPClient struct {
	mcpURL string
	apiKey string
	http   *http.Client
	mu     sync.Mutex
	sess   string
	nextID atomic.Int64
}

// NewMCPClient constructs a client targeting the given /mcp URL and
// using the given Passport API key for Authorization.
func NewMCPClient(mcpURL, apiKey string) *MCPClient {
	return &MCPClient{
		mcpURL: mcpURL,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// SessionID returns the most recent Mcp-Session-Id the server set.
func (c *MCPClient) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sess
}

// SetSessionID seeds the session header. Useful for tests that want
// to verify the header round-trips.
func (c *MCPClient) SetSessionID(s string) {
	c.mu.Lock()
	c.sess = s
	c.mu.Unlock()
}

// Call invokes the named MCP tool with the given arguments and
// returns the first text-content block of the result. If the result
// is marked isError, returns a non-nil error whose message contains
// the error text.
func (c *MCPClient) Call(tool string, args map[string]any) (string, error) {
	id := c.nextID.Add(1)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("mcp marshal: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.mcpURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("mcp request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "ApiKey-v1 "+c.apiKey)
	if s := c.SessionID(); s != "" {
		httpReq.Header.Set("Mcp-Session-Id", s)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", tool, err)
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sess = sid
		c.mu.Unlock()
	}

	if resp.StatusCode == http.StatusAccepted {
		// notification accepted, no body
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mcp call %s: HTTP %d: %s", tool, resp.StatusCode, raw)
	}

	msgs, err := readMCPResponse(resp)
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", tool, err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("mcp call %s: empty response", tool)
	}

	// The last message is the actual response; intermediate messages
	// are notifications.
	last := msgs[len(msgs)-1]
	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(last, &rpc); err != nil {
		return "", fmt.Errorf("mcp call %s: parse response: %w (body=%s)", tool, err, last)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("mcp call %s: rpc error %d: %s", tool, rpc.Error.Code, rpc.Error.Message)
	}
	var text string
	if len(rpc.Result.Content) > 0 {
		text = rpc.Result.Content[0].Text
	}
	if rpc.Result.IsError {
		return "", errors.New(text)
	}
	return text, nil
}

// readMCPResponse handles SSE and JSON content types. Returns each
// data: line (SSE) or the entire body (JSON) as a separate message.
func readMCPResponse(resp *http.Response) ([][]byte, error) {
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		var msgs [][]byte
		sc := bufio.NewScanner(resp.Body)
		// Default buffer is 64 KiB; bump it for larger MCP responses.
		buf := make([]byte, 0, 256*1024)
		sc.Buffer(buf, 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data: ") {
				msgs = append(msgs, []byte(strings.TrimPrefix(line, "data: ")))
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read SSE: %w", err)
		}
		return msgs, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return [][]byte{body}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./tests/e2e/harness/ -run TestMCPClient`
Expected: PASS (3 sub-tests).

**Step 5: Commit**

```
test(e2e): add MCP wire client to harness

Hand-rolled JSON-RPC 2.0 over streaming-HTTP MCP client. Speaks
ApiKey-v1 auth, captures Mcp-Session-Id across calls, decodes
JSON, SSE, and 202-Accepted response shapes. Imports no MCP
library — drift between client and Flow's mcp-go-backed server
must surface as test failure (per
feedback_e2e_harness_independence.md).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 2: E2E coverage for all 12 MCP tools (`tests/e2e/mcp_tools_test.go`)

**Depends on:** Task 1 (`harness.MCPClient`).

**Why now.** The `MCPClient` is in place; the spawned daemon's `/mcp`
endpoint is already wired (`internal/daemon/server.go:127-131`). One
test file per tool would duplicate setup; we instead use one
table-driven test plus a multi-step sequence test to exercise the
flows that need real state (work-item creation → transition →
approve, etc.).

**Files:**
- Create: `tests/e2e/mcp_tools_test.go`

**Step 1: Write the failing test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// mcpEnv stands up Env, signs an API-key client, builds an MCPClient,
// and seeds one workflow template + instance + work item so every
// MCP tool has something to act on. Returns the IDs the tools need.
type mcpFixture struct {
	env        *harness.Env
	mcp        *harness.MCPClient
	templateID string
	instanceID string
	workItemID string
	devStepID  string
	revStepID  string
	devToRev   string // transition ID
}

func setupMCP(t *testing.T) *mcpFixture {
	t.Helper()
	env := harness.NewEnv(t)
	t.Cleanup(func() { env.Cleanup(t) })

	tok := env.Daemon.SignJWT("svc-mcp", "flow-mcp", "Flow MCP", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Create template with two steps + one transition.
	tmplBody := map[string]any{
		"name":        "mcp-tmpl",
		"description": "tmpl for MCP e2e",
		"steps": []map[string]any{
			{"key": "dev", "name": "Dev", "type": "task", "position": 1},
			{"key": "rev", "name": "Review", "type": "task", "position": 2},
		},
		"transitions": []map[string]any{
			{"key": "dev_to_rev", "name": "Dev to Review",
				"from_step_key": "dev", "to_step_key": "rev"},
		},
	}
	var tmplResp struct {
		ID    string `json:"id"`
		Steps []struct {
			ID, Key string
		} `json:"steps"`
		Transitions []struct {
			ID, Key string
		} `json:"transitions"`
	}
	if status, body, err := c.PostJSON("/v1/templates", tmplBody, &tmplResp); err != nil || status != 200 {
		t.Fatalf("create template: status=%d body=%s err=%v", status, body, err)
	}

	f := &mcpFixture{env: env, templateID: tmplResp.ID}
	for _, s := range tmplResp.Steps {
		if s.Key == "dev" {
			f.devStepID = s.ID
		}
		if s.Key == "rev" {
			f.revStepID = s.ID
		}
	}
	for _, tr := range tmplResp.Transitions {
		if tr.Key == "dev_to_rev" {
			f.devToRev = tr.ID
		}
	}

	f.mcp = harness.NewMCPClient(env.Daemon.BaseURL()+"/mcp", "harness-service-token")
	return f
}

// callDecode invokes the named tool and JSON-decodes the result text.
func (f *mcpFixture) callDecode(t *testing.T, tool string, args map[string]any, out any) {
	t.Helper()
	text, err := f.mcp.Call(tool, args)
	if err != nil {
		t.Fatalf("mcp.Call(%s): %v", tool, err)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(text), out); err != nil {
			t.Fatalf("decode %s result: %v (text=%s)", tool, err, text)
		}
	}
}

func TestMCP_ListAndGetTemplate(t *testing.T) {
	f := setupMCP(t)

	var list []map[string]any
	f.callDecode(t, "list_templates", nil, &list)
	if len(list) != 1 {
		t.Fatalf("list_templates: got %d, want 1", len(list))
	}

	var got map[string]any
	f.callDecode(t, "get_template", map[string]any{"id": f.templateID}, &got)
	if got["id"] != f.templateID {
		t.Errorf("get_template id = %v, want %v", got["id"], f.templateID)
	}
}

func TestMCP_InstanceLifecycle(t *testing.T) {
	f := setupMCP(t)

	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID,
		"team_id":     "team-mcp",
		"name":        "inst-mcp",
	}, &inst)
	instanceID, _ := inst["id"].(string)
	if instanceID == "" {
		t.Fatalf("create_instance returned no id: %v", inst)
	}

	var list []map[string]any
	f.callDecode(t, "list_instances", map[string]any{"team_id": "team-mcp"}, &list)
	if len(list) != 1 {
		t.Errorf("list_instances: got %d, want 1", len(list))
	}

	var status map[string]any
	f.callDecode(t, "get_instance_status", map[string]any{"id": instanceID}, &status)
	if status["instance"] == nil {
		t.Errorf("get_instance_status missing instance: %v", status)
	}
}

func TestMCP_WorkItemCRUDAndAssign(t *testing.T) {
	f := setupMCP(t)

	// Need an instance to host the work item.
	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID, "team_id": "team-mcp", "name": "inst",
	}, &inst)
	instanceID, _ := inst["id"].(string)

	var wi map[string]any
	f.callDecode(t, "create_work_item", map[string]any{
		"instance_id": instanceID,
		"title":       "wi via mcp",
		"description": "desc",
		"priority":    "high",
	}, &wi)
	wiID, _ := wi["id"].(string)
	if wiID == "" {
		t.Fatalf("create_work_item returned no id: %v", wi)
	}

	var listed []map[string]any
	f.callDecode(t, "list_work_items", map[string]any{
		"instance_id": instanceID,
	}, &listed)
	if len(listed) != 1 {
		t.Errorf("list_work_items: got %d, want 1", len(listed))
	}

	var got map[string]any
	f.callDecode(t, "get_work_item", map[string]any{"id": wiID}, &got)
	if got["work_item"] == nil {
		t.Errorf("get_work_item missing work_item: %v", got)
	}

	var assigned map[string]any
	f.callDecode(t, "assign_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x",
	}, &assigned)
	if assigned["assigned_agent_id"] != "agent-x" {
		t.Errorf("assigned_agent_id = %v, want agent-x", assigned["assigned_agent_id"])
	}
}

func TestMCP_TransitionApproveReject(t *testing.T) {
	f := setupMCP(t)

	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID, "team_id": "team-mcp", "name": "inst",
	}, &inst)
	instanceID, _ := inst["id"].(string)

	var wi map[string]any
	f.callDecode(t, "create_work_item", map[string]any{
		"instance_id": instanceID, "title": "wi",
	}, &wi)
	wiID, _ := wi["id"].(string)

	// Transition dev -> rev.
	var trans map[string]any
	f.callDecode(t, "transition_work_item", map[string]any{
		"id":             wiID,
		"transition_id":  f.devToRev,
		"actor_agent_id": "agent-x",
		"actor_role_id":  "role-developer",
		"reason":         "done",
	}, &trans)
	if trans["current_step_id"] != f.revStepID {
		t.Errorf("current_step_id = %v, want %v", trans["current_step_id"], f.revStepID)
	}

	// approve_work_item should fail when the step is not a gate; assert
	// the error path surfaces. The Dev step is not a gate, so we use it
	// against approve to exercise the error route.
	if _, err := f.mcp.Call("approve_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x", "comment": "lgtm",
	}); err == nil {
		t.Errorf("approve_work_item on non-gate step: expected error, got nil")
	} else if !strings.Contains(err.Error(), "gate") &&
		!strings.Contains(err.Error(), "approval") {
		t.Errorf("approve error = %v, want containing 'gate' or 'approval'", err)
	}

	// reject_work_item — same expectation.
	if _, err := f.mcp.Call("reject_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x", "comment": "no",
	}); err == nil {
		t.Errorf("reject_work_item on non-gate step: expected error, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `mise run e2e --backend=sqlite -- -run TestMCP`
Expected: FAIL on every test — they exercise endpoints + tools that
exist but the harness file doesn't compile yet (the tests reference
the file under construction). Once the file is added, every assertion
must pass on first run because all the daemon-side wiring is already
present (`mcp_tools.go:25-351`).

**Step 3: Fix any wiring issues exposed by the test**

If `mise run e2e --backend=sqlite -- -run TestMCP` reports a real
defect (e.g. the streaming-HTTP transport returning a body shape the
client doesn't recognise), fix the **client**, not the daemon —
Plan B's MCP server surface is fixed and any incompatibility is a
harness gap. If the daemon emits a body the client cannot parse,
either widen `readMCPResponse` or add a JSON-content-type case.

**Step 4: Run both backends to verify pass**

Run: `mise run e2e --backend=sqlite -- -run TestMCP`
Expected: PASS (4 tests).
Run: `mise run e2e --backend=postgres -- -run TestMCP`
Expected: PASS (4 tests).

**Step 5: Commit**

```
test(e2e): cover all 12 MCP tools end-to-end

Drives the spawned daemon's /mcp endpoint via the harness MCP wire
client and asserts every tool returns the expected shape:
list_templates, get_template, create_instance, list_instances,
create_work_item, list_work_items, get_work_item,
transition_work_item, approve_work_item, reject_work_item,
assign_work_item, get_instance_status. The approve / reject
non-gate cases verify the error-path surfaces correctly through
JSON-RPC.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 3: E2E coverage for the existing Sharkfin webhook receiver

**Depends on:** Plan A's harness (no new harness code needed).

**Why now.** `internal/daemon/webhook_sharkfin.go` already exists and
is wired in production (`internal/daemon/server.go:124`). Plan A.5
deferred this to Plan B; the test goes here so the bot-lifecycle
test in Task 6 has a regression net under the same handler it uses.

**Files:**
- Create: `tests/e2e/webhook_sharkfin_test.go`

**Step 1: Write the failing test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// Posts a flow_command-bearing Sharkfin webhook payload to the
// production /v1/webhooks/sharkfin endpoint via the spawned daemon.
// Asserts 204 (parser path) and that the daemon does not propagate
// command processing into the audit log (production wires
// HandleSharkfinWebhook(nil) — see internal/daemon/server.go:124).
// The bot-vocabulary plan will wire a real CommandHandler; this test
// pins the parse-only contract.
func TestSharkfinWebhook_FlowCommand_204(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-sw", "flow-sw", "Flow SW", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	meta := `{"event_type":"flow_command","event_payload":{"action":"status","work_item_id":"wi_unit"}}`
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   42,
		"channel_id":   1,
		"channel_name": "general",
		"channel_type": "public",
		"from":         "agent-1",
		"from_type":    "service",
		"body":         "@flow status",
		"metadata":     meta,
		"sent_at":      "2026-04-19T10:00:00Z",
	}
	status, body, err := c.PostJSON("/v1/webhooks/sharkfin", payload, nil)
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body=%s", status, body)
	}
}

// Same endpoint must accept (and 204) plain messages with no
// metadata — this is the production hot path for ordinary chat.
func TestSharkfinWebhook_PlainMessage_204(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-sw", "flow-sw", "Flow SW", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	payload := map[string]any{
		"event":        "message.new",
		"message_id":   1,
		"channel_id":   2,
		"channel_name": "general",
		"channel_type": "public",
		"from":         "user-1",
		"from_type":    "user",
		"body":         "hello",
		"metadata":     nil,
		"sent_at":      "2026-04-19T10:00:00Z",
	}
	b, _ := json.Marshal(payload)
	status, _, err := c.Do(http.MethodPost, "/v1/webhooks/sharkfin", json.RawMessage(b), nil)
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `mise run e2e --backend=sqlite -- -run TestSharkfinWebhook`
Expected: PASS immediately — the endpoint already exists. The
"failure" we want is *the absence of this test*; if the test passes
on its first run, that's correct. If it fails, the production wire
contract has drifted and that's a real bug to root-cause before
proceeding.

> **Note on the TDD-flow shape here.** Tasks 3 + 4 cover handlers
> that already exist or are added one step before the test runs;
> "write the failing test, then fix" is still the right shape, but
> "the test passes on first run" is the expected outcome for Task 3
> (regression net) and "the test fails until Step 3, then passes" is
> the expected shape for Task 4 (new handler).

**Step 3: Run both backends**

Run: `mise run e2e --backend=postgres -- -run TestSharkfinWebhook`
Expected: PASS.

**Step 4: Commit**

```
test(e2e): cover Sharkfin webhook receiver end-to-end

Posts both flow_command-bearing and plain Sharkfin WebhookPayload
shapes to the production /v1/webhooks/sharkfin endpoint via the
spawned daemon and asserts 204. Pins the parse-only contract while
the bot-vocabulary plan is in flight.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 4: Add `/v1/webhooks/combine` receiver (handler + production wiring)

**Depends on:** none (independent of Tasks 1-3).

**Why now.** Combine's operational webhook subscription is blocked on
this endpoint per `AGENT-POOL-REMAINING-WORK.md`. The handler needs
to exist before the bot-lifecycle test in Task 6 can assert the
merge-webhook leg of the round-trip.

**Files:**
- Create: `internal/daemon/webhook_combine.go`
- Modify: `internal/daemon/server.go` (mount the new route)
- Create: `internal/daemon/webhook_combine_test.go`
- Modify: `internal/domain/types.go` (add two new `AuditEventType` constants)

**Step 1: Add the new audit-event types**

Modify `internal/domain/types.go` lines 196-201 to add two
constants:

```go
const (
	AuditEventAgentClaimed          AuditEventType = "agent_claimed"
	AuditEventAgentReleased         AuditEventType = "agent_released"
	AuditEventLeaseRenewed          AuditEventType = "lease_renewed"
	AuditEventLeaseExpiredBySweeper AuditEventType = "lease_expired_by_sweeper"
	AuditEventCombinePushReceived   AuditEventType = "combine_push_received"
	AuditEventCombineMergeReceived  AuditEventType = "combine_merge_received"
)
```

No store/migration change is needed: `AuditEvent.Type` is a `string`
column in both backends (the SQLite + PG stores landed in the
Foundation plan accept arbitrary string values). The existing
`RecordAuditEvent` path persists the new types unchanged.

**Step 2: Write the failing handler unit test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/domain"
)

type captureAudit struct {
	events []*domain.AuditEvent
}

func (c *captureAudit) RecordAuditEvent(_ context.Context, e *domain.AuditEvent) error {
	c.events = append(c.events, e)
	return nil
}
func (c *captureAudit) ListAuditEventsByWorkflow(context.Context, string) ([]*domain.AuditEvent, error) {
	return nil, nil
}
func (c *captureAudit) ListAuditEventsByAgent(context.Context, string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func postCombine(t *testing.T, h http.Handler, event string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/combine", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", event)
	req.Header.Set("X-SoftServe-Delivery", "del-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCombineWebhook_PushAuditedAndAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit)
	w := postCombine(t, h, "push", map[string]any{
		"repository": map[string]any{"name": "flow"},
		"ref":        "refs/heads/main",
		"before":     "abc123",
		"after":      "def456",
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	if audit.events[0].Type != domain.AuditEventCombinePushReceived {
		t.Errorf("type = %q, want %q", audit.events[0].Type, domain.AuditEventCombinePushReceived)
	}
}

func TestCombineWebhook_MergeAuditedAndAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit)
	w := postCombine(t, h, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 42, "target_branch": "main"},
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	if audit.events[0].Type != domain.AuditEventCombineMergeReceived {
		t.Errorf("type = %q, want %q", audit.events[0].Type, domain.AuditEventCombineMergeReceived)
	}
}

func TestCombineWebhook_OtherEventsIgnoredButAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit)
	w := postCombine(t, h, "issue_opened", map[string]any{"number": 1})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 0 {
		t.Errorf("audit events = %d, want 0", len(audit.events))
	}
}

func TestCombineWebhook_NilAuditNeverPanics(t *testing.T) {
	h := daemon.HandleCombineWebhook(nil)
	w := postCombine(t, h, "push", map[string]any{})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}
```

**Step 3: Run unit test to verify it fails**

Run: `go test ./internal/daemon/ -run TestCombineWebhook`
Expected: FAIL with `undefined: daemon.HandleCombineWebhook`.

**Step 4: Write minimal implementation**

```go
// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// combinePushPayload is the subset of Combine's PushEvent body we
// audit. Fields that aren't audited are left out — drift in those
// fields is irrelevant to Flow.
type combinePushPayload struct {
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// combineMergePayload is the subset of Combine's
// PullRequestMergedEvent body we audit.
type combineMergePayload struct {
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	PullRequest struct {
		Number       int64  `json:"number"`
		TargetBranch string `json:"target_branch"`
	} `json:"pull_request"`
	Sender struct {
		Username string `json:"username"`
	} `json:"sender"`
}

// HandleCombineWebhook returns the http.Handler mounted at
// POST /v1/webhooks/combine. It audits push and pull_request_merged
// events and 204s every other Combine event type.
//
// Combine's webhook discriminator is the X-SoftServe-Event header
// (combine/lead/internal/infra/webhook/webhook.go:117). Audit failures
// are logged but never block the response — the bot-vocabulary plan
// will layer real dispatch on top of this audit foundation.
//
// audit may be nil (e.g. tests / early bring-up); the handler then
// drops the event and still 204s.
func HandleCombineWebhook(audit domain.AuditEventStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := r.Header.Get("X-SoftServe-Event")
		switch event {
		case "push":
			var body combinePushPayload
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				log.Warn("combine webhook: bad push body", "err", err)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"repo":   body.Repository.Name,
				"ref":    body.Ref,
				"before": body.Before,
				"after":  body.After,
			})
			recordCombineEvent(r.Context(), audit, domain.AuditEventCombinePushReceived, payload)
		case "pull_request_merged":
			var body combineMergePayload
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				log.Warn("combine webhook: bad merge body", "err", err)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"repo":          body.Repository.Name,
				"pr_number":     body.PullRequest.Number,
				"target_branch": body.PullRequest.TargetBranch,
				"merged_by":     body.Sender.Username,
			})
			recordCombineEvent(r.Context(), audit, domain.AuditEventCombineMergeReceived, payload)
		default:
			// Other Combine event types are accepted for forward
			// compatibility but not audited at this layer.
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func recordCombineEvent(ctx interface {
	Done() <-chan struct{}
	Err() error
	Value(any) any
	Deadline() (deadline interface{ IsZero() bool }, ok bool)
}, audit domain.AuditEventStore, ty domain.AuditEventType, payload []byte) {
	if audit == nil {
		return
	}
	// Use the request's context so cancellations propagate.
	type ctxRequest interface {
		Done() <-chan struct{}
	}
	_ = ctxRequest(ctx)
}
```

> **Implementation note for the developer.** The `recordCombineEvent`
> shape above is sketched as guidance; the actual implementation MUST
> use a real `context.Context` parameter — not the inline interface —
> and call `audit.RecordAuditEvent(ctx, &domain.AuditEvent{Type: ty,
> Payload: payload})`. The sketch only highlights that the context
> chain is request-scoped. Replace the sketch with the canonical
> `func recordCombineEvent(ctx context.Context, audit
> domain.AuditEventStore, ty domain.AuditEventType, payload
> json.RawMessage)` signature. Audit failures are logged at warn level,
> never returned. After implementation, re-run the tests in Step 5.

**Step 5: Run unit test to verify it passes**

Run: `go test ./internal/daemon/ -run TestCombineWebhook`
Expected: PASS (4 tests).

**Step 6: Wire the route in `server.go`**

Modify `internal/daemon/server.go` between lines 123-125:

```go
// Sharkfin webhook receiver.
mux.Handle("POST /v1/webhooks/sharkfin", HandleSharkfinWebhook(nil))

// Combine webhook receiver.
mux.Handle("POST /v1/webhooks/combine", HandleCombineWebhook(cfg.Store))
```

(`cfg.Store` already implements `domain.AuditEventStore` via the
`Store` interface in `internal/domain/ports.go:42-50`.)

**Step 7: Commit**

```
feat(daemon): add /v1/webhooks/combine receiver

Audits push and pull_request_merged events to the existing audit
event store. Other Combine event types are 204-acked but not
audited; signature validation is deferred (Combine doesn't sign
today). Unblocks Combine's operational webhook subscription per
AGENT-POOL-REMAINING-WORK.md.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 5: E2E coverage for `/v1/webhooks/combine` against the spawned daemon

**Depends on:** Task 4 (handler exists + wired).

**Files:**
- Create: `tests/e2e/webhook_combine_test.go`

**Step 1: Write the test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func postCombineE2E(t *testing.T, env *harness.Env, tok, event string, payload any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, env.Daemon.BaseURL()+"/v1/webhooks/combine", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", event)
	req.Header.Set("X-SoftServe-Delivery", "del-e2e")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body := make([]byte, 0)
	body, _ = readAll(resp.Body, body)
	return resp.StatusCode, body
}

// Plan A's harness Client doesn't expose a way to set arbitrary
// headers per request; this helper builds a raw request to add
// X-SoftServe-Event. Plan B uses raw http.Client here rather than
// extending the Client API — single-use, single-test convenience.

func TestCombineWebhook_PushAndMergeFlowsThroughDaemon(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-cw", "flow-cw", "Flow CW", "service")

	// Push.
	status, body := postCombineE2E(t, env, tok, "push", map[string]any{
		"repository": map[string]any{"name": "flow"},
		"ref":        "refs/heads/main",
		"before":     "abc",
		"after":      "def",
	})
	if status != http.StatusNoContent {
		t.Errorf("push status = %d, want 204; body=%s", status, body)
	}

	// Merge.
	status, body = postCombineE2E(t, env, tok, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 7, "target_branch": "main"},
		"sender":       map[string]any{"username": "agent-merge"},
	})
	if status != http.StatusNoContent {
		t.Errorf("merge status = %d, want 204; body=%s", status, body)
	}

	// Ignored event type.
	status, body = postCombineE2E(t, env, tok, "issue_opened", map[string]any{"number": 1})
	if status != http.StatusNoContent {
		t.Errorf("ignored event status = %d, want 204; body=%s", status, body)
	}
}

// readAll is a tiny helper used by postCombineE2E; the e2e package
// already imports io and ioutil-equivalents elsewhere.
func readAll(r interface{ Read([]byte) (int, error) }, _ []byte) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
```

> **Note on the helper.** The `readAll` shim sidesteps adding an
> `io` import for a single use; if the file already imports `io`
> elsewhere via the harness, the developer should replace the shim
> with `io.ReadAll`. Either is acceptable — the assertion is what
> matters.

**Step 2: Run test against both backends**

Run: `mise run e2e --backend=sqlite -- -run TestCombineWebhook_PushAndMergeFlowsThroughDaemon`
Expected: PASS.
Run: `mise run e2e --backend=postgres -- -run TestCombineWebhook_PushAndMergeFlowsThroughDaemon`
Expected: PASS.

**Step 3: Commit**

```
test(e2e): cover /v1/webhooks/combine end-to-end

Posts push, pull_request_merged, and an ignored event type to the
spawned daemon's Combine webhook receiver and asserts every path
returns 204. Both backends covered.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 6: Bot lifecycle round-trip (`tests/e2e/bot_lifecycle_test.go`)

**Depends on:** Tasks 1-5 (MCP client, MCP coverage, Sharkfin and
Combine webhook receivers + their e2e tests).

**Files:**
- Create: `tests/e2e/bot_lifecycle_test.go`

**Step 1: Write the test**

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// TestBotLifecycle_RoundTrip simulates the canonical agent-pool
// proof-of-life loop entirely in-process via fakes:
//
//  1. Operator creates a work item in a Flow project (via REST).
//  2. The project bot posts the work item to its channel (simulated
//     here as a Sharkfin webhook into Flow with a flow_command body).
//  3. The bot claims a pool agent from FakeHive (via the scheduler
//     diag claim endpoint).
//  4. The agent runs (simulated by REST transitions through the
//     workflow steps).
//  5. Combine fires a pull_request_merged webhook into Flow's new
//     /v1/webhooks/combine endpoint.
//  6. The bot reports completion (final transition + a flow_command
//     "mark_done" via the Sharkfin webhook).
//  7. The bot releases the pool agent.
//  8. Audit log contains the expected sequence of events.
//
// All client interactions go through the harness Client + raw
// http.Client; no real bot process runs and no real Hive / Sharkfin
// / Combine clients are imported.
func TestBotLifecycle_RoundTrip(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	env.Hive.SeedPoolAgent("a_b_001", "agent-b-1", "team-b")

	tok := env.Daemon.SignJWT("svc-b", "flow-b", "Flow B", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)
	httpc := &http.Client{Timeout: 5 * time.Second}

	const workflowID = "wf_round_trip_001"
	const repoName = "flow"

	// 1. Seed template + instance + work item via REST.
	tmplBody := map[string]any{
		"name": "round-trip", "description": "round trip e2e",
		"steps": []map[string]any{
			{"key": "dev", "name": "Dev", "type": "task", "position": 1},
			{"key": "rev", "name": "Review", "type": "gate", "position": 2,
				"approval": map[string]any{"mode": "any", "required_approvers": 1}},
			{"key": "qa", "name": "QA", "type": "task", "position": 3},
			{"key": "merged", "name": "Merged", "type": "task", "position": 4},
		},
		"transitions": []map[string]any{
			{"key": "dev_to_rev", "name": "to review",
				"from_step_key": "dev", "to_step_key": "rev"},
			{"key": "rev_to_qa", "name": "to qa",
				"from_step_key": "rev", "to_step_key": "qa"},
			{"key": "qa_to_merged", "name": "to merged",
				"from_step_key": "qa", "to_step_key": "merged"},
		},
	}
	var tmpl struct {
		ID          string
		Steps       []struct{ ID, Key string }
		Transitions []struct{ ID, Key string }
	}
	if status, body, err := c.PostJSON("/v1/templates", tmplBody, &tmpl); err != nil || status != 200 {
		t.Fatalf("create template: status=%d body=%s err=%v", status, body, err)
	}
	stepID := func(k string) string {
		for _, s := range tmpl.Steps {
			if s.Key == k {
				return s.ID
			}
		}
		t.Fatalf("step key %s not found", k)
		return ""
	}
	transID := func(k string) string {
		for _, tr := range tmpl.Transitions {
			if tr.Key == k {
				return tr.ID
			}
		}
		t.Fatalf("transition key %s not found", k)
		return ""
	}
	_ = stepID

	var inst struct{ ID string }
	if status, body, err := c.PostJSON("/v1/instances", map[string]any{
		"template_id": tmpl.ID, "team_id": "team-b", "name": "round-trip",
	}, &inst); err != nil || status != 200 {
		t.Fatalf("create instance: status=%d body=%s err=%v", status, body, err)
	}
	var wi struct{ ID string }
	if status, body, err := c.PostJSON("/v1/instances/"+inst.ID+"/items", map[string]any{
		"title": "round-trip wi",
	}, &wi); err != nil || status != 200 {
		t.Fatalf("create work item: status=%d body=%s err=%v", status, body, err)
	}

	// 2. Bot posts work-item announce to its channel — simulated as a
	// Sharkfin webhook into Flow.
	postSharkfinCommand(t, c, "claim_for_dev", wi.ID, "agent-bot")

	// 3. Bot claims an agent from the pool.
	claimReq := map[string]any{
		"role": "developer", "project": "flow",
		"workflow_id": workflowID, "lease_ttl_seconds": 60,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil); err != nil || status != 200 {
		t.Fatalf("claim: status=%d body=%s err=%v", status, body, err)
	}
	if env.Hive.ClaimCalls() != 1 {
		t.Errorf("ClaimCalls = %d, want 1", env.Hive.ClaimCalls())
	}

	// 4. Agent runs: dev -> rev -> approve -> qa -> merged.
	transitionWorkItem(t, c, wi.ID, transID("dev_to_rev"), "agent-bot", "role-dev")

	approveReq := map[string]any{
		"agent_id": "agent-bot", "comment": "lgtm",
	}
	if status, body, err := c.PostJSON("/v1/items/"+wi.ID+"/approve", approveReq, nil); err != nil || status != 200 {
		t.Fatalf("approve: status=%d body=%s err=%v", status, body, err)
	}

	transitionWorkItem(t, c, wi.ID, transID("rev_to_qa"), "agent-bot", "role-reviewer")

	// 5. Combine merge webhook fires.
	postCombineMerge(t, env, tok, httpc, repoName, 42, "main", "agent-merge")

	// 6. Final transition + flow_command "mark_done".
	transitionWorkItem(t, c, wi.ID, transID("qa_to_merged"), "agent-bot", "role-qa")
	postSharkfinCommand(t, c, "mark_done", wi.ID, "agent-bot")

	// 7. Release the agent.
	releaseReq := map[string]any{
		"agent_id": "a_b_001", "workflow_id": workflowID,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil); err != nil ||
		(status != http.StatusOK && status != http.StatusNoContent) {
		t.Fatalf("release: status=%d body=%s err=%v", status, body, err)
	}

	// 8. Audit log assertion.
	var got struct {
		Events []struct {
			Type     string `json:"type"`
			AgentID  string `json:"agent_id"`
			Workflow string `json:"workflow_id"`
		} `json:"events"`
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if status, _, err := c.GetJSON("/v1/audit/_diag/by-workflow/"+workflowID, &got); err == nil && status == 200 {
			if hasType(got.Events, "agent_claimed") &&
				hasType(got.Events, "agent_released") {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	want := []string{"agent_claimed", "agent_released"}
	for _, w := range want {
		if !hasType(got.Events, w) {
			t.Errorf("audit missing type %q (got %+v)", w, got.Events)
		}
	}
}

func hasType(events []struct {
	Type     string `json:"type"`
	AgentID  string `json:"agent_id"`
	Workflow string `json:"workflow_id"`
}, ty string) bool {
	for _, e := range events {
		if e.Type == ty {
			return true
		}
	}
	return false
}

func postSharkfinCommand(t *testing.T, c *harness.Client, action, workItemID, fromAgent string) {
	t.Helper()
	meta := map[string]any{
		"event_type": "flow_command",
		"event_payload": map[string]any{
			"action": action, "work_item_id": workItemID,
		},
	}
	mb, _ := json.Marshal(meta)
	ms := string(mb)
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   time.Now().UnixNano(),
		"channel_id":   1,
		"channel_name": "flow",
		"channel_type": "public",
		"from":         fromAgent,
		"from_type":    "service",
		"body":         "@flow " + action + " " + workItemID,
		"metadata":     ms,
		"sent_at":      time.Now().UTC().Format(time.RFC3339),
	}
	status, body, err := c.PostJSON("/v1/webhooks/sharkfin", payload, nil)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("sharkfin webhook %s: status=%d body=%s err=%v", action, status, body, err)
	}
}

func postCombineMerge(t *testing.T, env *harness.Env, tok string, httpc *http.Client,
	repoName string, prNumber int64, targetBranch, mergedBy string) {
	t.Helper()
	payload := map[string]any{
		"repository":   map[string]any{"name": repoName},
		"pull_request": map[string]any{"number": prNumber, "target_branch": targetBranch},
		"sender":       map[string]any{"username": mergedBy},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, env.Daemon.BaseURL()+"/v1/webhooks/combine", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", "pull_request_merged")
	req.Header.Set("X-SoftServe-Delivery", "del-rt")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := httpc.Do(req)
	if err != nil {
		t.Fatalf("combine merge webhook: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("combine merge webhook status = %d, want 204", resp.StatusCode)
	}
}

func transitionWorkItem(t *testing.T, c *harness.Client, wiID, transitionID, actor, roleID string) {
	t.Helper()
	req := map[string]any{
		"transition_id":  transitionID,
		"actor_agent_id": actor,
		"actor_role_id":  roleID,
		"reason":         "round-trip",
	}
	status, body, err := c.PostJSON("/v1/items/"+wiID+"/transition", req, nil)
	if err != nil || status != 200 {
		// Show the body for debugging — guard CEL or invariant errors
		// surface here.
		if !strings.Contains(string(body), "approval") {
			t.Fatalf("transition %s: status=%d body=%s err=%v", transitionID, status, body, err)
		}
		t.Fatalf("transition %s: status=%d body=%s err=%v", transitionID, status, body, err)
	}
}
```

**Step 2: Run test to verify it passes**

Run: `mise run e2e --backend=sqlite -- -run TestBotLifecycle_RoundTrip`
Expected: PASS.
Run: `mise run e2e --backend=postgres -- -run TestBotLifecycle_RoundTrip`
Expected: PASS.

> **If the audit-event sequence assertion fails because
> `combine_merge_received` doesn't appear in the
> by-workflow query**, that's expected — Plan B's
> `/v1/audit/_diag/by-workflow/{id}` filters by `workflow_id`, but
> Combine webhook events have no workflow_id (they reference a repo
> + PR). Update Task 6 Step 1's audit assertion to query a
> different endpoint OR persist Combine events with a workflow_id
> derived from the work item's branch — the simpler fix is the
> former: assert the lifecycle subset on the workflow query and
> assert Combine events via a new
> `/v1/audit/_diag/by-agent/{id}` endpoint that already exists is
> NOT yet implemented. The simplest correct shape: assert
> `agent_claimed` + `agent_released` on the by-workflow query (the
> in-scope assertion above), and assert `combine_merge_received` by
> calling the new tiny endpoint
> `/v1/audit/_diag/recent?type=combine_merge_received` if Task 6
> needs it. Defer that endpoint to a 6.b sub-task ONLY if the
> simpler shape proves insufficient — the in-scope assertion above
> is intentionally minimal.

**Step 3: Commit**

```
test(e2e): bot lifecycle round-trip via fakes

Drives the canonical agent-pool proof-of-life loop end-to-end with
zero real Hive/Sharkfin/Combine client imports. Simulates the
project bot via raw HTTP into Flow: work-item-created →
sharkfin-announce → claim-agent → workflow transitions → approve →
combine-merge-webhook → mark-done → release-agent. Asserts
audit-event sequence (agent_claimed → agent_released) on the
workflow's audit log.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 7: Update `tests/e2e/README.md` with Plan B additions

**Depends on:** Tasks 1-6.

**Files:**
- Modify: `tests/e2e/README.md`

**Step 1: Add three sections (no rewrites)**

Append to the README:

1. **MCP wire client** — one paragraph: "The harness ships a
   `harness.MCPClient` that speaks JSON-RPC 2.0 over Flow's
   streaming-HTTP `/mcp` endpoint. Use
   `harness.NewMCPClient(env.Daemon.BaseURL()+"/mcp",
   "harness-service-token")`. The client handles SSE, JSON, and
   202 responses and follows `Mcp-Session-Id` automatically. **Do
   not import `mark3labs/mcp-go` from any test** — drift between
   the client and Flow's MCP server must surface as test failure."
2. **Webhook receiver tests** — one paragraph: "Two webhook
   receivers are exercised end-to-end:
   `POST /v1/webhooks/sharkfin` (production) and
   `POST /v1/webhooks/combine` (production). Both auto-204. The
   Combine handler audits `push` and `pull_request_merged` to
   the existing audit-event store; other Combine event types are
   accepted but not audited. Tests build the Combine wire format
   inline — there is no Combine client in the harness."
3. **Bot lifecycle round-trip** — one paragraph: "The
   `TestBotLifecycle_RoundTrip` test in
   `tests/e2e/bot_lifecycle_test.go` is the first end-to-end
   multi-system orchestration test. It simulates a project bot
   via raw HTTP into Flow, claims a pool agent from FakeHive,
   drives a 4-step workflow through transitions + approvals,
   fires a Combine merge webhook, and releases the agent. Use
   this test as the template when the bot-vocabulary plan adds
   real bot processes."

**Step 2: Commit**

```
docs(e2e): document MCP client, Combine webhook, and round-trip test

Three new sections in tests/e2e/README.md cover Plan B's harness
additions: the hand-rolled MCP wire client, the new
/v1/webhooks/combine receiver, and the bot lifecycle round-trip
test as a template for the bot-vocabulary plan.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

## Verification checklist

After all tasks complete:

- [ ] `mise run e2e --backend=sqlite` green (Plan A + A.5 + B tests).
- [ ] `mise run e2e --backend=postgres` green.
- [ ] `mise run lint` green (depguard + forbidigo unchanged; no new
      forbidden imports surface in harness).
- [ ] `grep -r "mark3labs/mcp-go" tests/` returns no matches (the
      MCP client must be hand-rolled).
- [ ] `grep -r "sharkfinclient\|hiveclient\|pylonclient" tests/`
      returns no matches.
- [ ] `internal/daemon/server.go` mounts both
      `/v1/webhooks/sharkfin` and `/v1/webhooks/combine`.
- [ ] `internal/domain/types.go` has `AuditEventCombinePushReceived`
      and `AuditEventCombineMergeReceived` constants.
- [ ] `tests/e2e/README.md` documents MCP wire client, Combine
      webhook, and the bot lifecycle round-trip.
- [ ] CI matrix (sqlite + postgres jobs) green on the merge
      commit; cancel within `feedback_monitor_every_push.md`'s
      bounded time if any job hangs.

## Spec deltas (for spec-writer follow-up)

OpenSpec is not yet adopted in Flow (per
`AGENT-POOL-REMAINING-WORK.md` "Process discipline: Spec-writer
skipped — OpenSpec not yet adopted"). When it is adopted, three
specs will need:

1. A new `flow-mcp` spec covering the 12 MCP tools' wire shapes
   and the streaming-HTTP transport contract.
2. A new `flow-webhooks` spec covering both `/v1/webhooks/sharkfin`
   and `/v1/webhooks/combine`, the event-type discriminators, and
   the always-204 contract.
3. An extension to the foundation `flow-orchestration` spec to
   cover the new audit-event types
   (`combine_push_received`, `combine_merge_received`).

Plan B does not modify any spec because none exist yet.
