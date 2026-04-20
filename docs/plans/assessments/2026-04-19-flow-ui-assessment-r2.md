---
type: assessment
plan: "2026-04-19-flow-ui.md"
round: 2
verdict: "PASS"
date: "2026-04-19"
---

# Flow UI Plan Re-Assessment (Round 2)

## Verdict

**PASS** — All 5 round-1 fixes (C1, C2, M1, M2, M3) landed correctly against the actual repo state; the revision introduces no regressions and the developer can proceed.

## Fix verification

### C1 — duplicate /ui/health route
**FIXED.** Repo state confirmed via grep: `internal/daemon/server.go:145` has `mux.HandleFunc("GET /ui/health", HandleUIHealth())`, `internal/daemon/health.go:182-183` has the `HandleUIHealth` helper, `internal/daemon/middleware.go:16` has the literal `r.URL.Path == "/ui/health"`. Task 4 (plan lines 753–1052) now:

- Plan lines 765–771: Files block calls out DELETE of the `mux.HandleFunc("GET /ui/health", HandleUIHealth())` line in `server.go`, with the line-number-may-have-shifted caveat.
- Plan lines 772–778: Files block calls out DELETE of `HandleUIHealth` from `health.go:183-193`, with a pre-grep step to confirm sole call site.
- Plan lines 779–787: Files block prescribes REPLACING the literal `r.URL.Path == "/ui/health"` clause with `strings.HasPrefix(r.URL.Path, "/ui/")` in `middleware.go:14-23`.
- Plan lines 966–1002: Step 3 implementation walkthrough repeats all three edits with exact prose.
- Plan lines 1018–1024: Step 4 boot test specifically asserts no startup panic.
- Commit message body (lines 1041–1048) explicitly names the duplicate-pattern panic risk and the publicPathSkip widening.

### C2 — Hive adapter field mapping
**FIXED.** Repo state confirmed: `hive/lead/client/types.go:24-36` `Agent` struct has flat fields `AssignedRole`, `CurrentProject`, `CurrentWorkflowID`, `LeaseExpiresAt` and no `Assignment` sub-struct. Task 5 (plan lines 1055–1378) now:

- Plan lines 1182–1192: `domain.HiveAgentRecord` mirrors Hive's vocabulary verbatim — `AssignedRole`, `CurrentProject`, `CurrentWorkflowID` — with inline comments documenting the Hive field source.
- Plan lines 1224–1233: Adapter `ListAgents` body assigns `ag.AssignedRole`, `ag.CurrentProject`, `ag.CurrentWorkflowID`, `ag.LeaseExpiresAt` directly. No `ag.Assignment.*` references anywhere in Task 5.
- Plan lines 1248–1258: Explicit translation table provided (Hive → Flow domain).
- Test fixture (line 1123) seeds a record with `CurrentWorkflowID: "wf-1"`, matching the new domain field.

### M1 — migration files Create not Modify
**FIXED.** Repo state confirmed: both `internal/infra/sqlite/migrations/` and `internal/infra/postgres/migrations/` have `001_init.sql`, `002_audit_events.sql`, `003_projects_bots_vocab.sql` — so `004` is the correct next sequence. Tasks 9 and 10 now:

- Task 9 (plan lines 1745–1766): Files block uses "Create:" wording, names `004_project_retention.sql` explicitly in BOTH `internal/infra/sqlite/migrations/` and `internal/infra/postgres/migrations/`, and includes a "do NOT append to 003 / confirm 003 is highest before naming 004" instruction.
- Task 10 (plan lines 1808–1841): Files block uses "Create:" wording, names `005_workflow_instance_project_fk.sql` explicitly in both backend dirs, with the same "confirm before writing" pre-flight.
- SQLite vs PG SQL bodies differ correctly (PG gets the inline `REFERENCES … ON DELETE SET NULL`; SQLite gets plain TEXT plus a comment explaining the ALTER TABLE limitation).

### M2 — bot_id audit filter dropped
**FIXED.** Task 6 (plan lines 1382–1574) now:

- Plan lines 1392–1401: Explicit "Scope decision (assessor M2)" block stating `bot_id` is excluded with rationale (no domain field, no schema column, work items are not bot-scoped, silent-empty-result footgun).
- Plan lines 1458–1467: `AuditFilter` struct fields are `Project, WorkflowID, AgentID, EventType, Since, Until, Limit, Offset`. No `BotID`.
- Plan lines 1500–1509: `AuditFilterInput` query params are `project_id, workflow_id, agent_id, event_type, since, until, limit, offset`. No `bot_id`.
- Plan lines 1523–1534: Handler plumbs only the eight chosen fields. No `BotID` pass-through.

Minor wording nit (non-blocking): the commit message body on line 1568 still says "project / workflow / agent / bot filters" — a stale reference to the old plan. The code in the task is correct; the developer should drop "/ bot" from the commit body when they write the actual commit. Worth flagging but not gating.

### M3 — LeaseExpiresAt *time.Time
**FIXED.** Task 5 now applies the pointer fix consistently across all three layers:

- `domain.HiveAgentRecord.LeaseExpiresAt` (plan line 1191): `*time.Time` with inline comment explaining the encoding/json zero-time quirk.
- Adapter (plan lines 1234–1237): explicit `if !ag.LeaseExpiresAt.IsZero()` guard, takes pointer of a local copy.
- `agentResponse.LeaseExpiresAt` (plan line 1287): `*time.Time` with `json:"lease_expires_at,omitempty"` and a multi-line comment explaining why the pointer is required.
- Handler (plan line 1345): straight pass-through `LeaseExpiresAt: r.LeaseExpiresAt` with comment confirming the adapter already returns nil for idle.
- Test fixture (plan line 1124): explicit `// idle: LeaseExpiresAt nil` comment on the second seed record demonstrates intent.

## New issues (introduced by the revision, not present in round 1)

None of substance. The single nit (Task 6 commit body still references "bot filters" on line 1568) is cosmetic — the developer will notice when they write the actual commit, and even if they don't, the code itself is correct.

## Recommendation

**PASS** — developer can proceed. All round-1 critical and major issues are addressed against the actual repo state, not just the plan text. The revision is tightly scoped to Tasks 4, 5, 6, 9, 10 and does not touch the unrelated tasks that round 1 already cleared. Optional: ask the developer to drop "/ bot" from the Task 6 commit body when they execute it (cosmetic, not blocking).
