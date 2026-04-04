# Flow Architecture Design

## Overview

Flow is a configurable workflow engine for the WorkFort platform. It manages
business processes — SDLC, onboarding, incident response, sales pipelines — as
directed graphs of steps that work items flow through.

Flow is **process-agnostic**. The SDLC is its first workflow template, but the
domain model makes no assumptions about software development. Steps, transitions,
guards, and actions are the primitives; specific processes are configurations.

**License**: GPL-v2.0-Only

## Design Principles

1. **Standalone capable, platform enhanced.** Flow is useful on its own but gains
   power when composed with other WorkFort services.
2. **API-first.** Every capability is available through the REST API and MCP.
   The UI is a consumer, never the only path.
3. **Federated authorization.** Flow references Hive role IDs but owns its own
   role-to-action mappings within processes. Hive says "who is what"; Flow says
   "what can that role do here." Role identity is synchronized via Hive role IDs,
   not string matching, to avoid silent breakage on role renames.
4. **Pluggable integrations.** External services (Git forges, chat) connect through
   port interfaces with swappable adapters. Combine and GitHub are both valid Git
   forge backends.
5. **Chat as command surface.** Day-to-day interaction flows through Sharkfin. Flow
   operates a bot identity that receives structured commands and posts state changes
   to channels.
6. **Transitions are intentional acts.** The workflow engine never auto-advances
   work items. Every transition is triggered by an agent, a human, or an explicit
   automation rule. Integration hooks fire side effects on transitions but cannot
   trigger further transitions, preventing runaway loops.
7. **Automation is separate from workflow.** The workflow engine owns state
   transitions. Side effects (notifications, issue creation, agent assignment) are
   configurable hooks on transitions, not embedded in workflow logic. This follows
   the Jira pattern of separating workflow state from automation concerns.

## Design Rationale

These decisions are informed by a cross-platform analysis of ServiceNow,
Atlassian Jira, and Salesforce:

### Graph Model: Directed Graph (not DAG)

All three major platforms (ServiceNow, Jira, Salesforce) support cycles in their
workflow graphs. Common patterns like "reject from Code Review, send back to
Implementation" are natural cycles — not errors. A DAG-only model would force
artificial workarounds (versioned rework items) that don't match how users think
about processes.

Cycles are safe because transitions are always intentional acts — the engine
validates whether a transition is legal but never auto-fires one. Integration
hooks cannot trigger transitions, preventing hook → transition → hook loops. The
audit trail (transition history on every work item) provides visibility into
excessive cycling, which is a monitoring concern rather than a graph constraint.

### Guard Expressions: CEL (Common Expression Language)

Rather than building a custom DSL or using JS-like scripting (ServiceNow) or
fixed condition types (Jira), Flow uses CEL via the `google/cel-go` library.

CEL is non-Turing-complete (safe to evaluate, no infinite loops), readable by
non-developers, and battle-tested in Kubernetes admission policies and Google
Cloud IAM. It provides the right level of expressiveness — more powerful than
Jira's fixed condition composition, safer than ServiceNow's Glide scripting.

Example guard: `assignee.role_id == "reviewer" && item.fields.tests_passing == true`

### State Ownership: State on the Record

All three platforms agree: state lives on the work item itself, not in a separate
workflow tracking table. The work item's `CurrentStepID` is the single source of
truth for where it is in the process.

### Approval as First-Class Concept

ServiceNow and Salesforce both treat approvals as dedicated mechanisms, not
generic transitions. Jira's lack of built-in approval is widely considered a gap.
Flow's `gate` step type has rich approval semantics: configurable approver count
(N of M), unanimous vs first-response modes, delegation, and rejection with
comments.

### Instance as Lightweight Config Container

The Workflow Instance is not a heavy orchestration entity. It serves the role of
Jira's "workflow scheme" or ServiceNow's "assignment group" — a binding layer
that connects a template to a team and its integration configuration. The work
item is the real entity that agents interact with.

## Platform Context

Flow sits between identity and artifacts in the WorkFort service graph:

```
Passport (auth)
  |
  +-- Hive (identity: agents, roles, teams)
  |     |
  |     +-- Flow (process: workflows, work items, transitions)
  |           |
  |           +-- Combine / GitHub (artifacts: repos, issues, commits)
  |
  +-- Sharkfin (communication: chat, bots, activity log)
  |
  +-- Pylon (service registry) --> Scope (desktop shell / BFF)
```

### Service Relationships

| Dependency | Type | Purpose |
|------------|------|---------|
| **Passport** | Hard | JWT/API key auth via `service-auth` Go library |
| **Hive** | Hard | Agent/role/team identity resolution via role IDs |
| **Sharkfin** | Hard | Bot identity for commands, notifications, audit trail |
| **Combine** | Soft | Git forge adapter (one of many possible backends) |
| **GitHub** | Soft | Alternative Git forge adapter |

Hard dependencies are always present. Soft dependencies are pluggable — a
deployment may use Combine, GitHub, both, or neither.

### Sharkfin as Front Door

Sharkfin is the primary command surface for the WorkFort platform. Users and
agents interact with Flow primarily through Sharkfin chat. Flow registers a
**bot identity** in Sharkfin that:

- Receives structured commands from agents and users
- Posts state change notifications to team channels
- Provides status queries on demand

Two types of Sharkfin participants interact with Flow:

- **Agents** — AI workers with Hive identities, having conversations about work
- **Bots** — service avatars representing Flow, Combine, Hive, etc.

When an agent sends a structured message to the Flow bot in Sharkfin, three
things happen simultaneously:

1. The message is recorded in Sharkfin (conversation history, visible to team)
2. The bot forwards it to Flow's API (state transition processed)
3. The bot responds in the channel (confirmation, new assignments)

This makes the Sharkfin channel a natural dashboard — every state change is a
chat event, and the conversation *is* the audit log.

## Tech Stack

| Layer | Library | Notes |
|-------|---------|-------|
| CLI framework | cobra | Command factory pattern |
| Config | viper | XDG paths, YAML, env vars (FLOW_*) |
| Logging | charmbracelet/log | JSON structured logging to file |
| HTTP framework | huma/v2 | OpenAPI-first, consistent with Hive |
| Storage | modernc.org/sqlite + pgx/v5 | Dual SQLite/PostgreSQL |
| Migrations | goose | SQL migration files |
| Guard expressions | google/cel-go | CEL evaluation for transition guards |
| MCP | mcp-go | stdio-to-HTTP bridge |
| Auth | Work-Fort/Passport/go/service-auth | JWT + API key validation |
| Build | mise | Go version + task management |

### Components

One binary (`flow`), three modes:

- `flow daemon` — HTTP server, systemd user service
- `flow mcp-bridge` — stdio-to-HTTP MCP bridge for Claude Code
- `flow admin` — CLI admin commands (seed, migrate, export/import)

## Domain Model

### Entity Relationships

```
Workflow Template
  |-- has many --> Steps (graph nodes)
  |-- has many --> Transitions (graph edges, may form cycles)
  |-- has many --> Role Mappings (role ID -> allowed actions per step)
  |-- has many --> Integration Hooks (transition events -> adapter calls)
  |
  +-- instantiated as --> Workflow Instance (lightweight config container)
                            |-- bound to --> Team (Hive ID ref)
                            |-- configured with --> Integration Configs
                            |-- contains --> Work Items
                                              |-- at --> current Step
                                              |-- assigned to --> Agent (Hive ID ref)
                                              |-- linked to --> External Resources
                                              |-- has --> Transition History
```

### Workflow Template

A reusable process blueprint. Defines the structure of a process without binding
it to a specific team or project.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| Name | string | Human-readable name (e.g., "SDLC", "Incident Response") |
| Description | string | Purpose and scope of this process |
| Version | int | Template versioning — instances snapshot the version at creation |
| Steps | []Step | Nodes in the workflow graph |
| Transitions | []Transition | Edges in the workflow graph (may form cycles) |
| RoleMappings | []RoleMapping | Role-to-action permissions per step |
| IntegrationHooks | []IntegrationHook | Side effects triggered on transition events |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |

### Step

A node in the workflow graph. Represents a stage that work items pass through.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| TemplateID | UUID | Parent template |
| Name | string | e.g., "Planning", "Implementation", "Code Review" |
| Type | enum | `task`, `gate` |
| Position | int | Ordering hint for display |

**Step types:**

- `task` — standard work step, requires agent action to complete
- `gate` — approval checkpoint with rich semantics (see Approval Model below)

**Note:** `parallel_fork` and `parallel_join` step types are deferred to v2.
The SDLC template and common enterprise workflows (HR, sales, incident
management) do not require parallel execution in v1.

### Approval Model (Gate Steps)

Gate steps have dedicated approval semantics, not composed from raw transitions:

| Field | Type | Description |
|-------|------|-------------|
| StepID | UUID | The gate step this config applies to |
| Mode | enum | `any` (first approver wins) or `unanimous` (all must approve) |
| RequiredApprovers | int | Number of approvals needed (for `any` mode) |
| ApproverRoleID | UUID | Hive role ID — agents with this role can approve |
| RejectionStepID | UUID | Where to send work items on rejection (optional) |

Approvals are tracked per work item:

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| WorkItemID | UUID | |
| StepID | UUID | The gate step |
| AgentID | UUID | Who approved/rejected |
| Decision | enum | `approved`, `rejected` |
| Comment | string | Reason for decision |
| Timestamp | timestamp | |

### Transition

An edge in the workflow graph. Defines how work items can move between steps.
Transitions may form cycles (e.g., reject from Code Review back to
Implementation).

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| TemplateID | UUID | Parent template |
| FromStepID | UUID | Source step |
| ToStepID | UUID | Destination step |
| Name | string | Human-readable label (e.g., "Approve", "Reject", "Send Back") |
| Guard | string | CEL expression evaluated against work item context (optional) |
| RequiredRoleID | UUID | Hive role ID required to trigger this transition |

**Guard expression context** — CEL expressions have access to:

```
item.title           -- work item title
item.priority        -- priority enum value
item.fields          -- custom field values (map)
item.step            -- current step name
actor.role_id        -- triggering agent's role ID
actor.agent_id       -- triggering agent's ID
approval.count       -- number of approvals on current step
approval.rejections  -- number of rejections on current step
```

### Workflow Instance

A lightweight config container that binds a template to a team and its
integration configuration.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| TemplateID | UUID | Which template this was created from |
| TemplateVersion | int | Snapshot of template version at creation |
| TeamID | UUID | Hive team ID reference |
| Name | string | Instance name (e.g., "Nexus SDLC", "Q2 Onboarding") |
| Status | enum | `active`, `paused`, `completed`, `archived` |
| IntegrationConfigs | []IntegrationConfig | Configured adapters for this instance |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |

### Work Item

The central entity. A unit of work flowing through a workflow instance. State
lives on the work item itself (not in a separate tracking table).

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| InstanceID | UUID | Parent workflow instance |
| Title | string | Short description |
| Description | string | Full details |
| CurrentStepID | UUID | Where in the workflow this item currently sits |
| AssignedAgentID | UUID | Hive agent ID reference (optional) |
| Priority | enum | `critical`, `high`, `normal`, `low` |
| Fields | JSON | Custom fields (process-specific data, available to CEL guards) |
| ExternalLinks | []ExternalLink | Links to Combine issues, GitHub PRs, etc. |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |

### External Link

Connects a work item to a resource in another service.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| WorkItemID | UUID | Parent work item |
| ServiceType | string | e.g., "git-forge", "chat", "document" |
| Adapter | string | e.g., "combine", "github" |
| ExternalID | string | ID in the external service |
| URL | string | Deep link (optional) |

### Transition History

Audit trail for every state change on a work item. Every transition is recorded
regardless of whether it moves forward or cycles back.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| WorkItemID | UUID | |
| FromStepID | UUID | |
| ToStepID | UUID | |
| TransitionID | UUID | Which transition definition was used |
| TriggeredBy | UUID | Agent ID or bot ID |
| Reason | string | Comment or context |
| Timestamp | timestamp | |

### Role Mapping

Maps Hive role IDs to permitted actions within a specific step of a template.
Role identity is synchronized from Hive via role IDs (not string names) to
prevent silent breakage on role renames.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| TemplateID | UUID | |
| StepID | UUID | |
| RoleID | UUID | Hive role ID |
| AllowedActions | []string | e.g., ["transition", "assign", "comment"] |

### Integration Hook

Side effects triggered on transition events. Hooks fire adapter calls but
**cannot trigger further transitions** — this is the key safety invariant
preventing runaway loops.

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Primary key |
| TemplateID | UUID | |
| TransitionID | UUID | Which transition triggers this hook |
| Event | enum | `on_transition` |
| AdapterType | string | e.g., "git-forge", "chat" |
| Action | string | Adapter-specific action (e.g., "create_issue", "post_message") |
| Config | JSON | Parameters for the action |

**Note:** Hooks are now attached to transitions (not steps), aligning with the
research finding that transitions are the core primitive — the interesting logic
lives on the edges, not the nodes.

## Port Interfaces

Flow's domain layer defines port interfaces. The infrastructure layer provides
adapters.

### GitForge (v1: issues only)

Operations on code repositories and issues. Merge request operations are deferred
to v2 to manage Combine's implementation scope.

```
CreateIssue(repo, title, body, labels) -> IssueID
UpdateIssueStatus(repo, issueID, status) -> void
GetIssue(repo, issueID) -> Issue
LinkCommit(repo, issueID, commitSHA) -> void
ListIssues(repo, filters) -> []Issue
RegisterWebhook(repo, events, callbackURL) -> WebhookID
```

### Chat

Operations on the messaging layer (Sharkfin).

```
PostMessage(channel, content, structured) -> MessageID
PostBotUpdate(channel, workItemID, transition) -> MessageID
CreateChannel(name, members) -> ChannelID
SubscribeToCommands(channel, commandPrefix) -> Subscription
```

### Identity

Operations on the agent identity layer (Hive). Uses Hive role/agent IDs, not
string names.

```
ResolveAgent(agentID) -> Agent
ResolveRole(roleID) -> Role
GetTeamMembers(teamID) -> []Agent
GetAgentRoles(agentID) -> []Role
SyncRoles(since timestamp) -> []RoleChange
```

## Adapters

### Infrastructure Layer

```
infra/
  combine/       -- Combine Git forge adapter (REST API + webhooks)
  github/        -- GitHub Git forge adapter (GitHub API + webhooks) [future]
  sharkfin/      -- Sharkfin chat adapter (bot identity, messaging)
  hive/          -- Hive identity adapter (agent/role resolution)
  sqlite/        -- SQLite store implementation
  postgres/      -- PostgreSQL store implementation
  httpapi/       -- REST API handlers
  mcp/           -- MCP tool handlers + stdio bridge
```

### Webhook Architecture

**Inbound** — Flow exposes `POST /v1/webhooks/{adapter}` endpoints:

- Combine/GitHub push events -> check if linked to a work item, update status
- Combine/GitHub issue events -> sync status back to work item

**Outbound** — Integration hooks on transitions fire adapter calls:

- Transition to "Implementation" -> create Combine issue, post to Sharkfin
- Transition "Approve" on Code Review -> update Combine issue status
- Transition to "QA" -> assign QA agent, notify in Sharkfin channel
- Transition to "Done" -> close Combine issue, post summary

**Safety invariant:** Inbound webhooks can update work item metadata and external
links but cannot trigger transitions. Only agents, humans, or explicit automation
rules can trigger transitions.

### Sharkfin Bot

Flow registers a bot identity in Sharkfin. The bot:

- **Receives** structured commands: `@flow transition #42 to review`
- **Posts** state changes: "Work item #42 moved to Code Review, assigned to @reviewer"
- **Provides** status on request: `@flow status #42` -> current step, assignee, history

All bot interactions are recorded in the Sharkfin channel, creating the audit
trail and activity dashboard.

## Combine Additions (v1)

To support standalone viability, Combine needs a lightweight issue tracker.
These are minimal — Flow projects richer state onto them when composed.

Merge requests are deferred to v2.

### Issue Model

| Field | Type | Description |
|-------|------|-------------|
| ID | int | Auto-increment per repo |
| RepoID | int | Parent repository |
| AuthorID | int | User who created it |
| Title | string | Short description |
| Body | string | Full details (markdown) |
| Status | enum | `open`, `in_progress`, `closed` |
| Resolution | enum | `fixed`, `wontfix`, `duplicate`, `null` |
| Labels | []string | Categorization tags |
| AssigneeID | int | Assigned user (optional) |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |
| ClosedAt | timestamp | When status changed to closed (optional) |

### State Ownership Between Flow and Combine

When Combine is deployed standalone, issue status is managed directly by users.

When Combine is composed with Flow, Flow is the authoritative source for process
state. Flow projects status onto Combine issues via the Git forge adapter (e.g.,
a work item entering "Implementation" sets the Combine issue to `in_progress`).
Combine's inbound webhooks notify Flow of direct changes for reconciliation, but
Flow's process state takes precedence.

### Combine Webhook Events (additions)

| Event | Payload | Fires when |
|-------|---------|------------|
| `issue_opened` | Issue | New issue created |
| `issue_status_changed` | Issue + old/new status | Status transitions |
| `issue_closed` | Issue + resolution | Issue closed |

These extend Combine's existing webhook events (push, branch_tag_create/delete,
collaborator, repository, repository_visibility_change).

## SDLC Template (First Implementation)

The first workflow template, used by WorkFort to manage its own development:

```
Backlog --> Planning --> Approval --> Implementation --> Code Review --> QA --> Done
                            |  ^                            |
                            |  |                            |
                            +--+                            +---> Implementation
                        (reject: back                    (reject: back to
                         to Planning)                     Implementation)
```

### Steps

| Step | Type | Description |
|------|------|-------------|
| Backlog | task | Unplanned work items awaiting triage |
| Planning | task | Agent develops plan, spec, or approach |
| Approval | gate | Product Manager approves or rejects plan |
| Implementation | task | Agent implements the planned work |
| Code Review | gate | Reviewer approves or rejects implementation |
| QA | task | QA agent tests the implementation |
| Done | task | Work complete, issue closed |

### Transitions

| From | To | Name | Required Role | Guard |
|------|----|------|---------------|-------|
| Backlog | Planning | Triage | Product Manager | — |
| Planning | Approval | Submit Plan | Planner | — |
| Approval | Implementation | Approve | Product Manager | — |
| Approval | Planning | Reject Plan | Product Manager | — |
| Implementation | Code Review | Submit for Review | Developer | — |
| Code Review | QA | Approve | Reviewer | — |
| Code Review | Implementation | Request Changes | Reviewer | — |
| QA | Done | Pass | QA Tester | — |
| QA | Implementation | Fail | QA Tester | — |

### Integration Hooks

| Transition | Hook | Adapter |
|------------|------|---------|
| Triage (Backlog → Planning) | Assign planner agent | Hive |
| Submit Plan (Planning → Approval) | Post plan summary to channel | Sharkfin |
| Approve (Approval → Implementation) | Create Combine issue and branch | GitForge |
| Submit for Review (Impl → Code Review) | Assign reviewer agent, post to channel | Hive, Sharkfin |
| Approve (Code Review → QA) | Assign QA agent, post review result | Hive, Sharkfin |
| Request Changes (Code Review → Impl) | Post review feedback to channel | Sharkfin |
| Pass (QA → Done) | Close Combine issue, post summary | GitForge, Sharkfin |
| Fail (QA → Impl) | Post failure details to channel | Sharkfin |

### Role Mappings

| Role | Allowed Actions by Step |
|------|------------------------|
| Product Manager | Backlog: transition, assign. Approval: approve, reject |
| Planner | Planning: transition, comment |
| Developer | Implementation: transition, comment |
| Reviewer | Code Review: approve, reject, comment |
| QA Tester | QA: transition, comment |

## API Surface

### REST API (`/v1`)

**Templates:**
- `GET /templates` — list workflow templates
- `POST /templates` — create a template
- `GET /templates/{id}` — get template with steps and transitions
- `PATCH /templates/{id}` — update template
- `DELETE /templates/{id}` — delete template (if no active instances)

**Instances:**
- `GET /instances` — list workflow instances (filterable by team)
- `POST /instances` — create instance from template
- `GET /instances/{id}` — get instance with current state
- `PATCH /instances/{id}` — update instance (pause, resume, archive)

**Work Items:**
- `POST /instances/{id}/items` — create work item in an instance
- `GET /instances/{id}/items` — list work items (filterable by step, agent, priority)
- `GET /items/{id}` — get work item with history
- `PATCH /items/{id}` — update work item (title, description, assignment, fields)
- `POST /items/{id}/transition` — trigger a step transition
- `GET /items/{id}/history` — get transition history

**Approvals (on gate steps):**
- `POST /items/{id}/approve` — approve a work item at a gate step
- `POST /items/{id}/reject` — reject a work item at a gate step
- `GET /items/{id}/approvals` — list approval decisions for a work item

**Webhooks:**
- `POST /webhooks/{adapter}` — inbound webhook receiver

**Health:**
- `GET /v1/health` — service health
- `GET /ui/health` — Pylon service discovery endpoint

### MCP Tools

Full parity with REST API:

| Tool | Description |
|------|-------------|
| `list_templates` | List available workflow templates |
| `get_template` | Get template details |
| `create_instance` | Start a workflow from a template |
| `list_instances` | List active workflow instances |
| `create_work_item` | Add a work item to an instance |
| `list_work_items` | List work items with filters |
| `get_work_item` | Get work item details and history |
| `transition_work_item` | Move a work item to the next step |
| `approve_work_item` | Approve at a gate step |
| `reject_work_item` | Reject at a gate step |
| `assign_work_item` | Assign an agent to a work item |
| `get_instance_status` | Dashboard view of an instance |

## Broader Platform Context

This section captures architectural context from the initial design discussion
that informs Flow's role in the WorkFort platform.

### WorkFort as a Business Operating System

WorkFort is a composable business platform — like ServiceNow or Oracle, but
designed for AI agent integration from day one. Flow is a core primitive that
different business functions plug into. SDLC is the first use case, but the
engine supports any configurable business process (HR, sales, incident
management, etc.).

### Dogfooding: WorkFort Builds WorkFort

The near-term goal is for each WorkFort service (Nexus, Hive, Sharkfin, Combine,
Flow, Scope, Pylon, Passport) to have its own AI development team managed
through Flow's SDLC workflow. Flow must support multi-project orchestration from
the start — not just "one team, one repo" but "multiple teams across multiple
repos with a single orchestration layer."

### Virgil: The Onboarding Agent

Virgil is a concierge agent with full platform access that guides new users
through initial setup. In the greenfield case, Virgil helps users create their
first team, import repos, set up workflows. Virgil has full MCP access to all
services including Flow, which is one reason MCP parity with the REST API is a
hard requirement. After bootstrapping, Virgil's permissions are locked down — the
platform's RBAC secures access post-onboarding.

### Sharkfin Participants: Agents vs Bots

Sharkfin has two types of participants:

- **Agents** — AI workers with Hive identities, having real conversations
- **Bots** — service avatars representing Flow, Combine, Hive, etc.

When an agent sends a structured message to the Flow bot, the message is recorded
in Sharkfin (audit trail), the bot processes it via Flow's API (state change),
and the bot responds in the channel (confirmation). The channel becomes both a
dashboard and an activity log.

### Import/Export and the Package Registry

Workflow templates are portable JSON files conforming to a published schema (see
`docs/workflow-schema-draft.json`). The long-term vision includes a centralized
public registry where users can publish, discover, fork, and share:

- Workflow templates (`workflow.json`)
- Team/role definitions (`team.json` — Hive export)
- Agent configurations (`agents.json` — Hive export)

These are three separate files, each with its own schema, importable
independently into their respective service. When bundled together, Virgil
orchestrates the full import — creating the team in Hive, the workflow in Flow,
and wiring role mappings together.

Templates use **semantic role names** (not Hive UUIDs) for portability. On
import, the consuming Flow instance maps names to local Hive role IDs.
Integration hooks reference adapter *types* (`git-forge`, `chat`), not specific
services — the target Combine instance or Sharkfin channel is configured at the
Instance level, not baked into the template.

## Future Considerations

### v2 Scope

- **Parallel fork/join step types** — for processes requiring concurrent work
  tracks (e.g., implementation and documentation in parallel)
- **Merge request support in GitForge port** — requires Combine MR model
- **SLA tracking** — time-based alerts on work items (similar to ServiceNow's
  Task SLA model)
- **Workflow template registry** — centralized public service for sharing
  workflow templates, team definitions, and agent configurations
- **Package manifest format** — ties workflow.json + team.json + agents.json
  together with cross-reference mappings
- **Additional Git forge adapters** — GitLab, Gitea, Bitbucket

### OpenSpec Integration

Combine could recognize OpenSpec directory structures (`openspec/` with
`proposal.md`, `specs/`, `design.md`, `tasks.md`) within repositories and
surface them in the UI. When composed with Flow, a new OpenSpec in a repo could
trigger a webhook that auto-creates a work item at the Planning step of an SDLC
workflow. The spec becomes the planning artifact, living in the code (Combine),
with Flow orchestrating the process around it.
