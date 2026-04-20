---
type: assessment
plan: "2026-04-19-flow-ui.md"
verdict: "REVISE"
date: "2026-04-19"
---

# Flow UI Plan Assessment

## Verdict

**REVISE** — Plan respects every user-facing hard constraint (Solid + @workfort/ui-solid, no per-service styling, Sharkfin gold-standard mirrored, `-tags ui`, no purge daemon, hexagonal port mediation), but two implementation defects will fail at compile/runtime if the developer follows the plan literally: a duplicate `GET /ui/health` route registration that will panic at startup, and an adapter snippet that reads non-existent `Assignment.*` sub-fields on the Hive client's `Agent` struct.

## Strengths

- Hard constraints faithfully recorded and acted on: stack, no-Tailwind, no per-service styling, Sharkfin as model, `-tags ui` (with explicit reasoning to fix the skill — Task 18), Q3 retention as recorded-intent only.
- Sharkfin reference verified end-to-end against `~/Work/WorkFort/sharkfin/lead/web/{embed.go,embed_ui.go,vite.config.ts,package.json}` — directory layout, federation singleton list, build-tag pattern, dual `embed.go`/`embed_ui.go` files, package layout (`@workfort/ui` + `@workfort/ui-solid` link:-resolved, `solid-js` ^1.9, vite ^6, vitest ^3, jsdom ^25) all match exactly.
- Hexagonal correctly preserved: `ChatProvider` already exists and the Sharkfin adapter implements it (`internal/infra/sharkfin/adapter.go:52`); Q1 plumbs through `ServerConfig.Chat domain.ChatProvider`. Q2 introduces a `PassportProvider` port before the handler call. New `GET /v1/agents` proxy extends `domain.HiveAgentClient` (already at `internal/domain/scheduler.go:50`) rather than reaching past the port.
- REST surface verified non-colliding (except `/ui/health`, see below): no overlap with existing routes in `rest_huma.go`, `rest_projects.go`, `scheduler_diag.go`, `webhook_*.go`. The existing `/v1/audit/_diag/by-workflow/{id}` does not collide with the new `/v1/audit`.
- Task ordering is sound: backend (1–10) before frontend scaffold (11), Project CRUD (12) before Bot panel (13) before Work-item viewer (14) — because each frontend task depends on the previous's screen-tab pattern and the project/bot context propagation.
- TDD cycle structured per task (failing test, minimal impl, passing test, commit).
- Honest disclosure on retention banner is the correct UX call given v1 has no purge daemon.
- Q1 / Q2 failure-recovery tradeoffs are explicit and documented (`channel_already_exists` / `bring_your_own_key` opt-outs, audit-event surfacing for failures).
- Task 18 correctly flags the `spa` → `ui` skill amendment so the next service doesn't re-discover the mismatch.
- Verification checklist is comprehensive: dual-backend e2e, lint, vitest, federation smoke, `strings build/flow-ui | grep remoteEntry`, `git status` cleanliness for accidental `dist/` commits, push-to-CI monitoring.
- Pre-1.0 commit hygiene respected: spot-checked Tasks 1, 4, 5, 7, 8, 9, 10, 18 — all are multi-line conventional, body-explains-why, `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer, no `!` markers, no `BREAKING CHANGE:` footers.

## Issues (severity-ordered)

### Critical (must-fix before dev starts) — 2 issues

**C1. Duplicate `GET /ui/health` registration will panic on startup.**
Task 4 (`internal/daemon/ui_routes.go` line ~911) registers `mux.HandleFunc("GET /ui/health", ...)`. But `server.go:145` already registers `mux.HandleFunc("GET /ui/health", HandleUIHealth())` — a stub manifest handler in `internal/daemon/health.go:183` that always returns 200. The Go 1.22+ enhanced ServeMux panics on duplicate `METHOD /path` registrations.

The plan is silent on the existing handler. The developer following the plan literally will:
1. Add the new `registerUIRoutes` call after `registerProjectRoutes` (line 141 / 142).
2. Leave the existing `mux.HandleFunc("GET /ui/health", HandleUIHealth())` at line 145 in place.
3. Get a panic on first request (`pattern "GET /ui/health" conflicts with pattern "GET /ui/health"`).

**Fix:** Task 4 must explicitly delete `health.go:183-193` (`HandleUIHealth`) and `server.go:145` (`mux.HandleFunc("GET /ui/health", HandleUIHealth())`), and update the existing `publicPathSkip` switch in `middleware.go:14-23` to include `strings.HasPrefix(r.URL.Path, "/ui/")` instead of just the literal `r.URL.Path == "/ui/health"`. Plan currently mentions extending `publicPathSkip` but does not call out the deletion of the conflicting handler.

**C2. Hive adapter snippet reads non-existent `ag.Assignment.*` sub-fields.**
Task 5, plan lines ~1158–1167, the `ListAgents` adapter:

```go
if ag.Assignment != nil {
    rec.AssignedWorkflowID = ag.Assignment.WorkflowID
    rec.AssignedRole = ag.Assignment.Role
    rec.AssignedProject = ag.Assignment.Project
    rec.LeaseExpiresAt = ag.Assignment.LeaseExpiresAt
}
```

`hive/lead/client/types.go:24-36` shows the actual `Agent` struct has FLAT fields, no `Assignment *struct{...}`:
- `AssignedRole string` (not under `.Assignment.Role`)
- `CurrentProject string` (note: `Current`, not `Assigned`)
- `CurrentWorkflowID string` (note: `Current`, not `Assigned`)
- `LeaseExpiresAt time.Time` (already top-level)

The plan parenthetically says "if the field names differ, adjust to match" but a developer working from the snippet will hit `ag.Assignment undefined` and have to re-derive the mapping. Plan should specify the exact mapping:

```go
rec.AssignedWorkflowID = ag.CurrentWorkflowID
rec.AssignedRole       = ag.AssignedRole
rec.AssignedProject    = ag.CurrentProject
rec.LeaseExpiresAt     = ag.LeaseExpiresAt
```

Also rename `domain.HiveAgentRecord.AssignedProject` → drop the inconsistency: Hive uses `CurrentProject`/`CurrentWorkflowID`/`AssignedRole` (mixed), Flow's domain wrapper uses `Assigned*` for all three. Either is fine, but the plan's domain field choice + the plan's snippet referencing nonexistent sub-fields together mislead the developer about the wire shape.

### Major (should-fix; not blocking but creates rework risk) — 4 issues

**M1. Migration files described as "Modify" should be "Create".**
Tasks 9 and 10 say `Modify: internal/infra/sqlite/migrations/<next>.sql` — but migrations are immutable once shipped (`003_projects_bots_vocab.sql` already exists in both backends). The developer must CREATE `004_project_retention.sql` (Task 9) and `005_workflow_instance_project_fk.sql` (Task 10), in both `internal/infra/sqlite/migrations/` and `internal/infra/postgres/migrations/`. The plan should name the new files explicitly (and use "Create:") to prevent the developer from accidentally appending to `003`.

**M2. `BotID` audit filter has no backing column.**
Task 6 plumbs `BotID string` through `AuditFilter` and the `/v1/audit?bot_id=` query param, but `domain.AuditEvent` (`internal/domain/types.go:212-223`) has no `BotID` field, and the `audit_events` table presumably has no bot_id column either (003 migration didn't add one). The plan acknowledges this with "pass-through for now" but doesn't say what the SQLite/Postgres impls should DO with the filter when set: silently match nothing? Return all? Error 422? The developer is left to invent semantics. Decide explicitly: either drop `bot_id` from the v1 filter (recommended — work items aren't bot-scoped today), or land the schema column in this plan.

**M3. Task 5 `LeaseExpiresAt` JSON tag with `,omitempty` on `time.Time`.**
The `agentResponse` struct uses `LeaseExpiresAt time.Time \`json:"lease_expires_at,omitempty"\``. Go's `encoding/json` doesn't omit zero-valued `time.Time` with `omitempty` — the tag has no effect on struct types. Idle agents will get `"lease_expires_at":"0001-01-01T00:00:00Z"` in the response, which the UI's lease-countdown will render as a wildly negative duration. Either:
- Use `*time.Time` and explicit nil for idle agents, or
- Custom MarshalJSON, or
- Filter out the field in the handler when zero.

Same issue affects `domain.HiveAgentRecord.LeaseExpiresAt` if it surfaces directly.

**M4. Verification checklist `mise run e2e --backend=sqlite` syntax — works, but the canonical task-arg form is space-separated.**
`.mise/tasks/e2e:25-30` accepts both `--backend sqlite` and `--backend=sqlite`. Not blocking, but the existing CI/docs use the space-separated form. Standardise to avoid future confusion.

### Minor (nice-to-have; non-blocking) — 5 issues

**m1. `web/dist/.gitkeep` keepalive is a known-quirk pattern; documented inline.**
The plan handles the empty-embed-FS guard correctly (`var Dist embed.FS` in the non-ui file, separate `//go:embed all:dist` in the ui file, and a `.gitkeep` to satisfy `embed all:dist` even before the first Vite build). Sharkfin uses the same trick. Optional improvement: a one-line comment in `embed_ui.go` saying "dist/.gitkeep ensures `go:embed` succeeds before the first Vite build."

**m2. `app-shell.tsx` uses inline web components (`<wf-banner>`, `<wf-button>`) without TypeScript JSX intrinsic-element registration.**
`@workfort/ui-solid` likely declares the JSX intrinsic elements; if not, `tsc --noEmit` (Task 1's `mise run lint:web`) will fail. Sharkfin works, so the typings are presumably exported. Worth noting in Task 11's verification step: confirm `tsc --noEmit` passes against the placeholder shell BEFORE moving to Task 12.

**m3. Task 8 `internal/infra/passport/client.go` re-implements an HTTP client when one might exist.**
The plan justifies "minimal HTTP client … no Passport client lib" by noting Flow already does this for Hive (no — Hive uses its published Go client `github.com/Work-Fort/Hive/client`). Worth a quick check: does Passport publish a Go client similar to Hive's? If yes, prefer that; if no (or if it's an intentional dependency-isolation choice), document the rationale in the commit body.

**m4. Q2 `BotPlaintextOutput` returns plaintext as `plaintext_api_key`.**
Reasonable name. Consider `api_key_plaintext` to mirror the `passport_api_key_id` ordering. Bikeshed.

**m5. Audit retention banner uses `<wf-banner variant="info">`.**
Verify the variant exists in `@workfort/ui` — Sharkfin uses `variant="warning"` in `chat.tsx`. If `info` doesn't exist, fall back to whatever's documented for honest-disclosure UX. Trivial.

## Cross-cutting checks

- [x] Stack constraint (Solid + @workfort/ui-solid + @workfort/auth) — `@workfort/auth` is consumed transitively via `useAuth` re-export from `@workfort/ui-solid`, mirroring Sharkfin's `src/index.tsx:10`. Plan correctly does NOT add `@workfort/auth` to the singleton list (Sharkfin doesn't either).
- [x] No per-service styling — `web/src/styles/app.css` is the only CSS file, contains only `var(--wf-space-*)` token references for layout grid + spacing. Explicitly forbids Tailwind / UnoCSS / app-level overrides.
- [x] Sharkfin web/ structure mirrored — verified file-by-file against `sharkfin/lead/web/`.
- [x] Build tag `ui` (not `spa`) — used everywhere; Task 18 amends the skill to match.
- [x] Commit hygiene — spot-checked 8 of 18 task commit messages, all multi-line + HEREDOC + `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` + no `!` / no `BREAKING CHANGE`.
- [x] Audit retention v1 = intent only, no purge daemon — Q3 explicit, banner surfaces the gap to operators.
- [x] Hexagonal: external integrations via ports — Q1 via `domain.ChatProvider` (already exists), Q2 via new `PassportProvider` port, Hive via extended `HiveAgentClient` interface.
- [x] Dual-backend migrations — Tasks 9 and 10 explicitly land SQLite + Postgres in lock-step (modulo M1 wording).
- [x] No silent skips — no `t.Skip` in plan; verification checklist requires both backend e2e suites green.
- [x] Federation singleton list complete — `solid-js`, `@workfort/ui`, `@workfort/ui-solid` all present, matches Sharkfin verbatim.
- [x] REST surface aligned with existing routes — verified no path collisions in `rest_huma.go`, `rest_projects.go`, `scheduler_diag.go`. **Exception:** `/ui/health` collides (see C1).
- [~] Auth scheme explicit per endpoint — Plan says `/ui/*` bypasses Passport (correct, Scope shell handles auth), and that REST calls attach `Authorization: Bearer <jwt>` from `useAuth`. Implicit but unambiguous: the new `/v1/agents`, `/v1/audit`, retention PATCH, bot rotate-key endpoints all land under the existing Passport middleware that accepts both `Bearer` and `ApiKey-v1`. Per the operator-UI assertion, JWT-only would be a future tightening; not gating.
- [x] Task granularity — every task names files, exact code, exact verification commands. No "developer decides X."
- [x] Verification commands per task — every task has Step 2 / Step 4 / final verify lines with exact commands.
- [x] Empty embed.FS guard correct — `var Dist embed.FS` in the non-ui file, `//go:embed all:dist` only in the ui file, `web/dist/.gitkeep` keeps the embed target valid pre-build.

## Recommendation

Plan is structurally sound and respects every hard constraint. Two critical defects (C1 duplicate route, C2 wrong field paths) and one major scoping ambiguity (M2 BotID filter semantics) require fixes before dev starts. Suggested revision in priority order:

1. **C1 fix** — Task 4: explicitly remove `HandleUIHealth` from `health.go:183-193`, remove `mux.HandleFunc("GET /ui/health", HandleUIHealth())` from `server.go:145`, and replace the literal `/ui/health` skip in `middleware.go:16` with a `strings.HasPrefix(r.URL.Path, "/ui/")` check.
2. **C2 fix** — Task 5: rewrite the adapter snippet against the actual `client.Agent` struct fields (`AssignedRole`, `CurrentProject`, `CurrentWorkflowID`, `LeaseExpiresAt` — all flat) and either rename `domain.HiveAgentRecord.Assigned*` to mirror Hive's vocabulary or document the translation.
3. **M2 decision** — Task 6: drop `bot_id` from the v1 audit filter (no backing column) OR land the schema column with explicit migration. Currently leaves the developer guessing.
4. **M1 wording** — Tasks 9 and 10: change "Modify: …/migrations/<next>.sql" to "Create: …/migrations/004_project_retention.sql" and "Create: …/migrations/005_workflow_instance_project_fk.sql" (in BOTH backend dirs, with parallel SQL).
5. **M3 fix** — Task 5: change `LeaseExpiresAt` to `*time.Time` (or filter zero in handler) so idle agents don't render `"0001-01-01T00:00:00Z"`.

Minors (m1–m5) are nice-to-haves; the developer can address them inline without a re-spin.

Once C1, C2, M1, M2, M3 are folded in, the plan is ready for development. The bones are right and the constraint compliance is genuinely thorough — this is a tight revision, not a rethink.
