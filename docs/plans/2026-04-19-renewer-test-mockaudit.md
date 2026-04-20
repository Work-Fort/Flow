---
type: plan
step: "fix"
title: "Use no-op audit store in TestRenewer_WaitIdleRaceUnderLoad"
status: approved
assessment_status: complete
provenance:
  source: issue
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - "2026-04-19-renewer-waitidle-flake.md"
  - "2026-04-19-renewer-test-budget.md"
---

# Use no-op audit store in TestRenewer_WaitIdleRaceUnderLoad

## Overview

`TestRenewer_WaitIdleRaceUnderLoad` (`internal/scheduler/renewer_test.go:113`) is still failing in CI even after the budget was widened to 30 s (run 24645456478 hung at 30.13 s). The earlier widening from 5 s → 30 s addressed CI scheduler jitter; the residual failure is a different problem: audit-store I/O dominates per-cycle latency and eventually overruns any reasonable budget.

`newAuditStore(t)` (`internal/scheduler/scheduler_test.go:59-67`) opens a real in-memory SQLite database via `sqlite.Open("")`. Each renew call inside the renewer loop writes one `domain.AuditEvent` through `RecordAuditEvent`. With `iterations = 200`, that is 200 synchronous SQLite writes inside the test's hot path. Under `-race` plus shared GitHub-Actions runner contention, those 200 writes alone exceed the 30 s budget.

The test's purpose is to exercise `WaitIdle`/`signalIdle` race semantics — not audit-store throughput. Audit writes are I/O-bound, dominate the wall time, and contribute nothing to what the test asserts (renew-call count + bounded completion). Swapping the audit store in this one test for an in-memory no-op `domain.AuditEventStore` removes the I/O entirely.

The `TestScheduler_*` family asserts against the audit log via `audit.ListAuditEventsByWorkflow` (`scheduler_test.go:98,113`); the two `TestRenewer_*` tests (`RenewsEveryActiveClaim`, `DropsClaimOnMismatch`) do not, but are left on `newAuditStore` because their write volume is trivial (~2-3 audit writes per test).

### Why a no-op mock and not real-but-faster

Three options were considered:

1. **No-op mock implementing `AuditEventStore`.** Returns `nil` from `RecordAuditEvent`, returns empty slices from the three `List*` methods. The test never reads back audit events, so no state is needed.
2. **In-memory map-backed mock.** Same shape as #1 but stores events in a slice under a mutex. Lets the test optionally assert on audit content.
3. **Per-iteration sub-budget on `WaitIdle`.** Wrap each `WaitIdle` in `context.WithTimeout` and reduce the outer budget.

Option 1 wins:

- **The test does not read audit events.** Look at `renewer_test.go:113-161` — there is no call against `audit` after the loop. A map-backed mock would store 200 entries that nothing inspects. YAGNI.
- **Mirrors the test's existing fakes.** `fakeHive` (`scheduler_test.go:17-57`) is a hand-rolled stub in the same file — a no-op `noopAuditStore` is the same pattern at smaller scope.
- **Eliminates the symptom at the root.** The slow component is removed entirely; no budget tuning is required.
- Option 3 still leaves SQLite in the loop and just changes which timeout fires.

### Why this scope, not a wider refactor

`scheduler.recordEvent` (`internal/scheduler/scheduler.go:226-237`) calls `s.audit.RecordAuditEvent` synchronously inside `Renew`. Making audit async or batched would be a larger behavior change with its own correctness questions (event ordering vs. claim mutations, shutdown drain semantics). That is out of scope for a test fix. The scheduler's production code already accepts any `domain.AuditEventStore` — using a no-op in one test exercises exactly the seam the interface was designed for.

## Prerequisites

- Working tree clean.
- `mise run test` currently green locally; the failure only reproduces under CI's `-race` + shared-runner load.
- `domain.AuditEventStore` interface is stable at `internal/domain/ports.go:96-112` (four methods: `RecordAuditEvent`, `ListAuditEventsByWorkflow`, `ListAuditEventsByAgent`, `ListAuditEventsByProject`).

## Task breakdown

### Task 1: Add `noopAuditStore` to the scheduler test package

**Files:**
- Modify: `internal/scheduler/scheduler_test.go` (add type immediately after `fakeHive` block, before `newAuditStore` at line 59)

**Step 1: Add the no-op type**

Insert this block after the `fakeHive.RenewAgentLease` method (currently ending at `scheduler_test.go:57`) and before the existing `newAuditStore` helper (currently at line 59):

```go
// noopAuditStore is a zero-cost domain.AuditEventStore for tests that
// exercise scheduler/renewer mechanics rather than audit semantics.
// RecordAuditEvent is a no-op; the List* methods return empty slices.
// Use this in place of newAuditStore(t) when the test does not read
// audit events back — it removes per-call SQLite I/O from the hot path.
type noopAuditStore struct{}

func (noopAuditStore) RecordAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}

func (noopAuditStore) ListAuditEventsByWorkflow(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (noopAuditStore) ListAuditEventsByAgent(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (noopAuditStore) ListAuditEventsByProject(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}
```

No new imports are required — `context` and `domain` are already imported by `scheduler_test.go` (lines 4-13).

**Step 2: Verify the type satisfies the interface**

Run: `go build ./internal/scheduler/...`

Expected: PASS (no output). If you see `cannot use noopAuditStore{} (...) as type domain.AuditEventStore`, double-check method signatures against `internal/domain/ports.go:96-112`.

**Step 3: Commit**

`test(scheduler): add noopAuditStore for I/O-free audit dependency`

Use HEREDOC, multi-line conventional commit, `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer. No `!`, no `BREAKING CHANGE:`.

Example body: "Adds a zero-cost `domain.AuditEventStore` implementation to the scheduler test package for tests that exercise renewer/scheduler mechanics but do not assert on audit log contents. Swapping this in place of the real SQLite-backed `newAuditStore` removes 1 disk write per renew call, which is necessary for high-iteration race tests under `-race` on shared CI runners."

### Task 2: Swap the audit store in `TestRenewer_WaitIdleRaceUnderLoad`

**Depends on:** Task 1 (`noopAuditStore` type must exist).

**Files:**
- Modify: `internal/scheduler/renewer_test.go:116`

**Step 1: Replace the audit-store construction**

In `TestRenewer_WaitIdleRaceUnderLoad`, change the existing line 116:

```go
	audit := newAuditStore(t)
```

to:

```go
	audit := noopAuditStore{}
```

Leave every other line in the test unchanged. The 30 s `time.After` guard at line 149 stays — it still serves as the regression catch for the original `WaitIdle` race.

**Step 2: Run the targeted test locally without `-race` to confirm it still passes**

Run: `go test -run TestRenewer_WaitIdleRaceUnderLoad ./internal/scheduler/...`

Expected: PASS in well under 1 s. Note the wall time printed by `go test`.

**Step 3: Run the targeted test under `-race` to confirm it stays well inside the budget**

Run: `go test -race -run TestRenewer_WaitIdleRaceUnderLoad ./internal/scheduler/...`

Expected: PASS in under 1 s. (Pre-fix, the same command on the same machine takes noticeably longer because of 200 SQLite writes; post-fix it should be near-instant.)

**Step 4: Run the rest of the scheduler test package to confirm no regression**

Run: `go test -race ./internal/scheduler/...`

Expected: PASS. In particular, `TestScheduler_AcquireReleaseRoundTrip` still uses `newAuditStore(t)` and still asserts on `audit.ListAuditEventsByWorkflow` results — it must remain green to prove the swap was scoped to one test.

**Step 5: Run the full project test suite**

Run: `mise run test`

Expected: PASS.

**Step 6: Commit**

`test(scheduler): use no-op audit store in WaitIdle race test`

Use HEREDOC, multi-line conventional commit, `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer. No `!`, no `BREAKING CHANGE:`.

Example body: "`TestRenewer_WaitIdleRaceUnderLoad` runs 200 tick/WaitIdle cycles to catch a regression of the previous `signalIdle`/`WaitIdle` race. Each cycle's renew call wrote one audit event through a real in-memory SQLite store, and under `-race` on GitHub-Actions runners those 200 writes exceeded the test's 30 s budget (run 24645456478 hung at 30.13 s). The test does not assert on audit-log contents, so swapping in `noopAuditStore` removes the I/O entirely without weakening what the test verifies. The other scheduler/renewer tests that DO assert on audit semantics keep `newAuditStore(t)`."

## Verification checklist

- [ ] `noopAuditStore` is defined in `internal/scheduler/scheduler_test.go` and implements all four methods of `domain.AuditEventStore`.
- [ ] `TestRenewer_WaitIdleRaceUnderLoad` (`internal/scheduler/renewer_test.go:113`) uses `noopAuditStore{}`; the rest of the test body is unchanged.
- [ ] `TestRenewer_RenewsEveryActiveClaim`, `TestRenewer_DropsClaimOnMismatch`, and every `TestScheduler_*` test still call `newAuditStore(t)` (no accidental sweep).
- [ ] `go test -race -run TestRenewer_WaitIdleRaceUnderLoad ./internal/scheduler/...` completes in well under 1 s.
- [ ] `go test -race ./internal/scheduler/...` PASS.
- [ ] `mise run test` PASS.
- [ ] Two commits landed, in order: `test(scheduler): add noopAuditStore for I/O-free audit dependency`, then `test(scheduler): use no-op audit store in WaitIdle race test`. Each is multi-line conventional with the `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer; neither uses `!` or `BREAKING CHANGE:`.
- [ ] Push and confirm the next CI run reports `TestRenewer_WaitIdleRaceUnderLoad` completing in sub-second wall time under `-race`.
