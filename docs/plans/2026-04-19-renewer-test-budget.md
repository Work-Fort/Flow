---
type: plan
step: "fix"
title: "Bump TestRenewer_WaitIdleRaceUnderLoad budget for CI variance"
status: complete
assessment_status: complete
provenance:
  source: issue
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: "2026-04-19"
related_plans:
  - "2026-04-19-renewer-waitidle-flake.md"
---

# Bump TestRenewer_WaitIdleRaceUnderLoad budget for CI variance

## Overview

`TestRenewer_WaitIdleRaceUnderLoad` (`internal/scheduler/renewer_test.go:112`) ran 5.11s on CI run 24645196869 against a 5s outer-deadline budget and timed out, firing `tick/WaitIdle loop hung — WaitIdle race regression`. The underlying fix from the previous workstream (`docs/plans/2026-04-19-renewer-waitidle-flake.md`) is correct — there is no real race. The failure is a budget problem: 200 cycles * (CI scheduling jitter + `-race` overhead) overran the test's 5-second `time.After` guard.

This plan widens the outer budget to 30 seconds. That is enough headroom for the slowest plausible CI runner while still catching the original failure mode.

### Why option 1 (budget bump), not redesign

Three options were considered:

1. **Bump budget 5s → 30s.** One-line change.
2. **Redesign deadline-agnostic.** Replace the outer `time.After` with a per-cycle deadline, e.g. `WaitIdle` wrapped in a 100 ms `context.WithTimeout` asserted on each iteration.
3. **Hybrid.** Generous outer budget plus a per-cycle assertion.

Option 1 is correct here:

- **The original failure mode is an unbounded hang.** The race the test was written to catch (`signalIdle` running before `WaitIdle` snapshots the channel) leaves both goroutines parked forever. The next tick is never sent, so the test only exits via the Go runtime's outer test timeout — currently `-timeout 10m` by default. **Any** finite outer budget far below 10 minutes catches the regression. 30 s catches it just as definitively as 5 s.
- **Per-cycle deadlines are more code and weaker signal.** A per-cycle timeout would have to pick a threshold (e.g. 100 ms) that is itself sensitive to CI jitter — exactly the failure we are trying to avoid. It adds a `context.WithTimeout`/cancel pair plus a select per iteration, all to detect a failure mode that is *already* detected by the outer guard. YAGNI.
- **Hybrid combines the downsides.** Two thresholds, two failure messages, two ways for the test itself to drift.
- **Idiomatic Go.** Bounded `time.After` around a worker goroutine is the standard pattern; widening the bound when CI gets slower is the standard fix.

The CI run completed in 5.11 s. 30 s is roughly a 6x margin against observed worst-case wall time, which absorbs `-race` overhead, shared-runner contention, and future cycle-count growth without reaching anywhere near the test-binary timeout.

## Prerequisites

- Working tree clean.
- `mise run test` currently green locally; the failure only reproduces under CI load.

## Task breakdown

### Task 1: Widen the outer-deadline guard from 5 s to 30 s

**Files:**
- Modify: `internal/scheduler/renewer_test.go:149`

**Step 1: Edit the timeout constant**

Open `internal/scheduler/renewer_test.go`. In `TestRenewer_WaitIdleRaceUnderLoad` (line 112), find the select that currently reads:

```go
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tick/WaitIdle loop hung — WaitIdle race regression")
	}
```

Change `5 * time.Second` to `30 * time.Second`. The fatal message is unchanged — the regression it catches is still the same unbounded hang.

After the edit, the block reads:

```go
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("tick/WaitIdle loop hung — WaitIdle race regression")
	}
```

**Step 2: Run the targeted test under `-race` to confirm it still passes well within budget**

Run:

```
go test -race -count=10 -run '^TestRenewer_WaitIdleRaceUnderLoad$' ./internal/scheduler/...
```

Expected: `ok  github.com/Work-Fort/Flow/internal/scheduler` ten times, no FAIL, no DATA RACE. Each run should complete in well under 30 s (locally typically <1 s; this just confirms the new guard is not regressed).

**Step 3: Run the full scheduler package suite under `-race`**

Run:

```
go test -race ./internal/scheduler/...
```

Expected: PASS, no race warnings.

**Step 4: Run the project test task**

Run: `mise run test`
Expected: PASS.

### Task 2: Commit

**Files:**
- Modify: `internal/scheduler/renewer_test.go`

**Step 1: Stage the modified file**

Run: `git add internal/scheduler/renewer_test.go`

**Step 2: Commit**

Use a HEREDOC so the body stays multi-line:

```
git commit -m "$(cat <<'EOF'
test(scheduler): widen WaitIdleRaceUnderLoad budget for CI variance

The 200-cycle tick/WaitIdle loop ran 5.11s on CI run 24645196869,
overrunning its 5s outer-deadline guard and firing the regression
fatal even though no real race occurred. The race the test catches
leaves both goroutines parked forever, so any finite outer budget
well below the test-binary timeout still catches it; 5s was simply
too tight for CI scheduling jitter plus -race overhead.

Bump the guard to 30s. That is roughly 6x the observed CI wall time
and still orders of magnitude below the default 10m test timeout, so
the regression remains caught without false positives.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

**Step 3: Verify the commit landed**

Run: `git log -1 --oneline`
Expected: a single line beginning `test(scheduler):`.

## Verification checklist

- [ ] `internal/scheduler/renewer_test.go` line in `TestRenewer_WaitIdleRaceUnderLoad` reads `case <-time.After(30 * time.Second):`.
- [ ] No other change to the test (iteration count, fatal message, tick-send loop, renewCalls assertion all unchanged).
- [ ] `go test -race -count=10 -run '^TestRenewer_WaitIdleRaceUnderLoad$' ./internal/scheduler/...` passes 10/10.
- [ ] `go test -race ./internal/scheduler/...` passes.
- [ ] `mise run test` passes.
- [ ] One commit, conventional subject `test(scheduler):`, multi-line body, `Co-Authored-By: Claude Sonnet 4.6` trailer, no `!`, no `BREAKING CHANGE:`.
