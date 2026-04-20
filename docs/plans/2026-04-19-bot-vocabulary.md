---
type: plan
step: "bot-vocabulary"
title: "Flow bot vocabulary — projects, project-bots, per-template event vocab"
status: pending
assessment_status: needed
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "bot-vocabulary"
dates:
  created: "2026-04-19"
  approved: null
  completed: null
related_plans:
  - "2026-04-18-flow-orchestration-01-foundation.md"
  - "2026-04-19-flow-plan-a5-auth-rest.md"
  - "2026-04-19-flow-plan-b-mcp-webhook.md"
  - "2026-04-19-nexus-runtime-driver.md"
---

# Flow Bot Vocabulary — Projects, Project-Bots, Per-Template Event Vocab

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

## Overview

Flow currently has workflow templates, instances, work items, the
scheduler (claim/release a Hive agent for a workflow), the audit log,
the `ChatProvider` port (with the Sharkfin REST adapter), and a tiny
hook system that posts a templated string to a configured channel
when a transition fires (`internal/workflow/hooks.go:24`). What it
does NOT yet have:

1. A **project** as a first-class entity. Today "project" appears
   only as a string field on `AgentClaim` and `AuditEvent` — there is
   no `projects` table, no project ID, no binding to a Sharkfin
   channel, no binding to a workflow template.
2. A **bot** as a first-class entity. Today the `flow-bot` /
   `nexus-bot` / etc. naming is purely operational sketch in
   `docs/2026-04-18-orchestration-impl.md:33`; no schema, no Passport
   key storage, no Hive-role binding.
3. A **vocabulary** mechanism on the workflow template. The existing
   `IntegrationHook` row is a per-transition chat post with a single
   Go-text/template body — not a discriminator over canonical
   workflow events, no message-type catalogue, no per-template
   declaration of "this template uses the SDLC vocabulary".

This plan adds those three primitives and the surface needed for an
operator (or, after the Flow UI plan ships, the Scope+Playwright
bootstrap drive) to:

- Create a project (name, description, template ref, channel
  binding, Passport API key).
- Bind a bot identity to it (1:1).
- Have the bot post template-vocabulary-rendered messages to the
  project's channel on every workflow event the template's
  vocabulary recognises.
- Have the bot claim/release Hive pool agents on the canonical
  `task_assigned` / `task_completed` events.
- Filter the audit log by project for the per-project audit view.

Vocabulary is a **property of the workflow template**, not
permanently SDLC: a custom template (e.g. a bug-tracker workflow)
declares its own event-types + message templates. The plan demonstrates
this by seeding two reference vocabularies — the SDLC one and a
bug-tracker one — and including round-trip e2e coverage of both.

The bot↔Sharkfin protocol uses the existing
`domain.ChatProvider` port at
`internal/domain/ports.go:54` unchanged. Slack readiness is preserved
via the same port + a future SlackProvider impl; no chat-platform-
specific code lives in the new bot or vocabulary domain types.

For Passport identity: the bot's API key is created externally for v1
(per `AGENT-POOL-REMAINING-WORK.md` "Bot bootstrap — manual via
Scope+Playwright") and stored on the `bots` row. Flow uses that key
as the Passport credential when its bot makes outbound calls (today
that means Sharkfin via the existing `ChatProvider` adapter; later
Combine and Hive will call the same path).

## Prerequisites

Before starting:
- [ ] Foundation plan landed (`status: complete`) — verified by
      `internal/scheduler/`, `internal/domain/scheduler.go`,
      `internal/domain/types.go` `AuditEvent*` constants existing.
- [ ] Plan A.5 landed (`status: complete`) — verified by REST coverage
      tests existing under `tests/e2e/`.
- [ ] Plan B landed (`status: complete`) — verified by
      `tests/e2e/bot_lifecycle_test.go`,
      `internal/daemon/webhook_combine.go`,
      `internal/daemon/mcp_tools.go` (12 tools) existing.
- [ ] Nexus driver impl plan landed — `internal/infra/runtime/nexus/`
      exists. Not a strict dependency for this plan's code, but the
      bot vocabulary's `task_assigned` flow drives it via the
      `RuntimeDriver` port.
- [ ] `mise run e2e --backend=sqlite` and
      `mise run e2e --backend=postgres` both green on master tip.
- [ ] Local Postgres reachable at
      `postgres://postgres@127.0.0.1/flow_test?sslmode=disable`
      (peer-trust auth as `postgres` user).

## Hard constraints (non-negotiable)

- **Vocabulary is per-workflow-template**, not permanently SDLC.
  Hard-coding SDLC event-type names into Flow Go code outside the
  seed file is a plan failure. The single exception is the defensive
  `"task_completed"` literal in the scheduler's release-dispatch
  fallback (Task 9), which is reachable only when a vocabulary
  forgets to declare `release_event` and is documented in-code with
  a "do not extend" comment.
- **Bot model**: 1 project = 1 bot = 1 Sharkfin channel. Schema and
  REST contracts enforce this with UNIQUE constraints + 409 conflict
  responses.
- **`ChatProvider` port unchanged.** Slack future-readiness depends
  on this. The plan does not modify
  `internal/domain/ports.go:54-64` and adds no chat-platform-specific
  fields anywhere in `internal/domain/`.
- **Hexagonal**: vocabulary + bot + project domain types live in
  `internal/domain/`, no infra leakage. The vocabulary renderer takes
  domain types in and returns a rendered string + a metadata
  `json.RawMessage`; nothing imports `sharkfin/client/go` from
  `internal/domain/`. The bot dispatcher is consumed by other
  packages (notably `internal/scheduler/`) **only through the
  `domain.BotDispatcher` port** — the concrete `*bot.Dispatcher`
  type never appears in another package's public surface. Same
  pattern as the existing `ChatProvider` / `IdentityProvider` ports.
- **Dual-backend e2e** per `feedback_e2e_dual_backend.md`. Every new
  store method has tests that run under both `--backend=sqlite` and
  `--backend=postgres`; no silent skips.
- **E2E harness independence** per
  `feedback_e2e_harness_independence.md`. The harness extension speaks
  raw HTTP to Flow's REST + raw JSON-RPC 2.0 to Flow's MCP. No new
  client-library imports.
- **No `!` markers**, **no `BREAKING CHANGE:` footers** (pre-1.0).
- **Sample commits**: multi-line conventional, HEREDOC, body +
  `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer.
- **No `t.Parallel()`** in e2e tests (daemon spawn cost ~200 ms).
- **No new runtime dependencies.** The vocabulary template renderer
  uses stdlib `text/template` (already in use at
  `internal/workflow/hooks.go:9`).
- **Project field on AgentClaim** stays a free-form string for v1.
  This plan adds a `projects` table but does NOT enforce that
  `AgentClaim.Project` matches a project ID — that coupling is a
  separate, smaller follow-up after the first dogfood projects exist.

## Tech stack

Unchanged from Plans A / A.5 / B / Nexus driver: Go 1.26, `net/http`,
`net/http/httptest`, `encoding/json`, `database/sql`, `text/template`,
`github.com/google/uuid`, `github.com/danielgtaylor/huma/v2`,
`github.com/mark3labs/mcp-go`, `modernc.org/sqlite`,
`github.com/jackc/pgx/v5/stdlib`, `github.com/pressly/goose/v3`.
**No new dependencies.**

## Vocabulary model (load-bearing)

A **vocabulary** is a named set of `EventType`s + per-event message
templates. It is referenced by ID from a workflow template. It is
declared once and reused across many templates.

```go
// internal/domain/vocabulary.go

// Vocabulary is a named catalogue of bot-event types and the
// human-readable message template each one renders to. It is the
// per-workflow-template language the project bot speaks in its
// Sharkfin channel.
//
// A vocabulary is independent of the workflow template — many
// templates can reference the same vocabulary, and a template can
// switch vocabularies without losing instance history. The
// vocabulary itself does not encode any workflow semantics; it is a
// flat map from event-name → message template.
type Vocabulary struct {
    ID          string
    Name        string                  // "sdlc", "bug-tracker", "release-train", ...
    Description string
    Events      []VocabularyEvent
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// VocabularyEvent is one entry in the vocabulary's event catalogue.
// MessageTemplate is rendered with text/template against
// VocabularyContext. MetadataKeys lists the keys that, when present
// in VocabularyContext.Payload, are copied verbatim into the chat
// message's metadata sidecar.
type VocabularyEvent struct {
    ID              string
    VocabularyID    string
    EventType       string  // "task_assigned", "branch_created", "bug_filed", ...
    MessageTemplate string  // "Task assigned: {{.WorkItem.Title}} → {{.AgentName}}"
    MetadataKeys    []string
}
```

The seed catalogue carries the canonical SDLC vocabulary and one
bug-tracker example. The two are loaded at daemon startup from
`docs/examples/vocabularies/{sdlc,bug-tracker}.json` via the existing
`cmd/admin/admin.go` seed pattern (Task 12).

A workflow template gains an optional `vocabulary_id` field
(`projects` references the same field via project; templates may
override). A project ALWAYS has a vocabulary (NOT NULL on the
`projects` row, defaulting to the SDLC seed when no override is
supplied at create time).

### Render contract

```go
// internal/domain/vocabulary.go (continued)

// VocabularyContext is the data the vocabulary's MessageTemplate is
// rendered against. Fields are pointer-typed so missing data renders
// to "<no value>" instead of crashing — matching text/template
// default behaviour.
type VocabularyContext struct {
    Project   *Project
    WorkItem  *WorkItem
    AgentName string
    Role      string
    Payload   map[string]any
}

// RenderEvent renders v.Events[k].MessageTemplate against ctx and
// returns the rendered string + a metadata sidecar containing
// MetadataKeys+core context fields. Returns ErrEventNotInVocabulary
// when eventType is not in v.Events, never panics.
func (v *Vocabulary) RenderEvent(eventType string, ctx VocabularyContext) (string, json.RawMessage, error)
```

The renderer is the entire vocabulary semantic surface. There is no
LLM, no DSL — it is `text/template.Execute` over the event's template
string + a `json.Marshal` of the metadata sidecar. The bot dispatcher
calls `RenderEvent` and feeds the result to
`ChatProvider.PostMessage`.

### What canonical SDLC events look like

Seeded from `docs/examples/vocabularies/sdlc.json`:

| event_type | when fired | metadata_keys |
|---|---|---|
| `task_assigned` | bot claims an agent for a work item | `agent_id`, `role`, `workflow_id` |
| `branch_created` | combine push webhook for a new branch | `branch`, `commit_sha` |
| `commit_landed` | combine push webhook against a known branch | `branch`, `commit_sha`, `author` |
| `pr_opened` | combine pull_request webhook (event=opened) | `pr_number`, `branch` |
| `review_requested` | flow transition into a gate step typed "review" | `agent_id`, `gate_step_id` |
| `tests_passing` | external integration push event with status "ok" | `repo`, `run_id` |
| `merged` | combine pull_request_merged webhook | `pr_number`, `merged_by`, `target_branch` |
| `deployed` | flow transition into a step keyed "deployed" | `commit_sha`, `env` |
| `task_completed` | scheduler release fires the vocab's `release_event` | `agent_id`, `role`, `workflow_id` |

A custom bug-tracker vocabulary
(`docs/examples/vocabularies/bug-tracker.json`) ships alongside it
covering the **full lifecycle** the design briefing calls out — file,
triage, assign, fix-proposed, verify, close, plus reopen — to prove
the per-template plug-in path. Concretely the seed declares
`bug_filed`, `bug_triaged`, `bug_assigned`, `bug_fix_proposed`,
`bug_verified`, `bug_closed`, `bug_reopened` (release_event:
`bug_closed`). Task 13 drives at least the
`bug_filed → bug_fix_proposed → bug_verified → bug_closed` segment
end-to-end.

## Bot model

```go
// internal/domain/project.go

type Project struct {
    ID          string
    Name        string  // unique
    Description string
    TemplateID  string  // optional, "" allowed
    ChannelName string  // Sharkfin channel name (e.g. "#flow")
    VocabularyID string // never empty — defaults to SDLC seed at create
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// internal/domain/bot.go

type Bot struct {
    ID                  string
    ProjectID           string  // unique — enforces 1 project = 1 bot
    PassportAPIKeyHash  string  // SHA-256 hex of the wf-svc_* key
    PassportAPIKeyID    string  // the key's identifier (returned by Passport)
    HiveRoleAssignments []string // role IDs the bot is permitted to claim for
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

The plain-text Passport API key is **never persisted**. The store
keeps the SHA-256 hash + the Passport key ID; the operator (or, after
the Flow UI plan, the auto-mint flow) is responsible for safekeeping
the plaintext at create time. When Flow needs to make an outbound
call as the bot, it loads the key out of an env-injected
`FLOW_BOT_KEYS_DIR/<bot_id>` file. Plan-failure mode: the bot-uses-
service-token shortcut. The plan rejects that shortcut at Task 7's
review.

Hive role assignments are stored as a JSON array of role IDs (TEXT
column with JSON contents — Flow does not validate against Hive at
write time; Hive's `ResolveRole` is the source of truth and would
return `ErrNotFound` if the bot tried to claim with an invalid role).

## REST surface (additions)

| Verb + Path | Purpose | Output |
|---|---|---|
| `POST /v1/projects` | create project | `{id, name, channel_name, template_id, vocabulary_id}` |
| `GET /v1/projects` | list projects | `[...]` |
| `GET /v1/projects/{id}` | get one project | full project row |
| `PATCH /v1/projects/{id}` | update name/description/template/vocab | full project row |
| `DELETE /v1/projects/{id}` | delete (409 if bot still attached) | 204 |
| `POST /v1/projects/{id}/bot` | create + bind bot identity (one-shot) | `{id, project_id, passport_api_key_id, hive_role_assignments}` |
| `GET /v1/projects/{id}/bot` | get the bot for the project | bot row (no key material) |
| `DELETE /v1/projects/{id}/bot` | unbind + soft-delete bot | 204 |
| `GET /v1/projects/{id}/audit` | audit events filtered to this project | `{events: [...]}` |
| `GET /v1/vocabularies` | list vocabularies | `[...]` |
| `GET /v1/vocabularies/{id}` | get one vocabulary including events | full vocab row |

The `POST /v1/projects/{id}/bot` body carries the operator-supplied
plaintext Passport API key one-time. The handler hashes it, stores
the hash + Passport-supplied key ID, and writes the plaintext to
`FLOW_BOT_KEYS_DIR/<bot_id>` (mode 0600). The plaintext **never**
appears in the response body. The corresponding RFC 6750
`Authorization: ApiKey-v1 <key>` is what the bot will use for
outbound calls; Passport-side validation is unchanged.

## MCP surface (additions)

Plan B's 12 MCP tools cover work-item / instance / template
operations. The bot vocabulary plan adds **3 more** the adjutant
needs to know "what project am I working in?" / "what work item?":

| Tool | Backing call |
|---|---|
| `get_my_project` | inputs: `agent_id`. Resolves via `ListAuditEventsByAgent` for the latest non-released claim row, then `Store.GetProject(claim.Project)`. Output: the Project row + its bound vocabulary. |
| `list_my_work_items` | inputs: `agent_id`. Returns all work items currently assigned to the agent across instances. |
| `get_vocabulary` | inputs: `id` (vocab id). Returns the vocab + events. Useful when the agent wants to know what messages the bot will emit on its behalf. |

The plan does NOT add tools for project/bot CRUD via MCP — those are
operator-driven via the REST surface (and, later, the Flow UI). MCP
stays oriented around adjutants reading their own context.

## Bot ↔ Hive claim/release flow

When a workflow event is registered as `task_assigned` in the
project's vocabulary AND the corresponding hook is fired, the bot
dispatcher:

1. Calls `Scheduler.AcquireAgent(ctx, role, project.Name, workflowID, leaseTTL)` —
   the existing primitive from
   `internal/domain/scheduler.go:30`.
2. Renders `task_assigned` via the project's vocabulary with the
   acquired claim's agent name as `VocabularyContext.AgentName`.
3. Posts to the project's Sharkfin channel via `ChatProvider.PostMessage`.
4. The lease renewer (already running) keeps the claim alive.

When a `task_completed` event fires (vocab name is fully under
template control; Flow keys off the vocabulary's `release_event`
declaration — see seed file for `sdlc.json` `"release_event":
"task_completed"`):

1. Bot dispatcher renders + posts the completion message.
2. Bot dispatcher calls `Scheduler.ReleaseAgent(ctx, claim)` —
   existing primitive.
3. Audit row `agent_released` lands automatically (existing
   scheduler behaviour).

The vocabulary file's `release_event` field tells Flow which event
ends the claim. SDLC: `task_completed`. Bug-tracker:
`bug_resolved`. Plan-failure mode: hard-coding `task_completed` as
the release trigger anywhere in Go.

## Bot ↔ Sharkfin protocol

Two paths:

1. **Outbound (bot speaks):** `BotDispatcher.Dispatch(ctx,
   project.ID, eventType, payload)` →
   `Vocabulary.RenderEvent(eventType, ctx)` →
   `chatProvider.PostMessage(project.ChannelName, content, metadata)`.
2. **Inbound (bot listens):** the existing
   `internal/daemon/webhook_sharkfin.go` handler is extended (Task
   10) to look up the project bot whose `ChannelName` matches the
   incoming `payload.Channel`, then route the message to the bot's
   workflow event handler. For v1 the inbound handler only logs +
   audits (no transition triggered) — bidirectional command dispatch
   is deferred per Plan B's existing scoping.

`ChatProvider` is unchanged. The `metadata` sidecar carries the
event-type discriminator + the per-event MetadataKeys, so a future
SlackProvider can re-render or post the same metadata in Slack-flavoured
attachments without Flow knowing.

## E2E coverage extension

Plan B's `tests/e2e/bot_lifecycle_test.go` already simulates the
canonical loop with hard-coded behaviours. This plan **extends it**
by adding a sibling test
`tests/e2e/bot_vocabulary_test.go` that drives the same loop **through
the new project + bot + vocabulary surface** and asserts:

- Each canonical SDLC event the loop fires renders the expected
  rendered text + metadata sidecar via the project's vocabulary
  (`FakeSharkfin.Messages()` returns `[]SharkfinMessage{Channel,
  Body, Metadata}` — assert against `Body` for the rendered string
  and `Metadata["event_type"]` for the discriminator).
- The bug-tracker vocabulary plug-in path works: a second project
  bound to the bug-tracker vocab drives the full
  `bug_filed → bug_fix_proposed → bug_verified → bug_closed` cycle
  and the harness asserts the recorded `Body` strings match the
  bug-tracker templates and never the SDLC text.
- The audit log filtered by project (new
  `GET /v1/projects/{id}/audit`) returns ONLY events scoped to that
  project — no cross-project leakage.

## Task breakdown

Tasks are sized for one sitting each. Task numbers indicate
dependency order; the first 6 tasks lay the schema + domain + REST
surface, the next 4 wire the dispatcher and bot↔Hive flow, the last
3 add MCP + seeds + e2e. Each task ends with a commit.

---

### Task 1: Domain types — Project, Bot, Vocabulary

**Why first.** Every other task imports these types. They must compile
on their own (no infra references) before the store interfaces in
Task 2 can declare port methods over them.

**Files:**
- Create: `internal/domain/project.go`
- Create: `internal/domain/bot.go`
- Create: `internal/domain/vocabulary.go`
- Modify: `internal/domain/errors.go` — add `ErrEventNotInVocabulary`,
  `ErrProjectHasBot` (block delete-with-bot), `ErrBotKeyMissing`.

**Step 1: Write the failing test**

```go
// internal/domain/vocabulary_test.go
// SPDX-License-Identifier: GPL-2.0-only
package domain_test

import (
    "encoding/json"
    "errors"
    "strings"
    "testing"

    "github.com/Work-Fort/Flow/internal/domain"
)

func TestVocabulary_RenderEvent_SDLCTaskAssigned(t *testing.T) {
    v := &domain.Vocabulary{
        ID:   "voc_sdlc",
        Name: "sdlc",
        Events: []domain.VocabularyEvent{{
            ID:              "ve_task_assigned",
            VocabularyID:    "voc_sdlc",
            EventType:       "task_assigned",
            MessageTemplate: "Task assigned: {{.WorkItem.Title}} → {{.AgentName}}",
            MetadataKeys:    []string{"agent_id", "role"},
        }},
    }
    text, meta, err := v.RenderEvent("task_assigned", domain.VocabularyContext{
        WorkItem:  &domain.WorkItem{Title: "Refactor parser"},
        AgentName: "agent-7",
        Payload:   map[string]any{"agent_id": "a_7", "role": "developer", "ignored": "x"},
    })
    if err != nil {
        t.Fatalf("RenderEvent: %v", err)
    }
    want := "Task assigned: Refactor parser → agent-7"
    if text != want {
        t.Errorf("text = %q, want %q", text, want)
    }
    var m map[string]any
    if err := json.Unmarshal(meta, &m); err != nil {
        t.Fatalf("metadata not JSON: %v", err)
    }
    if m["event_type"] != "task_assigned" || m["agent_id"] != "a_7" || m["role"] != "developer" {
        t.Errorf("metadata = %v", m)
    }
    if _, leaked := m["ignored"]; leaked {
        t.Errorf("metadata leaked unauthorised payload key 'ignored': %v", m)
    }
}

func TestVocabulary_RenderEvent_UnknownEventReturnsErr(t *testing.T) {
    v := &domain.Vocabulary{ID: "voc_x", Events: nil}
    _, _, err := v.RenderEvent("nope", domain.VocabularyContext{})
    if !errors.Is(err, domain.ErrEventNotInVocabulary) {
        t.Errorf("err = %v, want ErrEventNotInVocabulary", err)
    }
}

func TestVocabulary_RenderEvent_NoCrashOnNilFields(t *testing.T) {
    v := &domain.Vocabulary{
        ID: "voc_x",
        Events: []domain.VocabularyEvent{{
            EventType:       "x",
            MessageTemplate: "title={{.WorkItem.Title}} agent={{.AgentName}}",
        }},
    }
    text, _, err := v.RenderEvent("x", domain.VocabularyContext{})
    if err != nil {
        t.Fatalf("RenderEvent: %v", err)
    }
    if !strings.Contains(text, "<no value>") {
        t.Errorf("expected text/template <no value> sentinel, got %q", text)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestVocabulary ./internal/domain/...`
Expected: FAIL with `undefined: domain.Vocabulary` and friends.

**Step 3: Write minimal implementation**

```go
// internal/domain/project.go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import "time"

type Project struct {
    ID           string    `json:"id"`
    Name         string    `json:"name"`
    Description  string    `json:"description,omitempty"`
    TemplateID   string    `json:"template_id,omitempty"`
    ChannelName  string    `json:"channel_name"`
    VocabularyID string    `json:"vocabulary_id"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

```go
// internal/domain/bot.go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
    "context"
    "time"
)

type Bot struct {
    ID                  string    `json:"id"`
    ProjectID           string    `json:"project_id"`
    PassportAPIKeyHash  string    `json:"-"`
    PassportAPIKeyID    string    `json:"passport_api_key_id"`
    HiveRoleAssignments []string  `json:"hive_role_assignments"`
    CreatedAt           time.Time `json:"created_at"`
    UpdatedAt           time.Time `json:"updated_at"`
}

// BotDispatcher is the inward-facing port the scheduler + webhook
// handlers call to send vocabulary-rendered messages on a project's
// behalf. Implementations live in internal/bot/. Keeping the port in
// domain follows the same pattern as ChatProvider/IdentityProvider —
// no inward package imports the concrete implementation.
type BotDispatcher interface {
    Dispatch(ctx context.Context, projectID, eventType string, ctxData VocabularyContext) error
}
```

```go
// internal/domain/vocabulary.go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
    "bytes"
    "encoding/json"
    "fmt"
    "text/template"
    "time"
)

type Vocabulary struct {
    ID            string             `json:"id"`
    Name          string             `json:"name"`
    Description   string             `json:"description,omitempty"`
    ReleaseEvent  string             `json:"release_event,omitempty"`
    Events        []VocabularyEvent  `json:"events"`
    CreatedAt     time.Time          `json:"created_at"`
    UpdatedAt     time.Time          `json:"updated_at"`
}

type VocabularyEvent struct {
    ID              string   `json:"id"`
    VocabularyID    string   `json:"vocabulary_id"`
    EventType       string   `json:"event_type"`
    MessageTemplate string   `json:"message_template"`
    MetadataKeys    []string `json:"metadata_keys,omitempty"`
}

type VocabularyContext struct {
    Project   *Project
    WorkItem  *WorkItem
    AgentName string
    Role      string
    Payload   map[string]any
}

func (v *Vocabulary) RenderEvent(eventType string, ctx VocabularyContext) (string, json.RawMessage, error) {
    var ev *VocabularyEvent
    for i := range v.Events {
        if v.Events[i].EventType == eventType {
            ev = &v.Events[i]
            break
        }
    }
    if ev == nil {
        return "", nil, fmt.Errorf("%w: %q in vocabulary %q", ErrEventNotInVocabulary, eventType, v.Name)
    }
    t, err := template.New("event").Parse(ev.MessageTemplate)
    if err != nil {
        return "", nil, fmt.Errorf("parse vocabulary template %q: %w", eventType, err)
    }
    var buf bytes.Buffer
    if err := t.Execute(&buf, ctx); err != nil {
        return "", nil, fmt.Errorf("render vocabulary template %q: %w", eventType, err)
    }

    meta := map[string]any{"event_type": eventType}
    for _, k := range ev.MetadataKeys {
        if val, ok := ctx.Payload[k]; ok {
            meta[k] = val
        }
    }
    metaJSON, err := json.Marshal(meta)
    if err != nil {
        return "", nil, fmt.Errorf("marshal vocabulary metadata: %w", err)
    }
    return buf.String(), metaJSON, nil
}
```

```go
// internal/domain/errors.go (additions only — keep existing sentinels)
var (
    ErrEventNotInVocabulary = errors.New("event not in vocabulary")
    ErrProjectHasBot        = errors.New("project still has a bound bot")
    ErrBotKeyMissing        = errors.New("bot Passport key file missing")
)
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestVocabulary ./internal/domain/...`
Expected: PASS.

**Step 5: Commit**

```
feat(domain): add Project, Bot, Vocabulary types

Add Project, Bot, and Vocabulary domain types plus the
RenderEvent renderer that drives bot ChatProvider posts.
The renderer uses text/template for body and emits a metadata
sidecar keyed by the event's MetadataKeys allow-list, so
arbitrary payload fields cannot leak into chat metadata.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 2: Port interfaces — ProjectStore, BotStore, VocabularyStore

**Depends on:** Task 1 (entity types).

**Files:**
- Modify: `internal/domain/ports.go` — add four new store interfaces
  (Project, Bot, Vocabulary, plus a `WorkItemStore.ListWorkItemsByAgent`
  method needed by Task 11's `list_my_work_items` MCP tool) and embed
  the three new aggregator interfaces in the existing `Store`
  interface (current line 42-50).
- Modify: `internal/infra/sqlite/store.go` — add
  `var _ domain.Store = (*Store)(nil)` at the bottom of the file
  (idiomatic compile-time interface check; replaces the runtime test
  pattern).
- Modify: `internal/infra/postgres/store.go` — same
  `var _ domain.Store = (*Store)(nil)` assertion.

The compile-time assertion in each store file is the entire
correctness check: when Tasks 3 + 4 forget to implement a method on
either backend, the build breaks immediately, which is faster and
clearer than a separate test that builds an empty `var s domain.Store
= nil`. No new `_test.go` file is added for this task.

**Step 1: Write the failing assertion**

Append to `internal/infra/sqlite/store.go` (concrete `*Store` is
declared at line 19 in the existing file):

```go
// Compile-time check: *Store satisfies the full domain.Store
// aggregator. Updating the aggregator (Task 2) without implementing
// the new methods (Tasks 3-4) breaks the build here.
var _ domain.Store = (*Store)(nil)
```

(Same line in `internal/infra/postgres/store.go`.)

**Step 2: Run build to verify it fails**

Run: `go build ./...`
Expected: build error: `*Store does not implement domain.Store
(missing GetProject method)` — and likewise for `GetBotByProject`,
`GetVocabulary`, `ListWorkItemsByAgent`,
`ListAuditEventsByProject`.

**Step 3: Write minimal implementation**

```go
// internal/domain/ports.go (additions)

type ProjectStore interface {
    CreateProject(ctx context.Context, p *Project) error
    GetProject(ctx context.Context, id string) (*Project, error)
    GetProjectByName(ctx context.Context, name string) (*Project, error)
    ListProjects(ctx context.Context) ([]*Project, error)
    UpdateProject(ctx context.Context, p *Project) error
    DeleteProject(ctx context.Context, id string) error
}

type BotStore interface {
    CreateBot(ctx context.Context, b *Bot) error
    GetBotByProject(ctx context.Context, projectID string) (*Bot, error)
    DeleteBotByProject(ctx context.Context, projectID string) error
}

type VocabularyStore interface {
    CreateVocabulary(ctx context.Context, v *Vocabulary) error
    GetVocabulary(ctx context.Context, id string) (*Vocabulary, error)
    GetVocabularyByName(ctx context.Context, name string) (*Vocabulary, error)
    ListVocabularies(ctx context.Context) ([]*Vocabulary, error)
}

// Updated aggregator:
type Store interface {
    TemplateStore
    InstanceStore
    WorkItemStore
    ApprovalStore
    AuditEventStore
    ProjectStore
    BotStore
    VocabularyStore
    Ping(ctx context.Context) error
    io.Closer
}

// WorkItemStore extension — needed by Task 11's list_my_work_items
// MCP tool. The pre-existing ListWorkItems is keyed by instanceID,
// which forces a list-instances-then-iterate scan from the MCP tool;
// adding a direct by-agent query keeps the tool tractable at any
// instance count. Indexed in Tasks 3 + 4.
type WorkItemStore interface {
    CreateWorkItem(ctx context.Context, w *WorkItem) error
    GetWorkItem(ctx context.Context, id string) (*WorkItem, error)
    ListWorkItems(ctx context.Context, instanceID, stepID, agentID string, priority Priority) ([]*WorkItem, error)
    ListWorkItemsByAgent(ctx context.Context, agentID string) ([]*WorkItem, error)
    UpdateWorkItem(ctx context.Context, w *WorkItem) error
    RecordTransition(ctx context.Context, h *TransitionHistory) error
    GetTransitionHistory(ctx context.Context, workItemID string) ([]*TransitionHistory, error)
}

// Add a new audit-by-project query to AuditEventStore (project filter
// is the new GET /v1/projects/{id}/audit's backing call).
type AuditEventStore interface {
    RecordAuditEvent(ctx context.Context, e *AuditEvent) error
    ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*AuditEvent, error)
    ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*AuditEvent, error)
    ListAuditEventsByProject(ctx context.Context, project string) ([]*AuditEvent, error)
}
```

**Step 4: Run build to verify it passes**

Run: `go build ./...`
Expected: build succeeds (the `var _ domain.Store = (*Store)(nil)`
assertions in both backend `store.go` files compile because Tasks 3 +
4 implement every new method).

**Step 5: Commit**

```
feat(domain): add Project/Bot/Vocabulary store ports

Extend the Store aggregator with ProjectStore, BotStore, and
VocabularyStore. Add ListAuditEventsByProject so the per-project
audit view backing the /v1/projects/{id}/audit endpoint can be
implemented without scanning all events.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 3: SQLite migration + store impl

**Depends on:** Task 2 (port interfaces).

**Files:**
- Create: `internal/infra/sqlite/migrations/003_projects_bots_vocab.sql`
- Create: `internal/infra/sqlite/projects.go`
- Create: `internal/infra/sqlite/bots.go`
- Create: `internal/infra/sqlite/vocabularies.go`
- Modify: `internal/infra/sqlite/audit.go` — add
  `ListAuditEventsByProject`.
- Test: `internal/infra/sqlite/store_test.go` — add coverage for each
  new method.

**Step 1: Write the failing test**

Add to `internal/infra/sqlite/store_test.go`:

```go
func TestStore_ProjectCRUD(t *testing.T) {
    store := openTestStore(t)
    ctx := context.Background()

    voc := &domain.Vocabulary{
        ID: "voc_t1", Name: "t1",
        Events: []domain.VocabularyEvent{
            {ID: "ve_1", VocabularyID: "voc_t1", EventType: "task_assigned", MessageTemplate: "x"},
        },
    }
    if err := store.CreateVocabulary(ctx, voc); err != nil {
        t.Fatalf("CreateVocabulary: %v", err)
    }

    p := &domain.Project{
        ID: "prj_t1", Name: "flow", ChannelName: "#flow", VocabularyID: voc.ID,
    }
    if err := store.CreateProject(ctx, p); err != nil {
        t.Fatalf("CreateProject: %v", err)
    }

    got, err := store.GetProject(ctx, p.ID)
    if err != nil || got.Name != "flow" {
        t.Fatalf("GetProject: got=%v err=%v", got, err)
    }

    byName, err := store.GetProjectByName(ctx, "flow")
    if err != nil || byName.ID != p.ID {
        t.Fatalf("GetProjectByName: got=%v err=%v", byName, err)
    }

    // Duplicate name is rejected.
    if err := store.CreateProject(ctx, &domain.Project{
        ID: "prj_t2", Name: "flow", ChannelName: "#flow2", VocabularyID: voc.ID,
    }); !errors.Is(err, domain.ErrAlreadyExists) {
        t.Errorf("expected ErrAlreadyExists for duplicate name, got %v", err)
    }
}

func TestStore_BotCRUD(t *testing.T) {
    store := openTestStore(t)
    ctx := context.Background()
    voc := &domain.Vocabulary{ID: "voc_b1", Name: "b1"}
    _ = store.CreateVocabulary(ctx, voc)
    _ = store.CreateProject(ctx, &domain.Project{
        ID: "prj_b1", Name: "b1", ChannelName: "#b1", VocabularyID: voc.ID,
    })
    b := &domain.Bot{
        ID: "bot_b1", ProjectID: "prj_b1",
        PassportAPIKeyHash: "deadbeef", PassportAPIKeyID: "pak_1",
        HiveRoleAssignments: []string{"developer", "reviewer"},
    }
    if err := store.CreateBot(ctx, b); err != nil {
        t.Fatalf("CreateBot: %v", err)
    }
    if err := store.CreateBot(ctx, &domain.Bot{
        ID: "bot_b2", ProjectID: "prj_b1", PassportAPIKeyHash: "x", PassportAPIKeyID: "pak_2",
    }); !errors.Is(err, domain.ErrAlreadyExists) {
        t.Errorf("expected one-bot-per-project conflict, got %v", err)
    }
    got, err := store.GetBotByProject(ctx, "prj_b1")
    if err != nil || got.ID != "bot_b1" || len(got.HiveRoleAssignments) != 2 {
        t.Fatalf("GetBotByProject: %v %v", got, err)
    }
}

func TestStore_AuditByProject(t *testing.T) {
    store := openTestStore(t)
    ctx := context.Background()
    _ = store.RecordAuditEvent(ctx, &domain.AuditEvent{
        Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wf1", Project: "p1",
    })
    _ = store.RecordAuditEvent(ctx, &domain.AuditEvent{
        Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wf2", Project: "p2",
    })
    got, err := store.ListAuditEventsByProject(ctx, "p1")
    if err != nil || len(got) != 1 || got[0].AgentID != "a1" {
        t.Fatalf("ListAuditEventsByProject: %v err=%v", got, err)
    }
}

func TestStore_ListWorkItemsByAgent(t *testing.T) {
    store := openTestStore(t)
    ctx := context.Background()
    // Seed via the existing helpers — same shape as the
    // pre-existing TestStore_WorkItemCRUD setup.
    seedTwoInstancesOneAgent(t, store, "agent-x")
    items, err := store.ListWorkItemsByAgent(ctx, "agent-x")
    if err != nil {
        t.Fatalf("ListWorkItemsByAgent: %v", err)
    }
    if len(items) != 2 {
        t.Errorf("expected 2 items across 2 instances, got %d", len(items))
    }
    for _, it := range items {
        if it.AssignedAgentID != "agent-x" {
            t.Errorf("item %s assigned to %s, want agent-x", it.ID, it.AssignedAgentID)
        }
    }
}
```

(`seedTwoInstancesOneAgent` is a small test-helper that creates two
templates+instances and one work item per instance, both assigned to
`agent-x`. Add it to `store_test.go` alongside the test.)

**Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/sqlite/...`
Expected: FAIL — compile errors on the new methods.

**Step 3: Write minimal implementation**

```sql
-- internal/infra/sqlite/migrations/003_projects_bots_vocab.sql
-- +goose Up

CREATE TABLE vocabularies (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    release_event TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE vocabulary_events (
    id               TEXT PRIMARY KEY,
    vocabulary_id    TEXT NOT NULL REFERENCES vocabularies(id) ON DELETE CASCADE,
    event_type       TEXT NOT NULL,
    message_template TEXT NOT NULL,
    metadata_keys    TEXT NOT NULL DEFAULT '[]',
    UNIQUE (vocabulary_id, event_type)
);

CREATE TABLE projects (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    template_id   TEXT NOT NULL DEFAULT '',
    channel_name  TEXT NOT NULL,
    vocabulary_id TEXT NOT NULL REFERENCES vocabularies(id),
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE bots (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL UNIQUE
        REFERENCES projects(id) ON DELETE CASCADE,
    passport_api_key_hash  TEXT NOT NULL,
    passport_api_key_id    TEXT NOT NULL,
    hive_role_assignments  TEXT NOT NULL DEFAULT '[]',
    created_at             DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at             DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX audit_events_project_idx ON audit_events(project, occurred_at);
CREATE INDEX work_items_assigned_agent_idx ON work_items(assigned_agent_id);

-- +goose Down

DROP INDEX work_items_assigned_agent_idx;
DROP INDEX audit_events_project_idx;
DROP TABLE bots;
DROP TABLE projects;
DROP TABLE vocabulary_events;
DROP TABLE vocabularies;
```

```go
// internal/infra/sqlite/projects.go
// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/Work-Fort/Flow/internal/domain"
)

const projectCols = "id, name, description, template_id, channel_name, vocabulary_id, created_at, updated_at"

func (s *Store) CreateProject(ctx context.Context, p *domain.Project) error {
    now := time.Now().UTC()
    if p.CreatedAt.IsZero() { p.CreatedAt = now }
    if p.UpdatedAt.IsZero() { p.UpdatedAt = now }
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO projects (`+projectCols+`) VALUES (?,?,?,?,?,?,?,?)`,
        p.ID, p.Name, p.Description, p.TemplateID, p.ChannelName, p.VocabularyID,
        p.CreatedAt, p.UpdatedAt)
    if err != nil {
        if isUniqueViolation(err) {
            return fmt.Errorf("%w: project %q", domain.ErrAlreadyExists, p.Name)
        }
        return fmt.Errorf("insert project: %w", err)
    }
    return nil
}

func (s *Store) GetProject(ctx context.Context, id string) (*domain.Project, error) {
    return s.scanProject(s.db.QueryRowContext(ctx,
        `SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
}

func (s *Store) GetProjectByName(ctx context.Context, name string) (*domain.Project, error) {
    return s.scanProject(s.db.QueryRowContext(ctx,
        `SELECT `+projectCols+` FROM projects WHERE name = ?`, name))
}

func (s *Store) ListProjects(ctx context.Context) ([]*domain.Project, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT `+projectCols+` FROM projects ORDER BY name ASC`)
    if err != nil { return nil, fmt.Errorf("list projects: %w", err) }
    defer rows.Close()
    var out []*domain.Project
    for rows.Next() {
        p, err := s.scanProjectRow(rows)
        if err != nil { return nil, err }
        out = append(out, p)
    }
    return out, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, p *domain.Project) error {
    p.UpdatedAt = time.Now().UTC()
    res, err := s.db.ExecContext(ctx,
        `UPDATE projects SET name=?, description=?, template_id=?, channel_name=?, vocabulary_id=?, updated_at=? WHERE id=?`,
        p.Name, p.Description, p.TemplateID, p.ChannelName, p.VocabularyID, p.UpdatedAt, p.ID)
    if err != nil {
        if isUniqueViolation(err) {
            return fmt.Errorf("%w: project %q", domain.ErrAlreadyExists, p.Name)
        }
        return fmt.Errorf("update project: %w", err)
    }
    n, _ := res.RowsAffected()
    if n == 0 { return fmt.Errorf("%w: project %s", domain.ErrNotFound, p.ID) }
    return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
    var hasBot int
    if err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM bots WHERE project_id = ?`, id).Scan(&hasBot); err != nil {
        return fmt.Errorf("count bots: %w", err)
    }
    if hasBot > 0 {
        return fmt.Errorf("%w: project %s", domain.ErrProjectHasBot, id)
    }
    res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
    if err != nil { return fmt.Errorf("delete project: %w", err) }
    n, _ := res.RowsAffected()
    if n == 0 { return fmt.Errorf("%w: project %s", domain.ErrNotFound, id) }
    return nil
}

type rowScanner interface{ Scan(...any) error }

func (s *Store) scanProject(r rowScanner) (*domain.Project, error) {
    var p domain.Project
    var created, updated time.Time
    err := r.Scan(&p.ID, &p.Name, &p.Description, &p.TemplateID, &p.ChannelName,
        &p.VocabularyID, &created, &updated)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, fmt.Errorf("%w: project", domain.ErrNotFound)
    }
    if err != nil {
        return nil, fmt.Errorf("scan project: %w", err)
    }
    p.CreatedAt = created
    p.UpdatedAt = updated
    return &p, nil
}

func (s *Store) scanProjectRow(rows *sql.Rows) (*domain.Project, error) {
    return s.scanProject(rows)
}
```

```go
// internal/infra/sqlite/bots.go
// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/Work-Fort/Flow/internal/domain"
)

const botCols = "id, project_id, passport_api_key_hash, passport_api_key_id, hive_role_assignments, created_at, updated_at"

func (s *Store) CreateBot(ctx context.Context, b *domain.Bot) error {
    now := time.Now().UTC()
    if b.CreatedAt.IsZero() { b.CreatedAt = now }
    if b.UpdatedAt.IsZero() { b.UpdatedAt = now }
    rolesJSON, _ := json.Marshal(b.HiveRoleAssignments)
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO bots (`+botCols+`) VALUES (?,?,?,?,?,?,?)`,
        b.ID, b.ProjectID, b.PassportAPIKeyHash, b.PassportAPIKeyID,
        string(rolesJSON), b.CreatedAt, b.UpdatedAt)
    if err != nil {
        if isUniqueViolation(err) {
            return fmt.Errorf("%w: bot for project %s", domain.ErrAlreadyExists, b.ProjectID)
        }
        return fmt.Errorf("insert bot: %w", err)
    }
    return nil
}

func (s *Store) GetBotByProject(ctx context.Context, projectID string) (*domain.Bot, error) {
    var b domain.Bot
    var roles string
    var created, updated time.Time
    err := s.db.QueryRowContext(ctx,
        `SELECT `+botCols+` FROM bots WHERE project_id = ?`, projectID).Scan(
        &b.ID, &b.ProjectID, &b.PassportAPIKeyHash, &b.PassportAPIKeyID,
        &roles, &created, &updated)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID)
    }
    if err != nil { return nil, fmt.Errorf("get bot: %w", err) }
    _ = json.Unmarshal([]byte(roles), &b.HiveRoleAssignments)
    b.CreatedAt = created
    b.UpdatedAt = updated
    return &b, nil
}

func (s *Store) DeleteBotByProject(ctx context.Context, projectID string) error {
    res, err := s.db.ExecContext(ctx, `DELETE FROM bots WHERE project_id = ?`, projectID)
    if err != nil { return fmt.Errorf("delete bot: %w", err) }
    n, _ := res.RowsAffected()
    if n == 0 { return fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID) }
    return nil
}
```

```go
// internal/infra/sqlite/vocabularies.go
// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/Work-Fort/Flow/internal/domain"
)

func (s *Store) CreateVocabulary(ctx context.Context, v *domain.Vocabulary) error {
    now := time.Now().UTC()
    if v.CreatedAt.IsZero() { v.CreatedAt = now }
    if v.UpdatedAt.IsZero() { v.UpdatedAt = now }
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return fmt.Errorf("begin tx: %w", err) }
    defer tx.Rollback()

    _, err = tx.ExecContext(ctx,
        `INSERT INTO vocabularies (id, name, description, release_event, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
        v.ID, v.Name, v.Description, v.ReleaseEvent, v.CreatedAt, v.UpdatedAt)
    if err != nil {
        if isUniqueViolation(err) {
            return fmt.Errorf("%w: vocabulary %q", domain.ErrAlreadyExists, v.Name)
        }
        return fmt.Errorf("insert vocabulary: %w", err)
    }
    for _, e := range v.Events {
        keysJSON, _ := json.Marshal(e.MetadataKeys)
        if _, err := tx.ExecContext(ctx,
            `INSERT INTO vocabulary_events (id, vocabulary_id, event_type, message_template, metadata_keys) VALUES (?,?,?,?,?)`,
            e.ID, v.ID, e.EventType, e.MessageTemplate, string(keysJSON)); err != nil {
            return fmt.Errorf("insert vocabulary_event %q: %w", e.EventType, err)
        }
    }
    return tx.Commit()
}

func (s *Store) GetVocabulary(ctx context.Context, id string) (*domain.Vocabulary, error) {
    return s.loadVocabulary(ctx, "id", id)
}

func (s *Store) GetVocabularyByName(ctx context.Context, name string) (*domain.Vocabulary, error) {
    return s.loadVocabulary(ctx, "name", name)
}

func (s *Store) loadVocabulary(ctx context.Context, col, val string) (*domain.Vocabulary, error) {
    var v domain.Vocabulary
    var created, updated time.Time
    err := s.db.QueryRowContext(ctx,
        `SELECT id, name, description, release_event, created_at, updated_at FROM vocabularies WHERE `+col+` = ?`, val).Scan(
        &v.ID, &v.Name, &v.Description, &v.ReleaseEvent, &created, &updated)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, fmt.Errorf("%w: vocabulary", domain.ErrNotFound)
    }
    if err != nil { return nil, fmt.Errorf("get vocabulary: %w", err) }
    v.CreatedAt = created
    v.UpdatedAt = updated

    rows, err := s.db.QueryContext(ctx,
        `SELECT id, vocabulary_id, event_type, message_template, metadata_keys FROM vocabulary_events WHERE vocabulary_id = ? ORDER BY event_type`, v.ID)
    if err != nil { return nil, fmt.Errorf("list vocab events: %w", err) }
    defer rows.Close()
    for rows.Next() {
        var e domain.VocabularyEvent
        var keys string
        if err := rows.Scan(&e.ID, &e.VocabularyID, &e.EventType, &e.MessageTemplate, &keys); err != nil {
            return nil, fmt.Errorf("scan vocab event: %w", err)
        }
        _ = json.Unmarshal([]byte(keys), &e.MetadataKeys)
        v.Events = append(v.Events, e)
    }
    return &v, rows.Err()
}

// ListVocabularies loads every vocabulary + its events in a single
// pass. The implementation fetches vocabulary rows first, then all
// vocabulary_events rows, then groups events by vocabulary_id
// in-memory — avoiding the N+1 pattern that a per-row
// GetVocabulary call would incur. Acceptable at tenant scale on
// record (≤ 50 vocabularies); revisit if that assumption breaks.
func (s *Store) ListVocabularies(ctx context.Context) ([]*domain.Vocabulary, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, name, description, release_event, created_at, updated_at FROM vocabularies ORDER BY name ASC`)
    if err != nil { return nil, fmt.Errorf("list vocabularies: %w", err) }
    defer rows.Close()
    byID := map[string]*domain.Vocabulary{}
    var out []*domain.Vocabulary
    for rows.Next() {
        var v domain.Vocabulary
        if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.ReleaseEvent, &v.CreatedAt, &v.UpdatedAt); err != nil {
            return nil, fmt.Errorf("scan vocab: %w", err)
        }
        vp := &v
        byID[v.ID] = vp
        out = append(out, vp)
    }
    if err := rows.Err(); err != nil { return nil, err }

    eventRows, err := s.db.QueryContext(ctx,
        `SELECT id, vocabulary_id, event_type, message_template, metadata_keys FROM vocabulary_events ORDER BY vocabulary_id, event_type`)
    if err != nil { return nil, fmt.Errorf("list all vocab events: %w", err) }
    defer eventRows.Close()
    for eventRows.Next() {
        var e domain.VocabularyEvent
        var keys string
        if err := eventRows.Scan(&e.ID, &e.VocabularyID, &e.EventType, &e.MessageTemplate, &keys); err != nil {
            return nil, fmt.Errorf("scan vocab event: %w", err)
        }
        _ = json.Unmarshal([]byte(keys), &e.MetadataKeys)
        if vp := byID[e.VocabularyID]; vp != nil {
            vp.Events = append(vp.Events, e)
        }
    }
    return out, eventRows.Err()
}
```

Audit-by-project addition (modify `internal/infra/sqlite/audit.go`):

```go
func (s *Store) ListAuditEventsByProject(ctx context.Context, project string) ([]*domain.AuditEvent, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT `+auditCols+`
        FROM audit_events
        WHERE project = ?
        ORDER BY occurred_at ASC, id ASC`, project)
    if err != nil { return nil, fmt.Errorf("query audit_events by project: %w", err) }
    defer rows.Close()
    return scanAuditEvents(rows)
}
```

Work-item-by-agent addition (modify `internal/infra/sqlite/workitems.go`):

```go
func (s *Store) ListWorkItemsByAgent(ctx context.Context, agentID string) ([]*domain.WorkItem, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT `+workItemCols+`
        FROM work_items
        WHERE assigned_agent_id = ?
        ORDER BY updated_at DESC, id ASC`, agentID)
    if err != nil { return nil, fmt.Errorf("query work_items by agent: %w", err) }
    defer rows.Close()
    return scanWorkItems(rows)
}
```

(`workItemCols` and `scanWorkItems` already exist in the file.
Indexed by `work_items_assigned_agent_idx` from migration 003.)

**Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/sqlite/...`
Expected: PASS for all three new tests + the existing suite still
green.

**Step 5: Commit**

```
feat(sqlite): add projects/bots/vocabularies + audit-by-project

Migration 003 introduces vocabularies, vocabulary_events,
projects, and bots tables, plus a project index on audit_events.
The bots table's UNIQUE(project_id) enforces the 1-project-1-bot
invariant the design hard-codes.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 4: Postgres mirror migration + store impl

**Depends on:** Task 3 (sqlite parity).

Mirrors Task 3 against the postgres backend. Files:

- Create: `internal/infra/postgres/migrations/003_projects_bots_vocab.sql`
  — same DDL with TIMESTAMPTZ + JSONB for `metadata_keys` /
  `hive_role_assignments`. Includes the
  `CREATE INDEX work_items_assigned_agent_idx ON
  work_items(assigned_agent_id)` mirror.
- Create: `internal/infra/postgres/projects.go`,
  `internal/infra/postgres/bots.go`,
  `internal/infra/postgres/vocabularies.go` — `$1`/`$2` placeholders
  instead of `?`, otherwise identical to the sqlite impls (including
  the in-memory grouped `ListVocabularies` to avoid N+1).
- Modify: `internal/infra/postgres/audit.go` — add
  `ListAuditEventsByProject`.
- Modify: `internal/infra/postgres/workitems.go` — add
  `ListWorkItemsByAgent`.
- Test: `internal/infra/postgres/store_test.go` — copy the four
  sqlite test cases verbatim (`TestStore_ProjectCRUD`,
  `TestStore_BotCRUD`, `TestStore_AuditByProject`,
  `TestStore_ListWorkItemsByAgent`) under the postgres test harness.

**Step 1-4 mirror Task 3.** Test command:
`go test ./internal/infra/postgres/...`
Expected after impl: PASS.

**Step 5: Commit**

```
feat(postgres): mirror projects/bots/vocabularies migration + store

Postgres parity for the projects, bots, and vocabularies tables and
the ListAuditEventsByProject query. Uses TIMESTAMPTZ and JSONB
where the sqlite migration uses DATETIME and TEXT.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 5: REST surface — projects + vocabularies

**Depends on:** Tasks 2-4 (store ports + impls).

**Files:**
- Create: `internal/daemon/rest_projects.go` (handlers).
- Create: `internal/daemon/rest_vocabularies.go` (handlers).
- Modify: `internal/daemon/rest_types.go` — add input/output structs
  for the new endpoints.
- Modify: `internal/daemon/server.go` — call
  `registerProjectRoutes(api, cfg.Store)` +
  `registerVocabularyRoutes(api, cfg.Store)` from `NewServer`.

**Step 1: Write the failing test**

Add `tests/e2e/projects_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
    "net/http"
    "testing"

    "github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestProjects_CRUD(t *testing.T) {
    env := harness.NewEnv(t)
    defer env.Cleanup(t)
    tok := env.Daemon.SignJWT("svc-p", "flow-p", "Flow P", "service")
    c := harness.NewClient(env.Daemon.BaseURL(), tok)

    // List vocabularies — at least the SDLC seed must be present.
    var vocs []map[string]any
    if status, _, err := c.GetJSON("/v1/vocabularies", &vocs); err != nil || status != 200 {
        t.Fatalf("list vocab: status=%d err=%v", status, err)
    }
    var sdlcID string
    for _, v := range vocs {
        if v["name"] == "sdlc" { sdlcID = v["id"].(string) }
    }
    if sdlcID == "" { t.Fatal("SDLC vocab seed missing") }

    var created struct{ ID string `json:"id"` }
    if status, _, err := c.PostJSON("/v1/projects", map[string]any{
        "name": "p-test", "channel_name": "#p-test", "vocabulary_id": sdlcID,
    }, &created); err != nil || status != http.StatusCreated {
        t.Fatalf("create project: status=%d err=%v", status, err)
    }
    if created.ID == "" { t.Fatal("missing id") }

    // Duplicate name conflicts.
    if status, _, err := c.PostJSON("/v1/projects", map[string]any{
        "name": "p-test", "channel_name": "#dup", "vocabulary_id": sdlcID,
    }, nil); err != nil || status != http.StatusConflict {
        t.Errorf("expected 409, got status=%d err=%v", status, err)
    }

    var got map[string]any
    if status, _, err := c.GetJSON("/v1/projects/"+created.ID, &got); err != nil || status != 200 {
        t.Fatalf("get project: status=%d err=%v", status, err)
    }
    if got["name"] != "p-test" { t.Errorf("name = %v", got["name"]) }

    // Default-vocab path: omitting vocabulary_id resolves to the SDLC
    // seed via store.GetVocabularyByName(ctx, "sdlc").
    var defaulted struct {
        ID            string `json:"id"`
        VocabularyID  string `json:"vocabulary_id"`
    }
    if status, _, err := c.PostJSON("/v1/projects", map[string]any{
        "name": "p-default", "channel_name": "#p-default",
    }, &defaulted); err != nil || status != http.StatusCreated {
        t.Fatalf("create default-vocab project: status=%d err=%v", status, err)
    }
    if defaulted.VocabularyID != sdlcID {
        t.Errorf("default vocabulary_id = %q, want SDLC seed %q",
            defaulted.VocabularyID, sdlcID)
    }
}

**Step 2: Run test to verify it fails**

Run: `mise run e2e --backend=sqlite -- -run TestProjects_CRUD`
Expected: FAIL — `404` from server (route not registered).

**Step 3: Write minimal implementation**

```go
// internal/daemon/rest_projects.go (full file — see plan structure for length)
// Each handler follows the existing huma.Register pattern from rest_huma.go.
// The handler function bodies call store.CreateProject / store.GetProject /
// store.ListProjects / store.UpdateProject / store.DeleteProject and translate
// errors via mapDomainErr (which already maps ErrAlreadyExists → 409 and
// ErrNotFound → 404; add a switch arm for ErrProjectHasBot → 409).
// CreateProject defaults vocabulary_id to the SDLC seed when the request omits it
// (looked up via store.GetVocabularyByName(ctx, "sdlc")).
```

Add 5 huma.Register calls (`POST /v1/projects`, `GET /v1/projects`,
`GET /v1/projects/{id}`, `PATCH /v1/projects/{id}`, `DELETE
/v1/projects/{id}`). Bot-binding routes ship in Task 7. Vocabulary
read routes (`GET /v1/vocabularies`, `GET /v1/vocabularies/{id}`) ship
alongside in `rest_vocabularies.go`.

Modify `internal/daemon/rest_huma.go:26` to add:

```go
case errors.Is(err, domain.ErrProjectHasBot):
    return huma.NewError(http.StatusConflict, err.Error())
```

In `internal/daemon/server.go`'s `NewServer` (currently at line 55),
after the existing block of `register*Routes(api, …)` calls (today
that block ends with `registerSchedulerAndAuditDiagRoutes`), append:

```go
registerVocabularyRoutes(api, cfg.Store)
registerProjectRoutes(api, cfg.Store, cfg.BotKeysDir)
```

(`cfg.BotKeysDir` is added to `ServerConfig` in the additions block
below — Task 7 wires the actual key file management onto the bot
sub-routes registered inside `registerProjectRoutes`.)

#### ServerConfig additions (single source of truth)

This plan adds two `daemon.ServerConfig` fields. Tasks 7-10 all
assume them:

```go
// internal/daemon/server.go — appended to ServerConfig
type ServerConfig struct {
    // … existing fields unchanged …

    // BotKeysDir is the directory under which per-bot Passport API
    // key plaintexts are written (mode 0600). Empty disables bot
    // creation (the POST /v1/projects/{id}/bot handler returns 503).
    // Wired by --bot-keys-dir in cmd/daemon/daemon.go (default:
    // filepath.Join(config.GlobalPaths.StateDir, "bot-keys")).
    BotKeysDir string

    // Dispatcher is the optional vocabulary-driven message
    // dispatcher (Task 8). When nil, scheduler claim/release
    // (Task 9) and the Combine webhook (Task 10) skip outbound
    // chat posts and continue with audit-only behaviour.
    Dispatcher domain.BotDispatcher
}
```

The corresponding `cmd/daemon/daemon.go` changes (Task 7's `flag` +
viper bindings, Task 8's wiring of `bot.New(store, chatAdapter)`
into `Dispatcher`) reference these fields by name; nothing else in
the plan touches `ServerConfig`.

**Step 4: Run test to verify it passes**

Run: `mise run e2e --backend=sqlite -- -run TestProjects_CRUD`
Expected: PASS.
Run: `mise run e2e --backend=postgres -- -run TestProjects_CRUD`
Expected: PASS.

**Step 5: Commit**

```
feat(daemon): add /v1/projects + /v1/vocabularies REST routes

Operator-facing CRUD for projects + read-only vocabulary listing.
DELETE rejects with 409 when a bot is still bound. CreateProject
defaults to the SDLC vocabulary seed when no vocabulary_id is
supplied.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 6: Per-project audit endpoint

**Depends on:** Tasks 2-4 (store + audit method).

**Files:**
- Modify: `internal/daemon/scheduler_diag.go` (or new
  `rest_audit_project.go` co-located with project routes — preferred,
  to keep `_diag` boundary clean).
- Test: `tests/e2e/projects_test.go` — add
  `TestProjects_AuditFiltered`.

**Step 1: Write the failing test**

```go
func TestProjects_AuditFiltered(t *testing.T) {
    env := harness.NewEnv(t)
    defer env.Cleanup(t)
    env.Hive.SeedPoolAgent("a_p_1", "agent-p-1", "team-p")
    tok := env.Daemon.SignJWT("svc-p", "flow-p", "Flow P", "service")
    c := harness.NewClient(env.Daemon.BaseURL(), tok)

    // Drive a claim/release for "p1" via the existing scheduler diag.
    var claim struct{ AgentID, WorkflowID string }
    _, _, _ = c.PostJSON("/v1/scheduler/_diag/claim", map[string]any{
        "role": "developer", "project": "p1",
        "workflow_id": "wf-p1", "lease_ttl_seconds": 30,
    }, &claim)
    _, _, _ = c.PostJSON("/v1/scheduler/_diag/release",
        map[string]any{"agent_id": claim.AgentID, "workflow_id": claim.WorkflowID}, nil)

    // Seed a project named p1 (vocab_id defaults to SDLC).
    var prj struct{ ID string `json:"id"` }
    _, _, _ = c.PostJSON("/v1/projects",
        map[string]any{"name": "p1", "channel_name": "#p1"}, &prj)

    var resp struct {
        Events []map[string]any `json:"events"`
    }
    if status, _, err := c.GetJSON("/v1/projects/"+prj.ID+"/audit", &resp); err != nil || status != 200 {
        t.Fatalf("get audit: status=%d err=%v", status, err)
    }
    if len(resp.Events) != 2 {
        t.Errorf("expected 2 events (claim + release), got %d", len(resp.Events))
    }
}
```

**Step 2-4** mirror previous tasks; the handler GETs the project to
verify it exists, then calls `store.ListAuditEventsByProject(ctx,
project.Name)`. Tests run on both backends.

**Step 5: Commit**

```
feat(daemon): add per-project audit list endpoint

GET /v1/projects/{id}/audit returns audit events whose `project`
field matches the project's name. The Project row is loaded first
to translate ID→name; the audit table joins on `project` (a string
field today, scoped to the AgentClaim path).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 7: Bot bind/unbind REST + Passport key file management

**Depends on:** Task 5 (project routes already mounted).

**Files:**
- Modify: `internal/daemon/rest_projects.go` — add `POST
  /v1/projects/{id}/bot`, `GET /v1/projects/{id}/bot`, `DELETE
  /v1/projects/{id}/bot`.
- Create: `internal/daemon/bot_keys.go` — small helper that hashes a
  plaintext key (SHA-256) and writes it to
  `FLOW_BOT_KEYS_DIR/<bot_id>` mode 0600.
- Modify: `cmd/daemon/daemon.go` — add `--bot-keys-dir` flag bound to
  viper key `bot.keys-dir`; default to
  `filepath.Join(config.GlobalPaths.StateDir, "bot-keys")`.
- Modify: `internal/daemon/server.go` — propagate `BotKeysDir` field
  on `ServerConfig` into the project route registration.

**Step 1: Write the failing test**

```go
// tests/e2e/bot_bind_test.go
func TestBot_BindUnbind(t *testing.T) {
    env := harness.NewEnv(t)
    defer env.Cleanup(t)
    tok := env.Daemon.SignJWT("svc", "flow", "Flow", "service")
    c := harness.NewClient(env.Daemon.BaseURL(), tok)

    var prj struct{ ID string `json:"id"` }
    _, _, _ = c.PostJSON("/v1/projects",
        map[string]any{"name": "bot-prj", "channel_name": "#bot-prj"}, &prj)

    var bot struct {
        ID                string   `json:"id"`
        PassportAPIKeyID  string   `json:"passport_api_key_id"`
        HiveRoleAssignments []string `json:"hive_role_assignments"`
    }
    status, body, err := c.PostJSON("/v1/projects/"+prj.ID+"/bot", map[string]any{
        "passport_api_key":         "wf-svc_test_plaintext_xxxxxxxx",
        "passport_api_key_id":      "pak_test_001",
        "hive_role_assignments":    []string{"developer", "reviewer"},
    }, &bot)
    if err != nil || status != http.StatusCreated {
        t.Fatalf("bind bot: status=%d body=%s err=%v", status, body, err)
    }
    if bot.PassportAPIKeyID != "pak_test_001" || len(bot.HiveRoleAssignments) != 2 {
        t.Errorf("bot row missing fields: %+v", bot)
    }

    // Plaintext key never appears in the response.
    if string(body) != "" && bytesContains(body, "wf-svc_test_plaintext") {
        t.Errorf("plaintext key leaked in response body: %s", body)
    }

    // Idempotency: second create returns 409.
    if status, _, _ := c.PostJSON("/v1/projects/"+prj.ID+"/bot", map[string]any{
        "passport_api_key": "wf-svc_other", "passport_api_key_id": "pak_2",
    }, nil); status != http.StatusConflict {
        t.Errorf("expected 409 on duplicate bind, got %d", status)
    }

    // Unbind clears the row.
    if status, _, _ := c.DeleteJSON("/v1/projects/" + prj.ID + "/bot"); status != http.StatusNoContent {
        t.Errorf("delete bot: %d", status)
    }
    if status, _, _ := c.GetJSON("/v1/projects/"+prj.ID+"/bot", nil); status != http.StatusNotFound {
        t.Errorf("get bot after delete: status=%d, want 404", status)
    }

    // Project delete now succeeds.
    if status, _, _ := c.DeleteJSON("/v1/projects/" + prj.ID); status != http.StatusNoContent {
        t.Errorf("delete project after unbind: %d", status)
    }
}
```

**Step 2-4** mirror earlier tasks. Implementation notes:

- `POST /v1/projects/{id}/bot` validates the project exists, generates
  the `bot_<uuid>` ID, hashes the plaintext key with `sha256.Sum256`,
  then **inserts the bot row first**, **writes the key file second**.
  Ordering rationale: an insert failure (UNIQUE conflict, constraint
  violation, network blip) leaves no key-file droppings; a daemon
  crash between the row insert and the file write leaves an orphan
  row whose first use surfaces as `ErrBotKeyMissing` (a clean,
  loggable condition, fixable by re-binding). The reverse ordering
  (file-first) creates undetectable orphan key files on the host
  filesystem, which is the worse failure mode. The startup sweep
  below makes the row-first ordering self-healing.
- `GET /v1/projects/{id}/bot` returns the row WITHOUT the hash — the
  domain type's `json:"-"` tag on `PassportAPIKeyHash` already drops
  it.
- `DELETE /v1/projects/{id}/bot` removes the row + the key file.
  `os.Remove` errors on missing key file are tolerated (logged
  warning).
- **Startup orphan-file sweep.** On daemon start, `cmd/daemon/daemon.go`
  walks `<BotKeysDir>` and for each filename matching `bot_*` checks
  whether a bot row exists with that ID. Missing rows → orphan key
  file → log + delete. Missing files for present rows are NOT
  deleted (the row stays so the operator can re-bind via DELETE +
  POST). The sweep is best-effort: a single warning per orphan, no
  blocking. Add a unit test
  `TestBotKeys_StartupSweep_RemovesOrphans` in
  `internal/daemon/bot_keys_test.go`.

**Step 5: Commit**

```
feat(daemon): bind/unbind project bots with hashed Passport keys

Plaintext Passport API keys are accepted once on create, hashed
(SHA-256), persisted in the bot row, and the plaintext is written
to a per-bot 0600 file in FLOW_BOT_KEYS_DIR. The plaintext never
appears in any response body.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 8: BotDispatcher — vocabulary-driven message renderer

**Depends on:** Tasks 1-7.

**Files:**
- Create: `internal/bot/dispatcher.go` — the BotDispatcher type
  (lives outside `internal/workflow/` to keep it loadable
  independently from the workflow service).
- Test: `internal/bot/dispatcher_test.go`.

The dispatcher accepts `(ctx, projectID, eventType, payload)` and:

1. Loads the project (`Store.GetProject`).
2. Loads its vocabulary (`Store.GetVocabulary(project.VocabularyID)`).
3. Calls `vocabulary.RenderEvent(eventType, ctx)`.
4. Posts the rendered text + metadata to `chatProvider.PostMessage`
   on the project's channel.

```go
// internal/bot/dispatcher.go
// SPDX-License-Identifier: GPL-2.0-only

// Package bot owns the per-project bot dispatch surface — the thing
// that takes a workflow event, looks up the project's vocabulary,
// and routes the rendered message through the ChatProvider port.
//
// It is the only consumer of Vocabulary.RenderEvent + ChatProvider.PostMessage
// in production code (the integration-hook path in internal/workflow/hooks.go
// remains as a less-flexible legacy path until templates migrate to
// vocabulary-driven posting).
package bot

import (
    "context"
    "fmt"

    "github.com/Work-Fort/Flow/internal/domain"
)

type Dispatcher struct {
    store domain.Store
    chat  domain.ChatProvider
}

func New(store domain.Store, chat domain.ChatProvider) *Dispatcher {
    return &Dispatcher{store: store, chat: chat}
}

// Dispatch renders an event for the given project and posts it to
// the project's Sharkfin channel. Returns nil if the chat provider
// is nil (chat disabled) so callers don't need to nil-check.
func (d *Dispatcher) Dispatch(ctx context.Context, projectID, eventType string, ctxData domain.VocabularyContext) error {
    if d.chat == nil {
        return nil
    }
    p, err := d.store.GetProject(ctx, projectID)
    if err != nil {
        return fmt.Errorf("load project %s: %w", projectID, err)
    }
    v, err := d.store.GetVocabulary(ctx, p.VocabularyID)
    if err != nil {
        return fmt.Errorf("load vocabulary %s: %w", p.VocabularyID, err)
    }
    ctxData.Project = p
    text, meta, err := v.RenderEvent(eventType, ctxData)
    if err != nil {
        return err
    }
    if _, err := d.chat.PostMessage(ctx, p.ChannelName, text, meta); err != nil {
        return fmt.Errorf("chat post for project %s: %w", projectID, err)
    }
    return nil
}
```

Test (full code):

```go
// internal/bot/dispatcher_test.go (full)
// SPDX-License-Identifier: GPL-2.0-only
package bot_test

// imports + a minimal in-mem fake Store covering only Project + Vocabulary,
// plus a stubChat copying the pattern from
// internal/workflow/stub_chat_test.go:9. Asserts that:
//   - Dispatch with a known project + known event renders the right
//     text + the right metadata to the right channel.
//   - Dispatch with an unknown event surfaces ErrEventNotInVocabulary.
//   - Dispatch with a nil chat returns nil and posts nothing.
```

Run: `go test ./internal/bot/...` → PASS after impl.

**Commit:**

```
feat(bot): vocabulary-driven message dispatcher

The Dispatcher loads a project + its vocabulary, renders the event
via Vocabulary.RenderEvent, and posts to the project's channel via
the ChatProvider port. Slack readiness is preserved — no
chat-platform-specific code touched.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 9: Wire scheduler claim/release into the dispatcher

**Depends on:** Task 8 (Dispatcher).

**Files:**
- Modify: `internal/scheduler/scheduler.go` — add three optional
  hooks via `domain` ports (`BotDispatcher`, `ProjectStore`,
  `VocabularyStore`) so Acquire fires `task_assigned` and Release
  fires the vocabulary's declared `release_event`.
- Modify: `internal/daemon/server.go` — wire the dispatcher + the two
  store interfaces into the scheduler when both are configured (the
  store already satisfies both interfaces — no extra plumbing).
- Test: `internal/scheduler/scheduler_test.go` — extend with a fake
  dispatcher + two in-mem store fakes (Project + Vocabulary) to
  assert (a) claim → `task_assigned` dispatched with the right
  Payload keys and (b) release → the vocabulary's `release_event`
  dispatched (proven by a vocab whose `ReleaseEvent` is
  `bug_resolved`, NOT `task_completed`, so the test catches a
  hard-coded fallback).

The new path in the scheduler:

1. Acquire succeeds → look up project by `AgentClaim.Project` (a
   string). If a project exists for that name AND the project's
   vocabulary contains a `task_assigned` event, dispatcher fires
   that event with the claim's agent name + role.
2. Release → look up project, look up its vocabulary's
   `ReleaseEvent` field, fire that. Fall back to `"task_completed"`
   only when `ReleaseEvent == ""` (a defensive default for
   vocabularies that forgot the declaration). The fallback name is
   spelled in scheduler code only because vocabularies that omit
   `release_event` have no other handle on Flow's release path —
   it's a defensive constant, NOT a workflow assumption.

If no project matches the AgentClaim's `Project` string, the
scheduler logs a debug breadcrumb and continues — the AgentClaim/
audit path is unaffected. This is the v1 loose coupling between
AgentClaim's free-form `Project` field and the new `projects` table.

#### `scheduler.Config` extension (port-typed, no `internal/bot` import)

The hexagonal rule is "infra never appears in another package's
signature — only domain ports do." `internal/scheduler/` cannot
import `internal/bot/`. The dispatcher therefore enters as
`domain.BotDispatcher` (the port added in Task 1).

```go
// internal/scheduler/scheduler.go (Config extension)

type Config struct {
    Hive         domain.HiveAgentClient
    Audit        domain.AuditEventStore
    // Optional. Both nil → scheduler runs as today (audit-only).
    // Both non-nil → claim/release also fire vocabulary dispatch.
    Projects     domain.ProjectStore       // optional
    Vocabularies domain.VocabularyStore    // optional, paired with Projects
    Dispatcher   domain.BotDispatcher      // optional, paired with the above
}
```

`internal/daemon/server.go` already holds the full `domain.Store`
aggregator (which satisfies both `ProjectStore` and
`VocabularyStore` after Task 2), so wiring is a one-liner per field
in `NewServer`'s `scheduler.New(scheduler.Config{…})` call.

#### Acquire dispatch (after audit row writes)

```go
// internal/scheduler/scheduler.go — invoked from AcquireAgent
func (s *Scheduler) postClaimDispatch(ctx context.Context, claim *domain.AgentClaim) {
    if s.dispatcher == nil || s.projects == nil { return }
    p, err := s.projects.GetProjectByName(ctx, claim.Project)
    if err != nil {
        log.Debug("scheduler: no project for claim", "project", claim.Project, "err", err)
        return
    }
    err = s.dispatcher.Dispatch(ctx, p.ID, "task_assigned", domain.VocabularyContext{
        AgentName: claim.AgentName,
        Role:      claim.Role,
        Payload: map[string]any{
            "agent_id":    claim.AgentID,
            "role":        claim.Role,
            "workflow_id": claim.WorkflowID,
        },
    })
    if err != nil {
        log.Warn("scheduler: dispatch task_assigned failed", "err", err)
    }
}
```

#### Release dispatch (after audit row writes)

```go
// internal/scheduler/scheduler.go — invoked from ReleaseAgent
//
// Resolution order: (1) load the project, (2) load its vocabulary
// (we cannot ask the dispatcher because the dispatcher only knows
// how to fire a NAMED event — it doesn't expose
// `vocabulary.ReleaseEvent`), (3) pick the vocab's ReleaseEvent or
// the documented "task_completed" fallback. This is the only place
// in Flow Go code where `task_completed` appears as a literal —
// guarded by a comment so a future grep for SDLC names finds it.
func (s *Scheduler) postReleaseDispatch(ctx context.Context, claim *domain.AgentClaim) {
    if s.dispatcher == nil || s.projects == nil || s.vocabularies == nil {
        return
    }
    p, err := s.projects.GetProjectByName(ctx, claim.Project)
    if err != nil {
        log.Debug("scheduler: no project for release", "project", claim.Project, "err", err)
        return
    }
    voc, err := s.vocabularies.GetVocabulary(ctx, p.VocabularyID)
    if err != nil {
        log.Warn("scheduler: vocab load failed for release", "vocab", p.VocabularyID, "err", err)
        return
    }
    eventType := voc.ReleaseEvent
    if eventType == "" {
        // DEFENSIVE FALLBACK ONLY — a vocabulary that declares
        // ReleaseEvent (every seeded one does) overrides this.
        // Do NOT add other event-name literals to scheduler code.
        eventType = "task_completed"
    }
    err = s.dispatcher.Dispatch(ctx, p.ID, eventType, domain.VocabularyContext{
        AgentName: claim.AgentName,
        Role:      claim.Role,
        Payload: map[string]any{
            "agent_id":    claim.AgentID,
            "role":        claim.Role,
            "workflow_id": claim.WorkflowID,
        },
    })
    if err != nil {
        log.Warn("scheduler: dispatch release_event failed", "event", eventType, "err", err)
    }
}
```

The dispatcher post does NOT block / fail the claim — same swallow-
warn pattern as the audit path.

**Commit:**

```
feat(scheduler): fire vocabulary events on claim and release

AcquireAgent fires the project vocabulary's task_assigned event
after the audit row; ReleaseAgent fires the vocabulary's declared
release_event (defensive default: "task_completed"). Dispatcher,
ProjectStore, and VocabularyStore enter scheduler.Config as
domain ports — no internal/bot import.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 10: Webhook adapter — Combine event → vocabulary dispatch

**Depends on:** Task 8.

**Files:**
- Modify: `internal/daemon/webhook_combine.go` — accept a
  `domain.BotDispatcher` and a `domain.ProjectStore` (today the
  handler only takes `AuditEventStore`).
- Modify: `internal/daemon/server.go` — pass `cfg.Dispatcher` and
  `cfg.Store` (which satisfies `ProjectStore`) into
  `HandleCombineWebhook`.
- Test: `tests/e2e/webhook_combine_test.go` — extend an existing
  test to assert that for a project bound to the SDLC vocab, a
  `pull_request_merged` webhook produces a `merged` rendered post in
  `env.Sharkfin.Messages()` whose `Body` matches the SDLC seed's
  `merged` template and whose `Metadata["event_type"] == "merged"`.

The webhook handler grows a small `repo → project_name` resolution:
when the Combine push/merge body's `repository.name` matches a
project's `name`, the dispatcher fires the corresponding vocabulary
event. If no project matches, the handler **logs a debug
breadcrumb** (`log.Debug("combine: no project for repo", "repo", …)`)
so operators see why a Combine event arrived without a chat post,
then continues — the audit row still lands, as today. This keeps
the v1 coupling loose: operators opt into dispatch by naming the
project after the repo.

| Combine event | Vocabulary event fired | Payload metadata keys |
|---|---|---|
| `push` (any branch) | `commit_landed` | `branch`, `commit_sha`, `author` |
| `pull_request_merged` | `merged` | `pr_number`, `merged_by`, `target_branch` |

If the vocabulary lacks an entry (e.g. bug-tracker vocab without
`merged`), dispatch returns `ErrEventNotInVocabulary` and the handler
logs at debug + 204s.

**Commit:**

```
feat(webhook): route Combine push/merge through bot dispatcher

When a project's name matches the Combine repository name and the
project's vocabulary defines commit_landed / merged, the bot
dispatcher posts the rendered message to the project's channel.
Audit recording is unchanged — dispatch is additive.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 11: MCP tools — `get_my_project`, `list_my_work_items`, `get_vocabulary`

**Depends on:** Tasks 2-4 (store), Task 8 (dispatcher concept).

**Files:**
- Modify: `internal/daemon/mcp_tools.go` — append three `s.AddTool`
  calls. Replace the line-25 magic-number comment ("registers all 12
  MCP tools") with a comment that matches the call count without
  reciting it: "registers every MCP tool the adjutant needs;
  count is asserted in the test below". Add a co-located unit test
  `internal/daemon/mcp_tools_test.go::TestRegisterTools_Count` that
  introspects the `*server.MCPServer` and asserts
  `len(srv.ListTools().Tools) == 15` so any further additions break
  one test (here) instead of one comment + one test.
- Test: `tests/e2e/mcp_tools_test.go` — add three test functions
  that invoke each new tool through the harness MCP wire client and
  assert the JSON shape.

```go
// get_my_project: resolves via ListAuditEventsByAgent for the most
// recent unreleased claim, then GetProjectByName.
s.AddTool(
    mcp.NewTool("get_my_project",
        mcp.WithDescription("Look up the project the agent is currently claimed for."),
        mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
    ),
    func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        agentID := req.GetString("agent_id", "")
        events, err := deps.Store.ListAuditEventsByAgent(ctx, agentID)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        var projectName string
        for i := len(events) - 1; i >= 0; i-- {
            if events[i].Type == domain.AuditEventAgentClaimed {
                projectName = events[i].Project
                break
            }
            if events[i].Type == domain.AuditEventAgentReleased {
                // released — agent has no current claim
                return mcp.NewToolResultError("agent has no active claim"), nil
            }
        }
        if projectName == "" {
            return mcp.NewToolResultError("agent has no claim history"), nil
        }
        p, err := deps.Store.GetProjectByName(ctx, projectName)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        v, err := deps.Store.GetVocabulary(ctx, p.VocabularyID)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        return jsonResult(map[string]any{"project": p, "vocabulary": v})
    },
)
// list_my_work_items: backed by Store.ListWorkItemsByAgent (added
//   in Task 2 + indexed by work_items_assigned_agent_idx in
//   migration 003). Single indexed query — no instance scan.
s.AddTool(
    mcp.NewTool("list_my_work_items",
        mcp.WithDescription("List work items currently assigned to the agent."),
        mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
    ),
    func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        agentID := req.GetString("agent_id", "")
        items, err := deps.Store.ListWorkItemsByAgent(ctx, agentID)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        return jsonResult(items)
    },
)
// get_vocabulary: trivial wrapper over Store.GetVocabulary.
s.AddTool(
    mcp.NewTool("get_vocabulary",
        mcp.WithDescription("Get a vocabulary by ID, including its event catalogue."),
        mcp.WithString("id", mcp.Required(), mcp.Description("Vocabulary ID")),
    ),
    func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        id := req.GetString("id", "")
        v, err := deps.Store.GetVocabulary(ctx, id)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        return jsonResult(v)
    },
)
```

**Commit:**

```
feat(mcp): add get_my_project, list_my_work_items, get_vocabulary

Three tools the adjutant needs to know its own context: which
project it's claimed for, what work items are assigned to it, and
which vocabulary the project bot uses. Tool count moves from 12 to 15.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 12: Seeds — SDLC + bug-tracker vocabularies

**Depends on:** Tasks 3-4 (store impls).

**Files:**
- Create: `docs/examples/vocabularies/sdlc.json` (9-event SDLC
  catalogue including `task_completed` so the
  `release_event` declaration is satisfied).
- Create: `docs/examples/vocabularies/bug-tracker.json` (7-event
  bug-tracker catalogue covering the full lifecycle: file → triage →
  assign → fix-proposed → verify → close, plus reopen).
- Modify: `cmd/admin/admin.go` — add `seed-vocabularies` subcommand
  that walks `docs/examples/vocabularies/` and inserts each vocab
  via `Store.CreateVocabulary`. Idempotent: when the row already
  exists `CreateVocabulary` returns `domain.ErrAlreadyExists` and the
  loader logs `"vocabulary already loaded"` at debug level and
  continues. The daemon's `run()` function in
  `cmd/daemon/daemon.go` also calls this on startup so a fresh
  daemon always has the SDLC seed loaded — required by Task 5's
  e2e test.
- Test: `cmd/admin/admin_test.go::TestSeedVocabularies_Idempotent` —
  runs the loader twice against an in-memory sqlite store and
  asserts (a) no error propagates from the second pass, (b) the
  vocabulary count stays at 2, and (c) the SDLC and bug-tracker
  vocabs are both reachable by name.

ID-prefix convention. The seed files use the namespace
`voc_builtin_*` and `ve_builtin_*` to reserve a non-collidable
prefix; operator-supplied vocabularies (Task 5's REST `POST
/v1/vocabularies` is out of scope, but the future Flow UI plan adds
it) generate IDs prefixed `voc_*` without `_builtin_`.

`docs/examples/vocabularies/sdlc.json`:

```json
{
  "id": "voc_builtin_sdlc",
  "name": "sdlc",
  "description": "Canonical software-delivery lifecycle vocabulary.",
  "release_event": "task_completed",
  "events": [
    {"id": "ve_builtin_sdlc_assigned", "vocabulary_id": "voc_builtin_sdlc", "event_type": "task_assigned",
     "message_template": "Task assigned: {{.WorkItem.Title}} → {{.AgentName}} ({{.Role}})",
     "metadata_keys": ["agent_id", "role", "workflow_id"]},
    {"id": "ve_builtin_sdlc_branch", "vocabulary_id": "voc_builtin_sdlc", "event_type": "branch_created",
     "message_template": "Branch created: {{index .Payload \"branch\"}}",
     "metadata_keys": ["branch", "commit_sha"]},
    {"id": "ve_builtin_sdlc_commit", "vocabulary_id": "voc_builtin_sdlc", "event_type": "commit_landed",
     "message_template": "Commit on {{index .Payload \"branch\"}}: {{index .Payload \"commit_sha\"}} by {{index .Payload \"author\"}}",
     "metadata_keys": ["branch", "commit_sha", "author"]},
    {"id": "ve_builtin_sdlc_pr", "vocabulary_id": "voc_builtin_sdlc", "event_type": "pr_opened",
     "message_template": "PR opened: #{{index .Payload \"pr_number\"}} on {{index .Payload \"branch\"}}",
     "metadata_keys": ["pr_number", "branch"]},
    {"id": "ve_builtin_sdlc_review", "vocabulary_id": "voc_builtin_sdlc", "event_type": "review_requested",
     "message_template": "Review requested: {{.WorkItem.Title}} (gate {{index .Payload \"gate_step_id\"}})",
     "metadata_keys": ["agent_id", "gate_step_id"]},
    {"id": "ve_builtin_sdlc_tests", "vocabulary_id": "voc_builtin_sdlc", "event_type": "tests_passing",
     "message_template": "Tests passing: {{index .Payload \"repo\"}} run {{index .Payload \"run_id\"}}",
     "metadata_keys": ["repo", "run_id"]},
    {"id": "ve_builtin_sdlc_merged", "vocabulary_id": "voc_builtin_sdlc", "event_type": "merged",
     "message_template": "Merged PR #{{index .Payload \"pr_number\"}} → {{index .Payload \"target_branch\"}} by {{index .Payload \"merged_by\"}}",
     "metadata_keys": ["pr_number", "merged_by", "target_branch"]},
    {"id": "ve_builtin_sdlc_deployed", "vocabulary_id": "voc_builtin_sdlc", "event_type": "deployed",
     "message_template": "Deployed {{index .Payload \"commit_sha\"}} to {{index .Payload \"env\"}}",
     "metadata_keys": ["commit_sha", "env"]},
    {"id": "ve_builtin_sdlc_completed", "vocabulary_id": "voc_builtin_sdlc", "event_type": "task_completed",
     "message_template": "Task completed: {{.WorkItem.Title}} → {{.AgentName}}",
     "metadata_keys": ["agent_id", "role", "workflow_id"]}
  ]
}
```

`docs/examples/vocabularies/bug-tracker.json`:

```json
{
  "id": "voc_builtin_bugtracker",
  "name": "bug-tracker",
  "description": "Bug-tracker workflow vocabulary — proves the per-workflow vocab plug-in path. Covers the full lifecycle: file, triage, assign, fix-proposed, verify, close, reopen.",
  "release_event": "bug_closed",
  "events": [
    {"id": "ve_builtin_bug_filed", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_filed",
     "message_template": "Bug filed: {{.WorkItem.Title}} (priority {{.WorkItem.Priority}})",
     "metadata_keys": ["reporter"]},
    {"id": "ve_builtin_bug_triaged", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_triaged",
     "message_template": "Triaged: {{.WorkItem.Title}} severity {{index .Payload \"severity\"}}",
     "metadata_keys": ["severity"]},
    {"id": "ve_builtin_bug_assigned", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_assigned",
     "message_template": "Assigned bug {{.WorkItem.Title}} → {{.AgentName}}",
     "metadata_keys": ["agent_id"]},
    {"id": "ve_builtin_bug_fix_proposed", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_fix_proposed",
     "message_template": "Fix proposed for {{.WorkItem.Title}} by {{.AgentName}} (commit {{index .Payload \"commit_sha\"}})",
     "metadata_keys": ["agent_id", "commit_sha", "branch"]},
    {"id": "ve_builtin_bug_verified", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_verified",
     "message_template": "Verified fix for {{.WorkItem.Title}} by {{.AgentName}}",
     "metadata_keys": ["agent_id", "verified_in"]},
    {"id": "ve_builtin_bug_closed", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_closed",
     "message_template": "Closed: {{.WorkItem.Title}} (resolution {{index .Payload \"resolution\"}})",
     "metadata_keys": ["agent_id", "resolution"]},
    {"id": "ve_builtin_bug_reopened", "vocabulary_id": "voc_builtin_bugtracker", "event_type": "bug_reopened",
     "message_template": "Reopened: {{.WorkItem.Title}} (reason: {{index .Payload \"reason\"}})",
     "metadata_keys": ["reason"]}
  ]
}
```

`cmd/admin/admin.go` gains a `seed-vocabularies` subcommand. Run on
daemon startup from `cmd/daemon/daemon.go:run()` (after `infra.Open`,
before `NewServer`) — best-effort, logs warnings, does not block
startup. Idempotent test:

```go
// cmd/admin/admin_test.go (additions)
func TestSeedVocabularies_Idempotent(t *testing.T) {
    store := openInMemSqlite(t)
    // First pass: both seeds inserted.
    if err := admin.SeedVocabularies(context.Background(), store); err != nil {
        t.Fatalf("seed pass 1: %v", err)
    }
    // Second pass: no error, no duplicates.
    if err := admin.SeedVocabularies(context.Background(), store); err != nil {
        t.Fatalf("seed pass 2: %v", err)
    }
    vocs, err := store.ListVocabularies(context.Background())
    if err != nil { t.Fatalf("list: %v", err) }
    if len(vocs) != 2 {
        t.Errorf("expected 2 vocabularies after two-pass seed, got %d", len(vocs))
    }
    if _, err := store.GetVocabularyByName(context.Background(), "sdlc"); err != nil {
        t.Errorf("sdlc seed missing: %v", err)
    }
    if _, err := store.GetVocabularyByName(context.Background(), "bug-tracker"); err != nil {
        t.Errorf("bug-tracker seed missing: %v", err)
    }
}
```

**Commit:**

```
feat(seed): seed SDLC + bug-tracker vocabularies on startup

Two reference vocabularies ship in docs/examples/vocabularies/.
The daemon loads them on startup if absent (idempotent insert).
The bug-tracker catalogue exists to prove the per-workflow-template
plug-in path required by the bot vocabulary design.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 13: E2E — bot vocabulary round-trip with rendered-message assertions

**Depends on:** Tasks 5-12.

**Files:**
- Create: `tests/e2e/bot_vocabulary_test.go`.

No harness changes. `tests/e2e/harness/fake_sharkfin.go` already
exposes `Messages() []SharkfinMessage` where each message has
`Channel`, `Body`, and `Metadata map[string]any` — sufficient for
every assertion below.

The new test extends Plan B's `bot_lifecycle_test.go` flow with
project + vocabulary + bot machinery:

```go
// tests/e2e/bot_vocabulary_test.go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
    "encoding/json"
    "net/http"
    "strings"
    "testing"

    "github.com/Work-Fort/Flow/tests/e2e/harness"
)

// TestBotVocabulary_SDLCRoundTrip drives the canonical SDLC loop
// through the new project + bot + vocabulary surface and asserts:
//
//   - On Scheduler claim, env.Sharkfin.Messages() now contains a
//     SharkfinMessage on the project's channel whose Body matches
//     the SDLC seed's "Task assigned: <title> → <agent> (<role>)"
//     rendering and whose Metadata["event_type"] == "task_assigned".
//   - On the combine pull_request_merged webhook, the recorded
//     Messages contain a "Merged PR #...→ ... by ..." Body with
//     Metadata["event_type"] == "merged".
//   - On Scheduler release, the recorded Messages contain
//     "Task completed: <title> → <agent>" with
//     Metadata["event_type"] == "task_completed".
//   - GET /v1/projects/{id}/audit returns ONLY the project's events.
func TestBotVocabulary_SDLCRoundTrip(t *testing.T) {
    // Setup: env, JWT-auth client, seed pool agent, create the
    // "flow" project (vocab defaults to SDLC seed).
    // Step 1: claim via /v1/scheduler/_diag/claim.
    // Step 2: assert env.Sharkfin.Messages()[0].Body has
    //         "Task assigned" prefix and Metadata["event_type"] ==
    //         "task_assigned".
    // Step 3: POST /v1/webhooks/combine with a pull_request_merged
    //         payload whose repository.name == "flow".
    // Step 4: assert env.Sharkfin.Messages() now contains a
    //         "Merged PR #" message with event_type == "merged".
    // Step 5: release via /v1/scheduler/_diag/release.
    // Step 6: assert "Task completed" message with event_type ==
    //         "task_completed".
    // Step 7: GET /v1/projects/<id>/audit and assert only the
    //         flow-project events appear.
}

// TestBotVocabulary_BugTrackerVocabPlugIn proves the per-workflow
// plug-in path drives the FULL bug lifecycle. Asserts the
// bug-tracker vocab fires for every step of
// `bug_filed → bug_fix_proposed → bug_verified → bug_closed` and
// the recorded messages render with bug-tracker templates, never
// SDLC.
func TestBotVocabulary_BugTrackerVocabPlugIn(t *testing.T) {
    // Setup: env, client, seed pool agent.
    // Step 1: create a second project "tracker" bound to the
    //         bug-tracker vocabulary (POST /v1/projects with
    //         vocabulary_id resolved from
    //         GET /v1/vocabularies → name=="bug-tracker").
    // Step 2: claim for project="tracker" — assert the rendered
    //         message Body starts with "Bug filed:" or with the
    //         vocab's task_assigned-equivalent. The bug-tracker
    //         vocab does NOT define task_assigned, so the
    //         dispatcher returns ErrEventNotInVocabulary and the
    //         scheduler logs without posting; Messages() count for
    //         #tracker channel stays at 0 after claim.
    // Step 3: POST /v1/webhooks/combine with a `push` event for
    //         repo "tracker" — vocab does not define commit_landed,
    //         again no post; audit still records.
    // Step 4: drive the explicit lifecycle via direct dispatcher
    //         calls (the test obtains a *bot.Dispatcher over the
    //         daemon's store + a small chat-fake; OR uses a
    //         dedicated /v1/_diag/dispatch test endpoint added in
    //         this task only behind a build tag — pick one and keep
    //         it inside the test package). Fire bug_filed →
    //         bug_fix_proposed → bug_verified → bug_closed.
    // Step 5: assert env.Sharkfin.Messages() on the #tracker
    //         channel contains exactly 4 messages whose Bodies
    //         match the bug-tracker seed templates (substring
    //         "Bug filed:", "Fix proposed for", "Verified fix for",
    //         "Closed:") and whose Metadata["event_type"] values
    //         are exactly bug_filed, bug_fix_proposed,
    //         bug_verified, bug_closed in that order.
    // Step 6: assert NONE of those messages contain SDLC
    //         template substrings ("Task assigned:",
    //         "Task completed:", "Merged PR").
    // Step 7: release the claim — bug_closed is the vocab's
    //         release_event, so the scheduler-driven release path
    //         re-fires bug_closed via the post-release dispatch;
    //         assert exactly one new "Closed:" message appears
    //         (or, if the test pre-fired bug_closed in Step 4,
    //         assert two total bug_closed messages).
}
```

Both tests run on both backends.

**Commit:**

```
test(e2e): bot vocabulary round-trip + bug-tracker plug-in

Extends Plan B's bot lifecycle test with assertions over the
rendered chat content + metadata sidecar produced by the project's
vocabulary. A second test creates a project on the bug-tracker
vocabulary and verifies its events render with bug-tracker
templates, proving the per-workflow plug-in path.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

## Verification checklist

Run after Task 13 lands:

- [ ] `mise run e2e --backend=sqlite` — green.
- [ ] `mise run e2e --backend=postgres` — green.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` — all unit and integration tests pass.
- [ ] `golangci-lint run` — no new lint failures (depguard +
      forbidigo from the cross-repo audit still pass).
- [ ] `internal/domain/ports.go:54` (`ChatProvider`) is byte-for-byte
      unchanged from master tip — Slack-readiness invariant intact.
- [ ] No imports of `sharkfin/client/go` from `internal/domain/`
      (`grep -rn 'sharkfin' internal/domain/ | grep -v //` returns
      empty).
- [ ] `docs/examples/vocabularies/sdlc.json` and
      `bug-tracker.json` exist; daemon startup loads them
      idempotently (verify with `TestSeedVocabularies_Idempotent`
      green and a second daemon start showing the
      "vocabulary already loaded" debug log).
- [ ] `TestRegisterTools_Count` (Task 11) green, asserting
      `len(srv.ListTools().Tools) == 15`. Manual `s.AddTool(` count
      check is no longer the source of truth — the test is.
- [ ] `internal/scheduler/scheduler.go` does NOT import
      `internal/bot` (`grep '"github.com/Work-Fort/Flow/internal/bot"'
      internal/scheduler/` returns empty). Dispatcher enters via
      the `domain.BotDispatcher` port only.
- [ ] No `t.Parallel()` calls added in any new e2e test (matches
      the existing harness convention — each daemon spawn is ~200 ms).
- [ ] Commit log: every commit on the branch is conventional
      multi-line with `Co-Authored-By: Claude Sonnet 4.6
      <noreply@anthropic.com>`, no `!` markers, no
      `BREAKING CHANGE:` footers.
- [ ] Update `docs/remaining-work.md` — strike "Bot vocabulary plan
      + impl" and surface the Flow UI plan as the next critical-path
      item per `AGENT-POOL-REMAINING-WORK.md` sequencing.

## Out-of-scope follow-ups (NOT in this plan)

- Auto-mint Passport API key on `POST /v1/projects/{id}/bot` —
  belongs in the Flow UI plan (which adds the Flow→Passport API
  call).
- Auto-create Sharkfin channel on `POST /v1/projects` — same
  rationale.
- Bidirectional command dispatch from inbound Sharkfin webhook into
  workflow transitions — Plan B already deferred this; needs design
  alignment with Adjutant role docs.
- Tightening `AgentClaim.Project` to be a project ID rather than a
  free-form string — small follow-up after the first 5 dogfood
  projects exist.
- Slack provider impl — separate plan once Sharkfin path is proven
  in production.
- Removing the legacy `internal/workflow/hooks.go` chat path —
  defer until all dogfood templates have migrated to vocabularies.
