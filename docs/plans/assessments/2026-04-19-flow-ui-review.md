---
type: review
plan: "2026-04-19-flow-ui.md"
verdict: "APPROVED"
date: "2026-04-19"
commit_range: "7609701..b1b89b3"
---

# Flow UI Implementation Review

## Round 2: APPROVED (fixup `b1b89b3`)

Fixup commit scope confirmed: 10 files, all gofmt whitespace cleanup + one
forbidigo fix in `internal/infra/sqlite/audit_filtered_test.go` widening
`mustRecordAuditSQLite` from `*sqlite.Store` to `domain.AuditEventStore`.
The widening is correct — the function only calls `RecordAuditEvent`, which
is on the interface; no behaviour change.

Re-verification at HEAD (`b1b89b3`):
- `mise run lint:go` — 0 issues (exit 0)
- `mise run lint:web` — clean
- `mise run e2e --backend sqlite` — PASS (9.5s)
- `mise run e2e --backend postgres` — PASS (14.3s)

All gates green. Ready to push.

## Verdict

**CHANGES REQUESTED** — Implementation is substantively correct and all critical
R1/R2 fixes are applied cleanly, but `mise run lint:go` fails hard on gofmt
violations introduced by the developer across 8 files; the CI rule
`feedback_no_test_failures.md` treats red lint as a real defect that must
be fixed before merge.

---

## Commit-by-commit

> Note: The plan numbered 18 tasks. Only 16 commits landed in `flow/lead`.
> Task 17 explicitly produces no commit ("commit only if anything changed") —
> nothing changed. Task 18 (skill amendment) was committed to the `skills`
> repo as `0d81d49 docs(skill): use -tags ui not -tags spa`. Both absences
> are correct and intentional.

| Commit | Task | Plan Title | Verdict |
|--------|------|-----------|---------|
| `eb32602` | 1 | Mise tasks for web build | OK |
| `93c6409` | 2 | Empty web/ package — embed.go + embed_ui.go | NIT: gofmt whitespace in embed.go + embed_ui.go (build-constraint blank line); see lint defect below |
| `ecd7cdc` | 3 | .gitignore for web build artefacts | OK |
| `c07275c` | 4 | UI handler — /ui/health + /ui/* file server | OK — C1 fix verified: HandleUIHealth removed from health.go, duplicate mux registration removed from server.go, publicPathSkip uses `strings.HasPrefix(r.URL.Path, "/ui/")` |
| `55b161f` | 5 | GET /v1/agents — Hive proxy | OK — C2+M3 fixes verified: `CurrentRole`/`CurrentProject`/`CurrentWorkflowID` match v0.3.0 flat fields; `LeaseExpiresAt *time.Time` with `IsZero()` guard in adapter; NIT: gofmt alignment in rest_agents_test.go |
| `77a8e44` | 6 | GET /v1/audit multi-filter query | OK — M2 fix verified: no `bot_id` in AuditFilter, query params, or handler |
| `0243231` | 7 | Sharkfin channel auto-provisioning | OK — Q1 tradeoff correct: project row commits first, channel failure is non-fatal, audit event `project_channel_create_failed` recorded |
| `881c7fe` | 8 | Passport API key auto-mint + rotate-key | OK — Q2 flow correct; `BringYourOwnKey` escape hatch present; rotation best-effort revokes old key with logged warning on failure; NIT: gofmt alignment in rest_bots_test.go |
| `1b862c1` | 9 | Project retention_days schema field | OK — M1 fix verified: `004_project_retention.sql` created (not appended) in both sqlite/ and postgres/ migration dirs; NIT: gofmt alignment in rest_types.go |
| `fd26ecc` | 10 | Instance ↔ project binding + per-project list | OK — M1 fix verified: `005_workflow_instance_project_fk.sql` created in both dirs; NIT: gofmt alignment in dispatcher_test.go |
| `cc30ab1` | 11 | Frontend scaffold — Solid + module federation | OK — vite.config.ts matches Sharkfin verbatim (name, singletons, exposes); `app.css` contains only `var(--wf-space-*)` layout tokens |
| `d81ddeb` | 12 | Project CRUD screen | OK |
| `732889e` | 13 | Bot identity management panel + key-reveal modal | OK — KeyRevealModal dismiss is acknowledgement-gated (disabled until checkbox checked) |
| `2e0eed4` | 14 | Work-item viewer | OK |
| `7bbced9` | 15 | Agent-pool view | OK — 5s polling via `setInterval` + `onCleanup` + `createResource` source signal; no race condition |
| `b3bcb99` | 16 | Audit log viewer + retention controls | OK — wf-banner present with honest "recorded but not yet enforced" copy; no purge daemon |

---

## Cross-cutting findings

### Critical (must-fix before merge) — 1

**DEFECT: `mise run lint:go` fails — gofmt violations in 8 files**

`mise run lint:go` exits non-zero. The lint task (`./mise/tasks/lint/go`)
checks `gofmt -l` as its first step and fails loudly on unformatted files.
All 8 violations are cosmetic whitespace issues introduced when the developer
added new struct methods and constants — gofmt recomputed column alignment and
the committed code was not re-formatted before commit.

Files and the nature of each diff:

- `internal/daemon/rest_agents_test.go:27` — `ReleaseAgent` stub one-liner has
  extra spacing vs gofmt's preferred column (trivial, one char).
- `internal/daemon/rest_types.go:341-343` — `BotPlaintextOutput.Body` struct
  literal column alignment off by one space.
- `internal/daemon/rest_bots_test.go` — similar alignment issue in fake struct.
- `internal/daemon/rest_projects_test.go` — similar alignment.
- `internal/bot/dispatcher_test.go:125` — `UpdateBot` stub one-liner spacing.
- `internal/domain/types.go:198-204` — `AuditEventType` const block alignment
  shifted by one char after new constant added.
- `web/embed.go` — missing blank line between build constraint comment and
  SPDX comment (gofmt requires a blank line after `//go:build`).
- `web/embed_ui.go` — same: missing blank line after `//go:build ui`.

Fix: run `gofmt -w` on all 8 files and amend the relevant commits (or add a
fixup commit). The changes are all whitespace — they will not affect behaviour.

**Evidence:** `mise run lint:go` output:
```
Unformatted files:
internal/bot/dispatcher_test.go
internal/daemon/health.go
internal/daemon/rest_agents_test.go
internal/daemon/rest_bots_test.go
internal/daemon/rest_projects_test.go
internal/daemon/rest_types.go
internal/domain/types.go
web/embed.go
web/embed_ui.go
[lint:go] ERROR task failed
```

### Major (should-fix) — 0

None found.

### Minor (nice-to-have) — 2

**m1. `KeyRevealModal` uses inline `style=` strings rather than `@workfort/ui`
layout primitives.**
`web/src/components/key-reveal-modal.tsx` uses `style="border: 1px solid
var(--wf-color-border, #ccc); ..."` for the container and several child
elements. This is technically within the "layout tokens from @workfort/ui"
constraint (all values are CSS custom properties), but the hardcoded fallbacks
(`#ccc`, `#fff`, `#f5f5f5`) are app-level colour values that should come from
the token system, not literals. Not blocking — the intent is correct and the
values are minimal — but if `@workfort/ui` has a `<wf-card>` or `<wf-code>`
component, prefer it over hand-rolled inline styles.

**m2. `agent-pool-view.tsx` `leaseCountdown` runs at render time; countdown
does not tick.**
The countdown in `leaseCountdown(expiresAt)` is called during the `<For>`
render pass. It computes the remaining time once per poll cycle (every 5 s)
but does not update in between. An agent with 90 seconds left will show
`1m 30s` for 5 seconds, then snap to `1m 25s`. This is acceptable (the plan
says "poll on a 5-second interval") but operators will see the lease count
freeze between polls. Not worth a code change for v1; document as known
behaviour or add a `createMemo` + `setInterval`-based live countdown later.

---

## Cross-cutting checklist

- [x] C1 duplicate /ui/health route gone — `HandleUIHealth` removed from `health.go`, duplicate `mux.HandleFunc` removed from `server.go`
- [x] C2 Hive adapter uses real client.Agent fields — `ag.CurrentRole`, `ag.CurrentProject`, `ag.CurrentWorkflowID`, `ag.LeaseExpiresAt` (flat, no `.Assignment`)
- [x] M1 migration files Create not Modify — `004_project_retention.sql` and `005_workflow_instance_project_fk.sql` created as new files in both sqlite/ and postgres/ dirs
- [x] M2 bot_id audit filter dropped — `AuditFilter` has no `BotID`; `/v1/audit` handler has no `bot_id` query param
- [x] M3 LeaseExpiresAt *time.Time — `domain.HiveAgentRecord.LeaseExpiresAt *time.Time`, adapter uses `IsZero()` guard, `agentResponse.LeaseExpiresAt *time.Time` with `omitempty`
- [x] Hexagonal: external integrations via ports — `ChatProvider` (Q1), `PassportProvider` (Q2, new in `ports.go:137`), `HiveAgentClient` extended (Task 5); no direct HTTP calls from handlers
- [x] Build tag = ui — `web/embed.go` (`//go:build !ui`), `web/embed_ui.go` (`//go:build ui`), `mise run release:ui` uses `-tags ui`
- [x] No per-service styling — `web/src/styles/app.css` contains only `nav` and `main` layout rules using `var(--wf-space-sm/md)` tokens
- [x] Federation singletons complete — `solid-js`, `@workfort/ui`, `@workfort/ui-solid` in `shared` block; matches Sharkfin `vite.config.ts` verbatim
- [x] Empty embed.FS guard — `web/embed.go` has `var Dist embed.FS` (no `//go:embed`); `web/embed_ui.go` has `//go:embed all:dist`
- [x] Dual-backend e2e green at HEAD — `mise run e2e --backend sqlite`: PASS (11.4s); `mise run e2e --backend postgres`: PASS (15.2s)
- [x] No silent skips — no `t.Skip` in any new test file
- [x] pnpm test green at HEAD — 4 test files, 6 tests, all passed (1.28s)
- [ ] **mise run lint green at HEAD — FAIL** (gofmt violations; see Critical section above)
- [x] go build ./... compiles — success (default tag, empty embed)
- [x] go build -tags ui ./... compiles — success (ui tag, embeds dist)
- [x] Federation smoke (strings + curl /ui/remoteEntry.js) — `strings build/flow-ui | grep remoteEntry` produces hits; `dist/remoteEntry.js` present in binary; `mise run release:ui` succeeds
- [x] No accidental dist/ commits — only `web/dist/.gitkeep` in the diff; all build artefacts gitignored
- [x] Audit retention is intent-only (no purge daemon) — `retention_days` column present; grep for `purge`/`DeleteAudit`/`ticker.*audit` finds nothing

---

## Recommendation

**CHANGES REQUESTED.** The implementation is complete, correct, and
plan-faithful. All R1/R2 critical and major fixes are applied against the
actual repo state. Both backends compile and pass e2e. Frontend tests pass.
Federation smoke passes.

The single blocker is gofmt. The fix is mechanical: run `gofmt -w` on the 8
listed files and produce a fixup commit (or amend the last commit if the
developer prefers — all 8 are whitespace, no logic change). After that commit,
`mise run lint:go` must exit 0, then this is ready to push.

Prioritised fix list for the developer:

1. `gofmt -w internal/bot/dispatcher_test.go internal/daemon/rest_agents_test.go internal/daemon/rest_bots_test.go internal/daemon/rest_projects_test.go internal/daemon/rest_types.go internal/domain/types.go web/embed.go web/embed_ui.go` and commit as `style: gofmt fixes`.
2. Verify `mise run lint:go` exits 0.
3. No other code changes required.

Once the lint commit lands the developer can hand off to QA tester (team-lead
dispatches).
