---
type: plan
step: "2026-04-19-pg-test-isolation"
title: "Isolate PG state between Flow's unit-test and e2e runs"
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
  - 2026-04-19-ci-pg-fix.md
  - 2026-04-18-flow-orchestration-01-foundation.md
---

# Plan: Isolate PG state between Flow's unit-test and e2e runs

## Overview

Flow's `Release` workflow `test` job is failing intermittently with two
distinct PG errors:

```
TestStoreOpen  Open: run migrations: ERROR 001_init.sql:
  ... ERROR: duplicate key value violates unique constraint
  "pg_type_typname_nsp_index" (SQLSTATE 23505)

TestStore_ListWorkItemsByAgent  Open: run migrations:
  ERROR: relation "goose_db_version" does not exist (SQLSTATE 42P01)
```

Both symptoms are the same root cause: **two processes concurrently
mutate the schema of the single `flow_test` database**, so one
process's `goose.Up` sees a half-existing schema or, worse, has its
`goose_db_version` table deleted out from under it.

Two concurrency layers contribute:

1. **Cross-task parallelism (the dominant cause).** `.mise/tasks/ci`
   declares `#MISE depends=["lint", "test", "e2e"]`. Mise runs declared
   dependencies in parallel — confirmed by run `24645386882` log
   timestamps where `[test]`, `[lint]`, and `[e2e]` start within the
   same second. Both `mise run test` (the `internal/infra/postgres`
   suite) and `mise run e2e` (the daemon harness) point at
   `FLOW_DB`/`FLOW_E2E_PG_DSN` = `postgres://.../flow_test`. The e2e
   harness calls `resetPostgres` (`tests/e2e/harness/daemon.go:275`)
   which `DROP SCHEMA public CASCADE; CREATE SCHEMA public` — that's
   exactly what removes `goose_db_version` from under a unit test
   running concurrently in another process.
2. **Within-package non-isolation.** Each `openTestStore` call invokes
   `goose.Up` against a database whose schema state is whatever the
   previous test left behind. Goose Up is normally idempotent, but it
   is not idempotent against a half-built schema — exactly the state
   the e2e harness's reset can leave during the race window. It also
   leaves test data (templates, work items, audit events) lying around
   between tests, which has not yet caused failures but is a latent
   bug.

The fix has two complementary parts:

**Part A — Cross-task isolation (mandatory).** The `test` task and the
`e2e` task must use separate PostgreSQL databases when they run inside
the same `mise run ci` invocation. This is a CI-environment fix: the
`test` task already defaults its DSN, but CI sets `FLOW_DB` and
`FLOW_E2E_PG_DSN` to the same value. We split them so the unit test
suite uses `flow_test` and the e2e harness uses `flow_e2e_test`. Both
DBs sit on the same Postgres service container, so no container
changes are needed beyond ensuring the second DB exists.

**Part B — Per-test schema reset for the postgres unit suite (matches
Hive/Combine pattern).** Replace `openTestStore` with a helper that
drops and recreates the `public` schema before opening the store and
running migrations. This makes each test in `internal/infra/postgres`
start from a clean DB regardless of what other tests in the same
package did, and makes the suite robust to anything the e2e harness
might do if database isolation is ever weakened. The reset SQL exactly
mirrors `tests/e2e/harness/daemon.go:275-289` and Hive's
`tests/e2e/pg_helpers_test.go:27-42`.

We deliberately do NOT pursue:

- **Per-test fresh database (`flow_test_<rand>`)**, because that
  requires a privileged admin DSN to run `CREATE DATABASE` and adds
  meaningful overhead (each `CREATE DATABASE` in PG is ~80 ms on the
  CI runner). Hive's `AltDB` uses a fixed sibling DB
  (`hive_test → hive_test_b`), not random per-test DBs, for the same
  reason. Per-test schema reset gives us full isolation at a fraction
  of the cost.
- **Per-package serialization (`go test -p 1`)**. This kills `go
  test`'s package-level parallelism for all of `./...` — currently
  ~12 packages — to fix a problem that only one package (the PG one)
  has. Wrong scope, real perf cost.

## Bug context

- **Failing run:** `Release` workflow, run id `24645386882`, job
  `test`, step `Run mise run ci`.
- **Failing tests (from log):** `TestStoreOpen` (duplicate
  `pg_type_typname_nsp_index`), `TestStore_ListWorkItemsByAgent`
  (`relation "goose_db_version" does not exist`).
- **Reproduces** under `mise run ci` whenever `test` and `e2e` are
  mid-flight at the same instant. Single-task runs (`mise run test`
  alone) pass.

## Cross-repo consistency check

- **Hive.** `hive/lead/internal/daemon/test_helpers_test.go` only
  exercises in-memory SQLite; Hive does not have a PG-backed unit test
  package, so this issue cannot occur there. The reset pattern we
  adopt for Part B is lifted from Hive's e2e helper
  (`tests/e2e/pg_helpers_test.go:27-42`).
- **Combine.** Same shape as Hive — no PG-backed unit test package.
- **Flow** is the first WorkFort repo where unit tests AND e2e tests
  both bind to a single shared PG instance under `mise run ci`. The
  fix is repo-local and does not need to ripple to Hive or Combine.

## Prerequisites

- Working tree clean on `master`.
- Local Postgres reachable on `127.0.0.1:5432` as user `postgres`
  (peer-trust). Confirm with `psql postgres://postgres@127.0.0.1
  -c 'SELECT 1'`.
- Authenticated `gh` for verifying CI runs after push.

## Task breakdown

### Task 1: Add per-test schema reset to the postgres unit suite

**Files:**
- Modify: `internal/infra/postgres/store_test.go:19-36`

**Step 1: Inspect current `openTestStore`**

Read lines 19-36 of `internal/infra/postgres/store_test.go`. Confirm
it matches:

```go
func dsn(t *testing.T) string {
    t.Helper()
    v := os.Getenv("FLOW_DB")
    if v == "" {
        t.Fatal("FLOW_DB not set — the mise test runner sets a default; if you're running `go test` directly, set FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable")
    }
    return v
}

func openTestStore(t *testing.T) domain.Store {
    t.Helper()
    s, err := postgres.Open(dsn(t))
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    t.Cleanup(func() { s.Close() })
    return s
}
```

Expected: matches verbatim. If not, stop and report drift.

**Step 2: Replace with reset-then-open**

Replace the whole `openTestStore` block (lines 28-36) with:

```go
// resetSchema drops and recreates the public schema so each test
// starts from a clean database. Goose migrations re-run on the next
// postgres.Open call. All three DDL statements run in a single Exec
// so we don't leave a window where another connection sees the new
// schema before grants are restored. Mirrors the e2e harness reset
// at tests/e2e/harness/daemon.go:275 and Hive's
// tests/e2e/pg_helpers_test.go:27.
func resetSchema(t *testing.T, dsn string) {
    t.Helper()
    db, err := sql.Open("pgx", dsn)
    if err != nil {
        t.Fatalf("resetSchema open: %v", err)
    }
    defer db.Close()
    if _, err := db.Exec(`
        DROP SCHEMA IF EXISTS public CASCADE;
        CREATE SCHEMA public;
        GRANT ALL ON SCHEMA public TO PUBLIC;
    `); err != nil {
        t.Fatalf("resetSchema exec: %v", err)
    }
}

// openTestStore returns a store backed by a freshly-reset PG schema.
// Each call truncates all schema state, then runs migrations from
// scratch. This makes every test independent of every other test in
// the suite. Cost: ~30-60 ms per call on CI Postgres for the
// drop/create + 3 goose migrations; acceptable for v1 (~13 tests in
// this package, run sequentially within the suite).
func openTestStore(t *testing.T) domain.Store {
    t.Helper()
    d := dsn(t)
    resetSchema(t, d)
    s, err := postgres.Open(d)
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    t.Cleanup(func() { s.Close() })
    return s
}
```

**Step 3: Add the `database/sql` and pgx-stdlib imports**

The existing import block does not have `database/sql` or the pgx
stdlib driver (those live inside the `postgres` package). Update the
import block at the top of `store_test.go` (lines 4-17) to:

```go
import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "os"
    "strings"
    "testing"
    "time"

    "github.com/google/uuid"
    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/Work-Fort/Flow/internal/domain"
    "github.com/Work-Fort/Flow/internal/infra/postgres"
)
```

**Step 4: Run the suite locally to verify it still passes**

Run: `FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable
go test -race -count=1 ./internal/infra/postgres/...`

Expected: `ok  github.com/Work-Fort/Flow/internal/infra/postgres
<duration>`. All 13 tests pass.

**Step 5: Run the suite under `-count=10` to verify isolation**

Run: `FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable
go test -race -count=10 ./internal/infra/postgres/...`

Expected: still passes — 130 invocations, no `ErrAlreadyExists` from
duplicate IDs and no `pg_type_typname_nsp_index` violations. If a
single test now fails because it depended on data left behind by an
earlier test, fix that test in this same task (the helper is the
contract; tests must self-seed).

**Step 6: Commit**

```
test(postgres): reset schema before each unit test

Each openTestStore call now drops and recreates the public schema,
then runs migrations from scratch. Eliminates state leakage between
tests in internal/infra/postgres and makes the suite robust to
concurrent schema mutation by other test runners (e.g. the e2e
harness when both `mise run test` and `mise run e2e` execute in
parallel under `mise run ci`).

Mirrors the reset pattern used by the e2e harness at
tests/e2e/harness/daemon.go:275 and by Hive's e2e helper.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

### Task 2: Split `FLOW_DB` and `FLOW_E2E_PG_DSN` to different databases in CI

**Depends on:** Task 1 (per-test reset is required for stable runs even
with split DBs, because future maintainers will set the DSNs equal
again locally).

**Files:**
- Modify: `.github/workflows/release.yaml:11-37`
- Modify: `.github/workflows/ci.yaml` (verify: split may already be
  needed there too)

**Step 1: Inspect both workflow files**

Read `.github/workflows/release.yaml` lines 11-37 and the `e2e` job
section of `.github/workflows/ci.yaml`. Identify every place that sets
`FLOW_DB` or `FLOW_E2E_PG_DSN`. Build a small table of (workflow,
job, var, value) before editing. Stop and report drift if either
workflow does not match the patterns assumed below.

Expected (`release.yaml:11-37`):

```yaml
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_DB: flow_test
          POSTGRES_PASSWORD: flow
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0
      - uses: jdx/mise-action@v3
      - run: mise run ci
        env:
          FLOW_DB: postgres://postgres:flow@127.0.0.1/flow_test?sslmode=disable
          FLOW_E2E_PG_DSN: postgres://postgres:flow@127.0.0.1/flow_test?sslmode=disable
```

**Step 2: Add a setup step that creates the e2e database**

The Postgres service container creates `flow_test` from `POSTGRES_DB`,
but the e2e DB has to be created explicitly. Insert a new step after
`jdx/mise-action@v3` and before `Run mise run ci` (still within the
`test` job's `steps:` list):

```yaml
      - name: Create e2e database
        env:
          PGPASSWORD: flow
        run: |
          psql -h 127.0.0.1 -U postgres -d flow_test \
            -c 'CREATE DATABASE flow_e2e_test;'
```

**Step 3: Point `FLOW_E2E_PG_DSN` at the new database**

Change the `Run mise run ci` step's `env:` block to:

```yaml
      - run: mise run ci
        env:
          FLOW_DB: postgres://postgres:flow@127.0.0.1/flow_test?sslmode=disable
          FLOW_E2E_PG_DSN: postgres://postgres:flow@127.0.0.1/flow_e2e_test?sslmode=disable
```

`FLOW_DB` keeps `flow_test`; only `FLOW_E2E_PG_DSN` moves.

**Step 4: Apply the same split to `ci.yaml` if needed**

Re-read `.github/workflows/ci.yaml`. The `e2e` matrix job uses
`FLOW_E2E_PG_DSN` for the postgres backend; the `lint-and-unit` job
uses `FLOW_DB`. These two jobs are separate runners with separate PG
service containers, so they cannot collide and need no change. Confirm
this and document the conclusion in the commit body.

**Step 5: Commit**

```
ci(release): isolate unit-test and e2e Postgres databases

The `test` job in Release.yaml runs `mise run ci`, which kicks off
`mise run test` and `mise run e2e` in parallel (per
`#MISE depends=["lint", "test", "e2e"]` in `.mise/tasks/ci`). Both
were pointed at the same `flow_test` DB. The e2e harness's
`resetPostgres` helper (DROP SCHEMA public CASCADE) would then race
with the unit suite's goose migrations, surfacing as either
"duplicate key on pg_type_typname_nsp_index" (concurrent CREATE TABLE)
or "relation goose_db_version does not exist" (DROP wins, then unit
test reads the missing table).

Give them separate databases on the same Postgres service: `flow_test`
for the unit suite, `flow_e2e_test` for the harness. The e2e DB is
created in a setup step before `mise run ci`. ci.yaml is unaffected
because its `lint-and-unit` and `e2e` jobs already run on separate
runners with separate PG service containers.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

### Task 3: Document local-dev expectations

**Depends on:** Task 2.

**Files:**
- Modify: `tests/e2e/README.md`

**Step 1: Read the README's PG section**

Open `tests/e2e/README.md`. Find the section that documents
`FLOW_E2E_PG_DSN` (search for `FLOW_E2E_PG_DSN`). If no such section
exists, add one immediately after the existing `FLOW_DB` documentation.

**Step 2: Add a note that local devs running `mise run ci` need a
separate e2e DB**

Append (or insert under the existing PG section):

```markdown
### Running `mise run ci` locally

`mise run ci` runs `mise run test` and `mise run e2e` in parallel.
Both bind to Postgres; if they share a database they will race on
schema state. Either:

- **Recommended:** create a sibling DB and point the e2e harness at
  it:

  ```sh
  createdb flow_e2e_test
  export FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable
  export FLOW_E2E_PG_DSN=postgres://postgres@127.0.0.1/flow_e2e_test?sslmode=disable
  mise run ci
  ```

- Or run them sequentially: `mise run test && mise run e2e`.

The Release CI workflow uses the recommended split.
```

**Step 3: Commit**

```
docs(e2e): note PG isolation requirement for `mise run ci`

`mise run ci` runs `test` and `e2e` in parallel. They must point at
different Postgres databases or they race on schema state. Document
the recommended split (`flow_test` + `flow_e2e_test`) so local devs
hit the same configuration as CI.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

## Verification checklist

After all three tasks land:

- [ ] **Per-test reset audited.** Re-grep `openTestStore` across the
      repo: `grep -rn 'openTestStore' internal/ tests/`. Every PG-side
      caller (`internal/infra/postgres/store_test.go`) should hit the
      reset path; SQLite callers (`internal/infra/sqlite/store_test.go`,
      `internal/workflow/fixtures_test.go`) are untouched and use their
      own in-memory helper. The plans/ directory hits are documentation
      and may be ignored.
- [ ] **Local stress test.** With local PG running:
      ```sh
      createdb -U postgres flow_e2e_test 2>/dev/null || true
      FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable \
      FLOW_E2E_PG_DSN=postgres://postgres@127.0.0.1/flow_e2e_test?sslmode=disable \
        mise run ci
      ```
      Expected: green. Run it three times; expected: all green.
- [ ] **Single-package stress.** `FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable
      go test -race -count=20 ./internal/infra/postgres/...`. Expected:
      green across 260 invocations.
- [ ] **CI green.** Push to a branch; `Release` workflow `test` job
      passes. Watch one full run from start to finish; do not push and
      walk away (per `feedback_monitor_every_push.md`).
- [ ] **No new test failures.** No "pre-existing" failures introduced
      or papered over (per `feedback_no_test_failures.md`).

## Notes on cost and scope

- **Per-test cost.** Drop/create schema + 3 goose migrations on the
  CI Postgres service measures roughly 30-60 ms per call. With ~13
  tests in `internal/infra/postgres`, that's an extra ~0.5-0.8 s on
  the suite — negligible compared to the parallel `e2e` job (~30 s)
  and `lint` (~20 s).
- **No new dependencies.** `database/sql` and the pgx stdlib driver
  are already in `go.mod` (used by `internal/infra/postgres/store.go`
  and the e2e harness). No `go.mod` change.
- **No new mise tasks.** The `test` task is unchanged; only the CI
  environment that calls it is reconfigured.
- **Out of scope.** Refactoring the PG store to allow injecting a
  custom schema name (which would let parallel tests live in
  per-test schemas without DROP/CREATE) is a real future option but
  is well beyond the bug at hand. We can revisit if the per-test
  reset cost ever becomes load-bearing on CI wall time.
