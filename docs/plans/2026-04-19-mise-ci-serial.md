---
type: plan
step: "fix"
title: "Serialize mise run ci to eliminate parallel-startup contention"
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
  - "2026-04-19-daemon-ready-timeout.md"
  - "2026-04-19-pg-test-isolation.md"
---

# Serialize `mise run ci` to eliminate parallel-startup contention

## Overview

`mise run ci` currently delegates to three sibling tasks via mise's `depends` mechanism:

```bash
#MISE depends=["lint", "test", "e2e"]
```

`mise` runs declared dependencies **in parallel by default**. On a 2-core GitHub Actions runner sharing a single Postgres service, the simultaneous load from `lint` (Go compilation under `-race`), `test` (PG-backed unit suite using `FLOW_DB`), and `e2e` (builds an `-tags e2e` binary, spawns the daemon, runs migrations on `FLOW_E2E_PG_DSN`) starves the daemon's startup path. Even after the daemon-ready timeout was bumped from 5s to 30s in commit `1df141e`, Release CI still fails: `gh run view 24646054968` shows `TestRuntime_DiagReturns503WithoutStub` waiting 30.64s for the daemon to become ready before timing out.

The fix is to make `mise run ci` invoke its child tasks **sequentially**: `lint` → `test` → `e2e`. This removes the contention root cause without weakening assertions or extending timeouts further.

### Why a body, not `depends`

`#MISE depends=[…]` is mise's native dependency declaration and runs in parallel. Replacing it with an explicit script body that calls each task in order is the simplest way to enforce serial execution while keeping each task independently runnable (`mise run lint`, `mise run test`, `mise run e2e --backend …` all stay valid).

### Why this is safe for `ci.yaml`

`.github/workflows/ci.yaml` does **not** invoke `mise run ci`. It runs `mise run lint`, `mise run test`, and `mise run e2e --backend ${{ matrix.backend }}` directly across two separate jobs (`lint-and-unit` and `e2e`). Cross-job parallelism continues to come from GitHub's job scheduler, not from mise. This change has **no effect** on `ci.yaml`.

### Why this fixes `release.yaml`

`.github/workflows/release.yaml`'s `test` job runs `mise run ci` in a single job container against a single Postgres service (with separate `flow_test` and `flow_e2e_test` databases). Today this triggers the parallel storm. After the change, the three child tasks run in series within the same job, so the daemon startup in `e2e` no longer races against PG queries from `test` or `-race` compilation from `lint`.

### Trade-off (acknowledged)

Local developers running `mise run ci` will see longer wall-clock time — the previous parallel run (~max of three task durations) becomes sequential (~sum of three task durations). The benefit is deterministic CI. Devs who want parallelism on a beefier machine can still run the tasks individually in separate shells; the contention failures only manifest on resource-constrained CI runners with shared PG.

## Prerequisites

- Working tree on `master` (or a branch off `master`), no uncommitted changes to `.mise/tasks/ci`.
- `mise` installed; `mise run lint`, `mise run test`, and `mise run e2e --backend sqlite` work locally against a reachable Postgres.

## Task breakdown

### Task 1: Rewrite `.mise/tasks/ci` to serialize child tasks

**Files:**
- Modify: `.mise/tasks/ci` (entire file — currently 5 lines)

**Step 1: Replace the file contents**

Open `.mise/tasks/ci` and replace the entire file with:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run all CI checks serially (lint -> test -> e2e). The CI workflow's matrix runs the same children in parallel jobs; release.yaml runs this single-job aggregator and depends on serial execution to avoid PG/daemon contention."
set -euo pipefail

mise run lint
mise run test
mise run e2e
```

Notes:
- The `#MISE depends=[…]` directive is removed. `mise` will no longer auto-spawn the children in parallel.
- Each child runs sequentially under `set -euo pipefail`; the first failure aborts the script with a non-zero exit code, matching prior behavior.
- `mise run e2e` defaults to `--backend sqlite` (see `.mise/tasks/e2e:22`). `release.yaml` exports `FLOW_E2E_PG_DSN` but never passes `--backend postgres` to `mise run ci`; `ci.yaml`'s matrix is the path that exercises `postgres`. Preserving the no-flag invocation matches today's behavior of `mise run ci` (which also called `e2e` without arguments via `depends`).
- The updated `description` records the rationale so future readers understand why this task is not using `depends=[…]`.

**Step 2: Make sure the file is executable**

Run: `ls -l .mise/tasks/ci`
Expected: mode includes `x` (e.g. `-rwxr-xr-x`). If not, run `chmod +x .mise/tasks/ci`.

**Step 3: Verify mise still recognizes the task**

Run: `mise tasks ls | grep -E '^ci\b'`
Expected: one row for `ci` with the new description.

**Step 4: Smoke-test serial execution locally**

Run: `mise run ci`
Expected:
- `lint` runs first; on success, `test` starts.
- `test` runs second; on success, `e2e` starts.
- `e2e` runs last and exits 0.
- Total wall time ≈ sum of the three task durations (longer than before).
- No interleaved output between tasks.

If any task fails locally for an unrelated reason (e.g., missing PG), fix the local environment and rerun — do not commit until the serial run completes successfully end-to-end.

**Step 5: Commit**

```bash
git add .mise/tasks/ci
git commit -m "$(cat <<'EOF'
fix(ci): serialize mise run ci to avoid parallel-startup contention

The ci task previously declared #MISE depends=["lint", "test", "e2e"],
which mise runs in parallel by default. On 2-core GitHub Actions runners
with a single shared Postgres service, the simultaneous load from
-race-instrumented compilation (lint), PG-backed unit tests (test), and
daemon spawn + migrations (e2e) starved the daemon startup path. Even
after bumping the daemon-ready timeout from 5s to 30s in 1df141e,
release.yaml's test job kept hitting TestRuntime_DiagReturns503WithoutStub
timing out at ~30.6s waiting for daemon readiness.

Replace the depends directive with an explicit script body that invokes
lint, test, and e2e sequentially. Cross-task parallelism is preserved at
the workflow layer: ci.yaml splits lint+test from e2e across separate
jobs and never invokes `mise run ci`. release.yaml's single-job test
matrix benefits directly from the serial behavior. Local devs running
`mise run ci` trade parallel wall time for deterministic CI.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

## Verification checklist

After Task 1 is committed:

- [ ] `.mise/tasks/ci` no longer contains `#MISE depends=`.
- [ ] `.mise/tasks/ci` body invokes `mise run lint`, then `mise run test`, then `mise run e2e` (in that order, each on its own line, under `set -euo pipefail`).
- [ ] `.mise/tasks/ci` retains `chmod +x` (executable bit).
- [ ] `mise tasks ls` shows the `ci` task with the updated description.
- [ ] `mise run ci` completes locally with serial output (no interleaving).
- [ ] `.github/workflows/ci.yaml` is unchanged and continues to invoke `mise run lint`, `mise run test`, and `mise run e2e --backend …` directly (it never calls `mise run ci`).
- [ ] `.github/workflows/release.yaml`'s `test` job is unchanged and continues to invoke `mise run ci`; this is the consumer that benefits from serialization.
- [ ] After push, the next Release CI run reaches the `tag` job (i.e., `test` job passes without daemon-ready timeouts).

## Out of scope

- Tuning individual task internals (e.g., bumping the e2e daemon-ready timeout further, changing PG pool sizes). Those were previously attempted; serialization is the structural fix.
- Splitting `release.yaml`'s `test` job into parallel jobs mirroring `ci.yaml`. That is a larger refactor and is not required to unblock release tagging.
- Adding a new `ci:parallel` task variant for local devs who want the old behavior. Not requested; add only if devs report friction.
