# Orchestration Implementation

Flow's component-level work for the agent pool design. The
cross-cutting design lives in `2026-04-18-agent-pool.md`; this doc
breaks down the implementation into shippable pieces.

Most of the new code lives here — Flow is the orchestrator and
owns scheduling, runtime lifecycle, audit, and Combine integration.

## Subsystems

### 1. Scheduler

A workflow primitive that workflows call to acquire/release agents.

- `acquire_agent(role, project, lease_ttl)`: calls Hive's
  `POST /v1/agents/claim`. Returns the chosen agent's identity.
  Retries with backoff if pool is exhausted.
- `release_agent(agent_id)`: calls Hive's `release`.
- Failure path: if the workflow ends without releasing (crash,
  cancel), the lease expires via Hive's sweeper. Flow's own
  workflow event store records the release-on-crash for audit.

### 2. Lease renewer

Background goroutine. Every ~30s, for every claim Flow currently
holds, calls Hive's `renew`. Lease TTL is short (~2 min) so Flow
crashes recover within a couple sweeper cycles.

### 3. Per-project bot processes

One Sharkfin identity per project: `flow-bot`, `nexus-bot`,
`hive-bot`, `combine-bot`, `sharkfin-bot`. Each bot:

- Registers as a Sharkfin `service` identity (via Passport service
  token).
- Joins its project channel.
- Registers a webhook back to Flow.
- Forwards every received message into Flow's bot vocabulary
  parser.
- Sends Flow-driven outgoing messages with structured metadata.

These bots can be one process per project or one process handling
all bots — implementation choice. Probably one process for
simplicity, with bot-identity being a routing concern internally.

### 4. Bot vocabulary parser/dispatcher

Mechanical map from `metadata.event_type` to a Flow workflow event.
Receives webhook payloads from Sharkfin, routes by `event_type`,
emits the corresponding workflow transition.

Initial vocabulary (from the design doc):

| event_type | Direction | Action |
|---|---|---|
| `task_assigned` | bot → agent | (outgoing only) |
| `task_started` | agent → bot | record agent acknowledged |
| `request_review` | agent → bot | start the review phase |
| `task_completed` | agent → bot | record completion + trigger workflow advance |
| `blocked` | agent → bot | escalate to human via channel post |
| `lease_expiring` | bot → agent | (outgoing only) |
| `agent_assigned` | bot → channel | (outgoing only, audit) |
| `agent_released` | bot → channel | (outgoing only, audit) |

Off-vocabulary messages: bot replies in the channel with "didn't
understand X" so the failed handoff is in the audit transcript.

### 5. Combine integration

- Receive Combine webhooks for push and merge events.
- On push: nothing (Flow doesn't need to react to mid-work
  pushes).
- On merge to a project's main branch: refresh that project's
  source master.
- Issue Combine API calls for approve/merge when workflow policy
  is satisfied. Flow holds a Combine service token; agents do not.

### 6. Project source master management

Per project, Flow maintains a btrfs subvolume containing source +
warm state.

- Create on first need.
- Refresh on Combine merge webhook: pull from Combine, run
  whatever warming steps the project's seed config specifies (e.g.
  `make build`, `npm install`).
- Storage: lives on Nexus's btrfs drive area, managed via Nexus
  drive APIs.

### 7. Per-work-item drive management

For each work item Flow runs:

- Create: btrfs-clone the project source master into a new drive
  (`work-item-<id>`). Use Nexus's `CloneSnapshot` or a drive-level
  equivalent.
- Attach: when an agent is claimed for the work item, attach this
  drive to the chosen runtime VM.
- Detach: when the agent's phase ends, detach. Drive persists.
- Delete: when the work item ends (merged, rejected, canceled),
  delete the drive.

### 8. claude-cli VM lifecycle

For the current runtime, Flow drives Nexus directly:

- VM pool: Flow knows about a set of fungible Nexus VMs (e.g.
  `pool-vm-01..N`) capable of running adjutant-claude-cli.
  Operational provisioning, not Flow-managed today.
- On claim: pick a free VM from the pool, attach the agent's
  per-agent credentials drive + the work item's drive, start the
  VM.
- On release: stop the VM, detach drives, return the VM to the
  pool.

### 9. Audit event log

Every workflow transition, agent claim/release, bot exchange,
Combine action lands in Flow's existing workflow event store.
Sharkfin transcripts derive from this; Flow is the legal audit.

May need extension if the existing event store doesn't cover
agent lifecycle events.

## New seeds (in `docs/examples/`)

The role definitions and bot vocabulary are content, not code.
They ship as seed files Flow loads on bootstrap.

- `docs/examples/sdlc-template.json` — already exists; the SDLC
  workflow template.
- `docs/examples/roles/` — new directory. One file per role
  (`developer.json`, `reviewer.json`, `qa-tester.json`,
  `planner.json`, ...). Each file specifies the role doc content
  Hive should publish, the bot vocabulary the role uses, and any
  role-specific work-plan setup steps.
- `docs/examples/bot-vocabulary.json` — the canonical mapping
  from `event_type` to handler. One source of truth used by both
  the bot parser and the role docs Flow seeds into Hive.

## Implementation order

Approximate dependency order:

1. Hive schema + endpoints land (separate doc; required before
   anything below).
2. Combine webhook + service token configured (operational).
3. Flow scheduler primitive (`acquire_agent` / `release_agent`)
   and lease renewer.
4. Project source master management for one project.
5. Per-work-item drive management + claude-cli VM lifecycle for
   that one project.
6. One project bot + bot vocabulary parser, end-to-end with one
   workflow type (e.g. "review this PR").
7. Seeds for the first project's roles + workflow.
8. Expand to remaining projects.

## Out of scope

- Hot drive attach. Each work item gets its own VM cycle today.
- Direct-API runtime (`adjutant-go-adk`). Same Flow code applies
  with a different VM lifecycle adapter.
- Rebalancer. Imbalance handling is operator-driven for now.
- Multi-agent parallelism within a single work item (e.g. two
  developers on the same branch). Work items are single-agent at
  any moment in this design.
