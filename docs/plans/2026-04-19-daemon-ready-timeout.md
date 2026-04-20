---
type: plan
step: "fix"
title: "Widen e2e daemon-ready timeouts for CI parallel-load contention"
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
related_plans: []
---

# Widen e2e daemon-ready timeouts for CI parallel-load contention

## Overview

CI run 24646023228 (Release rerun) failed with:

```
daemon did not become ready on 127.0.0.1:39617 within 5s
```

inside `TestTemplates_CreateGetUpdateDelete`. The failure originates from `tests/e2e/harness/daemon.go:177`, where `StartDaemon` waits 5 seconds for the spawned `flow daemon` subprocess to bind its listener and answer `/v1/health`.

Under `mise run ci` the test job, the lint job, and the e2e job all run in parallel on the same 2-core GitHub-hosted runner, contending for CPU and for the same Postgres service container. A 5-second startup budget is the floor of what the daemon needs locally with no contention; on a saturated CI runner it is below the floor and the test flakes spuriously even though the daemon is healthy.

This plan widens daemon-readiness budgets to 30 seconds across the e2e harness. The new budget gives ~6x headroom over typical local startup, absorbs CI scheduler jitter and Postgres-container contention, and is still small enough that a genuinely stuck daemon fails the test in a useful timeframe (no test reaches the default `-timeout 10m` outer bound).

### Scope of timeout changes

There are four 5-second `time.After`/deadline values in the e2e harness daemon helpers. They split into two distinct categories:

1. **Daemon-readiness waits.** These poll a freshly-spawned daemon's HTTP `/health` endpoint until the listener answers. They are subject to CI parallel-load contention: the daemon needs CPU to start, plus Postgres to accept the schema-reset connection (Flow daemon) or to initialise containerd (Nexus daemon). All such waits get bumped 5s → 30s.

2. **Shutdown grace periods.** These wait for a daemon process to exit cleanly after SIGTERM before escalating to SIGKILL. They are not subject to startup contention; the daemon is already running and just needs to flush state and unwind goroutines. They are **out of scope** for this plan.

| Location                                       | Purpose                                       | Category    | Action       |
|------------------------------------------------|-----------------------------------------------|-------------|--------------|
| `tests/e2e/harness/daemon.go:177`              | Flow daemon `/v1/health` readiness            | readiness   | **bump**     |
| `tests/e2e/harness/nexus_daemon.go:94`         | Nexus probe daemon `/health` readiness        | readiness   | **bump**     |
| `tests/e2e/harness/nexus_daemon.go:230`        | Nexus full daemon `/health` readiness         | readiness   | **bump**     |
| `tests/e2e/harness/nexus_daemon.go:260`        | Nexus daemon SIGTERM-then-SIGKILL grace       | shutdown    | **leave**    |
| `tests/e2e/harness/daemon.go:210`              | Flow daemon SIGTERM-then-SIGKILL grace        | shutdown    | **leave** (out of scope) |

#### Per-line justification

- **`daemon.go:177` (Flow daemon readiness, 5s → 30s).** This is the timeout that actually fired in CI. The daemon spawned here runs goose migrations against Postgres and binds an HTTP listener before answering `/v1/health`; both steps share CPU and Postgres connections with the parallel `mise run ci` jobs. Bump.

- **`nexus_daemon.go:94` (Nexus probe-daemon readiness, 5s → 30s).** `requireNexusCloneDriveEndpoint` spawns a minimal Nexus daemon to detect whether the `POST /v1/drives/clone` route is registered. It runs as part of `RequireNexusBinary`, which every Nexus-driven e2e test calls in setup. Same CI runner, same parallel-load profile, plus containerd init. Bump.

- **`nexus_daemon.go:230` (Nexus full-daemon readiness, 5s → 30s).** `StartNexusDaemon` spawns the Nexus daemon used by Flow's `NexusDriver` e2e tests. Same readiness semantics as the probe daemon above and same CI exposure. Bump.

- **`nexus_daemon.go:260` (Nexus shutdown grace, leave at 5s).** This is the `time.After` inside `(*NexusDaemon).stop` that bounds how long we wait for `cmd.Wait` to return after sending SIGTERM. It does not gate readiness; it gates the *teardown* path. Nexus shutdown is fast (no migrations to run, no client connections to drain in the e2e configuration), and bumping it would make a stuck shutdown take six times longer to escalate to SIGKILL. **Leave as-is.**

- **`daemon.go:210` (Flow shutdown grace, out of scope).** Same reasoning as `nexus_daemon.go:260`. Not touched by this plan.

### Why widen rather than redesign

The same options that came up for the renewer-budget plan apply here:

1. **Bump the deadline.** Smallest change. The readiness waits already use a poll loop (`waitReady` in `daemon.go:260` and the inline poll loops in `nexus_daemon.go`); they exit as soon as `/health` answers, so a wider budget only matters when the daemon is actually slow to start. There is no cost to legitimate fast-path runs.

2. **Health-condition-aware retry.** Replace the time bound with an adaptive backoff or a startup-event hook on the daemon. More code, no clear payoff: the daemon already exposes `/health`, and the `200 ms` HTTP timeout per probe (line 263) plus the `50 ms` poll sleep (line 270) already produce ~5 probes per second — the loop is not failing for lack of probe density, it is failing because the daemon hasn't bound its listener yet.

3. **Per-test budget overrides.** Push the budget into each test. Spreads the same constant across N call sites and invites drift.

Option 1 is correct. The harness already has the right shape; only the literal needs to move.

## Prerequisites

- Working tree clean.
- `mise run test` and (locally) `mise run e2e` currently pass.

## Task breakdown

### Task 1: Widen Flow daemon readiness budget

**Files:**
- Modify: `tests/e2e/harness/daemon.go:177`

**Step 1: Edit the `StartDaemon` readiness wait**

Open `tests/e2e/harness/daemon.go`. Find the call inside `StartDaemon` (around line 177):

```go
	if err := waitReady(addr, 5*time.Second); err != nil {
		d.kill()
		return nil, err
	}
```

Change `5*time.Second` to `30*time.Second` and add a brief comment explaining the CI rationale. The block becomes:

```go
	// 30s, not 5s: under `mise run ci` the test, lint, and e2e jobs
	// share a 2-core GitHub runner plus the Postgres service container;
	// daemon startup (goose migrations + listener bind) overran 5s on
	// run 24646023228. 30s is well below the test-binary outer timeout
	// while absorbing CI scheduler jitter and PG contention.
	if err := waitReady(addr, 30*time.Second); err != nil {
		d.kill()
		return nil, err
	}
```

**Step 2: Confirm no other readiness call site needs updating in this file**

Run:

```
grep -n 'waitReady' tests/e2e/harness/daemon.go
```

Expected: two lines — the call inside `StartDaemon` (now `30*time.Second`) and the function definition `func waitReady(addr string, timeout time.Duration) error` at line 260. The function itself is parameterised on `timeout`, so no body change is needed.

### Task 2: Widen Nexus probe-daemon readiness budget

**Files:**
- Modify: `tests/e2e/harness/nexus_daemon.go:94`

**Step 1: Edit the probe-daemon health-poll deadline**

Open `tests/e2e/harness/nexus_daemon.go`. Inside `requireNexusCloneDriveEndpoint`, find the block around line 93:

```go
	// Wait for /health.
	deadline := time.Now().Add(5 * time.Second)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get("http://" + addr + "/health"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
```

Change `5 * time.Second` to `30 * time.Second` and update the comment:

```go
	// Wait for /health. 30s for the same CI parallel-load reason as
	// flow daemon startup — see daemon.go:177.
	deadline := time.Now().Add(30 * time.Second)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get("http://" + addr + "/health"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
```

The `300 * time.Millisecond` per-request HTTP timeout and the `50 * time.Millisecond` sleep are unchanged — those control probe density, not the overall budget.

### Task 3: Widen Nexus full-daemon readiness budget

**Files:**
- Modify: `tests/e2e/harness/nexus_daemon.go:230`

**Step 1: Edit the StartNexusDaemon health-poll deadline**

Still in `tests/e2e/harness/nexus_daemon.go`. Inside `StartNexusDaemon`, find the block around line 229:

```go
	// Wait for /health to respond.
	deadline := time.Now().Add(5 * time.Second)
	healthURL := "http://" + addr + "/health"
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 200 * time.Millisecond}
		if resp, err := client.Get(healthURL); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
```

Change `5 * time.Second` to `30 * time.Second` and update the comment:

```go
	// Wait for /health to respond. 30s for the same CI parallel-load
	// reason as flow daemon startup — see daemon.go:177.
	deadline := time.Now().Add(30 * time.Second)
	healthURL := "http://" + addr + "/health"
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 200 * time.Millisecond}
		if resp, err := client.Get(healthURL); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
```

**Step 2: Verify the shutdown-grace `5 * time.Second` at line ~260 is unchanged**

Run:

```
grep -n '5 \* time.Second' tests/e2e/harness/nexus_daemon.go
```

Expected: one remaining match — the `case <-time.After(5 * time.Second):` inside `(*NexusDaemon).stop`. That is the SIGTERM→SIGKILL escalation grace period and is intentionally left at 5s (see plan rationale).

### Task 4: Local verification

**Files:** none modified.

**Step 1: Build the e2e harness package**

Run:

```
go build ./tests/e2e/harness/...
```

Expected: no output, exit 0. Verifies the edits compile.

**Step 2: Run `mise run test`**

Run: `mise run test`
Expected: PASS. (The unit-test suite does not exercise the e2e harness directly, but the build above plus this run confirm nothing else is broken.)

**Step 3: Run a fast slice of the e2e suite locally to confirm the readiness path still works**

Run:

```
go test -count=1 -run '^TestTemplates_CreateGetUpdateDelete$' ./tests/e2e/...
```

Expected: PASS. The widened budget should not change local timing — daemon readiness completes in well under 5 s locally, so the new 30 s ceiling is never approached.

### Task 5: Commit

**Files:**
- Modify: `tests/e2e/harness/daemon.go`
- Modify: `tests/e2e/harness/nexus_daemon.go`

**Step 1: Stage the modified files**

Run:

```
git add tests/e2e/harness/daemon.go tests/e2e/harness/nexus_daemon.go
```

**Step 2: Commit using a HEREDOC for the multi-line body**

```
git commit -m "$(cat <<'EOF'
test(e2e): widen daemon-ready timeouts to 30s for CI contention

CI run 24646023228 failed TestTemplates_CreateGetUpdateDelete with
"daemon did not become ready on 127.0.0.1:39617 within 5s". Under
`mise run ci` the test, lint, and e2e jobs share a 2-core GitHub
runner plus one Postgres service container; flow daemon startup
(goose migrations + listener bind) overran the 5s budget.

Bump readiness waits in the e2e harness from 5s to 30s in three
places:

  - tests/e2e/harness/daemon.go:177       flow daemon /v1/health
  - tests/e2e/harness/nexus_daemon.go:94  nexus probe /health
  - tests/e2e/harness/nexus_daemon.go:230 nexus full /health

The shutdown-grace `time.After(5 * time.Second)` calls in both
files are left alone — they bound SIGTERM→SIGKILL escalation, not
startup, and are not affected by CI startup contention.

30s is roughly 6x typical local startup and well below the default
10m test-binary timeout, so a genuinely stuck daemon still fails
quickly while CI jitter no longer flakes the suite.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

**Step 3: Verify the commit landed**

Run: `git log -1 --oneline`
Expected: a single line beginning `test(e2e):`.

## Verification checklist

- [ ] `tests/e2e/harness/daemon.go` line in `StartDaemon` reads `if err := waitReady(addr, 30*time.Second); err != nil {` with the CI-rationale comment immediately above.
- [ ] `tests/e2e/harness/nexus_daemon.go` `requireNexusCloneDriveEndpoint` reads `deadline := time.Now().Add(30 * time.Second)` with an updated comment.
- [ ] `tests/e2e/harness/nexus_daemon.go` `StartNexusDaemon` reads `deadline := time.Now().Add(30 * time.Second)` with an updated comment.
- [ ] `(*NexusDaemon).stop` SIGTERM-grace `case <-time.After(5 * time.Second):` is **unchanged**.
- [ ] `(*Daemon).Stop` SIGTERM-grace `case <-time.After(5 * time.Second):` in `daemon.go` is **unchanged** (not touched by this plan).
- [ ] `go build ./tests/e2e/harness/...` succeeds.
- [ ] `mise run test` passes.
- [ ] `go test -count=1 -run '^TestTemplates_CreateGetUpdateDelete$' ./tests/e2e/...` passes locally.
- [ ] One commit, conventional subject `test(e2e):`, multi-line body, `Co-Authored-By: Claude Sonnet 4.6` trailer, no `!`, no `BREAKING CHANGE:`.
