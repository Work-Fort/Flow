# Design: Agent Assignment + Runtime Pooling

Status: **Design.** No implementation yet. Cross-repo: changes land in
Hive, Flow, and (docs only) Adjutant. Sharkfin already has the bot
infrastructure assumed by this design.

## Problem

WorkFort wants 10–20 agents that workflows can dispatch work to on
demand. Each agent has its own subscription auth (per the per-agent
credentials work) so they refresh independently. Work spans multiple
roles (planner, developer, reviewer, qa-tester, ...) across multiple
projects (flow, nexus, hive, combine, sharkfin, ...). The work must
be visible to humans in real time.

## Layering

Three layers; the only pool that matters in this design is Nexus's
underlying capacity to provision compute on demand. VMs aren't
pre-warmed — they exist when an agent has work, otherwise they're
shut down.

| Layer | What | Lifecycle |
|---|---|---|
| **Agents** | Identities (Passport user + Sharkfin user + Hive agent record) plus a current assignment. | Permanent. Enumerated, not pooled. Workflows claim agents (set assignment); when assignment is null the agent is free. |
| **Runtimes** | What actually executes the agent's loop. Today: `adjutant-claude-cli` in a Nexus VM. Future: a single `adjutant-go-adk` process hosting many agents at once. | claude-cli: a **fungible VM pool** — Flow picks one from the pool on claim, attaches the agent's drives, starts it; on release the VM stops and returns to the pool. direct-api: long-lived process(es) holding many agents in-process. |
| **Action infrastructure** | Where agent-initiated work runs (builds, tests, file edits). For claude-cli the agent's own VM (conflated with runtime). For direct-api a per-agent sandbox VM started on assignment. | Ephemeral in both cases — tied to the assignment lifecycle. |
| **Nexus capacity** | CPU, memory, disk, network — the resources VMs are provisioned into. | The real pool concern. Finite. If 20 agents are all assigned simultaneously, Nexus needs capacity to host 20 runtime VMs (claude-cli) or 20 sandbox VMs (direct-api). |

The agent-side design (identity, assignment, claim semantics,
coordination) is **runtime-agnostic**. The runtime-side design is
specific to each runtime variant.

## Agent identity

Identity per agent is **static**. Role is **contextual**.

| Layer | What | Lifetime |
|---|---|---|
| Passport user | `agent-3` | Permanent — the OAuth-bearing identity. |
| Sharkfin user | `agent-3` | Permanent — resolved from Passport. |
| Hive agent record | `agent-3` | Permanent — what `get_provisioning` returns for. |
| **Current assignment** | `{ role, project, workflow_id, lease_expires_at } \| null` | Dynamic — set when claimed, cleared when released. |

The existing `agent-1`/`agent-2`/`agent-3` Nexus VMs share names with
their agent identities because that's how end-to-end testing was set
up. **That is incidental, not a design property.** A VM's name is
operator-facing infrastructure metadata; the agent identity is the
durable thing.

When `agent-3` posts in `#flow`, Sharkfin shows `agent-3` (the
identity). Whichever VM (or runtime instance) executed the
message-sending adjutant process is operationally invisible to
Sharkfin readers.

## Assignment model (Hive)

A single nullable field on the Hive agent record:

```
current_assignment: { role, project, workflow_id, lease_expires_at } | null
```

Semantics:

- `null` ⇒ agent is free (claimable).
- non-null ⇒ agent is claimed by `workflow_id`, working as `role` on `project`, until `lease_expires_at`.
- There is no separate "claimed" boolean. Having an assignment **is** being claimed.
- An agent cannot be claimed without an assignment.

Endpoints:

- `POST /v1/agents/claim` — body `{ role, project, workflow_id, lease_ttl }`. Atomic CAS: among free agents, pick one and set its `current_assignment`. Returns the chosen agent, or 409 if pool is exhausted.
- `POST /v1/agents/{id}/release` — atomic CAS from non-null to null. Caller must supply the same `workflow_id` to prevent stealing a release across workflows.
- `POST /v1/agents/{id}/renew` — extends `lease_expires_at` if `current_assignment.workflow_id` matches. **Called by Flow itself** to keep its own claims alive (see "Liveness and lease renewal" below). Not an agent-facing protocol.
- Background sweeper: clears `current_assignment` when `lease_expires_at` is past. Recovers from Flow crashes.
- `GET /v1/agents?assigned=false` — pool query for free agents (or `assigned=true&workflow_id=...` for inspection).

Note: Hive doesn't track "what was this agent doing last time."
"Warm" routing (preferring agents whose runtime state already
matches the new request, to avoid rebind cost) is a runtime-side
optimization deferred to future work.

### Liveness and lease renewal

**There is no agent-facing heartbeat protocol.** Agents don't send
`lease_renew` messages; Flow doesn't require them to. Liveness is a
runtime-level concern — Flow asks the runtime "is this agent doing
something" and gets a real answer.

- **claude-cli runtime**: Flow watches the agent's VM. If `claude`
  is running and not blocked in `wait_for_messages`, the agent is
  working. If `claude` exited cleanly, the iteration is done. If
  the process is gone unexpectedly, the agent is dead.
- **direct-api runtime (future)**: the Adjutant runtime tracks each
  agent's coroutine state directly. Long-running tool actions are
  bounded by runtime-enforced timeouts. No external monitoring
  needed.

The `lease_expires_at` field exists only as a **Flow-crash safety
net**. While Flow is alive and holding a claim, its background
worker periodically calls `renew` on Hive to push the lease out.
If Flow crashes mid-workflow, no one renews; the lease eventually
expires; Hive's sweeper clears the assignment so the agent isn't
stuck claimed forever.

Lease TTL should be short enough to bound recovery time
(e.g. 2 minutes) and renewal interval should be well under that
(e.g. 30 seconds).

## Coordination (Flow + Sharkfin)

Runtime-agnostic. The agent doesn't care whether it's running in a
VM or in a direct-API process.

### Flow — workflow + scheduler + audit

- **Project-level tasks.** Flow holds the multi-step, multi-role
  lifecycle (e.g. "implement feature X in flow project"). Each task
  has phases, transitions, and a record of every state change. This
  is the audit primary.
- **Per-project bots.** One Sharkfin identity per project
  (`flow-bot`, `nexus-bot`, `hive-bot`, `combine-bot`, `sharkfin-bot`).
  Each bot lives in its project's channel.
- **Scheduler / orchestrator.** Flow owns the full assignment +
  runtime lifecycle. When a workflow needs an agent for
  `(role, project)`:
  1. Calls `POST /v1/agents/claim` on Hive. Hive picks any free
     agent and sets its `current_assignment`.
  2. Creates an immediate task in Hive assigned to that agent, with
     a `flow_task_ref` field linking back to the project task.
  3. Brings runtime online for the agent. Per the runtime variant:
     - **claude-cli**: Flow assigns a VM to the agent (picks one
       from the Nexus VM pool capable of hosting
       adjutant-claude-cli, attaches the agent's per-agent
       credentials drive and `(agent, role, project)` working
       drive, starts the VM).
     - **direct-api** (future): Flow starts the agent's sandbox
       VM. The agent's coroutine in the long-lived Adjutant
       process picks up the assignment and uses the VM for
       actions.
  4. Has the project bot DM the agent in Sharkfin: human-readable
     instruction in the body, structured payload in `metadata`
     including the Hive task ID and Flow task ref.
  5. Posts a public message in the channel announcing the
     assignment (audit visibility).
- **Release.** On task completion (or lease expiry), Flow:
  1. Stops the runtime (claude-cli: stop the VM, detach drives,
     return VM to pool; direct-api: stop the sandbox VM).
  2. Calls `POST /v1/agents/{id}/release` on Hive to clear the
     assignment.
  3. Posts release announcement to the project channel.
- **Triggers Combine actions.** When workflow policy is satisfied
  (e.g. reviewer approved + qa passed), Flow calls Combine to merge.
  Agents never hold Combine write tokens.
- **Audit log.** Every workflow transition, agent claim/release,
  bot message, and Combine action lands in Flow's own event log.

### Hive — identity + assignment + immediate tasks

- Existing Hive `tasks` table extended with a `flow_task_ref` field
  (free-form string; Hive doesn't interpret it).
- `get_provisioning` is extended: when an agent has a
  `current_assignment`, it returns the role document for that role
  and the project context. The existing permission-filtering for
  tools applies per role.

### Sharkfin — coordination interface

Already has what's needed (migrations 009–013):
- Bot role + permissions
- Per-identity webhook registration
- Message metadata field
- Channel-member webhook delivery

Each project bot registers as `type: "service"`, joins its project
channel, registers a webhook back to Flow. Flow receives webhooks for
every channel message; parses `metadata` for structured events;
sends bot messages with `metadata: { event_type, event_payload }`
plus a human-readable body.

### Adjutant — no code change

Role docs (in Hive) tell agents:
- Which project bot to message for which intent
- The structured metadata vocabulary
- What goes in human-readable body vs metadata sidecar

The agent's existing `wait_for_messages` loop is the right primitive:
bot DMs agent → loop wakes → role doc tells agent how to respond.

## Bot vocabulary

Mechanical — no LLM interpretation, just a flat map from `event_type`
to handler.

| Direction | event_type | event_payload | Meaning |
|---|---|---|---|
| Bot → agent | `task_assigned` | `{ hive_task_id, flow_task_ref, role, project }` | You have a new immediate task. |
| Agent → bot | `task_started` | `{ hive_task_id }` | I've begun work. |
| Agent → bot | `request_review` | `{ hive_task_id, branch, pr_url }` | I've pushed; please route to reviewer. |
| Agent → bot | `task_completed` | `{ hive_task_id, summary }` | Work is done. |
| Agent → bot | `blocked` | `{ hive_task_id, reason, needs }` | I cannot proceed. |
| Bot → agent | `lease_expiring` | `{ hive_task_id, expires_at }` | Renew or release. |
| Bot → channel | `agent_assigned` | `{ agent, role, project, workflow_id }` | Audit/visibility. |
| Bot → channel | `agent_released` | `{ agent, hive_task_id, outcome }` | Audit/visibility. |

Off-script messages (no recognized `event_type`, or unknown
vocabulary) are not silently ignored — the bot replies in the channel
with "didn't understand X" and the failed handoff lands in the audit
transcript.

## Lifecycle in one go

A reviewer is needed for a flow PR.

1. **Flow scheduler:** "I need an agent for `(reviewer, flow)`."
2. **Atomic claim:** `POST /v1/agents/claim` succeeds against
   `agent-7`, sets its `current_assignment` to
   `{ role: "reviewer", project: "flow", workflow_id: "flow-task-117", lease_expires_at: now+30m }`.
3. **Hive immediate task:** Flow creates Hive task `TK-883` assigned
   to agent-7, with `flow_task_ref="flow-task-117"`.
4. **Bot DMs agent:** in `#flow`, `flow-bot` DMs `agent-7`
   with body "Review PR #42 (branch feature/foo)" and
   `metadata: { event_type: "task_assigned", event_payload: { hive_task_id: "TK-883", flow_task_ref: "flow-task-117", role: "reviewer", project: "flow" } }`.
5. **Bot announces in channel:** `flow-bot` posts to `#flow`:
   "agent-7 is reviewing PR #42 (TK-883)" with
   `metadata: { event_type: "agent_assigned", ... }`.
6. **Agent wakes** from `wait_for_messages`, calls
   `hive.get_provisioning` (which now returns reviewer role for flow
   project), reviews the PR, posts comments, decides
   approve/changes-requested.
7. **Agent → bot:** sends to `#flow` with
   `metadata: { event_type: "task_completed", event_payload: { hive_task_id: "TK-883", outcome: "approved" } }`.
8. **Flow records completion** in its audit log; if workflow policy
   says "approved + qa passed → merge", Flow calls Combine to merge.
9. **Hive release:** `POST /v1/agents/agent-7/release` with the
   matching workflow_id atomically clears `current_assignment` to
   null. agent-7 is now free.
10. **Bot announces release:** `flow-bot` posts to `#flow` with
    `metadata: { event_type: "agent_released", ... }`.

## claude-cli runtime — how this manifests today

In the current runtime, each agent runs in its own Nexus VM with
adjutant-claude-cli inside. The VM IS both the runtime and the
action infrastructure.

### VM lifecycle is tied to the assignment

VMs in the claude-cli pool are fungible — they're identical Nexus
VMs capable of hosting adjutant-claude-cli with any agent's drives
mounted. Flow picks one on claim and returns it on release. VMs
are not pre-warmed; they're started/stopped on demand.

| Trigger | What Flow does |
|---|---|
| Hive `claim` succeeds for `agent-3` on a work item | Picks a VM from the claude-cli pool, attaches `agent-3`'s per-agent credentials drive + the work item's drive (forking the project master if it's a new work item), starts the VM. |
| Agent finishes its phase but the work item continues (e.g. developer done, reviewer next) | Stops the VM, detaches drives. The work item's drive persists for the next agent. |
| Work item completes (merged, rejected, or canceled) | Same as above, plus the work item's drive is **deleted**. |

Effective relationship: **1:1 agent ↔ VM while assigned**. 20 agents
exist; if 5 are currently assigned, 5 VMs are running. Which
specific VM hosts agent-3 today may differ from tomorrow.

### Drives

Three kinds, with very different lifetimes:

| Drive | Owned by | Holds | Mounted at | Lifetime |
|---|---|---|---|---|
| Per-agent credentials | Agent identity | Anthropic OAuth credentials | `/root/.claude/` | Permanent. Attaches when the agent's VM starts; detaches on stop. |
| Per-project source master | Project | Source at HEAD plus warm state (built artifacts, dep caches) | n/a — masters aren't mounted directly | Permanent. Updated by Flow on every merge to the project. |
| Per-work-item drive | The work item (Flow) | btrfs CoW fork of the project master + the work item's git branch checkout + agent scratch (drafts, Claude session history, build outputs) | `/work/` | Created when Flow starts the work item; persists across multiple agents working its phases; deleted when the work item ends (merged, rejected, or canceled). |

The work-item drive is what makes "QA shouldn't redownload
libraries" work: it forks from the project master, which is
already warm. CoW means the fork is near-instant and uses
near-zero extra storage at first.

The work-item drive is the only drive in this stack that crosses
agents. Developer works on it, releases; reviewer claims, drive
re-attaches to reviewer's VM; reviewer finishes, releases; QA
claims, same drive re-attaches; etc. When the work item closes,
the drive is destroyed.

### No rebind step

Because the VM only exists while an assignment is active, there's
no "rebind" operation. Reassigning agent-3 from one work item to
another is just:

1. release agent-3 from the first work item (VM stops, drives detach)
2. claim agent-3 with the new assignment
3. VM starts with the new working drive

If the agent has been the new (role, project) before, the working
drive already exists with prior context. If not, it's created fresh.

## Future runtime — direct-API adjutant

Sketched here for forward compatibility; not in scope for
implementation.

A future `adjutant-go-adk` (or similar) variant talks to model
provider APIs directly and hosts many agents in one process. In this
shape:

- Runtime instance is a long-lived process, not a per-agent VM.
- Agents are coroutines/workers within the process; their state
  (credentials, workspace metadata, context) lives on the Adjutant
  host's disk or a backing store.
- Tool actions (Bash, Edit, builds) execute in **per-assignment
  sandbox VMs** with the **work item drive attached** — same drive
  model as claude-cli, just decoupled from the runtime: when an
  agent gets `current_assignment != null`, the runtime starts a
  sandbox VM for it and attaches the work item's drive; on release,
  the VM is shut down.
- Reassignment is fast: shut down the old sandbox VM, start a new
  one with the appropriate work item drive.

The Hive assignment model, Flow + Sharkfin coordination, and the
project-master + work-item-drive model all work without change.
Only the per-runtime details (where the agent process lives, what's
in the VM, how state is held) differ.

## Audit story

Two views, one truth:

- **Flow's event log** — authoritative. Every state transition,
  every claim/release, every bot exchange, every Combine action,
  with timestamps and references.
- **Sharkfin transcripts** — human-readable derivation. Per-project
  channels show the work as a chronological narrative. Auditors
  read here; messages reference Flow workflow IDs and Hive task IDs
  to pivot deeper.

Sharkfin retention only needs to be human-useful, not legally
sufficient. Flow owns the legal audit.

## Out of scope (deferred)

- **Project bot bootstrap automation.** Each project bot needs a
  Passport service token, a Sharkfin identity, a webhook
  registration, and to be added to its project channel. Done
  manually for now; will become Virgil's responsibility once
  Virgil exists.
- **Hot drive attach.** Nexus's `AttachDrive` requires the VM to
  be stopped. Each work item is its own VM lifecycle so this
  doesn't bite as hard as it would with rebinding, but
  hot-attach would still let the same VM serve multiple agents
  in sequence without restart. Nexus feature request, not part
  of this design.
- **Tamper-evident audit.** Flow's event log is database-backed,
  not a signed ledger. Adequate for current threat model.

## Schema additions summary

**Hive** (migration to add):
- `agents` table: `current_role TEXT NULL`, `current_project TEXT NULL`, `current_workflow_id TEXT NULL`, `lease_expires_at DATETIME NULL`. Either all four are NULL (free) or all four are set (claimed). DB constraint: `(current_role IS NULL) = (current_workflow_id IS NULL)` etc.
- `tasks` table: `flow_task_ref TEXT NULL` (opaque to Hive).
- New endpoints: `claim`, `release`, `renew`, plus a sweeper that nulls expired assignments.

**Flow** (no schema today; design-time):
- New domain concept: scheduler that calls Hive `claim` and dispatches via Sharkfin.
- New workflow primitive: `acquire_agent(role, project, lease_ttl)` and
  `release_agent(agent_id)`.
- Per-project bot configuration: identity, channel, webhook URL.
- Bot vocabulary parser/dispatcher.

**Adjutant**: no code change. Updated role-doc authoring guide.

**Sharkfin**: no change — infrastructure already in place.

**Combine**: no change — Flow's bot uses an existing service token to
trigger merges.

