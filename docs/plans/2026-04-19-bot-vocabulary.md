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
  Hard-coding any of the 8 SDLC event-type names into Flow Go code
  outside the seed file is a plan failure.
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
  `internal/domain/`.
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

A custom bug-tracker vocabulary
(`docs/examples/vocabularies/bug-tracker.json`) ships alongside it
with `bug_filed`, `bug_triaged`, `bug_assigned`, `bug_resolved`,
`bug_reopened` to prove the per-template plug-in path. Task 13
exercises this via e2e.

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
  (`FakeSharkfin.RecordedPosts()` returns the rendered strings).
- The bug-tracker vocabulary plug-in path works: a second project
  bound to the bug-tracker vocab fires `bug_filed` + `bug_resolved`
  and the harness asserts neither event renders to the SDLC text.
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

import "time"

type Bot struct {
    ID                  string    `json:"id"`
    ProjectID           string    `json:"project_id"`
    PassportAPIKeyHash  string    `json:"-"`
    PassportAPIKeyID    string    `json:"passport_api_key_id"`
    HiveRoleAssignments []string  `json:"hive_role_assignments"`
    CreatedAt           time.Time `json:"created_at"`
    UpdatedAt           time.Time `json:"updated_at"`
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
- Modify: `internal/domain/ports.go` — add three new store interfaces
  and embed them in the existing `Store` aggregator interface
  (line 42-50).

**Step 1: Write the failing test**

```go
// internal/domain/ports_test.go (new file)
// SPDX-License-Identifier: GPL-2.0-only
package domain_test

import (
    "context"
    "testing"

    "github.com/Work-Fort/Flow/internal/domain"
)

// Verifies the store aggregator embeds the three new interfaces. The
// test exists to lock the aggregator surface — adding a method to
// any of the four interfaces without updating Store would break this.
func TestStore_EmbedsProjectBotVocabulary(t *testing.T) {
    var s domain.Store = nil
    if s != nil {
        // never run — exists to assert method-set coverage at compile time.
        _, _ = s.GetProject(context.Background(), "x")
        _, _ = s.GetBotByProject(context.Background(), "x")
        _, _ = s.GetVocabulary(context.Background(), "x")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestStore_EmbedsProjectBotVocabulary ./internal/domain/...`
Expected: FAIL — compile error: `s.GetProject undefined`.

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

// Add a new audit-by-project query to AuditEventStore (project filter
// is the new GET /v1/projects/{id}/audit's backing call).
type AuditEventStore interface {
    RecordAuditEvent(ctx context.Context, e *AuditEvent) error
    ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*AuditEvent, error)
    ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*AuditEvent, error)
    ListAuditEventsByProject(ctx context.Context, project string) ([]*AuditEvent, error)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestStore_EmbedsProjectBotVocabulary ./internal/domain/...`
Expected: PASS.

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
```

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

-- +goose Down

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

func (s *Store) ListVocabularies(ctx context.Context) ([]*domain.Vocabulary, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id FROM vocabularies ORDER BY name ASC`)
    if err != nil { return nil, fmt.Errorf("list vocab ids: %w", err) }
    defer rows.Close()
    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil { return nil, err }
        ids = append(ids, id)
    }
    if err := rows.Err(); err != nil { return nil, err }
    var out []*domain.Vocabulary
    for _, id := range ids {
        v, err := s.GetVocabulary(ctx, id)
        if err != nil { return nil, err }
        out = append(out, v)
    }
    return out, nil
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
  `hive_role_assignments`.
- Create: `internal/infra/postgres/projects.go`,
  `internal/infra/postgres/bots.go`,
  `internal/infra/postgres/vocabularies.go` — `$1`/`$2` placeholders
  instead of `?`, otherwise identical to the sqlite impls.
- Modify: `internal/infra/postgres/audit.go` — add
  `ListAuditEventsByProject`.
- Test: `internal/infra/postgres/store_test.go` — copy the three
  sqlite test cases verbatim under postgres test harness.

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
}
```

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

Modify `internal/daemon/server.go:118` (after the existing
`register*Routes` calls):

```go
registerVocabularyRoutes(api, cfg.Store)
registerProjectRoutes(api, cfg.Store)
```

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
    if status, _, _ := c.DeleteJSON("/v1/projects/"+prj.ID+"/bot", nil); status != http.StatusNoContent {
        t.Errorf("delete bot: %d", status)
    }
    if status, _, _ := c.GetJSON("/v1/projects/"+prj.ID+"/bot", nil); status != http.StatusNotFound {
        t.Errorf("get bot after delete: status=%d, want 404", status)
    }

    // Project delete now succeeds.
    if status, _, _ := c.DeleteJSON("/v1/projects/"+prj.ID, nil); status != http.StatusNoContent {
        t.Errorf("delete project after unbind: %d", status)
    }
}
```

**Step 2-4** mirror earlier tasks. Implementation notes:

- `POST /v1/projects/{id}/bot` validates the project exists, hashes
  the plaintext key with `sha256.Sum256`, writes the plaintext to
  `<botKeysDir>/<bot_id>` (0600) **before** the row is committed (so a
  store conflict doesn't leave an orphan key file), then inserts the
  bot row. On store failure it deletes the key file.
- `GET /v1/projects/{id}/bot` returns the row WITHOUT the hash — the
  domain type's `json:"-"` tag on `PassportAPIKeyHash` already drops
  it.
- `DELETE /v1/projects/{id}/bot` removes the row + the key file.
  `os.Remove` errors on missing key file are tolerated (logged
  warning).

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
- Modify: `internal/scheduler/scheduler.go` — add an optional
  `BotDispatcher` hook + `ProjectByName` lookup so Acquire/Release
  fire dispatcher events.
- Modify: `internal/daemon/server.go` — wire the dispatcher into the
  scheduler when both are configured.
- Test: `internal/scheduler/scheduler_test.go` — extend with a fake
  dispatcher to assert claim → `task_assigned` posted, release →
  `task_completed` posted (or whatever the vocab's `release_event`
  declares).

The new path in the scheduler:

1. Acquire succeeds → look up project by `AgentClaim.Project` (a
   string). If a project exists for that name AND the project's
   vocabulary contains a `task_assigned` event, dispatcher fires
   that event with the claim's agent name + role.
2. Release → look up project, find vocabulary's `release_event`
   (defaulting to `task_completed`), dispatcher fires it.

If no project matches the AgentClaim's `Project` string, the
scheduler logs a warning and continues — the AgentClaim/audit path
is unaffected. This is the v1 loose coupling between AgentClaim's
free-form `Project` field and the new `projects` table.

```go
// internal/scheduler/scheduler.go (additions)

type Config struct {
    Hive       domain.HiveAgentClient
    Audit      domain.AuditEventStore
    Projects   domain.ProjectStore  // optional
    Dispatcher *bot.Dispatcher      // optional
}

// AcquireAgent (existing) — after audit row written:
func (s *Scheduler) postClaimDispatch(ctx context.Context, claim *domain.AgentClaim) {
    if s.dispatcher == nil || s.projects == nil {
        return
    }
    p, err := s.projects.GetProjectByName(ctx, claim.Project)
    if err != nil {
        log.Warn("scheduler: no project for claim", "project", claim.Project, "err", err)
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
// (Same shape for ReleaseAgent → fire vocabulary's release_event.)
```

The dispatcher post does NOT block / fail the claim — same swallow-
warn pattern as the audit path.

**Commit:**

```
feat(scheduler): fire vocabulary task_assigned on claim, release_event on release

When a project + dispatcher are configured, AcquireAgent posts the
project vocabulary's task_assigned event after the claim audit row,
and ReleaseAgent posts the vocabulary's declared release_event (or
task_completed by default). Dispatch failures are logged but never
block the scheduler primitive.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

---

### Task 10: Webhook adapter — Combine event → vocabulary dispatch

**Depends on:** Task 8.

**Files:**
- Modify: `internal/daemon/webhook_combine.go` — accept a
  `BotDispatcher` (currently it only takes `AuditEventStore`).
- Modify: `internal/daemon/server.go` — pass dispatcher in.
- Test: `tests/e2e/webhook_combine_test.go` — extend an existing
  test to assert that for a project bound to the SDLC vocab, a
  `pull_request_merged` webhook produces a `merged` rendered post in
  the FakeSharkfin recorded posts.

The webhook handler grows a small `repo → project_name` resolution:
when the Combine push/merge body's `repository.name` matches a
project's `name`, the dispatcher fires the corresponding vocabulary
event. If no project matches, the handler is a no-op for dispatch
(the audit row still lands, as today). This keeps the v1 coupling
loose — the operator names the project after the repo to opt into
dispatch.

| Combine event | Vocabulary event fired | Payload metadata keys |
|---|---|---|
| `push` (any branch) | `commit_landed` | `branch`, `commit_sha`, `author` |
| `pull_request_merged` | `merged` | `pr_number`, `merged_by`, `target_branch` |

If the vocabulary lacks an entry (e.g. bug-tracker vocab without
`merged`), dispatch returns `ErrEventNotInVocabulary` and the handler
logs but still 204s.

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
  calls. Update the comment at line 25 to read "registers all 15
  MCP tools".
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
// list_my_work_items: scans every active instance for work items where
//   AssignedAgentID matches; returns the union.
// get_vocabulary: trivial wrapper over Store.GetVocabulary.
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
- Create: `docs/examples/vocabularies/sdlc.json` (8-event SDLC catalogue
  matching the table in "What canonical SDLC events look like" above).
- Create: `docs/examples/vocabularies/bug-tracker.json` (5-event
  bug-tracker catalogue).
- Modify: `cmd/admin/admin.go` — add `seed-vocabularies` subcommand
  that walks `docs/examples/vocabularies/` and inserts each vocab
  via `Store.CreateVocabulary` (idempotent: skip if name already
  exists). The daemon's `run()` function in `cmd/daemon/daemon.go`
  also calls this on startup so a fresh daemon always has the SDLC
  seed loaded — required by Task 5's e2e test.

`docs/examples/vocabularies/sdlc.json`:

```json
{
  "id": "voc_seed_sdlc",
  "name": "sdlc",
  "description": "Canonical software-delivery lifecycle vocabulary.",
  "release_event": "task_completed",
  "events": [
    {"id": "ve_seed_sdlc_assigned", "vocabulary_id": "voc_seed_sdlc", "event_type": "task_assigned",
     "message_template": "Task assigned: {{.WorkItem.Title}} → {{.AgentName}} ({{.Role}})",
     "metadata_keys": ["agent_id", "role", "workflow_id"]},
    {"id": "ve_seed_sdlc_branch", "vocabulary_id": "voc_seed_sdlc", "event_type": "branch_created",
     "message_template": "Branch created: {{index .Payload \"branch\"}}",
     "metadata_keys": ["branch", "commit_sha"]},
    {"id": "ve_seed_sdlc_commit", "vocabulary_id": "voc_seed_sdlc", "event_type": "commit_landed",
     "message_template": "Commit on {{index .Payload \"branch\"}}: {{index .Payload \"commit_sha\"}} by {{index .Payload \"author\"}}",
     "metadata_keys": ["branch", "commit_sha", "author"]},
    {"id": "ve_seed_sdlc_pr", "vocabulary_id": "voc_seed_sdlc", "event_type": "pr_opened",
     "message_template": "PR opened: #{{index .Payload \"pr_number\"}} on {{index .Payload \"branch\"}}",
     "metadata_keys": ["pr_number", "branch"]},
    {"id": "ve_seed_sdlc_review", "vocabulary_id": "voc_seed_sdlc", "event_type": "review_requested",
     "message_template": "Review requested: {{.WorkItem.Title}} (gate {{index .Payload \"gate_step_id\"}})",
     "metadata_keys": ["agent_id", "gate_step_id"]},
    {"id": "ve_seed_sdlc_tests", "vocabulary_id": "voc_seed_sdlc", "event_type": "tests_passing",
     "message_template": "Tests passing: {{index .Payload \"repo\"}} run {{index .Payload \"run_id\"}}",
     "metadata_keys": ["repo", "run_id"]},
    {"id": "ve_seed_sdlc_merged", "vocabulary_id": "voc_seed_sdlc", "event_type": "merged",
     "message_template": "Merged PR #{{index .Payload \"pr_number\"}} → {{index .Payload \"target_branch\"}} by {{index .Payload \"merged_by\"}}",
     "metadata_keys": ["pr_number", "merged_by", "target_branch"]},
    {"id": "ve_seed_sdlc_deployed", "vocabulary_id": "voc_seed_sdlc", "event_type": "deployed",
     "message_template": "Deployed {{index .Payload \"commit_sha\"}} to {{index .Payload \"env\"}}",
     "metadata_keys": ["commit_sha", "env"]},
    {"id": "ve_seed_sdlc_completed", "vocabulary_id": "voc_seed_sdlc", "event_type": "task_completed",
     "message_template": "Task completed: {{.WorkItem.Title}} → {{.AgentName}}",
     "metadata_keys": ["agent_id", "role", "workflow_id"]}
  ]
}
```

`docs/examples/vocabularies/bug-tracker.json`:

```json
{
  "id": "voc_seed_bugtracker",
  "name": "bug-tracker",
  "description": "Bug-tracker workflow vocabulary — proves the per-workflow vocab plug-in path.",
  "release_event": "bug_resolved",
  "events": [
    {"id": "ve_seed_bug_filed", "vocabulary_id": "voc_seed_bugtracker", "event_type": "bug_filed",
     "message_template": "Bug filed: {{.WorkItem.Title}} (priority {{.WorkItem.Priority}})",
     "metadata_keys": ["reporter"]},
    {"id": "ve_seed_bug_triaged", "vocabulary_id": "voc_seed_bugtracker", "event_type": "bug_triaged",
     "message_template": "Triaged: {{.WorkItem.Title}} severity {{index .Payload \"severity\"}}",
     "metadata_keys": ["severity"]},
    {"id": "ve_seed_bug_assigned", "vocabulary_id": "voc_seed_bugtracker", "event_type": "bug_assigned",
     "message_template": "Assigned bug {{.WorkItem.Title}} → {{.AgentName}}",
     "metadata_keys": ["agent_id"]},
    {"id": "ve_seed_bug_resolved", "vocabulary_id": "voc_seed_bugtracker", "event_type": "bug_resolved",
     "message_template": "Resolved: {{.WorkItem.Title}} by {{.AgentName}}",
     "metadata_keys": ["agent_id", "fix_commit"]},
    {"id": "ve_seed_bug_reopened", "vocabulary_id": "voc_seed_bugtracker", "event_type": "bug_reopened",
     "message_template": "Reopened: {{.WorkItem.Title}} (reason: {{index .Payload \"reason\"}})",
     "metadata_keys": ["reason"]}
  ]
}
```

`cmd/admin/admin.go` gains a `seed-vocabularies` subcommand. Run on
daemon startup from `cmd/daemon/daemon.go:run()` (after `infra.Open`,
before `NewServer`) — best-effort, logs warnings, does not block
startup.

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
- Modify: `tests/e2e/harness/fake_sharkfin.go` — ensure
  `RecordedPosts()` returns enough metadata to assert against
  (channel, content, metadata bytes). If it already does, no change.

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
//   - On Scheduler claim, FakeSharkfin received the rendered
//     "Task assigned: ..." string with metadata.event_type ==
//     "task_assigned" on the project's channel.
//   - On Combine pull_request_merged webhook, FakeSharkfin received
//     the rendered "Merged PR #..." string with
//     metadata.event_type == "merged".
//   - On Scheduler release, FakeSharkfin received the rendered
//     "Task completed: ..." string.
//   - GET /v1/projects/{id}/audit returns ONLY the project's events.
func TestBotVocabulary_SDLCRoundTrip(t *testing.T) { /* full impl */ }

// TestBotVocabulary_BugTrackerVocabPlugIn proves the per-workflow
// plug-in path:
//
//   - Create a second project bound to the bug-tracker vocabulary.
//   - Drive a claim/release for that project's name.
//   - Assert the recorded posts use the bug-tracker template strings
//     (Bug filed:, Resolved:), not the SDLC strings.
func TestBotVocabulary_BugTrackerVocabPlugIn(t *testing.T) { /* full impl */ }
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
      idempotently (verify with two consecutive starts — second log
      shows "vocabulary already loaded" warnings or silent skip).
- [ ] `internal/daemon/mcp_tools.go:25` comment reads "registers all
      15 MCP tools" and the file contains exactly 15 `s.AddTool(`
      invocations.
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
