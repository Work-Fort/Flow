---
type: plan
step: "1"
title: "flow e2e harness — orphan-leak hardening"
status: approved
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - 2026-04-18-flow-e2e-harness-01-foundation.md
---

# Flow E2E Harness — Orphan-Leak Hardening

**Goal:** Stop the e2e harness from leaking orphan processes when the
`flow daemon` subprocess exits before its stderr writers drain.
The current `tests/e2e/harness/daemon.go` (just landed in the foundation
plan, not yet pushed) wires `cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)`,
which makes `exec.Cmd` create a pipe and a copy goroutine. If the daemon
ever spawns a descendant that inherits the pipe write end (or simply
exits while a buffered write is in flight), `cmd.Wait()` blocks until
the workflow timeout cancels the CI step. Sharkfin's CI hung 55 minutes
the same week from the same bug class.

**Canonical fix** (see `/home/kazw/Work/WorkFort/skills/lead/go-service-architecture/references/architecture-reference.md` — section
"Orphan-Process Hardening (Required)"):

1. **`Setpgid: true`** in `cmd.SysProcAttr` so the daemon and any
   descendants share a process group equal to the daemon PID.
2. **`*os.File` for stdout/stderr** (not `io.Writer`) — eliminates the
   copy goroutine that holds the read end.
3. **Negative-pid kill** (`syscall.Kill(-pgid, sig)`) on cleanup so
   leaked descendants get the same signal as the daemon.
4. **`cmd.WaitDelay = 10 * time.Second`** so `cmd.Wait()` force-closes
   I/O if any pipe outlives the process.

All four parts are load-bearing — drop one and the leak returns.

**Repo specifics.** Flow only spawns one subprocess (`build/flow daemon`);
there are no helpers, no MCP bridge, no shims. The current harness
already captures stderr to a `bytes.Buffer` so the test can grep it for
`DATA RACE` after stop — that capture is preserved by switching to a
temp file we read at stop time. The logged tee to `os.Stderr` (line 117)
is preserved by writing the temp file path with `t.Logf` on failure
instead, so a failing run still dumps daemon output without inheriting
the pipe.

**Tech stack:** Go 1.26, `os/exec`, `syscall`. No new dependencies.

**Commands:** `mise run e2e` (the existing task at `.mise/tasks/e2e`)
runs `mise run build:dev` then `cd tests/e2e && go test -race -count=1 ./...`.
Targeted TDD runs use `cd tests/e2e && go test -run TestX ./harness/...`
per planner.md's native-test-runner exception.

---

## Prerequisites

- The foundation plan (`2026-04-18-flow-e2e-harness-01-foundation.md`)
  is committed locally. This plan applies on top before the foundation
  is pushed.
- `build/flow` produced by `mise run build:dev` is the only subprocess
  the harness spawns.
- Go is `1.26` per `mise.toml` — `cmd.WaitDelay` (added in 1.20) is
  available.

---

## Conventions

- Run all build/test commands via `mise run <task>` from `flow/lead/`.
  Targeted go test runs are permitted from inside `tests/e2e/`.
- Commit after each task with the multi-line conventional-commits
  HEREDOC and the Co-Authored-By trailer below. Release tooling depends
  on this format.

```bash
git add <files>
git commit -m "$(cat <<'EOF'
<type>(<scope>): <description>

<body explaining why, not what>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task Breakdown

### Task 1: Write the failing leak-detection test

**Files:**
- Create: `tests/e2e/harness/daemon_leak_test.go`

**Step 1: Write the test**

The test starts a daemon, calls `Stop`, then asserts no live process
remains in the daemon's process group. Without the fix, this fails
immediately on a freshly built daemon because `Setpgid` is never set
— `getpgid(daemonPID)` returns the harness's group, which the test
must explicitly skip. With the fix, `getpgid(daemonPID)` returns the
daemon's PID (its own pgid) and `kill(-pgid, 0)` returns ESRCH after
`Stop` because every member of the group is gone.

```go
// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestDaemonStop_KillsProcessGroup(t *testing.T) {
	if os.Getenv("FLOW_BINARY") == "" {
		// NewEnv falls back to ../../build/flow when FLOW_BINARY is
		// unset; the leak test still needs a real binary to spawn.
		if _, err := os.Stat("../../build/flow"); err != nil {
			t.Skip("FLOW_BINARY not set and ../../build/flow missing; run via 'mise run e2e'")
		}
	}

	// NewEnv wires the JWKS stub, Pylon stub, fake Hive, fake Sharkfin
	// and spawns the daemon. We use it instead of calling StartDaemon
	// directly so the leak test exercises the same code path as every
	// other e2e test.
	env := NewEnv(t)

	pid := env.Daemon.cmd.Process.Pid

	// pgid must equal pid because StartDaemon sets Setpgid.
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Fatalf("daemon pgid = %d, want %d (Setpgid not set)", pgid, pid)
	}
	// Defence against the (vanishingly rare) case where the test
	// process itself happens to be in a group whose id equals the
	// daemon PID — that would let pgid == pid pass spuriously.
	if pgid == os.Getpid() {
		t.Fatalf("daemon pgid (%d) equals harness pid; daemon inherited harness group", pgid)
	}

	env.Cleanup(t) // tears down daemon + stubs in reverse order

	// After Cleanup, signalling the group with sig 0 must report no
	// such process — the canonical "is the group empty?" check.
	// Use errors.Is (not direct ==) because syscall.Errno implements
	// the errors.Is contract and errors.Is is the idiomatic choice.
	err = syscall.Kill(-pgid, 0)
	if !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill(-%d, 0) = %v, want ESRCH (group still has live members)", pgid, err)
	}
}
```

The test exercises `NewEnv` (which calls `StartDaemon` after setting
up the JWKS stub via `StartJWKSStub()` and the Pylon stub via
`StartPylonStub(pylonServices)` — both established by the foundation
plan). The assertions on `pgid`, `pgid != os.Getpid()`, and
`kill(-pgid, 0) == ESRCH` are the load-bearing part.

**Step 2: Run the test to verify it fails**

Run from `flow/lead/`:

```
mise run build:dev
cd tests/e2e && go test -run TestDaemonStop_KillsProcessGroup -count=1 ./harness/...
```

Expected: FAIL with `daemon pgid = <harness_pgid>, want <daemon_pid> (Setpgid not set)`.
This proves the test catches the missing `Setpgid`.

**Step 3: Commit the failing test**

```bash
git add tests/e2e/harness/daemon_leak_test.go
git commit -m "$(cat <<'EOF'
test(e2e): add failing TestDaemonStop_KillsProcessGroup

Asserts the daemon spawns into its own process group and that Stop
empties the group. Currently fails because StartDaemon does not set
Setpgid; the next task fixes the harness.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Apply the four-part canonical fix to `StartDaemon`/`Stop`

**Depends on:** Task 1

**Files:**
- Modify: `tests/e2e/harness/daemon.go` — `StartDaemon`, `Stop`, `kill`,
  and the `Daemon` struct

**Step 1: Replace the stderr-buffer field with a temp file**

The current `Daemon` struct (line 42-49) holds `stderr *bytes.Buffer`
written by `io.MultiWriter`. Replace it with `stderrFile *os.File`.

Modify `tests/e2e/harness/daemon.go`. Replace the imports block and
the `Daemon` struct (lines 4-49) so it reads:

```go
import (
	"bytes"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// daemonCfg captures the per-spawn configuration. Tests build it via
// DaemonOption helpers.
type daemonCfg struct {
	pylonAddr      string
	passportAddr   string
	webhookBaseURL string
	dbDSN          string
}

type DaemonOption func(*daemonCfg)

func WithWebhookBaseURL(u string) DaemonOption {
	return func(c *daemonCfg) { c.webhookBaseURL = u }
}

func WithDB(dsn string) DaemonOption {
	return func(c *daemonCfg) { c.dbDSN = dsn }
}

// Daemon represents a spawned flow daemon subprocess.
type Daemon struct {
	cmd        *exec.Cmd
	addr       string
	xdgDir     string
	stderrFile *os.File // temp file backing stdout+stderr; read at Stop time
	signJWT    func(id, username, displayName, userType string) string
	stops      []func()
}
```

The `io` import is no longer needed; `bytes` stays for the
`bytes.Contains` check at stop time.

**Step 2: Rewrite `StartDaemon`'s spawn block**

Replace lines 110-122 (the stderr buffer setup, `cmd := exec.Command...`,
and `cmd.Start`) with:

```go
	stderrFile, err := os.CreateTemp("", "flow-e2e-stderr-*")
	if err != nil {
		os.RemoveAll(xdgDir)
		return nil, fmt.Errorf("create stderr temp file: %w", err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+xdgDir+"/config",
		"XDG_STATE_HOME="+xdgDir+"/state",
	)
	// *os.File (not io.Writer) so exec.Cmd does not create a copy
	// goroutine; Setpgid puts the daemon and any descendants in a
	// fresh process group; WaitDelay force-closes any inherited fds
	// after the daemon exits. See the orphan-process hardening
	// section of go-service-architecture.
	cmd.Stdout = stderrFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		stderrFile.Close()
		os.Remove(stderrFile.Name())
		os.RemoveAll(xdgDir)
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	d := &Daemon{
		cmd: cmd, addr: addr, xdgDir: xdgDir,
		stderrFile: stderrFile, signJWT: signJWT,
	}
```

The `bytes.Buffer` and `io.MultiWriter` are gone. The live tee to
`os.Stderr` is gone too — restored on failure by the `t.Logf` block
in `Stop`.

**Step 3: Rewrite `Stop` to signal the process group**

Replace the existing `Stop` (lines 149-170) with:

```go
// Stop sends SIGTERM to the daemon's process group, waits up to 5s,
// then SIGKILLs the group. Cleans tempdir. Fails the test if the
// daemon emitted a DATA RACE marker on stderr. On test failure, dumps
// the captured stderr to t.Logf so a daemon panic, fatal log line,
// or context-cancel chain explains the failure even after stderr
// scrolled off the live tty.
func (d *Daemon) Stop(t testing.TB) {
	t.Helper()
	if d.cmd.Process != nil {
		// pgid == pid because of Setpgid; signal the whole group so
		// any leaked descendants die with the daemon.
		pgid := d.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- d.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("daemon did not exit after SIGTERM, killing group")
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
	}

	var stderrBytes []byte
	if d.stderrFile != nil {
		// Read the captured output before unlinking.
		stderrBytes, _ = os.ReadFile(d.stderrFile.Name())
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
	}

	if t.Failed() && len(stderrBytes) > 0 {
		t.Logf("daemon stderr:\n%s", stderrBytes)
	}
	os.RemoveAll(d.xdgDir)
	if bytes.Contains(stderrBytes, []byte("DATA RACE")) {
		t.Fatal("data race detected in daemon (see stderr above)")
	}
}
```

**Step 4: Rewrite `kill` to match**

The existing `kill` (lines 172-178) is the failure-path cleanup used
by `StartDaemon` if `waitReady` fails. Replace it with:

```go
func (d *Daemon) kill() {
	if d.cmd.Process != nil {
		pgid := d.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		d.cmd.Wait()
	}
	if d.stderrFile != nil {
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
	}
	os.RemoveAll(d.xdgDir)
}
```

**Step 5: Run the leak test to verify it passes**

Run from `flow/lead/`:

```
mise run build:dev
cd tests/e2e && go test -run TestDaemonStop_KillsProcessGroup -count=1 ./harness/...
```

Expected: PASS. The daemon pgid now equals its PID, and the group is
empty after `Stop`.

**Step 6: Run the full e2e suite to verify no regression**

Run: `mise run e2e`

Expected: PASS. Existing tests still see the daemon start, hit health
endpoints, and shut down cleanly. Stderr capture still feeds the
DATA RACE check and the on-failure log dump.

**Step 7: Commit**

```bash
git add tests/e2e/harness/daemon.go
git commit -m "$(cat <<'EOF'
fix(e2e): harden daemon harness against orphan-process leaks

Spawn the flow daemon into its own process group (Setpgid), capture
stdout+stderr to an *os.File instead of an io.MultiWriter (eliminates
the copy goroutine that holds pipe fds), signal the whole group on
shutdown (kill(-pgid, ...)), and set WaitDelay so cmd.Wait force-
closes any inherited fd after the daemon exits.

Without this, a daemon that buffers output through a pipe can leave
the harness blocked in cmd.Wait until the CI workflow timeout fires.
Sharkfin's e2e step hung ~55 minutes from the same bug class.

Implements the canonical e2e-harness orphan-leak hardening pattern
documented in skills/lead/go-service-architecture/references/architecture-reference.md
(section "Orphan-Process Hardening (Required)").

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Verify cleanup is bounded under simulated test failure

**Depends on:** Task 2

**Files:**
- (Temporary, reverted in Step 4) `tests/e2e/health_test.go` — inject a
  forced failure to confirm `mise run e2e` returns within the bounded
  shutdown window even when a test fails.

**Step 1: Confirm working tree is clean**

Run `git status`. Expected: clean. The next step injects a temporary
edit and uses `git stash` to revert it; a clean tree before this step
ensures `git stash pop` restores the exact prior state.

**Step 2: Inject a forced failure and stash the diff**

Pick the simplest existing health test (the foundation plan's first
e2e test) and add a `t.Fatal("synthetic failure to verify cleanup
bound")` immediately after `StartDaemon` returns. Then run
`git stash push -k -m "synthetic-failure"` so the diff is recoverable
even if the next step is interrupted; immediately `git stash pop` to
restore it for the timing run. Do not commit.

(If you prefer not to stash, simply remember the exact line you
added — the revert in Step 4 must restore the file byte-for-byte.)

**Step 3: Time `mise run e2e`**

Run: `time mise run e2e` from `flow/lead/`.

Expected:
- The synthetic-failure test FAILs.
- The full command returns in well under 30 seconds (typically
  10-15 seconds: ~5s graceful SIGTERM window + harness teardown +
  go test framework overhead). Without the fix, this would block on
  `cmd.Wait` until either the shell or CI timeout fires.

If `mise run e2e` runs longer than 30 seconds, the fix is incomplete:
inspect for surviving processes with `ps -o pid,pgid,cmd -p $(pgrep
-f flow.*daemon)` and re-check `Setpgid`, the negative-pid kill, and
`WaitDelay`.

**Step 4: Revert the synthetic failure**

`git checkout -- <test_file>` (or `git stash drop` and re-apply the
clean state) to remove the injected `t.Fatal`. Run `git status` and
confirm the working tree is clean — the verification is a sanity
check, not part of the suite, and must not be committed.

**Step 5: Final regression run**

Run: `mise run e2e`
Expected: PASS, all tests green.

No commit for this task — verification only.

---

## Verification Checklist

After all tasks complete:

- [ ] `mise run e2e` passes from `flow/lead/`.
- [ ] `TestDaemonStop_KillsProcessGroup` (in
  `tests/e2e/harness/daemon_leak_test.go`) passes; removing the
  `Setpgid` line from `daemon.go` makes it fail with the expected
  message.
- [ ] `tests/e2e/harness/daemon.go` no longer imports `io` and no
  longer references `bytes.Buffer` for the live writer (only for the
  `bytes.Contains` check on the read-back stderr).
- [ ] `cmd.SysProcAttr.Setpgid == true`, `cmd.WaitDelay == 10s`,
  `cmd.Stdout`/`cmd.Stderr` are both `*os.File`.
- [ ] `Stop` and `kill` use `syscall.Kill(-pgid, sig)`, never
  `cmd.Process.Signal`/`cmd.Process.Kill`.
- [ ] `mise run e2e` returns within ~30 seconds even when a test
  injects `t.Fatal` immediately after `NewEnv` (Task 3 spot check).
- [ ] Stderr capture still feeds the DATA RACE check and the
  failure-path log dump.

## Out of Scope

- Changes to anything outside `tests/e2e/harness/daemon.go` and the
  new test file. The fix is mechanical and bounded.
- Adding subprocess management for any future helper binaries Flow
  might spawn from the harness (none today). Apply the same pattern
  if added.
- Cross-repo coordination — each affected harness gets its own plan.
