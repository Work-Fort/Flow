# Remaining Work — Flow

Tracks known bugs and follow-ups. Items are roughly priority-ordered within each section.

---

## Test Coverage Gaps

**Convention:** Tests that require external infrastructure (a running PostgreSQL
instance, a pre-built binary) use `t.Skip` with a clear reason rather than
failing unconditionally. These skips are expected and acceptable in local
development; CI jobs that target those backends must set the required environment
variables to exercise them.

### Conditional skips (`t.Skip`)

| File | Condition | Reason |
|------|-----------|--------|
| `internal/infra/postgres/store_test.go` | `FLOW_DB` env var not set | All Postgres adapter tests skip locally; the PG CI job sets `FLOW_DB`. |
| `tests/e2e/harness/daemon_leak_test.go` | `FLOW_BINARY` env var not set and `../../build/flow` missing | Binary not built; tests skip unless launched via `mise run e2e` (which builds first). |

---

## Open

### Security
- [ ] **Service token in OpenRC `command_args`** — The init.d script used to run `flow daemon` passes `--service-token <token>` in `command_args`, which puts the secret in argv — visible via `/proc/<pid>/cmdline`, `ps -ef`, and system logs. Fix: source the token from `/etc/conf.d/flow` as an env var, and have `flow daemon` read it via viper's `FLOW_SERVICE_TOKEN` binding instead of the CLI flag.
