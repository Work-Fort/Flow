# Remaining Work — Flow

Tracks known bugs and follow-ups. Items are roughly priority-ordered within each section.

---

## Open

### Security
- [ ] **Service token in OpenRC `command_args`** — The init.d script used to run `flow daemon` passes `--service-token <token>` in `command_args`, which puts the secret in argv — visible via `/proc/<pid>/cmdline`, `ps -ef`, and system logs. Fix: source the token from `/etc/conf.d/flow` as an env var, and have `flow daemon` read it via viper's `FLOW_SERVICE_TOKEN` binding instead of the CLI flag.
