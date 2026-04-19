---
type: plan
step: "fix"
title: "Fix LeaseRenewer.WaitIdle race causing CI hang"
status: pending
assessment_status: needed
provenance:
  source: issue
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: null
  completed: null
related_plans:
  - "2026-04-18-flow-orchestration-01-foundation.md"
---

# Fix LeaseRenewer.WaitIdle race causing CI hang

## Overview

`TestRenewer_DropsClaimOnMismatch` (`internal/scheduler/renewer_test.go:71`) hangs in CI for the full 9-minute test timeout. Goroutine dump shows:

- Goroutine 39 (test): blocked at `chan receive` inside `WaitIdle` (`renewer.go:100`).
- Goroutine 43 (renewer): blocked at `select` inside `Run` (`renewer.go:82`), having already finished one tick.

Test passes locally because the race window is tiny; CI scheduling pressure widens it enough to lose the signal.

### The race

`Run` (`renewer.go:74-91`) handles a tick by calling `renewOnce(ctx)` then `signalIdle()`. `signalIdle` closes the current `idleCh` and immediately replaces it with a fresh, un-closed channel under `idleMu`.

`WaitIdle` (`renewer.go:96-101`) snapshots `idleCh` under `idleMu` then waits for it to close.

The test sequence is:

```go
tickCh <- time.Now()   // (1) send unblocks the moment Run reads tickCh
r.WaitIdle()           // (2) snapshot idleCh, wait for close
```

`tickCh` is unbuffered, so the send completes the instant `Run`'s `case <-tickCh` accepts the value. At that point Run has **not yet** called `renewOnce` or `signalIdle`. Two interleavings then race:

- **Slow Run (local):** test reaches `WaitIdle`, snapshots the original `idleCh`, and parks on it. Run later calls `signalIdle`, which closes that channel. WaitIdle returns. Test passes.
- **Fast Run (CI):** Run finishes `renewOnce` and enters `signalIdle` *before* the test reaches `WaitIdle`. `signalIdle` closes the original `idleCh` and replaces it with a fresh channel. The test then runs `WaitIdle`, snapshots the **new** (un-closed) channel, and blocks. The next tick never arrives — the test only sends one — so Run sits at the select and the test hangs until timeout.

The bug is that "did the next tick complete?" is encoded as "the channel I observe right now will close." Whether the close has already happened is invisible to the observer.

### The fix

Replace the close-and-replace channel with a monotonic counter of completed ticks plus a `sync.Cond`. `WaitIdle` snapshots the counter and waits until it strictly increases. This makes "the next tick after I called WaitIdle" well-defined regardless of scheduling.

This is a minimal, race-free pattern — no new dependencies, no API change for callers (`WaitIdle()` keeps the same signature).

## Prerequisites

- Working tree clean.
- `mise run test` (or `go test ./...`) currently green locally; the bug only reproduces under CI load.

## Task breakdown

### Task 1: Replace close-and-replace channel with counter+Cond

**Files:**
- Modify: `internal/scheduler/renewer.go` (struct + `WaitIdle` + `signalIdle` + `NewLeaseRenewer`)

**Step 1: Replace the `idleCh` field and `signalIdle` / `WaitIdle` implementation**

Edit `internal/scheduler/renewer.go`. Change the struct, constructor, and idle-signalling helpers as follows.

Replace the struct (lines 35-44) with:

```go
// LeaseRenewer is a background goroutine that keeps every claim the
// live Flow process holds alive by calling Hive's renew endpoint every
// Interval until its context is cancelled.
type LeaseRenewer struct {
	sch      *Scheduler
	hive     domain.HiveAgentClient
	interval time.Duration
	ttl      time.Duration
	tick     <-chan time.Time

	idleMu   sync.Mutex
	idleCond *sync.Cond
	ticks    uint64 // number of completed renewOnce calls; monotonic
}
```

Replace the constructor body (lines 60-68) — the `idleCh: make(chan struct{})` line goes away; initialise the cond instead:

```go
	r := &LeaseRenewer{
		sch:      cfg.Scheduler,
		hive:     cfg.Hive,
		interval: cfg.Interval,
		ttl:      cfg.LeaseTTL,
		tick:     cfg.Tick,
	}
	r.idleCond = sync.NewCond(&r.idleMu)
	return r
```

Replace `WaitIdle` (lines 93-101) with:

```go
// WaitIdle blocks until the renewer completes at least one renewOnce
// after the call begins. Used by tests after sending a manual tick to
// wait for the renewer to drain it. Production code does not call this.
func (r *LeaseRenewer) WaitIdle() {
	r.idleMu.Lock()
	defer r.idleMu.Unlock()
	start := r.ticks
	for r.ticks == start {
		r.idleCond.Wait()
	}
}
```

Replace `signalIdle` (lines 103-108) with:

```go
func (r *LeaseRenewer) signalIdle() {
	r.idleMu.Lock()
	r.ticks++
	r.idleCond.Broadcast()
	r.idleMu.Unlock()
}
```

Leave `Run` and `renewOnce` untouched — `Run` still calls `r.signalIdle()` after each `renewOnce`.

**Step 2: Verify the package builds**

Run: `go build ./internal/scheduler/...`
Expected: no output, exit 0.

### Task 2: Add a regression test that reliably reproduces the original race

**Files:**
- Modify: `internal/scheduler/renewer_test.go` (append a new test at end of file)

**Step 1: Write the regression test**

Append to `internal/scheduler/renewer_test.go`:

```go
// TestRenewer_WaitIdleRaceUnderLoad exercises the WaitIdle/signalIdle
// pairing under repeated tick-then-wait cycles. With the original
// close-and-replace idleCh implementation, this test would hang for
// the test timeout when signalIdle ran before WaitIdle snapshotted the
// channel. The counter-based implementation must complete deterministically.
func TestRenewer_WaitIdleRaceUnderLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})
	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-load", time.Minute); err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch, Hive: hive, Tick: tickCh, LeaseTTL: time.Minute,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); r.Run(ctx) }()

	const iterations = 200
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < iterations; i++ {
			tickCh <- time.Now()
			r.WaitIdle()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tick/WaitIdle loop hung — WaitIdle race regression")
	}

	cancel()
	wg.Wait()

	hive.mu.Lock()
	defer hive.mu.Unlock()
	if hive.renewCalls != iterations {
		t.Errorf("renew calls: got %d, want %d", hive.renewCalls, iterations)
	}
}
```

**Step 2: Run the new test plus the previously flaking tests**

Run: `go test -race -count=10 -run 'TestRenewer_(DropsClaimOnMismatch|RenewsEveryActiveClaim|WaitIdleRaceUnderLoad)$' ./internal/scheduler/...`
Expected: `ok  github.com/Work-Fort/Flow/internal/scheduler` 10x, no FAIL, no DATA RACE.

Rationale for `-count=10`: confirms the fix is stable across repeated runs, not just lucky on one run. The 5-second guard inside the test bounds total runtime even under `-race`.

**Step 3: Run the full package test suite**

Run: `mise run test` (or, if no mise alias for a single package, `go test -race ./internal/scheduler/...`)
Expected: PASS, no race warnings.

### Task 3: Commit

**Files:**
- Modify: `internal/scheduler/renewer.go`
- Modify: `internal/scheduler/renewer_test.go`

**Step 1: Stage the two modified files**

Run: `git add internal/scheduler/renewer.go internal/scheduler/renewer_test.go`

**Step 2: Commit**

Use a HEREDOC to keep the body multi-line:

```
git commit -m "$(cat <<'EOF'
fix(scheduler): make LeaseRenewer.WaitIdle race-free

WaitIdle previously snapshotted an idleCh that signalIdle would
close-and-replace after every renewOnce. When signalIdle ran before
WaitIdle's snapshot — common under CI load — WaitIdle observed the
fresh, un-closed replacement and blocked until the test timeout.

Replace the channel with a monotonic tick counter guarded by a
sync.Cond. WaitIdle records the current count and waits until it
strictly increases, so "the next renewOnce after I called WaitIdle" is
well-defined regardless of goroutine scheduling.

Add a regression test that runs 200 tick/WaitIdle cycles under -race
within a 5s budget; the original implementation hangs this test, the
fix completes it deterministically.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

**Step 3: Verify the commit landed**

Run: `git log -1 --oneline`
Expected: a single line beginning `fix(scheduler):`.

## Verification checklist

- [ ] `internal/scheduler/renewer.go` no longer references `idleCh` (counter+Cond only).
- [ ] `WaitIdle` and `signalIdle` both take `idleMu`; `WaitIdle` uses `idleCond.Wait()`.
- [ ] `Run` is unchanged apart from continuing to call `r.signalIdle()` after each `renewOnce`.
- [ ] `go test -race -count=10 -run TestRenewer_ ./internal/scheduler/...` passes (10/10).
- [ ] `go test -race ./...` passes.
- [ ] One commit, conventional subject `fix(scheduler):`, multi-line body, `Co-Authored-By: Claude Sonnet 4.6` trailer, no `!`, no `BREAKING CHANGE:`.
