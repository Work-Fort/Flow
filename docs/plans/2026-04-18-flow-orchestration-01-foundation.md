---
type: plan
step: "1"
title: "Flow orchestration — scheduler, lease renewer, RuntimeDriver port, audit events"
status: complete
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "1"
dates:
  created: "2026-04-18"
  revised: "2026-04-19"
  approved: "2026-04-19"
  completed: "2026-04-19"
related_plans:
  - "2026-04-18-flow-e2e-harness-01-foundation.md"
---

# Flow Orchestration — Step 1: Scheduler + Lease Renewer + RuntimeDriver Port + Audit Events

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Lay the runtime-agnostic foundation for Flow's agent-pool
orchestration. Step 1 delivers four orthogonal subsystems, each
exercised end-to-end through the daemon, plus the test-runner
hardening (no-silent-skip + e2e in `ci`) needed to make the safety
net real:

1. A **`Scheduler`** domain service that wraps the Hive client's
   `ClaimAgent` / `ReleaseAgent` and exposes workflow-facing primitives
   `AcquireAgent(role, project, leaseTTL)` and `ReleaseAgent(claim)`.
   Retries with bounded exponential backoff when the Hive pool is
   exhausted.
2. A **`LeaseRenewer`** background goroutine that tracks every claim
   the live Flow process holds and, every ~30 s, calls Hive's
   `RenewAgentLease` for each. Short default lease TTL (2 min). Exits
   cleanly on context cancel.
3. A **`RuntimeDriver`** port (7-method interface) in
   `internal/domain/ports.go` plus a **`StubDriver`** test double in
   `internal/infra/runtime/stub/`. The interface is deliberately
   shaped so the future k8s impl maps 1:1 to k8s primitives — see
   the "k8s mapping" subsection at the end of Task 2. No real driver
   yet; concrete Nexus and k8s drivers are each their own future plan.
4. An **audit event log**: new `audit_events` table (sqlite + postgres
   parity), a `domain.AuditEventStore`, and four concrete event types
   — `agent_claimed`, `agent_released`, `lease_renewed`,
   `lease_expired_by_sweeper`. Every claim / release / renew in the
   scheduler and renewer writes an event. Flow is the legal audit
   primary; the table lives in Flow's DB because Flow outlives any
   single Hive daemon's deployment.

**Cross-cutting plumbing delivered in this step:**

5. **Test-runner hardening.** `mise run test` defaults `FLOW_DB` to
   the host's local Postgres (`postgres://postgres@127.0.0.1/flow_test?sslmode=disable`)
   so the PG path is exercised on every developer run. If PG is
   unreachable, the task fails loudly (no silent skip). The skip-on-
   `FLOW_DB`-unset paths in unit tests get replaced with explicit
   reachability checks that fail with a "start PG" message.
6. **`mise run ci` includes e2e.** Local `ci` chains
   `lint + test + e2e` (e2e default backend = SQLite). Dual-backend
   matrix stays in the CI workflow as parallel jobs (one SQLite, one
   Postgres). The `mise run e2e` task gains a `--backend` flag.
7. **End-to-end coverage for every subsystem.** A new
   `tests/e2e/agent_pool_e2e_test.go` exercises acquire → renew →
   release end-to-end through the spawned daemon, with the harness's
   FakeHive extended to serve the three agent-pool endpoints. A
   second test exercises the audit event log via the same path. A
   third test exercises a `/v1/runtime/_diag/*` diagnostic endpoint
   that drives the bound `RuntimeDriver` (the StubDriver in the
   harness) end-to-end without inventing user-facing API.

**Architecture (cross-cutting):** Flow must run on both Nexus
(local/dev) and k8s/k3s (hosted/production); k8s is the long-term
primary per `AGENT-POOL-REMAINING-WORK.md` "Load-bearing decisions".
Step 1 defines the `RuntimeDriver` port deliberately k8s-shaped and
ships only a stub impl — enough for the scheduler's tests and for
the diagnostic endpoint to exercise the interface without real
infrastructure.

**Explicitly NOT in Step 1** (each is its own plan):

- Any real `RuntimeDriver` implementation (Nexus or k8s).
- Per-project bot processes, Sharkfin bot vocabulary parser, Combine
  integration.
- Project-master / work-item drive management — depends on a real
  driver.
- Wiring the scheduler into transitions or integration-hook
  evaluation (the diagnostic endpoint is the driver-side surface for
  Step 1 only).
- Sweeper-side `lease_expired_by_sweeper` event production. The
  enum value is reserved for a future plan that wires either a Hive
  webhook or a Flow-side reconciliation pass; Step 1 only declares
  the type and ensures the column accepts it.

**Tech Stack:** Go 1.26, `database/sql`, `pressly/goose` migrations,
`charmbracelet/log`, Hive's existing Go client
(`github.com/Work-Fort/Hive/client` — already in `go.mod`), Huma v2
for the new `/v1/runtime/_diag/*` endpoint (already used elsewhere
in `internal/daemon/`).

---

## Prerequisites

- Hive agent-pool endpoints (Hive plans
  `2026-04-17-agent-pool-{01-schema, 02-endpoints, 03-sweeper,
  04-get-provisioning}.md`) merged and reachable. The Hive client's
  `ClaimAgent` / `ReleaseAgent` / `RenewAgentLease` methods exist in
  `github.com/Work-Fort/Hive/client` (already pinned in Flow's
  `go.mod`).
- Flow builds and tests green on `master`.
- Local Postgres 18 reachable as the host's `postgres` user via
  peer-trust (per the WorkFort dev convention) on
  `127.0.0.1:5432`. A `flow_test` database exists or can be created
  by the test runner.
- `flow/lead/tests/e2e/harness/` exists from
  `2026-04-18-flow-e2e-harness-01-foundation.md` (status: complete).
  This plan extends that harness; it does not replace it.

## Testing posture

Every subsystem in this plan ships with **all three layers**:

- **Unit tests** for domain logic (scheduler retry / backoff,
  audit serialisation).
- **Integration tests** against real storage (sqlite +
  Postgres parity).
- **E2E tests** in `flow/lead/tests/e2e/` that spawn the real flow
  daemon (built by `mise run build:dev`), point it at a fake Hive
  speaking the raw agent-pool wire protocol, and exercise the
  acquire → renew → release path end-to-end. Per
  `feedback_e2e_harness_independence.md`, the harness extension
  speaks raw `net/http` with hand-rolled wire-format JSON — it
  does NOT import `github.com/Work-Fort/Hive/client`. Per
  `feedback_e2e_dual_backend.md`, the e2e suite runs against both
  SQLite (default) and Postgres (selected by harness option, driven
  by the `--backend` flag on `mise run e2e`).

The pre-existing `tests/e2e/harness/fake_hive.go` already serves
`GET /v1/agents/{id}`, `GET /v1/agents`, `GET /v1/roles/{id}`. This
plan extends the same fake with the three pool endpoints; it does
NOT spawn a separate sub-server.

## Conventions (apply to every task)

- Never run `go build`/`go test` directly — use `mise run <task>`
  from repo root. The verified task surface in
  `flow/lead/.mise/tasks/` is:
  - `mise run build:dev` — dev build (writes `build/flow`).
  - `mise run build:release` — static release build.
  - `mise run test` — `go test -v -race -coverprofile=...` over `./...`.
    After Task 14, this defaults `FLOW_DB` to the local PG.
  - `mise run lint` — `gofmt -l` + `go vet ./...` + `golangci-lint run ./...`.
  - `mise run e2e` — spawns the daemon and runs `tests/e2e`. After
    Task 14, accepts `--backend sqlite|postgres`.
  - `mise run ci` — chains `lint + test + e2e` after Task 14.
- Focused unit-test run during TDD: `go test -run TestName ./internal/...`
  is acceptable (per planner.md "Targeted test runs by name during
  TDD are acceptable with the language's native test runner").
- Log style matches the rest of `internal/daemon/`: use
  `github.com/charmbracelet/log` with key/value pairs, e.g.
  `log.Info("scheduler: claim succeeded", "agent_id", id, "workflow_id", wf)`.
- Commit after each task with Conventional Commits using the
  HEREDOC + `Co-Authored-By` trailer format (per
  `feedback_commit_format.md`):

  ```
  git commit -m "$(cat <<'EOF'
  <type>(<scope>): <description>

  <one-paragraph body explaining the why, not the what>

  Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
  EOF
  )"
  ```

  Scopes used below: `domain`, `scheduler`, `runtime`, `audit`,
  `sqlite`, `postgres`, `daemon`, `hive`, `e2e`, `mise`. **No `!`
  markers, no `BREAKING CHANGE:` footers** — WorkFort is pre-1.0.

---

## Task breakdown

### Task 1: Domain error sentinels for the scheduler

**Files:**
- Modify: `internal/domain/errors.go`

**Step 1: Append sentinel errors**

Append to `internal/domain/errors.go` (after `ErrPermissionDenied`):

```go
	// ErrPoolExhausted is returned from Scheduler.AcquireAgent when Hive has
	// no free agent after all retries. The caller should surface this to
	// the workflow engine, which will retry later or mark the step blocked.
	ErrPoolExhausted = errors.New("agent pool exhausted")

	// ErrWorkflowMismatch is returned when Flow tries to release or renew
	// a lease with a workflow ID that does not match the one currently
	// held in Hive. This is almost always a bug in the caller.
	ErrWorkflowMismatch = errors.New("workflow id does not match current claim")

	// ErrRuntimeUnavailable is returned from RuntimeDriver operations when
	// the underlying runtime (Nexus, k8s, …) is unreachable or rejected
	// the call. Distinct from ErrNotFound so callers can retry transient
	// infrastructure outages without muddying not-found semantics.
	ErrRuntimeUnavailable = errors.New("runtime driver unavailable")
```

**Step 2: Verify compile**

Run: `mise run lint`
Expected: exits 0.

**Step 3: Commit**

```
git add internal/domain/errors.go
git commit -m "$(cat <<'EOF'
feat(domain): add scheduler and runtime error sentinels

Step 1 of the agent-pool foundation needs three new error
sentinels so callers can distinguish pool exhaustion (retry-later)
from workflow-id mismatches (caller bug) from runtime-driver
unavailability (transient infra). Defining them in domain keeps
infra adapters free to wrap with context.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: RuntimeDriver port + value types (k8s-shaped)

**Depends on:** Task 1.

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`

**Step 1: Append runtime value types**

Append to `internal/domain/types.go` (after `IdentityRole`):

```go
// --- runtime driver value types ---

// VolumeRef identifies a volume/drive the runtime can attach to an agent
// runtime. It is deliberately opaque: the orchestrator passes whatever
// the driver returned from CloneWorkItemVolume or GetProjectMasterRef
// back into StartAgentRuntime. Drivers interpret the Ref themselves.
//
// Maps cleanly to k8s: Kind="k8s-pvc", ID=<PVC name in the agent
// namespace>. Maps to Nexus: Kind="nexus-drive", ID=<drive UUID>.
type VolumeRef struct {
	// Kind is driver-specific ("nexus-drive", "k8s-pvc", "stub", …).
	Kind string
	// ID is driver-specific (drive ID, PVC name, …).
	ID string
}

// RuntimeHandle identifies a running agent runtime instance. The
// orchestrator retains the handle between StartAgentRuntime and
// StopAgentRuntime; drivers interpret it themselves.
//
// Maps to k8s: Kind="k8s-pod", ID=<pod name in the agent namespace>.
// Maps to Nexus: Kind="nexus-vm", ID=<VM ID>.
type RuntimeHandle struct {
	// Kind is driver-specific ("nexus-vm", "k8s-pod", "stub", …).
	Kind string
	// ID is driver-specific (VM ID, pod name, …).
	ID string
}
```

**Step 2: Append `RuntimeDriver` interface**

Append to `internal/domain/ports.go` (after `IdentityProvider`):

```go
// RuntimeDriver abstracts the runtime that actually executes an agent's
// adjutant loop. Today there is one concrete driver (Nexus VMs, future
// plan); tomorrow the long-term primary will be k8s pods + CSI
// VolumeSnapshot. Per AGENT-POOL-REMAINING-WORK.md "Load-bearing
// decisions": every method must map 1:1 to k8s primitives (Pods, PVCs,
// CSI snapshots) so the k8s driver is a translation layer, not a
// re-architecture.
//
// Seven methods. Resist bloating it.
type RuntimeDriver interface {
	// StartAgentRuntime brings an agent's runtime online, attaching the
	// per-agent credentials volume and the per-work-item working volume.
	// Returns a handle the caller must pass back to StopAgentRuntime.
	//
	// k8s mapping: create a Pod from a per-role PodTemplate with two
	// volume mounts whose PVCs are creds.ID and work.ID; return
	// {Kind:"k8s-pod", ID: pod.Name}. Nexus mapping: pick a free VM from
	// the pool, attach drives creds.ID + work.ID, start the VM.
	StartAgentRuntime(ctx context.Context, agentID string, creds, work VolumeRef) (RuntimeHandle, error)

	// StopAgentRuntime shuts down a runtime previously started with
	// StartAgentRuntime and detaches its volumes. Idempotent on already-
	// stopped handles.
	//
	// k8s mapping: kubectl delete pod h.ID (with grace period). Nexus
	// mapping: stop VM h.ID, detach drives, return VM to pool.
	StopAgentRuntime(ctx context.Context, h RuntimeHandle) error

	// IsRuntimeAlive returns true when the runtime at h is still
	// executing. Used by higher-level liveness checks; MUST NOT block
	// indefinitely — drivers should cap internal timeouts at ctx's
	// deadline or ~2 s, whichever is smaller.
	//
	// k8s mapping: Pod.Status.Phase == "Running". Nexus mapping: VM
	// status query.
	IsRuntimeAlive(ctx context.Context, h RuntimeHandle) (bool, error)

	// CloneWorkItemVolume forks the project master into a new volume
	// dedicated to `workItemID`. Returns a VolumeRef the caller passes
	// to StartAgentRuntime.
	//
	// k8s mapping: create a VolumeSnapshot of projectMaster.ID, then a
	// PVC dataSourceRef'd at the snapshot (CSI clone-from-snapshot).
	// Nexus mapping: btrfs subvolume snapshot of the master drive into
	// a new drive named work-item-<workItemID>.
	CloneWorkItemVolume(ctx context.Context, projectMaster VolumeRef, workItemID string) (VolumeRef, error)

	// DeleteVolume destroys a volume previously returned from
	// CloneWorkItemVolume. Idempotent.
	//
	// k8s mapping: delete the PVC (CSI driver handles the snapshot/
	// volume reclaim). Nexus mapping: delete the drive.
	DeleteVolume(ctx context.Context, v VolumeRef) error

	// RefreshProjectMaster pulls the given git ref into the project
	// master volume for `projectID`, running whatever warming steps
	// (build, install) the project configures. Creates the volume on
	// first call.
	//
	// k8s mapping: launch a one-shot Job that mounts the master PVC,
	// runs `git pull` + warming script, then exits; CSI snapshot of
	// the resulting PVC becomes the next clone source. Nexus mapping:
	// run an ephemeral VM with the master drive attached, run the
	// warming script, snapshot the result.
	RefreshProjectMaster(ctx context.Context, projectID string, gitRef string) error

	// GetProjectMasterRef returns the VolumeRef for `projectID`'s
	// current master, or a zero-value VolumeRef when the project has no
	// master yet (caller should RefreshProjectMaster first).
	//
	// k8s mapping: return the per-project master PVC name. Nexus
	// mapping: return the project's master drive UUID.
	GetProjectMasterRef(projectID string) VolumeRef
}
```

`context` is already imported at the top of `ports.go`. No new imports
required.

**Step 3: Verify compile**

Run: `mise run lint`
Expected: exits 0 (the interface has zero implementations yet, but
nothing references it so compilation succeeds).

**Step 4: Commit**

```
git add internal/domain/types.go internal/domain/ports.go
git commit -m "$(cat <<'EOF'
feat(domain): add RuntimeDriver port with VolumeRef and RuntimeHandle

Defines the seven-method runtime abstraction Flow's orchestrator
will drive. Each method's doc comment includes its k8s and Nexus
mappings so future driver impls translate, not redesign. The
interface is deliberately shaped so PVCs, CSI VolumeSnapshots,
PodTemplates, and one-shot Jobs cover every method end-to-end on
k8s — the long-term primary runtime per the agent-pool decisions
doc.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: StubDriver implementation + unit tests

**Depends on:** Task 2 (implements the `domain.RuntimeDriver`
interface).

**Files:**
- Create: `internal/infra/runtime/stub/driver.go`
- Create: `internal/infra/runtime/stub/driver_test.go`

**Step 1: Write the failing test first**

Create `internal/infra/runtime/stub/driver_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package stub_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/runtime/stub"
)

func TestStubDriver_RecordsCalls(t *testing.T) {
	d := stub.New()
	ctx := context.Background()

	if err := d.RefreshProjectMaster(ctx, "flow", "main"); err != nil {
		t.Fatalf("RefreshProjectMaster: %v", err)
	}
	master := d.GetProjectMasterRef("flow")
	if master.Kind != "stub" || master.ID == "" {
		t.Fatalf("master ref: got %+v", master)
	}

	vol, err := d.CloneWorkItemVolume(ctx, master, "wi-1")
	if err != nil {
		t.Fatalf("CloneWorkItemVolume: %v", err)
	}

	creds := domain.VolumeRef{Kind: "stub", ID: "creds-a3"}
	h, err := d.StartAgentRuntime(ctx, "a_003", creds, vol)
	if err != nil {
		t.Fatalf("StartAgentRuntime: %v", err)
	}

	alive, err := d.IsRuntimeAlive(ctx, h)
	if err != nil || !alive {
		t.Errorf("alive: got (%v, %v), want (true, nil)", alive, err)
	}

	if err := d.StopAgentRuntime(ctx, h); err != nil {
		t.Fatalf("StopAgentRuntime: %v", err)
	}
	alive, _ = d.IsRuntimeAlive(ctx, h)
	if alive {
		t.Error("runtime should be dead after StopAgentRuntime")
	}

	if err := d.DeleteVolume(ctx, vol); err != nil {
		t.Fatalf("DeleteVolume: %v", err)
	}

	want := []string{
		"RefreshProjectMaster:flow:main",
		"CloneWorkItemVolume:flow:wi-1",
		"StartAgentRuntime:a_003",
		"IsRuntimeAlive",
		"StopAgentRuntime",
		"IsRuntimeAlive",
		"DeleteVolume",
	}
	got := d.Calls()
	if len(got) != len(want) {
		t.Fatalf("call log length: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("call[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestStubDriver_SatisfiesRuntimeDriverInterface(t *testing.T) {
	var _ domain.RuntimeDriver = stub.New()
}
```

**Step 2: Run — confirm fail**

Run: `go test -run TestStubDriver ./internal/infra/runtime/...`
Expected: fails with `no Go files in .../internal/infra/runtime/stub`
(package does not yet exist).

**Step 3: Implement `stub.Driver`**

Create `internal/infra/runtime/stub/driver.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package stub provides a test-only RuntimeDriver implementation. It
// records every call so tests can assert on the sequence and returns
// deterministic VolumeRef / RuntimeHandle values. Used by the e2e
// harness's runtime diagnostic endpoint and by scheduler unit tests.
// Never used in production builds.
package stub

import (
	"context"
	"fmt"
	"sync"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Driver is a test double for domain.RuntimeDriver. Safe for
// concurrent use.
type Driver struct {
	mu      sync.Mutex
	calls   []string
	started map[string]bool
	masters map[string]domain.VolumeRef
	nextID  int
}

// New returns an empty Driver.
func New() *Driver {
	return &Driver{
		started: make(map[string]bool),
		masters: make(map[string]domain.VolumeRef),
	}
}

// Calls returns a copy of the recorded call log.
func (d *Driver) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.calls))
	copy(out, d.calls)
	return out
}

// Reset clears the call log.
func (d *Driver) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = nil
	d.started = make(map[string]bool)
	d.masters = make(map[string]domain.VolumeRef)
	d.nextID = 0
}

func (d *Driver) record(s string) {
	d.calls = append(d.calls, s)
}

func (d *Driver) nextRef(kind string) string {
	d.nextID++
	return fmt.Sprintf("%s-%d", kind, d.nextID)
}

// StartAgentRuntime implements domain.RuntimeDriver.
func (d *Driver) StartAgentRuntime(_ context.Context, agentID string, _ domain.VolumeRef, _ domain.VolumeRef) (domain.RuntimeHandle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("StartAgentRuntime:" + agentID)
	h := domain.RuntimeHandle{Kind: "stub", ID: d.nextRef("rt")}
	d.started[h.ID] = true
	return h, nil
}

// StopAgentRuntime implements domain.RuntimeDriver.
func (d *Driver) StopAgentRuntime(_ context.Context, h domain.RuntimeHandle) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("StopAgentRuntime")
	d.started[h.ID] = false
	return nil
}

// IsRuntimeAlive implements domain.RuntimeDriver.
func (d *Driver) IsRuntimeAlive(_ context.Context, h domain.RuntimeHandle) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("IsRuntimeAlive")
	return d.started[h.ID], nil
}

// CloneWorkItemVolume implements domain.RuntimeDriver.
func (d *Driver) CloneWorkItemVolume(_ context.Context, master domain.VolumeRef, workItemID string) (domain.VolumeRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	project := "unknown"
	for p, m := range d.masters {
		if m == master {
			project = p
			break
		}
	}
	d.record("CloneWorkItemVolume:" + project + ":" + workItemID)
	return domain.VolumeRef{Kind: "stub", ID: d.nextRef("work")}, nil
}

// DeleteVolume implements domain.RuntimeDriver.
func (d *Driver) DeleteVolume(_ context.Context, _ domain.VolumeRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("DeleteVolume")
	return nil
}

// RefreshProjectMaster implements domain.RuntimeDriver.
func (d *Driver) RefreshProjectMaster(_ context.Context, projectID string, gitRef string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("RefreshProjectMaster:" + projectID + ":" + gitRef)
	if _, ok := d.masters[projectID]; !ok {
		d.masters[projectID] = domain.VolumeRef{Kind: "stub", ID: d.nextRef("master")}
	}
	return nil
}

// GetProjectMasterRef implements domain.RuntimeDriver.
func (d *Driver) GetProjectMasterRef(projectID string) domain.VolumeRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.masters[projectID]
}
```

**Step 4: Run — confirm pass**

Run: `go test -run TestStubDriver ./internal/infra/runtime/...`
Expected: PASS for both tests.

**Step 5: Commit**

```
git add internal/infra/runtime/stub/driver.go internal/infra/runtime/stub/driver_test.go
git commit -m "$(cat <<'EOF'
feat(runtime): StubDriver test double implementing RuntimeDriver

Concurrent-safe stub used by scheduler unit tests and by the e2e
harness's runtime diagnostic endpoint. Records every call so tests
can assert sequencing; returns deterministic VolumeRef and
RuntimeHandle values so e2e assertions can pin them.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Scheduler port and types (no implementation yet)

**Depends on:** Task 1 (error sentinels).

**Files:**
- Create: `internal/domain/scheduler.go`

**Step 1: Write the scheduler port and value types**

Create `internal/domain/scheduler.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"time"
)

// AgentClaim is the value returned from Scheduler.AcquireAgent. The
// caller owns the claim until Scheduler.ReleaseAgent is invoked (or
// the Flow process exits, in which case Hive's sweeper will eventually
// clear the lease).
type AgentClaim struct {
	AgentID        string    // Passport agent ID (stable across claims).
	AgentName      string    // Human-friendly display name.
	Role           string    // Role the agent will fill for this claim.
	Project        string    // Project scope for this claim.
	WorkflowID     string    // Flow workflow ID that owns the lease.
	LeaseExpiresAt time.Time // Absolute expiry — renew before this.
}

// Scheduler manages the per-Flow-process agent-pool lifecycle. All
// public methods are safe for concurrent use.
//
// The interface intentionally exposes only the workflow-facing surface
// (Acquire, Release, ActiveClaims). Lease-renewal hooks
// (UpdateLease, HiveClient) are concrete-only on *scheduler.Scheduler;
// daemon wiring uses the concrete type so this interface stays a
// minimal public contract.
type Scheduler interface {
	// AcquireAgent asks Hive for a free agent matching (role, project),
	// sets its current assignment to workflowID with a lease of leaseTTL,
	// registers the claim with the lease renewer, and writes an
	// `agent_claimed` audit event. Returns ErrPoolExhausted after all
	// retries fail.
	AcquireAgent(ctx context.Context, role, project, workflowID string, leaseTTL time.Duration) (*AgentClaim, error)

	// ReleaseAgent clears the claim in Hive, de-registers it from the
	// lease renewer, and writes an `agent_released` audit event.
	ReleaseAgent(ctx context.Context, claim *AgentClaim) error

	// ActiveClaims returns a snapshot of every claim currently held by
	// this Flow process. Used by the lease renewer and by diagnostics.
	ActiveClaims() []AgentClaim
}

// HiveAgentClient is the slice of the Hive Go client the scheduler
// depends on. Declared as an interface here so scheduler tests can
// substitute a fake without importing the Hive client package.
type HiveAgentClient interface {
	ClaimAgent(ctx context.Context, role, project, workflowID string, ttlSeconds int) (*HiveAgent, error)
	ReleaseAgent(ctx context.Context, id, workflowID string) error
	RenewAgentLease(ctx context.Context, id, workflowID string, ttlSeconds int) error
}

// HiveAgent mirrors the fields of github.com/Work-Fort/Hive/client.Agent
// the scheduler reads. Declaring it here keeps domain free of any
// hive-client dependency; the adapter layer translates.
type HiveAgent struct {
	ID             string
	Name           string
	LeaseExpiresAt time.Time
}
```

**Step 2: Verify compile**

Run: `mise run lint`
Expected: exits 0.

**Step 3: Commit**

```
git add internal/domain/scheduler.go
git commit -m "$(cat <<'EOF'
feat(domain): Scheduler port with AgentClaim and HiveAgentClient

Defines the workflow-facing Scheduler interface plus the slim
HiveAgentClient interface scheduler tests substitute against. The
HiveAgent value type duplicates three fields from
hiveclient.Agent so the domain stays free of any hive-client
import; the adapter layer translates.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Hive-client adapter — pool methods + httptest unit test

**Depends on:** Task 4.

**Files:**
- Modify: `internal/infra/hive/adapter.go`
- Create: `internal/infra/hive/adapter_test.go`

**Step 1: Write the failing test first**

Create `internal/infra/hive/adapter_test.go`. The adapter wraps the
real `hiveclient.Client`; we exercise it against an
`httptest.Server` returning canned 409 / 404 responses to prove the
error-mapping is correct end-to-end through the client.

```go
// SPDX-License-Identifier: GPL-2.0-only
package hive_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	hiveinfra "github.com/Work-Fort/Flow/internal/infra/hive"
)

func newAdapter(t *testing.T, h http.Handler) *hiveinfra.Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return hiveinfra.New(srv.URL, "test-token")
}

func TestAdapter_ClaimAgent_PoolExhausted(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/claim" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"agent pool exhausted"}`))
	}))

	_, err := a.ClaimAgent(context.Background(), "developer", "flow", "wf-1", 60)
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Fatalf("want ErrPoolExhausted, got %v", err)
	}
}

func TestAdapter_ReleaseAgent_WorkflowMismatch(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/release") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"workflow id mismatch"}`))
	}))

	err := a.ReleaseAgent(context.Background(), "a_003", "wf-x")
	if !errors.Is(err, domain.ErrWorkflowMismatch) {
		t.Fatalf("want ErrWorkflowMismatch, got %v", err)
	}
}

func TestAdapter_ReleaseAgent_NotFound(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail":"agent not found"}`))
	}))

	err := a.ReleaseAgent(context.Background(), "a_missing", "wf-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "a_missing") {
		t.Errorf("expected error to mention agent ID, got %v", err)
	}
}

func TestAdapter_RenewAgentLease_WorkflowMismatch(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"workflow id mismatch"}`))
	}))

	err := a.RenewAgentLease(context.Background(), "a_003", "wf-x", 60)
	if !errors.Is(err, domain.ErrWorkflowMismatch) {
		t.Fatalf("want ErrWorkflowMismatch, got %v", err)
	}
}

func TestAdapter_SatisfiesHiveAgentClient(t *testing.T) {
	var _ domain.HiveAgentClient = (*hiveinfra.Adapter)(nil)
}
```

**Step 2: Run — confirm fail**

Run: `go test -run TestAdapter ./internal/infra/hive/...`
Expected: FAIL — `(*hive.Adapter).ClaimAgent undefined`.

**Step 3: Append methods to `Adapter`**

The existing `hive.Adapter` implements `domain.IdentityProvider` but
does not yet expose the pool methods. Append to
`internal/infra/hive/adapter.go` (after `GetAgentRoles`, around line
92):

```go
// ClaimAgent implements domain.HiveAgentClient — delegates to the Hive
// client. Maps hiveclient.ErrConflict to domain.ErrPoolExhausted so the
// scheduler can retry-vs-surface on a single sentinel.
//
// Per-endpoint disambiguation: Hive's /claim only returns 409 on
// pool-exhausted; /release and /renew only return 409 on workflow-id
// mismatch (see hive/lead/internal/daemon/rest_huma.go:34-37). This
// adapter exploits that to map ErrConflict differently per method.
func (a *Adapter) ClaimAgent(ctx context.Context, role, project, workflowID string, ttlSeconds int) (*domain.HiveAgent, error) {
	ag, err := a.client.ClaimAgent(ctx, role, project, workflowID, ttlSeconds)
	if err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return nil, domain.ErrPoolExhausted
		}
		return nil, fmt.Errorf("hive claim agent: %w", err)
	}
	return &domain.HiveAgent{
		ID:             ag.ID,
		Name:           ag.Name,
		LeaseExpiresAt: ag.LeaseExpiresAt,
	}, nil
}

// ReleaseAgent implements domain.HiveAgentClient — maps 409 to
// domain.ErrWorkflowMismatch (the only 409 case at /release).
func (a *Adapter) ReleaseAgent(ctx context.Context, id, workflowID string) error {
	if err := a.client.ReleaseAgent(ctx, id, workflowID); err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return domain.ErrWorkflowMismatch
		}
		if errors.Is(err, hiveclient.ErrNotFound) {
			return fmt.Errorf("agent %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("hive release agent %s: %w", id, err)
	}
	return nil
}

// RenewAgentLease implements domain.HiveAgentClient.
func (a *Adapter) RenewAgentLease(ctx context.Context, id, workflowID string, ttlSeconds int) error {
	if err := a.client.RenewAgentLease(ctx, id, workflowID, ttlSeconds); err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return domain.ErrWorkflowMismatch
		}
		if errors.Is(err, hiveclient.ErrNotFound) {
			return fmt.Errorf("agent %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("hive renew lease %s: %w", id, err)
	}
	return nil
}
```

Imports already include `context`, `errors`, `fmt`, `hiveclient`, and
`domain` (see `internal/infra/hive/adapter.go:6-14`). No import
changes required.

**Step 4: Compile-time assertion**

Append to the bottom of `internal/infra/hive/adapter.go`:

```go
// Compile-time assertions.
var _ domain.IdentityProvider = (*Adapter)(nil)
var _ domain.HiveAgentClient  = (*Adapter)(nil)
```

**Step 5: Run — confirm pass**

Run: `go test -run TestAdapter ./internal/infra/hive/...`
Expected: PASS for all five tests.

**Step 6: Commit**

```
git add internal/infra/hive/adapter.go internal/infra/hive/adapter_test.go
git commit -m "$(cat <<'EOF'
feat(hive): adapter implements HiveAgentClient (claim/release/renew)

Adds three pool-method delegations to the existing Hive adapter so
*hive.Adapter satisfies both domain.IdentityProvider and
domain.HiveAgentClient. Per-endpoint error disambiguation maps
ErrConflict to ErrPoolExhausted on /claim and ErrWorkflowMismatch
on /release and /renew. httptest-backed unit tests pin the mapping
end-to-end through the real Hive client.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Audit event schema (sqlite + postgres migrations)

**Depends on:** none — pure data layer.

**Files:**
- Create: `internal/infra/sqlite/migrations/002_audit_events.sql`
- Create: `internal/infra/postgres/migrations/002_audit_events.sql`

**Step 1: Write the SQLite migration**

Create `internal/infra/sqlite/migrations/002_audit_events.sql`:

```sql
-- +goose Up

CREATE TABLE audit_events (
    id            TEXT PRIMARY KEY,
    occurred_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    event_type    TEXT NOT NULL
        CHECK (event_type IN (
            'agent_claimed',
            'agent_released',
            'lease_renewed',
            'lease_expired_by_sweeper'
        )),
    agent_id      TEXT NOT NULL,
    agent_name    TEXT NOT NULL DEFAULT '',
    workflow_id   TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT '',
    project       TEXT NOT NULL DEFAULT '',
    lease_expires_at DATETIME,
    payload       TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX audit_events_workflow_idx ON audit_events(workflow_id, occurred_at);
CREATE INDEX audit_events_agent_idx    ON audit_events(agent_id, occurred_at);
CREATE INDEX audit_events_type_idx     ON audit_events(event_type, occurred_at);

-- +goose Down

DROP INDEX audit_events_type_idx;
DROP INDEX audit_events_agent_idx;
DROP INDEX audit_events_workflow_idx;
DROP TABLE audit_events;
```

**Step 2: Write the Postgres migration**

Create `internal/infra/postgres/migrations/002_audit_events.sql`:

```sql
-- +goose Up

CREATE TABLE audit_events (
    id               TEXT PRIMARY KEY,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type       TEXT NOT NULL
        CHECK (event_type IN (
            'agent_claimed',
            'agent_released',
            'lease_renewed',
            'lease_expired_by_sweeper'
        )),
    agent_id         TEXT NOT NULL,
    agent_name       TEXT NOT NULL DEFAULT '',
    workflow_id      TEXT NOT NULL,
    role             TEXT NOT NULL DEFAULT '',
    project          TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX audit_events_workflow_idx ON audit_events(workflow_id, occurred_at);
CREATE INDEX audit_events_agent_idx    ON audit_events(agent_id, occurred_at);
CREATE INDEX audit_events_type_idx     ON audit_events(event_type, occurred_at);

-- +goose Down

DROP INDEX audit_events_type_idx;
DROP INDEX audit_events_agent_idx;
DROP INDEX audit_events_workflow_idx;
DROP TABLE audit_events;
```

The `lease_expired_by_sweeper` value is reserved for a future plan
(Hive sweeper webhook or Flow-side reconciliation). Step 1 ships it
in the CHECK constraint so the future producer can land without a
migration; no Step 1 code path inserts events of this type.

**Step 3: Verify migrations load**

Run: `go test -run TestStoreOpen ./internal/infra/sqlite/...`
Expected: PASS (confirms the new migration applies cleanly on an
empty DB — `store.Open("")` runs all migrations on every call).

**Step 4: Commit**

```
git add internal/infra/sqlite/migrations/002_audit_events.sql internal/infra/postgres/migrations/002_audit_events.sql
git commit -m "$(cat <<'EOF'
feat(audit): audit_events table (sqlite + postgres)

New durable event log for agent-pool lifecycle transitions. The
table schema is deliberately denormalised — each row carries the
agent name, role, project, and workflow ID at event time so
auditors can reconstruct history without joining against the
mutable agents/projects tables. JSONB payload column reserves
space for future event-type-specific context.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: `AuditEvent` domain types + `AuditEventStore` port

**Depends on:** Task 6 (table), Task 1 (errors).

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`

**Step 1: Verify no in-tree mocks of `domain.Store` will break**

Run: `git grep -n "var _ domain.Store" -- ':!docs/'`
Expected: zero hits. (If hits exist, list them in the commit body
so the fix-up scope is explicit. The aggregate Store extension in
Step 3 below will fail to compile against any such mock; we want to
know that going in.)

**Step 2: Append domain types**

Append to `internal/domain/types.go` (after the runtime value types
from Task 2):

```go
// --- audit events ---

type AuditEventType string

const (
	AuditEventAgentClaimed         AuditEventType = "agent_claimed"
	AuditEventAgentReleased        AuditEventType = "agent_released"
	AuditEventLeaseRenewed         AuditEventType = "lease_renewed"
	AuditEventLeaseExpiredBySweeper AuditEventType = "lease_expired_by_sweeper"
)

// AuditEvent is a single durable record of an agent-pool lifecycle
// transition. Flow is the legal audit primary; Sharkfin transcripts are
// a human-readable derivation.
//
// AuditEventLeaseExpiredBySweeper is reserved for a future plan that
// wires either a Hive-sweeper webhook into Flow or a Flow-startup
// reconciliation pass. No Step 1 code path produces this type.
type AuditEvent struct {
	ID             string
	OccurredAt     time.Time
	Type           AuditEventType
	AgentID        string
	AgentName      string
	WorkflowID     string
	Role           string
	Project        string
	LeaseExpiresAt time.Time       // zero-valued when not applicable.
	Payload        json.RawMessage // free-form context; may be nil.
}
```

**Step 3: Append `AuditEventStore` port**

Append to `internal/domain/ports.go` (after `RuntimeDriver`):

```go
// AuditEventStore persists AuditEvent records. Every scheduler claim,
// release, and renewal writes one event.
type AuditEventStore interface {
	// RecordAuditEvent writes a new event. The store assigns ID and
	// OccurredAt if either is zero-valued.
	RecordAuditEvent(ctx context.Context, e *AuditEvent) error

	// ListAuditEventsByWorkflow returns every event for a workflow ID,
	// oldest first.
	ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*AuditEvent, error)

	// ListAuditEventsByAgent returns every event for an agent, oldest
	// first.
	ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*AuditEvent, error)
}
```

Then extend the aggregate `Store` interface (around
`internal/domain/ports.go:41-49`):

```go
// Store combines all storage interfaces.
type Store interface {
	TemplateStore
	InstanceStore
	WorkItemStore
	ApprovalStore
	AuditEventStore
	Ping(ctx context.Context) error
	io.Closer
}
```

**Step 4: Verify compile fails on SQLite/Postgres stores**

Run: `mise run lint`
Expected: FAIL with `*sqlite.Store does not implement domain.Store`
(missing `RecordAuditEvent`, `ListAuditEventsByWorkflow`,
`ListAuditEventsByAgent`). Same for `*postgres.Store`. This is
intentional — the next two tasks add the implementations.

**Step 5: Commit**

```
git add internal/domain/types.go internal/domain/ports.go
git commit -m "$(cat <<'EOF'
feat(audit): AuditEvent domain type and AuditEventStore port

Introduces the AuditEvent value type, the four event-type constants,
and the three-method AuditEventStore port. Folds AuditEventStore
into the aggregate domain.Store so callers can keep depending on
one Store interface; the next two tasks implement the methods on
both backends.

The lease_expired_by_sweeper constant is reserved — no Step 1 code
produces it, but its presence here lets future plans land the
producer without touching domain.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: SQLite `AuditEventStore` implementation + unit tests

**Depends on:** Task 6 (migration), Task 7 (port).

**Files:**
- Create: `internal/infra/sqlite/audit.go`
- Create: `internal/infra/sqlite/audit_test.go`

**Step 1: Write the failing test**

Create `internal/infra/sqlite/audit_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func TestAuditEvent_RoundTrip(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	lease := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)

	e := &domain.AuditEvent{
		Type:           domain.AuditEventAgentClaimed,
		AgentID:        "a_003",
		AgentName:      "agent-3",
		WorkflowID:     "wf-117",
		Role:           "reviewer",
		Project:        "flow",
		LeaseExpiresAt: lease,
	}
	if err := s.RecordAuditEvent(ctx, e); err != nil {
		t.Fatalf("RecordAuditEvent: %v", err)
	}
	if e.ID == "" {
		t.Errorf("RecordAuditEvent should populate ID, got empty")
	}
	if e.OccurredAt.IsZero() {
		t.Errorf("RecordAuditEvent should populate OccurredAt, got zero")
	}

	byWF, err := s.ListAuditEventsByWorkflow(ctx, "wf-117")
	if err != nil {
		t.Fatalf("ListAuditEventsByWorkflow: %v", err)
	}
	if len(byWF) != 1 {
		t.Fatalf("want 1 event by workflow, got %d", len(byWF))
	}
	if byWF[0].Type != domain.AuditEventAgentClaimed {
		t.Errorf("Type: got %q, want agent_claimed", byWF[0].Type)
	}
	if !byWF[0].LeaseExpiresAt.Equal(lease) {
		t.Errorf("LeaseExpiresAt round-trip: got %v, want %v", byWF[0].LeaseExpiresAt, lease)
	}

	byAgent, err := s.ListAuditEventsByAgent(ctx, "a_003")
	if err != nil {
		t.Fatalf("ListAuditEventsByAgent: %v", err)
	}
	if len(byAgent) != 1 {
		t.Errorf("want 1 event by agent, got %d", len(byAgent))
	}
}

func TestAuditEvent_OrderedByOccurredAt(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i, ty := range []domain.AuditEventType{
		domain.AuditEventAgentClaimed,
		domain.AuditEventLeaseRenewed,
		domain.AuditEventAgentReleased,
	} {
		e := &domain.AuditEvent{
			OccurredAt: base.Add(time.Duration(i) * time.Second),
			Type:       ty,
			AgentID:    "a_003", WorkflowID: "wf-200",
		}
		if err := s.RecordAuditEvent(ctx, e); err != nil {
			t.Fatalf("RecordAuditEvent %s: %v", ty, err)
		}
	}

	events, _ := s.ListAuditEventsByWorkflow(ctx, "wf-200")
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	if events[0].Type != domain.AuditEventAgentClaimed ||
		events[1].Type != domain.AuditEventLeaseRenewed ||
		events[2].Type != domain.AuditEventAgentReleased {
		t.Errorf("wrong order: got %v / %v / %v",
			events[0].Type, events[1].Type, events[2].Type)
	}
}
```

**Step 2: Run — confirm fail**

Run: `go test -run TestAuditEvent ./internal/infra/sqlite/...`
Expected: FAIL at build time —
`s.RecordAuditEvent undefined (*sqlite.Store has no RecordAuditEvent method)`.

**Step 3: Implement in SQLite**

Create `internal/infra/sqlite/audit.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
)

const auditCols = "id, occurred_at, event_type, agent_id, agent_name, workflow_id, role, project, lease_expires_at, payload"

func newAuditID() string {
	return "ae_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
}

// RecordAuditEvent inserts a new audit event. Populates ID and
// OccurredAt on the caller's struct when either is zero-valued.
func (s *Store) RecordAuditEvent(ctx context.Context, e *domain.AuditEvent) error {
	if e.ID == "" {
		e.ID = newAuditID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	var lease sql.NullTime
	if !e.LeaseExpiresAt.IsZero() {
		lease = sql.NullTime{Time: e.LeaseExpiresAt.UTC(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (`+auditCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.OccurredAt.UTC(), string(e.Type),
		e.AgentID, e.AgentName, e.WorkflowID, e.Role, e.Project,
		lease, string(payload))
	if err != nil {
		return fmt.Errorf("insert audit_events: %w", err)
	}
	return nil
}

// ListAuditEventsByWorkflow returns events for a workflow, oldest first.
func (s *Store) ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditCols+`
		FROM audit_events
		WHERE workflow_id = ?
		ORDER BY occurred_at ASC, id ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("query audit_events by workflow: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

// ListAuditEventsByAgent returns events for an agent, oldest first.
func (s *Store) ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditCols+`
		FROM audit_events
		WHERE agent_id = ?
		ORDER BY occurred_at ASC, id ASC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("query audit_events by agent: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func scanAuditEvents(rows *sql.Rows) ([]*domain.AuditEvent, error) {
	var out []*domain.AuditEvent
	for rows.Next() {
		var (
			e       domain.AuditEvent
			typ     string
			lease   sql.NullTime
			payload string
		)
		if err := rows.Scan(
			&e.ID, &e.OccurredAt, &typ,
			&e.AgentID, &e.AgentName, &e.WorkflowID, &e.Role, &e.Project,
			&lease, &payload,
		); err != nil {
			return nil, fmt.Errorf("scan audit_events: %w", err)
		}
		e.Type = domain.AuditEventType(typ)
		if lease.Valid {
			e.LeaseExpiresAt = lease.Time
		}
		if payload != "" && payload != "{}" {
			e.Payload = json.RawMessage(payload)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
```

**Step 4: Run — confirm pass**

Run: `go test -run TestAuditEvent ./internal/infra/sqlite/...`
Expected: PASS for both tests.

**Step 5: Commit**

```
git add internal/infra/sqlite/audit.go internal/infra/sqlite/audit_test.go
git commit -m "$(cat <<'EOF'
feat(sqlite): AuditEventStore implementation with round-trip tests

Adds RecordAuditEvent / ListAuditEventsByWorkflow /
ListAuditEventsByAgent to the SQLite store. Round-trip test pins
LeaseExpiresAt UTC truncation behaviour; ordering test pins
oldest-first guarantee. After this task *sqlite.Store satisfies
the extended domain.Store interface; *postgres.Store still does
not (Task 9).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Postgres `AuditEventStore` implementation + tests

**Depends on:** Task 6 (migration), Task 7 (port), Task 8 (SQLite
reference behavior to mirror).

**Files:**
- Create: `internal/infra/postgres/audit.go`
- Modify: `internal/infra/postgres/store_test.go` (append test)

**Step 1: Implement in Postgres**

Create `internal/infra/postgres/audit.go`. Port the SQLite `audit.go`
from Task 8 with three differences and nothing else:

1. Placeholders `$1`..`$10` instead of `?`.
2. Cast the payload argument to `jsonb` in the INSERT (`$10::jsonb`).
3. `package postgres` instead of `package sqlite`.

Everything else — the `auditCols` constant, `newAuditID()`,
`scanAuditEvents`, the `sql.NullTime` handling, the zero-value
payload substitution, the sort order — is identical. Copy and adjust.

Full insert statement:

```go
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (`+auditCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)`,
		e.ID, e.OccurredAt.UTC(), string(e.Type),
		e.AgentID, e.AgentName, e.WorkflowID, e.Role, e.Project,
		lease, string(payload))
```

List queries use `WHERE workflow_id = $1` / `WHERE agent_id = $1`.

**Step 2: Append test**

Append to `internal/infra/postgres/store_test.go`. (After Task 14
the `dsn` helper fails loudly when PG is unreachable instead of
silently skipping; for now the existing helper still gates on
`FLOW_DB`. The test itself is unchanged regardless.)

```go
func TestAuditEvent_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	lease := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	e := &domain.AuditEvent{
		Type:           domain.AuditEventAgentClaimed,
		AgentID:        newID("a"),
		AgentName:      "agent-pg",
		WorkflowID:     newID("wf"),
		Role:           "reviewer",
		Project:        "flow",
		LeaseExpiresAt: lease,
	}
	if err := s.RecordAuditEvent(ctx, e); err != nil {
		t.Fatalf("RecordAuditEvent: %v", err)
	}

	events, err := s.ListAuditEventsByWorkflow(ctx, e.WorkflowID)
	if err != nil {
		t.Fatalf("ListAuditEventsByWorkflow: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if !events[0].LeaseExpiresAt.Equal(lease) {
		t.Errorf("LeaseExpiresAt round-trip: got %v, want %v", events[0].LeaseExpiresAt, lease)
	}
}
```

**Step 3: Run tests**

Run: `go test -run TestAuditEvent ./internal/infra/postgres/...`
Expected: PASS when `FLOW_DB` points at a reachable PG; SKIPPED
otherwise. (Task 14 makes the default invocation set `FLOW_DB`
automatically; until then the SKIP behaviour is unchanged.)

Run: `go test ./internal/infra/sqlite/...`
Expected: existing sqlite tests still PASS — the aggregate
`domain.Store` is now satisfied by both `*sqlite.Store` and
`*postgres.Store`.

**Step 4: Commit**

```
git add internal/infra/postgres/audit.go internal/infra/postgres/store_test.go
git commit -m "$(cat <<'EOF'
feat(postgres): AuditEventStore implementation and round-trip test

Postgres mirror of the SQLite audit store: parameterised
placeholders, jsonb cast on payload insert, identical scan logic.
After this task both backends satisfy the extended domain.Store.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: `Scheduler` implementation + unit tests with a fake Hive client

**Depends on:** Tasks 1, 4, 7, 8 (errors, port, audit port, sqlite
audit store ready).

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`

The scheduler lives in `internal/scheduler/` (a new package) rather
than inside `internal/workflow/`. `workflow.Service` is the
state-machine engine; the scheduler sits alongside it so the two can
be wired together without a circular import.

**Step 1: Write the failing test**

Create `internal/scheduler/scheduler_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

// fakeHive implements domain.HiveAgentClient with scripted responses.
type fakeHive struct {
	mu sync.Mutex

	claimResponses []claimResp
	claimCalls     int
	releaseCalls   int
	renewCalls     int
	releaseErr     error
	renewErr       error
}

type claimResp struct {
	agent *domain.HiveAgent
	err   error
}

func (f *fakeHive) ClaimAgent(_ context.Context, _, _, _ string, _ int) (*domain.HiveAgent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.claimCalls
	f.claimCalls++
	if idx >= len(f.claimResponses) {
		return nil, errors.New("fakeHive: out of scripted responses")
	}
	r := f.claimResponses[idx]
	return r.agent, r.err
}

func (f *fakeHive) ReleaseAgent(_ context.Context, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	return f.releaseErr
}

func (f *fakeHive) RenewAgentLease(_ context.Context, _, _ string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewCalls++
	return f.renewErr
}

func newAuditStore(t *testing.T) domain.AuditEventStore {
	t.Helper()
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestScheduler_AcquireReleaseRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	expiry := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_003", Name: "agent-3", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "reviewer", "flow", "wf-1", 2*time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}
	if claim.AgentID != "a_003" || claim.WorkflowID != "wf-1" {
		t.Errorf("claim: got %+v", claim)
	}
	if len(sch.ActiveClaims()) != 1 {
		t.Fatalf("ActiveClaims after acquire: got %d, want 1", len(sch.ActiveClaims()))
	}

	events, _ := audit.ListAuditEventsByWorkflow(ctx, "wf-1")
	if len(events) != 1 || events[0].Type != domain.AuditEventAgentClaimed {
		t.Errorf("audit after acquire: got %+v", events)
	}

	if err := sch.ReleaseAgent(ctx, claim); err != nil {
		t.Fatalf("ReleaseAgent: %v", err)
	}
	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims after release: should be empty")
	}
	if hive.releaseCalls != 1 {
		t.Errorf("hive release calls: got %d, want 1", hive.releaseCalls)
	}

	events, _ = audit.ListAuditEventsByWorkflow(ctx, "wf-1")
	if len(events) != 2 {
		t.Fatalf("audit after release: got %d events, want 2", len(events))
	}
	if events[1].Type != domain.AuditEventAgentReleased {
		t.Errorf("second event: got %q, want agent_released", events[1].Type)
	}
}

func TestScheduler_AcquireRetriesOnPoolExhausted(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	expiry := time.Now().UTC().Add(time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
			{agent: &domain.HiveAgent{ID: "a_007", Name: "agent-7", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-2", time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent should eventually succeed: %v", err)
	}
	if claim.AgentID != "a_007" {
		t.Errorf("claim: got %+v", claim)
	}
	if hive.claimCalls != 3 {
		t.Errorf("claim calls: got %d, want 3", hive.claimCalls)
	}
}

func TestScheduler_AcquireReturnsErrPoolExhaustedAfterAllRetries(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	hive := &fakeHive{
		claimResponses: []claimResp{
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
		},
	}
	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	_, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-3", time.Minute)
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
	if hive.claimCalls != 3 {
		t.Errorf("claim calls: got %d, want 3", hive.claimCalls)
	}
	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims should be empty after failed acquire")
	}
}
```

**Step 2: Run — confirm fail**

Run: `go test -run TestScheduler ./internal/scheduler/...`
Expected: FAIL with `no Go files in .../internal/scheduler`
(package does not yet exist).

**Step 3: Implement the scheduler**

Create `internal/scheduler/scheduler.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package scheduler implements the runtime-agnostic agent pool
// scheduler. Workflows call AcquireAgent / ReleaseAgent; the scheduler
// wraps Hive's claim/release endpoints, registers active claims for
// the lease renewer, and emits audit events.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Config holds dependencies and tuning knobs for a Scheduler. Zero
// values for tuning fields fall back to sensible defaults.
type Config struct {
	Hive  domain.HiveAgentClient // required
	Audit domain.AuditEventStore // required

	// MaxClaimRetries is the number of ClaimAgent attempts before
	// AcquireAgent returns ErrPoolExhausted. Default 5.
	MaxClaimRetries int
	// BackoffBase is the base wait between claim retries. Actual wait is
	// BackoffBase * 2^(attempt-1) capped at BackoffMax. Default 100ms.
	BackoffBase time.Duration
	// BackoffMax caps exponential backoff. Default 5s.
	BackoffMax time.Duration
}

const (
	defaultMaxClaimRetries = 5
	defaultBackoffBase     = 100 * time.Millisecond
	defaultBackoffMax      = 5 * time.Second
)

// Scheduler is the concrete implementation of domain.Scheduler.
type Scheduler struct {
	hive  domain.HiveAgentClient
	audit domain.AuditEventStore

	maxRetries  int
	backoffBase time.Duration
	backoffMax  time.Duration

	mu     sync.RWMutex
	claims map[claimKey]*domain.AgentClaim
}

type claimKey struct {
	agentID    string
	workflowID string
}

// New constructs a Scheduler from Config. Panics if Hive or Audit is nil.
func New(cfg Config) *Scheduler {
	if cfg.Hive == nil {
		panic("scheduler: Hive is required")
	}
	if cfg.Audit == nil {
		panic("scheduler: Audit is required")
	}
	if cfg.MaxClaimRetries <= 0 {
		cfg.MaxClaimRetries = defaultMaxClaimRetries
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = defaultBackoffBase
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = defaultBackoffMax
	}
	return &Scheduler{
		hive:        cfg.Hive,
		audit:       cfg.Audit,
		maxRetries:  cfg.MaxClaimRetries,
		backoffBase: cfg.BackoffBase,
		backoffMax:  cfg.BackoffMax,
		claims:      make(map[claimKey]*domain.AgentClaim),
	}
}

// AcquireAgent implements domain.Scheduler.
func (s *Scheduler) AcquireAgent(ctx context.Context, role, project, workflowID string, leaseTTL time.Duration) (*domain.AgentClaim, error) {
	ttlSeconds := int(leaseTTL.Round(time.Second).Seconds())
	if ttlSeconds <= 0 {
		return nil, fmt.Errorf("scheduler: leaseTTL must be positive, got %v", leaseTTL)
	}

	var lastErr error
	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		ag, err := s.hive.ClaimAgent(ctx, role, project, workflowID, ttlSeconds)
		if err == nil {
			claim := &domain.AgentClaim{
				AgentID:        ag.ID,
				AgentName:      ag.Name,
				Role:           role,
				Project:        project,
				WorkflowID:     workflowID,
				LeaseExpiresAt: ag.LeaseExpiresAt,
			}
			s.register(claim)
			s.recordEvent(ctx, domain.AuditEventAgentClaimed, claim)
			log.Info("scheduler: claim succeeded",
				"agent_id", claim.AgentID, "workflow_id", workflowID,
				"role", role, "project", project)
			return claim, nil
		}
		if !errors.Is(err, domain.ErrPoolExhausted) {
			return nil, fmt.Errorf("scheduler: claim agent: %w", err)
		}
		lastErr = err
		log.Debug("scheduler: pool exhausted, backing off",
			"attempt", attempt, "role", role, "project", project)
		if attempt < s.maxRetries {
			if err := s.sleep(ctx, s.backoffFor(attempt)); err != nil {
				return nil, err
			}
		}
	}
	return nil, lastErr
}

// ReleaseAgent implements domain.Scheduler.
func (s *Scheduler) ReleaseAgent(ctx context.Context, claim *domain.AgentClaim) error {
	if claim == nil {
		return fmt.Errorf("scheduler: nil claim")
	}
	err := s.hive.ReleaseAgent(ctx, claim.AgentID, claim.WorkflowID)
	s.unregister(claim)
	if err != nil {
		log.Warn("scheduler: release failed",
			"agent_id", claim.AgentID, "workflow_id", claim.WorkflowID, "err", err)
		return fmt.Errorf("scheduler: release agent: %w", err)
	}
	s.recordEvent(ctx, domain.AuditEventAgentReleased, claim)
	log.Info("scheduler: release succeeded",
		"agent_id", claim.AgentID, "workflow_id", claim.WorkflowID)
	return nil
}

// ActiveClaims implements domain.Scheduler.
func (s *Scheduler) ActiveClaims() []domain.AgentClaim {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.AgentClaim, 0, len(s.claims))
	for _, c := range s.claims {
		out = append(out, *c)
	}
	return out
}

// UpdateLease is called by the LeaseRenewer after a successful renew to
// keep the in-memory claim's LeaseExpiresAt current. Writes a
// lease_renewed audit event.
func (s *Scheduler) UpdateLease(ctx context.Context, agentID, workflowID string, newExpiry time.Time) {
	s.mu.Lock()
	c, ok := s.claims[claimKey{agentID: agentID, workflowID: workflowID}]
	if ok {
		c.LeaseExpiresAt = newExpiry
	}
	s.mu.Unlock()
	if ok {
		snap := *c
		s.recordEvent(ctx, domain.AuditEventLeaseRenewed, &snap)
	}
}

// HiveClient returns the HiveAgentClient the scheduler was built
// with. Used by the LeaseRenewer so daemon wiring doesn't need to
// thread the adapter through twice.
func (s *Scheduler) HiveClient() domain.HiveAgentClient { return s.hive }

// --- internal helpers ---

func (s *Scheduler) register(c *domain.AgentClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims[claimKey{agentID: c.AgentID, workflowID: c.WorkflowID}] = c
}

func (s *Scheduler) unregister(c *domain.AgentClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claims, claimKey{agentID: c.AgentID, workflowID: c.WorkflowID})
}

func (s *Scheduler) backoffFor(attempt int) time.Duration {
	d := s.backoffBase << (attempt - 1)
	if d <= 0 || d > s.backoffMax {
		return s.backoffMax
	}
	return d
}

func (s *Scheduler) sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Scheduler) recordEvent(ctx context.Context, ty domain.AuditEventType, c *domain.AgentClaim) {
	e := &domain.AuditEvent{
		Type:           ty,
		AgentID:        c.AgentID,
		AgentName:      c.AgentName,
		WorkflowID:     c.WorkflowID,
		Role:           c.Role,
		Project:        c.Project,
		LeaseExpiresAt: c.LeaseExpiresAt,
	}
	if err := s.audit.RecordAuditEvent(ctx, e); err != nil {
		// Audit failures are warnings, not errors — the scheduler's
		// primary duty (Hive CAS) already succeeded. The e2e test
		// asserts Hive succeeds while audit fails (Task 12 step 4).
		log.Warn("scheduler: audit record failed",
			"event_type", ty, "agent_id", c.AgentID, "err", err)
	}
}

// Compile-time assertion.
var _ domain.Scheduler = (*Scheduler)(nil)
```

**Step 4: Run — confirm pass**

Run: `go test -run TestScheduler ./internal/scheduler/...`
Expected: PASS (all three scenarios).

**Step 5: Commit**

```
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git commit -m "$(cat <<'EOF'
feat(scheduler): AcquireAgent/ReleaseAgent with retry and audit events

The scheduler primitive exposes AcquireAgent/ReleaseAgent to
workflows, tracks active claims for the lease renewer, retries
ClaimAgent on ErrPoolExhausted with bounded exponential backoff,
and writes one audit event per claim/release. Audit failures are
logged but do not unwind the Hive CAS that already succeeded.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: `LeaseRenewer` background goroutine + deterministic unit tests

**Depends on:** Task 10.

**Files:**
- Create: `internal/scheduler/renewer.go`
- Create: `internal/scheduler/renewer_test.go`

The renewer test injects a tick channel rather than relying on
wall-clock `time.Sleep`. This eliminates CI flakiness under `-race`.

**Step 1: Write the failing test**

Create `internal/scheduler/renewer_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

func TestRenewer_RenewsEveryActiveClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)

	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
			{agent: &domain.HiveAgent{ID: "a_002", Name: "agent-2", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})

	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-a", time.Minute); err != nil {
		t.Fatalf("AcquireAgent wf-a: %v", err)
	}
	if _, err := sch.AcquireAgent(ctx, "reviewer", "flow", "wf-b", time.Minute); err != nil {
		t.Fatalf("AcquireAgent wf-b: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch,
		Hive:      hive,
		Tick:      tickCh,
		LeaseTTL:  time.Minute,
	})
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	// Send three explicit ticks; assert exactly 2 (claims) * 3 (ticks)
	// = 6 renew calls. Deterministic regardless of CI scheduling.
	for i := 0; i < 3; i++ {
		tickCh <- time.Now()
	}
	// Give the renewer one scheduling slice to drain the last tick
	// before we cancel; this is bounded, not racy — the renewer's
	// processing loop is synchronous within a tick.
	r.WaitIdle()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewer did not exit within 1 s of cancel")
	}

	hive.mu.Lock()
	defer hive.mu.Unlock()
	if hive.renewCalls != 6 {
		t.Errorf("renew calls: got %d, want 6", hive.renewCalls)
	}
}

func TestRenewer_DropsClaimOnMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
		},
		renewErr: domain.ErrWorkflowMismatch,
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})
	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-1", time.Minute); err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch, Hive: hive, Tick: tickCh, LeaseTTL: time.Minute,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); r.Run(ctx) }()

	tickCh <- time.Now()
	r.WaitIdle()
	cancel()
	wg.Wait()

	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims after mismatch: got %d, want 0", len(sch.ActiveClaims()))
	}
}
```

**Step 2: Run — confirm fail**

Run: `go test -run TestRenewer ./internal/scheduler/...`
Expected: FAIL — `NewLeaseRenewer` / `RenewerConfig` undefined.

**Step 3: Implement the renewer**

Create `internal/scheduler/renewer.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// RenewerConfig configures a LeaseRenewer. Zero values for Interval and
// LeaseTTL get sensible defaults. Tick is optional — when nil, the
// renewer creates its own time.NewTicker(Interval). Tests pass a
// hand-driven channel to make timing deterministic.
type RenewerConfig struct {
	Scheduler *Scheduler             // required
	Hive      domain.HiveAgentClient // required
	Interval  time.Duration          // default 30s; ignored when Tick != nil
	LeaseTTL  time.Duration          // default 2m
	Tick      <-chan time.Time       // optional, for tests
}

const (
	defaultRenewInterval = 30 * time.Second
	defaultRenewTTL      = 2 * time.Minute
)

// LeaseRenewer is a background goroutine that keeps every claim the
// live Flow process holds alive by calling Hive's renew endpoint every
// Interval until its context is cancelled.
type LeaseRenewer struct {
	sch      *Scheduler
	hive     domain.HiveAgentClient
	interval time.Duration
	ttl      time.Duration
	tick     <-chan time.Time

	idleMu sync.Mutex
	idleCh chan struct{} // closed-and-replaced after every renewOnce
}

// NewLeaseRenewer constructs a LeaseRenewer.
func NewLeaseRenewer(cfg RenewerConfig) *LeaseRenewer {
	if cfg.Scheduler == nil {
		panic("renewer: Scheduler is required")
	}
	if cfg.Hive == nil {
		panic("renewer: Hive is required")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultRenewInterval
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = defaultRenewTTL
	}
	r := &LeaseRenewer{
		sch:      cfg.Scheduler,
		hive:     cfg.Hive,
		interval: cfg.Interval,
		ttl:      cfg.LeaseTTL,
		tick:     cfg.Tick,
		idleCh:   make(chan struct{}),
	}
	return r
}

// Run ticks every Interval (or each value sent on the test Tick) and
// renews every claim currently held by the Scheduler. Returns when
// ctx is cancelled.
func (r *LeaseRenewer) Run(ctx context.Context) {
	tickCh := r.tick
	if tickCh == nil {
		t := time.NewTicker(r.interval)
		defer t.Stop()
		tickCh = t.C
	}
	for {
		select {
		case <-ctx.Done():
			log.Debug("renewer: shutting down")
			return
		case <-tickCh:
			r.renewOnce(ctx)
			r.signalIdle()
		}
	}
}

// WaitIdle blocks until the next renewOnce completes. Used by tests
// after sending a manual tick to wait for the renewer to drain it.
// Production code does not call this.
func (r *LeaseRenewer) WaitIdle() {
	r.idleMu.Lock()
	ch := r.idleCh
	r.idleMu.Unlock()
	<-ch
}

func (r *LeaseRenewer) signalIdle() {
	r.idleMu.Lock()
	close(r.idleCh)
	r.idleCh = make(chan struct{})
	r.idleMu.Unlock()
}

func (r *LeaseRenewer) renewOnce(ctx context.Context) {
	ttlSeconds := int(r.ttl.Round(time.Second).Seconds())
	newExpiry := time.Now().UTC().Add(r.ttl)
	for _, c := range r.sch.ActiveClaims() {
		err := r.hive.RenewAgentLease(ctx, c.AgentID, c.WorkflowID, ttlSeconds)
		switch {
		case err == nil:
			r.sch.UpdateLease(ctx, c.AgentID, c.WorkflowID, newExpiry)
		case errors.Is(err, domain.ErrWorkflowMismatch), errors.Is(err, domain.ErrNotFound):
			log.Warn("renewer: claim gone, dropping",
				"agent_id", c.AgentID, "workflow_id", c.WorkflowID, "err", err)
			claim := c
			r.sch.unregister(&claim)
		default:
			log.Warn("renewer: renew failed, will retry next tick",
				"agent_id", c.AgentID, "workflow_id", c.WorkflowID, "err", err)
		}
	}
}
```

`unregister` is unexported but `renewer.go` is in the same
`package scheduler` so it can call `sch.unregister` directly.

**Step 4: Run — confirm pass**

Run: `go test -run TestRenewer ./internal/scheduler/...`
Expected: PASS for both tests (deterministic — exactly 6 renew
calls, no timing-based assertions).

**Step 5: Full package run**

Run: `go test ./internal/scheduler/...`
Expected: scheduler + renewer all pass together.

**Step 6: Commit**

```
git add internal/scheduler/renewer.go internal/scheduler/renewer_test.go
git commit -m "$(cat <<'EOF'
feat(scheduler): LeaseRenewer with injectable tick for deterministic tests

Renewer ticks production-time via time.Ticker (default 30s), but
RenewerConfig accepts an optional Tick channel so tests drive the
loop with hand-sent values and assert exact renew counts. WaitIdle
exposes a per-tick completion signal so the test can synchronise
without sleeps. Mismatch and not-found errors drop the claim from
in-memory state; transient errors retry next tick.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Daemon wiring — scheduler, renewer, runtime, and `_diag` endpoint

**Depends on:** Tasks 3, 5, 10, 11 (stub driver, adapter, scheduler,
renewer).

**Files:**
- Modify: `internal/daemon/server.go` (Hive resolution block ~lines
  57-100; `NewServer` signature; new route registration)
- Modify: `cmd/daemon/daemon.go` (around lines 99-128; add scheduler
  + renewer to startup)
- Modify: `internal/config/config.go` (add `time` import + viper
  defaults)
- Create: `internal/daemon/runtime_diag.go` (the `_diag` Huma
  handler)

The scheduler, renewer, and a process-global `RuntimeDriver` are
wired with the same lifetime as `healthCtx`. The `RuntimeDriver` in
production is `nil` until a real driver lands; the e2e harness
overrides it with `stub.New()` via a `ServerConfig` field so the
diagnostic endpoint can exercise the interface end-to-end.

**Step 1: Audit no other `NewServer` callers**

Run: `git grep -n "flowDaemon.NewServer\|daemon.NewServer\|NewServer(" -- '*.go'`
Expected: exactly one production caller in `cmd/daemon/daemon.go`;
zero callers in `tests/e2e/` (notably `tests/e2e/harness/daemon.go`
and `tests/e2e/harness/daemon_leak_test.go` spawn the daemon as a
subprocess and do NOT call `NewServer` directly); zero test callers
in `internal/` (daemon tests use `httptest` against handler funcs).
If the audit surfaces additional callers, update them in the same
task. Save the verbatim grep output for the Task 12 commit body
(see Step 5).

**Step 2: Extend `ServerConfig` with `Runtime` slot**

In `internal/daemon/server.go`'s `ServerConfig` struct, add:

```go
	// Runtime is the runtime driver bound to /v1/runtime/_diag/*. nil
	// in production until a real driver lands; the e2e harness injects
	// stub.Driver so the diag endpoint exercises the interface.
	Runtime domain.RuntimeDriver
```

**Step 3: Resolve the Hive adapter as both interfaces**

In `internal/daemon/server.go`, replace the existing single-assignment
Hive resolution block (around line 66-70) with the two-assignment
form:

```go
	var hiveAgentClient domain.HiveAgentClient
	if hiveSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Hive); err == nil {
		adapter := hiveinfra.New(hiveSvc.BaseURL, cfg.ServiceToken)
		identityProvider = adapter
		hiveAgentClient = adapter
	} else {
		log.Warn("hive not found in Pylon, identity checks disabled", "err", err)
	}
```

The compile-time assertion from Task 5 guarantees the adapter
satisfies both interfaces.

**Step 4: Construct the scheduler in `NewServer`**

After `workflow.New(...)` in `internal/daemon/server.go` (around
line 93), add:

```go
	var sch *scheduler.Scheduler
	if hiveAgentClient != nil {
		sch = scheduler.New(scheduler.Config{
			Hive:  hiveAgentClient,
			Audit: cfg.Store,
		})
	}
```

Add `"github.com/Work-Fort/Flow/internal/scheduler"` to the imports.

**Step 5: Change `NewServer` signature**

The existing `NewServer` returns `*http.Server`. Change to also
return `*scheduler.Scheduler` so `cmd/daemon/daemon.go` can start
the renewer:

```go
func NewServer(cfg ServerConfig) (*http.Server, *scheduler.Scheduler) {
    // … existing body …
    return &http.Server{…}, sch
}
```

**Hard requirement — caller audit before signature change.** Run:

```
git grep -n "flowDaemon.NewServer\|daemon.NewServer\|NewServer(" -- '*.go'
```

The expected callers are:
- `cmd/daemon/daemon.go` — production entrypoint (this task updates it).
- Zero callers in `tests/e2e/` (the harness spawns the daemon as a
  subprocess via `exec.Command`; `tests/e2e/harness/daemon.go` and
  `tests/e2e/harness/daemon_leak_test.go` do NOT call `NewServer`
  directly — confirm this with the grep).
- Zero callers in `internal/` (daemon-package tests use `httptest`
  against handler funcs, not `NewServer`).

Paste the verbatim grep output into the Task 12 commit body so the
reviewer can verify zero callers were missed. If any unexpected
caller surfaces, update it in this same task before committing —
this is a one-task scope, not a follow-up.

**Step 6: Register the `_diag` runtime endpoint**

Create `internal/daemon/runtime_diag.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

// registerRuntimeDiagRoutes installs an internal-only diagnostic
// endpoint that drives the bound RuntimeDriver. Used by the e2e
// harness to exercise the RuntimeDriver interface end-to-end through
// the daemon. Returns 503 when no driver is bound (production today).
//
// Not part of the published API. Tagged "Runtime/_diag" in the OpenAPI
// surface and gated to localhost in a future plan once production
// drivers land.
func registerRuntimeDiagRoutes(api huma.API, rt domain.RuntimeDriver) {
	type startInput struct {
		Body struct {
			ProjectID  string `json:"project_id"`
			WorkItemID string `json:"work_item_id"`
			AgentID    string `json:"agent_id"`
			GitRef     string `json:"git_ref"`
		}
	}
	type startOutput struct {
		Body struct {
			Master domain.VolumeRef    `json:"master"`
			Work   domain.VolumeRef    `json:"work"`
			Handle domain.RuntimeHandle `json:"handle"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "runtime-diag-start",
		Method:        http.MethodPost,
		Path:          "/v1/runtime/_diag/start",
		Summary:       "Internal: drive RuntimeDriver end-to-end (refresh → clone → start)",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Runtime/_diag"},
	}, func(ctx context.Context, input *startInput) (*startOutput, error) {
		if rt == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "runtime driver not configured")
		}
		if err := rt.RefreshProjectMaster(ctx, input.Body.ProjectID, input.Body.GitRef); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		master := rt.GetProjectMasterRef(input.Body.ProjectID)
		work, err := rt.CloneWorkItemVolume(ctx, master, input.Body.WorkItemID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		creds := domain.VolumeRef{Kind: "stub", ID: "creds-" + input.Body.AgentID}
		h, err := rt.StartAgentRuntime(ctx, input.Body.AgentID, creds, work)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		out := &startOutput{}
		out.Body.Master = master
		out.Body.Work = work
		out.Body.Handle = h
		return out, nil
	})

	type stopInput struct {
		Body struct {
			Handle domain.RuntimeHandle `json:"handle"`
			Volume domain.VolumeRef     `json:"volume"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "runtime-diag-stop",
		Method:        http.MethodPost,
		Path:          "/v1/runtime/_diag/stop",
		Summary:       "Internal: drive RuntimeDriver stop + delete-volume",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Runtime/_diag"},
	}, func(ctx context.Context, input *stopInput) (*struct{}, error) {
		if rt == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "runtime driver not configured")
		}
		if err := rt.StopAgentRuntime(ctx, input.Body.Handle); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		if err := rt.DeleteVolume(ctx, input.Body.Volume); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		return nil, nil
	})
}
```

In `internal/daemon/server.go`, after the existing route
registrations (search for the existing `register*Routes` calls), add:

```go
	registerRuntimeDiagRoutes(api, cfg.Runtime)
```

**Step 7: Start the renewer in `cmd/daemon/daemon.go`**

Modify the `NewServer` call (currently around line 99) to capture
the scheduler:

```go
	srv, sched := flowDaemon.NewServer(flowDaemon.ServerConfig{
		// … unchanged fields …
	})
```

Then, immediately after the `go health.StartPeriodic(healthCtx, 30*time.Second)`
line (currently line 127), add:

```go
	if sched != nil {
		renewer := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
			Scheduler: sched,
			Hive:      sched.HiveClient(),
			Interval:  viper.GetDuration("lease-renewer-interval"),
			LeaseTTL:  viper.GetDuration("lease-ttl"),
		})
		go renewer.Run(healthCtx)
	}
```

Add `"github.com/Work-Fort/Flow/internal/scheduler"` to the imports
in `cmd/daemon/daemon.go`.

**Step 8: Add viper defaults**

Modify `internal/config/config.go`'s `InitViper` function (around
lines 77-96):

1. Add `"time"` to the top-level import block (it is not currently
   imported).
2. Add the two defaults:

```go
	viper.SetDefault("lease-renewer-interval", 30*time.Second)
	viper.SetDefault("lease-ttl",              2*time.Minute)
```

These correspond to the TTL discussion in
`flow/lead/docs/2026-04-18-agent-pool.md` lines 107-109: short
TTL (2 min) bounds recovery; renewal interval (30 s) leaves
margin for one missed tick.

**Step 9: Verify everything compiles and existing tests still pass**

Run: `mise run lint`
Expected: exits 0.

Run: `go test ./...`
Expected: all unit + integration tests pass, including the
scheduler package and the unchanged daemon/workflow/infra packages.

Run: `mise run build:dev`
Expected: builds `build/flow` successfully.

**Step 10: Commit**

```
git add internal/daemon/server.go internal/daemon/runtime_diag.go cmd/daemon/daemon.go internal/scheduler/scheduler.go internal/config/config.go
git commit -m "$(cat <<'EOF'
feat(daemon): wire Scheduler, LeaseRenewer, and runtime _diag endpoint

NewServer constructs the Scheduler when Hive is reachable and
returns it alongside the http.Server so the daemon entrypoint can
start the LeaseRenewer goroutine. ServerConfig.Runtime is the
RuntimeDriver slot used by the new /v1/runtime/_diag/* endpoint —
nil in production until a real driver lands; the e2e harness wires
stub.Driver so the diag endpoint exercises the RuntimeDriver
interface end-to-end. Viper gains two duration defaults for the
renewer's tuning knobs.

Caller audit (paste verbatim from Step 1):

<grep output of "git grep -n flowDaemon.NewServer|daemon.NewServer|NewServer(" -- '*.go'>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: E2E coverage for every subsystem

**Depends on:** Tasks 5, 8, 9, 10, 11, 12.

**Files:**
- Modify: `tests/e2e/harness/fake_hive.go` (extend with the three
  pool endpoints + control surface)
- Modify: `tests/e2e/harness/env.go` (add a `WithStubRuntime` option
  and seed a default agent)
- Modify: `tests/e2e/harness/daemon.go` (forward a new env var or
  flag for runtime injection)
- Modify: `tests/e2e/go.mod` / `tests/e2e/go.sum` (no new deps —
  existing `net/http`, `encoding/json`, `pgx` already cover
  everything)
- Create: `tests/e2e/agent_pool_test.go`
- Create: `tests/e2e/audit_events_test.go`
- Create: `tests/e2e/runtime_diag_test.go`

The harness extension keeps the wire-protocol-only discipline:
no import of `github.com/Work-Fort/Hive/client`, no import of
Flow's domain package. All wire-format JSON is hand-rolled in the
harness with field tags matching what `hive/lead/client/agents.go`
sends/receives.

**Step 1: Extend `fake_hive.go` with the three pool endpoints**

Append to `tests/e2e/harness/fake_hive.go`:

```go
// --- pool endpoints ---

// PoolAgent is the wire-shape Hive's claim/release/renew endpoints
// emit and consume. Field tags match hive/lead/client/types.go.
type PoolAgent struct {
	ID                string    `json:"ID"`
	Name              string    `json:"Name"`
	TeamID            string    `json:"TeamID"`
	AssignedRole      string    `json:"AssignedRole,omitempty"`
	CurrentProject    string    `json:"CurrentProject,omitempty"`
	CurrentWorkflowID string    `json:"CurrentWorkflowID,omitempty"`
	LeaseExpiresAt    time.Time `json:"LeaseExpiresAt,omitempty"`
}

// poolState tracks per-agent claim state inside the FakeHive.
type poolState struct {
	role, project, workflowID string
	leaseExpiresAt            time.Time
}

// SeedPoolAgent registers a free agent the fake will hand out via
// /v1/agents/claim. Name and TeamID are echoed back in the wire
// response so test assertions can pin them.
func (h *FakeHive) SeedPoolAgent(id, name, teamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pool == nil {
		h.pool = make(map[string]*poolState)
	}
	h.pool[id] = nil // nil = unclaimed
	h.poolMeta = append(h.poolMeta, PoolAgent{ID: id, Name: name, TeamID: teamID})
}

// ClaimCalls / ReleaseCalls / RenewCalls return the per-method call
// counts so tests can assert acquire/release/renew traffic.
func (h *FakeHive) ClaimCalls() int   { h.mu.RLock(); defer h.mu.RUnlock(); return h.claimCalls }
func (h *FakeHive) ReleaseCalls() int { h.mu.RLock(); defer h.mu.RUnlock(); return h.releaseCalls }
func (h *FakeHive) RenewCalls() int   { h.mu.RLock(); defer h.mu.RUnlock(); return h.renewCalls }
```

Add `pool map[string]*poolState`, `poolMeta []PoolAgent`,
`claimCalls int`, `releaseCalls int`, `renewCalls int` to the
existing `FakeHive` struct.

In `Start()`, register three new handlers BEFORE the existing
`/v1/agents` handlers (so the more-specific paths win):

```go
	mux.HandleFunc("POST /v1/agents/claim", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Role            string `json:"role"`
			Project         string `json:"project"`
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeHiveError(w, http.StatusBadRequest, "bad json")
			return
		}
		h.mu.Lock()
		h.claimCalls++
		var picked string
		for _, meta := range h.poolMeta {
			if h.pool[meta.ID] == nil {
				picked = meta.ID
				break
			}
		}
		if picked == "" {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "agent pool exhausted")
			return
		}
		expiry := time.Now().UTC().Add(time.Duration(body.LeaseTTLSeconds) * time.Second)
		h.pool[picked] = &poolState{
			role: body.Role, project: body.Project,
			workflowID: body.WorkflowID, leaseExpiresAt: expiry,
		}
		var meta PoolAgent
		for _, m := range h.poolMeta {
			if m.ID == picked {
				meta = m
				break
			}
		}
		meta.AssignedRole = body.Role
		meta.CurrentProject = body.Project
		meta.CurrentWorkflowID = body.WorkflowID
		meta.LeaseExpiresAt = expiry
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, meta)
	})

	mux.HandleFunc("POST /v1/agents/{id}/release", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			WorkflowID string `json:"workflow_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		h.mu.Lock()
		h.releaseCalls++
		st, ok := h.pool[id]
		if !ok {
			h.mu.Unlock()
			writeHumaError(w, http.StatusNotFound, "agent not found")
			return
		}
		if st == nil || st.workflowID != body.WorkflowID {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "workflow id mismatch")
			return
		}
		h.pool[id] = nil
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /v1/agents/{id}/renew", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		h.mu.Lock()
		h.renewCalls++
		st, ok := h.pool[id]
		if !ok {
			h.mu.Unlock()
			writeHumaError(w, http.StatusNotFound, "agent not found")
			return
		}
		if st == nil || st.workflowID != body.WorkflowID {
			h.mu.Unlock()
			writeHumaError(w, http.StatusConflict, "workflow id mismatch")
			return
		}
		st.leaseExpiresAt = time.Now().UTC().Add(time.Duration(body.LeaseTTLSeconds) * time.Second)
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
```

Add a small `writeHumaError` helper at the bottom of the file (Hive's
real Huma server emits `{"detail":"..."}`, distinct from the
existing `{"error":"..."}` writeHiveError used for the simpler
GET endpoints — the Hive Go client's error parser handles both):

```go
func writeHumaError(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}
```

**Step 2: Add a `--inject-runtime-stub` env-var hook to the daemon
binary, harness opts, and Server**

Production builds keep `cfg.Runtime = nil`. The e2e harness needs to
inject `stub.New()` without exposing a CLI flag in production. The
cleanest path: a build-tagged file behind `//go:build e2e`. To keep
this plan simple, use an env var read by `cmd/daemon/daemon.go`:

In `cmd/daemon/daemon.go`, after the `cfg :=` block, add:

```go
	if os.Getenv("FLOW_E2E_RUNTIME_STUB") == "1" {
		cfg.Runtime = stub.New()
	}
```

Add `"github.com/Work-Fort/Flow/internal/infra/runtime/stub"` to the
imports. (Yes, this leaves a hook in the production binary. It is
gated on an env var no production deployment would set, and the
stub package is harmless on its own. Documenting the trade-off is
sufficient — adding a build tag would force the e2e harness to
build a separate binary and maintain a parallel mise task. Not
worth it for Step 1.)

In `tests/e2e/harness/daemon.go`, extend `daemonCfg` with:

```go
	stubRuntime bool
```

Add a `WithStubRuntime()` option:

```go
func WithStubRuntime() DaemonOption {
	return func(c *daemonCfg) { c.stubRuntime = true }
}
```

**All harness env-var forwarding lives in one place.** In
`StartDaemon`, replace the existing `cmd.Env = append(...)` block
(currently sets `XDG_CONFIG_HOME` and `XDG_STATE_HOME`) with the
consolidated form below. This is the single edit that wires every
e2e env var the harness controls — `FLOW_E2E_RUNTIME_STUB`,
`FLOW_LEASE_RENEWER_INTERVAL`, and `FLOW_LEASE_TTL`. Step 4's
"forward both via cmd.Env" instruction refers back to this same
edit; do not introduce a second `cmd.Env = append(...)` site.

```go
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+xdgDir+"/config",
		"XDG_STATE_HOME="+xdgDir+"/state",
		// Renewer cadence overrides — viper.BindEnv (added in
		// internal/config/config.go's InitViper) maps these to
		// "lease-renewer-interval" / "lease-ttl". Bound to short
		// values so the agent_pool e2e test observes a renew within
		// its 2-second poll window without serialising on a 30-second
		// production tick.
		"FLOW_LEASE_RENEWER_INTERVAL=100ms",
		"FLOW_LEASE_TTL=2s",
	)
	if cfg.stubRuntime {
		cmd.Env = append(cmd.Env, "FLOW_E2E_RUNTIME_STUB=1")
	}
```

The lease overrides are unconditional — every spawned daemon under
the harness uses the short cadence. They have no effect when no
agent is claimed (the renewer's `ActiveClaims` returns empty and
no Hive calls land), so always-on is harmless and avoids per-test
plumbing.

**Step 3: Extend `tests/e2e/harness/env.go` with backend selection
and pool seeding**

Add a `BackendOption` for SQLite (default) vs Postgres so e2e tests
can select per the `--backend` flag from Task 14:

```go
// EnvOption configures harness.NewEnv.
type EnvOption func(*envCfg)

type envCfg struct {
	backend     string // "sqlite" (default) or "postgres"
	stubRuntime bool
}

// WithBackend selects the daemon's storage backend. "sqlite" (the
// default) leaves FLOW_DB unset; "postgres" sets FLOW_DB to the value
// of FLOW_E2E_PG_DSN (or the local PG default if unset).
func WithBackend(b string) EnvOption {
	return func(c *envCfg) { c.backend = b }
}

// WithStubRuntimeEnv injects stub.Driver as the daemon's RuntimeDriver
// so the /v1/runtime/_diag/* endpoint is exercisable.
func WithStubRuntimeEnv() EnvOption {
	return func(c *envCfg) { c.stubRuntime = true }
}
```

Update `NewEnv` to accept `...EnvOption` and forward each option to
the appropriate downstream call:

```go
func NewEnv(t testing.TB, opts ...EnvOption) *Env {
	t.Helper()
	cfg := &envCfg{backend: "sqlite"}
	for _, o := range opts {
		o(cfg)
	}

	binary := os.Getenv("FLOW_BINARY")
	if binary == "" {
		binary = "../../build/flow"
	}

	jwks := StartJWKSStub()
	hive := NewFakeHive()
	hiveBase, stopHive := hive.Start()
	sharkfin := NewFakeSharkfin()
	sharkfinBase, stopSharkfin := sharkfin.Start()

	pylonServices := []PylonService{
		{Name: "hive", BaseURL: hiveBase, Label: "Hive", Route: "/hive"},
		{Name: "sharkfin", BaseURL: sharkfinBase, Label: "Sharkfin", Route: "/sharkfin"},
	}
	pylonAddr, stopPylon := StartPylonStub(pylonServices)

	var daemonOpts []DaemonOption
	if cfg.stubRuntime {
		daemonOpts = append(daemonOpts, WithStubRuntime())
	}
	if cfg.backend == "postgres" {
		dsn := os.Getenv("FLOW_E2E_PG_DSN")
		if dsn == "" {
			dsn = "postgres://postgres@127.0.0.1/flow_test?sslmode=disable"
		}
		daemonOpts = append(daemonOpts, WithDB(dsn))
	}

	d, err := StartDaemon(t, binary, pylonAddr, jwks.Addr, jwks.SignJWT, daemonOpts...)
	if err != nil {
		stopSharkfin(); stopHive(); stopPylon(); jwks.Stop()
		t.Fatalf("start daemon: %v", err)
	}

	return &Env{
		Daemon: d, JWKS: jwks, Hive: hive, Sharkfin: sharkfin,
		stopPylon: stopPylon, stopHive: stopHive, stopSharkfin: stopSharkfin,
	}
}
```

**Step 4: Write the agent-pool E2E test**

Create `tests/e2e/agent_pool_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// agentPoolE2E pins acquire → renew → release through the spawned
// daemon's wired-up scheduler + LeaseRenewer + Hive adapter, using
// the harness FakeHive to serve the three pool endpoints. This
// exercises the entire foundation in a single test:
//   - Hive adapter HTTP round-trip to fake Hive
//   - Scheduler.AcquireAgent retry and audit recording
//   - LeaseRenewer running against real time (via short tuning)
//   - Scheduler.ReleaseAgent and audit cleanup
//
// The test runs the renewer at a 100ms interval and waits ~250ms,
// which is long enough for ≥1 renew on real time. We only assert
// "at least one renew", not an exact count, since wall-clock-driven
// renewer ticks in the daemon are not deterministic from outside.
func TestAgentPool_AcquireRenewRelease(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	env.Hive.SeedPoolAgent("a_e2e_001", "agent-e2e-1", "team-e2e")

	// Override the renewer cadence via env vars before /v1/scheduler/_diag
	// drives a claim. The daemon reads viper at startup, so this requires
	// a small extension: tests/e2e/harness/daemon.go forwards
	// FLOW_LEASE_RENEWER_INTERVAL and FLOW_LEASE_TTL into the spawned
	// process's env, which viper picks up via its env-binding
	// (already configured in internal/config). Wire those env-var
	// passes in StartDaemon if not already present.

	// /v1/scheduler/_diag/claim drives one AcquireAgent through the
	// running daemon. (Endpoint added in Task 12 alongside _diag.)
	c := harness.NewClientNoAuth(env.Daemon.BaseURL())

	claimReq := map[string]any{
		"role": "developer", "project": "flow",
		"workflow_id": "wf-e2e-1", "lease_ttl_seconds": 60,
	}
	status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "a_e2e_001") {
		t.Fatalf("claim response should contain agent ID: %s", body)
	}

	// Wait for ≥1 renew (default daemon renewer is 30s — too slow for
	// e2e; this test depends on the harness setting
	// FLOW_LEASE_RENEWER_INTERVAL=100ms). Bound the wait at 2s.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.Hive.RenewCalls() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if env.Hive.RenewCalls() < 1 {
		t.Errorf("renewer never called Hive: claim=%d release=%d renew=%d",
			env.Hive.ClaimCalls(), env.Hive.ReleaseCalls(), env.Hive.RenewCalls())
	}

	releaseReq := map[string]any{
		"agent_id": "a_e2e_001", "workflow_id": "wf-e2e-1",
	}
	status, body, err = c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("release status=%d body=%s", status, body)
	}
	if env.Hive.ReleaseCalls() != 1 {
		t.Errorf("release calls: got %d, want 1", env.Hive.ReleaseCalls())
	}
}
```

This test depends on two small endpoints being added in Task 12 —
`POST /v1/scheduler/_diag/claim` and `POST /v1/scheduler/_diag/release`
— that drive the wired Scheduler. Add them in `runtime_diag.go` (or
a new `scheduler_diag.go` file in the same `internal/daemon`
package) following the same pattern as the runtime diag handlers.
Each takes the relevant fields, calls `sched.AcquireAgent` /
`sched.ReleaseAgent`, and returns the result. The endpoints are
gated to no-op when `sched == nil` (return 503).

Concretely, add this registration to `internal/daemon/server.go`'s
route block right after `registerRuntimeDiagRoutes`:

```go
	registerSchedulerAndAuditDiagRoutes(api, sch, cfg.Store)
```

(The audit-diag handler in Step 5 below shares this registration
function — one diag file, one register call. The scaffold below
shows just the scheduler half; Step 5 fills in the audit half.)

And create `internal/daemon/scheduler_diag.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

// In-memory map keyed by (agent_id, workflow_id) so the release
// endpoint can find the live *AgentClaim the scheduler returned.
// Local to this file because nothing else in production needs it —
// a real workflow engine will hold AgentClaim values directly.
var diagClaims = struct {
	m map[string]*domain.AgentClaim
}{m: make(map[string]*domain.AgentClaim)}

// registerSchedulerAndAuditDiagRoutes registers the scheduler diag
// claim/release endpoints and (folded in by Step 5 below) the audit
// list-by-workflow endpoint. sch may be nil (returns 503 from the
// scheduler endpoints); audit must NOT be nil — the daemon always
// has a Store.
func registerSchedulerAndAuditDiagRoutes(api huma.API, sch *scheduler.Scheduler, audit domain.AuditEventStore) {
	type claimInput struct {
		Body struct {
			Role            string `json:"role"`
			Project         string `json:"project"`
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
	}
	type claimOutput struct {
		Body struct {
			AgentID    string `json:"agent_id"`
			WorkflowID string `json:"workflow_id"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "scheduler-diag-claim",
		Method:        http.MethodPost,
		Path:          "/v1/scheduler/_diag/claim",
		Summary:       "Internal: drive Scheduler.AcquireAgent",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Scheduler/_diag"},
	}, func(ctx context.Context, in *claimInput) (*claimOutput, error) {
		if sch == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "scheduler not configured")
		}
		ttl := time.Duration(in.Body.LeaseTTLSeconds) * time.Second
		claim, err := sch.AcquireAgent(ctx, in.Body.Role, in.Body.Project, in.Body.WorkflowID, ttl)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		diagClaims.m[claim.AgentID+"|"+claim.WorkflowID] = claim
		out := &claimOutput{}
		out.Body.AgentID = claim.AgentID
		out.Body.WorkflowID = claim.WorkflowID
		return out, nil
	})

	type releaseInput struct {
		Body struct {
			AgentID    string `json:"agent_id"`
			WorkflowID string `json:"workflow_id"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "scheduler-diag-release",
		Method:        http.MethodPost,
		Path:          "/v1/scheduler/_diag/release",
		Summary:       "Internal: drive Scheduler.ReleaseAgent",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Scheduler/_diag"},
	}, func(ctx context.Context, in *releaseInput) (*struct{}, error) {
		if sch == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "scheduler not configured")
		}
		claim, ok := diagClaims.m[in.Body.AgentID+"|"+in.Body.WorkflowID]
		if !ok {
			return nil, huma.NewError(http.StatusNotFound, "no live diag claim")
		}
		if err := sch.ReleaseAgent(ctx, claim); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		delete(diagClaims.m, in.Body.AgentID+"|"+in.Body.WorkflowID)
		return nil, nil
	})
}
```

**`viper.BindEnv` calls in `internal/config/config.go`.** Add to
`InitViper`:

```go
	viper.BindEnv("lease-renewer-interval", "FLOW_LEASE_RENEWER_INTERVAL")
	viper.BindEnv("lease-ttl", "FLOW_LEASE_TTL")
```

**Placement matters.** `viper.BindEnv` registers an explicit env-var
mapping for a key; `viper.AutomaticEnv` (already called in
`InitViper`) is a wildcard that maps every key to a transformed env
var. Both pathways resolve to the same lookup result, but explicit
`BindEnv` calls are still required because
`lease-renewer-interval` contains hyphens that `AutomaticEnv` does
not transform to underscores by default. Place the two `BindEnv`
lines **immediately after** the existing `viper.SetDefault`
calls for `lease-renewer-interval` / `lease-ttl` (added in Task 12
Step 8) and **before** `viper.AutomaticEnv()` so the explicit
mapping is visible at the time `AutomaticEnv` registers its
fallback. If `BindEnv` runs after `AutomaticEnv`, the AutomaticEnv
key transformer (which expects underscores, not hyphens) will have
already cached a different env var name, and `BindEnv` becomes a
silent no-op.

The harness-side env-var forwarding for `FLOW_LEASE_RENEWER_INTERVAL`
and `FLOW_LEASE_TTL` lives in the consolidated `cmd.Env = append(...)`
block in Step 2 above — do NOT add a second forwarding site here.

(The renewer is bounded; even if a test forgets to seed an agent,
the renewer's `ActiveClaims` returns empty and no calls land.)

**Step 5: Write the audit-events E2E test**

Create `tests/e2e/audit_events_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// auditEventsE2E pins that AcquireAgent and ReleaseAgent each leave
// one durable event in Flow's audit_events table, queryable through
// the daemon's audit query endpoint.
//
// Adds a small read-only diag endpoint:
//   GET /v1/audit/_diag/by-workflow/{id}
// that calls Store.ListAuditEventsByWorkflow and returns JSON.
func TestAuditEvents_RecordedThroughDaemon(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	env.Hive.SeedPoolAgent("a_audit_001", "agent-audit-1", "team-audit")

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())

	claimReq := map[string]any{
		"role": "reviewer", "project": "flow",
		"workflow_id": "wf-audit-1", "lease_ttl_seconds": 60,
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/claim", claimReq, nil); err != nil || status != http.StatusOK {
		t.Fatalf("claim: status=%d err=%v body=%s", status, err, body)
	}

	releaseReq := map[string]any{
		"agent_id": "a_audit_001", "workflow_id": "wf-audit-1",
	}
	if status, body, err := c.PostJSON("/v1/scheduler/_diag/release", releaseReq, nil); err != nil {
		t.Fatalf("release: %v", err)
	} else if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("release status=%d body=%s", status, body)
	}

	status, body, err := c.GetJSON("/v1/audit/_diag/by-workflow/wf-audit-1", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("audit list: status=%d err=%v body=%s", status, err, body)
	}
	var resp struct {
		Events []struct {
			Type      string `json:"type"`
			AgentID   string `json:"agent_id"`
			Workflow  string `json:"workflow_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(resp.Events) < 2 {
		t.Fatalf("want >= 2 events, got %d (%s)", len(resp.Events), body)
	}
	first, second := resp.Events[0], resp.Events[1]
	if first.Type != "agent_claimed" {
		t.Errorf("event[0].type = %q, want agent_claimed", first.Type)
	}
	if second.Type != "agent_released" {
		t.Errorf("event[1].type = %q, want agent_released", second.Type)
	}
	if first.AgentID != "a_audit_001" {
		t.Errorf("event[0].agent_id = %q, want a_audit_001", first.AgentID)
	}
	_ = fmt.Sprintf // keep fmt import if unused
}
```

The `/v1/audit/_diag/by-workflow/{id}` endpoint is folded into the
existing `registerSchedulerAndAuditDiagRoutes` function in
`internal/daemon/scheduler_diag.go` (declared back in Task 12 with
the `(api, sch, audit)` signature). Add this handler block at the
end of the function body, after the scheduler release handler:

```go
	// --- audit list endpoint ---

	type eventResp struct {
		Type     string `json:"type"`
		AgentID  string `json:"agent_id"`
		Workflow string `json:"workflow_id"`
	}
	type listInput struct {
		ID string `path:"id"`
	}
	type listOutput struct {
		Body struct {
			Events []eventResp `json:"events"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "audit-diag-by-workflow",
		Method:      http.MethodGet,
		Path:        "/v1/audit/_diag/by-workflow/{id}",
		Summary:     "Internal: list audit events by workflow ID",
		Tags:        []string{"Audit/_diag"},
	}, func(ctx context.Context, in *listInput) (*listOutput, error) {
		events, err := audit.ListAuditEventsByWorkflow(ctx, in.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		out := &listOutput{}
		out.Body.Events = make([]eventResp, 0, len(events))
		for _, e := range events {
			out.Body.Events = append(out.Body.Events, eventResp{
				Type:     string(e.Type),
				AgentID:  e.AgentID,
				Workflow: e.WorkflowID,
			})
		}
		return out, nil
	})
```

The `eventResp` JSON keys (`type`, `agent_id`, `workflow_id`) match
what `audit_events_test.go` decodes. `out.Body.Events` is initialized
to a non-nil empty slice so the JSON encodes as `[]` rather than
`null` when there are no rows.

The caller-site registration was already added in Task 12:

```go
	registerSchedulerAndAuditDiagRoutes(api, sch, cfg.Store)
```

`cfg.Store` is a `domain.Store`, which embeds `AuditEventStore`, so
the assignment satisfies the parameter type without a cast.

**Step 6: Write the runtime-diag E2E test**

Create `tests/e2e/runtime_diag_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// runtimeDiagE2E pins that the RuntimeDriver port wires through the
// daemon and that the StubDriver records the expected sequence of
// operations.
func TestRuntime_DiagDrivesStubDriver(t *testing.T) {
	env := harness.NewEnv(t, harness.WithStubRuntimeEnv())
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())

	startReq := map[string]any{
		"project_id":   "flow",
		"work_item_id": "wi-rt-1",
		"agent_id":     "a_rt_001",
		"git_ref":      "main",
	}
	status, body, err := c.PostJSON("/v1/runtime/_diag/start", startReq, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("start: status=%d err=%v body=%s", status, err, body)
	}
	var startResp struct {
		Master struct{ Kind, ID string } `json:"master"`
		Work   struct{ Kind, ID string } `json:"work"`
		Handle struct{ Kind, ID string } `json:"handle"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if startResp.Handle.Kind != "stub" || startResp.Handle.ID == "" {
		t.Errorf("handle: got %+v", startResp.Handle)
	}
	if startResp.Master.Kind != "stub" || startResp.Work.Kind != "stub" {
		t.Errorf("volume kinds: master=%+v work=%+v", startResp.Master, startResp.Work)
	}

	stopReq := map[string]any{
		"handle": startResp.Handle,
		"volume": startResp.Work,
	}
	status, body, err = c.PostJSON("/v1/runtime/_diag/stop", stopReq, nil)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("stop status=%d body=%s", status, body)
	}
}

// Also assert that without the stub injection, the diag endpoint
// returns 503 — proves the env-var hook is the only path to wire
// a driver, no accidental production exposure.
func TestRuntime_DiagReturns503WithoutStub(t *testing.T) {
	env := harness.NewEnv(t) // no WithStubRuntimeEnv()
	defer env.Cleanup(t)

	c := harness.NewClientNoAuth(env.Daemon.BaseURL())
	status, _, err := c.PostJSON("/v1/runtime/_diag/start", map[string]any{
		"project_id": "flow", "work_item_id": "wi", "agent_id": "a", "git_ref": "main",
	}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no stub bound, got %d", status)
	}
}
```

If `harness.NewClientNoAuth` doesn't have `PostJSON`, add it
following the existing `GetJSON` pattern in `harness/client.go`.

**Step 7: Run the e2e suite**

Build first, then run:

Run: `mise run build:dev`
Expected: builds `build/flow`.

Run: `mise run e2e`
Expected: PASS for all four new e2e tests plus the existing ones.

**Step 8: Commit**

```
git add tests/e2e/harness/fake_hive.go tests/e2e/harness/env.go tests/e2e/harness/daemon.go tests/e2e/agent_pool_test.go tests/e2e/audit_events_test.go tests/e2e/runtime_diag_test.go internal/daemon/scheduler_diag.go internal/daemon/server.go internal/config/config.go
git commit -m "$(cat <<'EOF'
test(e2e): cover scheduler, renewer, audit, and runtime end-to-end

Extends the harness FakeHive with the three pool endpoints (raw
wire format, no Hive client import) and adds three e2e tests that
spawn the daemon, drive Scheduler.AcquireAgent / RenewAgentLease /
ReleaseAgent through internal _diag endpoints, query the audit
event log, and exercise the RuntimeDriver port through the
StubDriver. The _diag endpoints are scoped to never wire in
production: scheduler/audit diags are inert until an active
scheduler exists, and runtime diags require an env-var hook the
harness sets and production never does.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 14: `mise run test` defaults to local PG; `mise run e2e` gains `--backend`; `ci` chains e2e

**Depends on:** Tasks 9, 13.

**Files:**
- Modify: `.mise/tasks/test`
- Modify: `.mise/tasks/e2e`
- Modify: `.mise/tasks/ci`
- Modify: `internal/infra/postgres/store_test.go` (replace silent skip
  with loud failure)
- Modify: `.github/workflows/ci.yaml` (matrix the e2e job per backend)
- Modify: `tests/e2e/harness/env.go` (already done in Task 13 — verify
  `WithBackend` is wired)

**Step 1: Update `.mise/tasks/test` to default `FLOW_DB`**

Replace the body of `.mise/tasks/test`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run tests with coverage (PG path mandatory; falls back loudly)"
set -euo pipefail

BUILD_DIR=build
mkdir -p "$BUILD_DIR"

# Default the PG DSN to the local host's Postgres 18 (peer-trust as the
# postgres user, per WorkFort dev convention). CI overrides FLOW_DB
# before invocation. Per feedback_no_test_failures.md, silent skip on
# missing PG is forbidden — if PG is unreachable, the test runner must
# fail loudly with a clear "start PG" message instead of pretending to
# pass.
if [[ -z "${FLOW_DB:-}" ]]; then
  export FLOW_DB="postgres://postgres@127.0.0.1/flow_test?sslmode=disable"
fi

# Probe PG reachability before launching the suite so the failure
# message is "PG not reachable" rather than a wall of skipped tests.
if ! psql "$FLOW_DB" -c 'SELECT 1' >/dev/null 2>&1; then
  echo "ERROR: FLOW_DB ($FLOW_DB) not reachable." >&2
  echo "Start local Postgres or set FLOW_DB to a reachable DSN." >&2
  echo "Per feedback_no_test_failures.md: silent skips on missing PG are forbidden." >&2
  exit 1
fi

go test -v -race -coverprofile="$BUILD_DIR/coverage.out" ./...
```

**Step 2: Replace silent skip in postgres unit-test helper**

In `internal/infra/postgres/store_test.go`, replace the existing
`dsn` helper:

```go
func dsn(t *testing.T) string {
	t.Helper()
	v := os.Getenv("FLOW_DB")
	if v == "" {
		t.Fatal("FLOW_DB not set — the mise test runner sets a default; if you're running `go test` directly, set FLOW_DB=postgres://postgres@127.0.0.1/flow_test?sslmode=disable")
	}
	return v
}
```

(`t.Fatal` instead of `t.Skip` — loud failure instead of silent
skip.)

**Step 3: Update `.mise/tasks/e2e` to accept `--backend`**

Replace the body of `.mise/tasks/e2e`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run end-to-end tests against a real flow daemon (rebuilds via build:dev)"
#MISE depends=["build:dev"]
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
export FLOW_BINARY="$REPO_ROOT/build/flow"

if [[ ! -x "$FLOW_BINARY" ]]; then
  echo "FLOW_BINARY not found at $FLOW_BINARY — run 'mise run build:dev' first" >&2
  exit 1
fi

# --backend sqlite|postgres selects which storage layer the spawned
# daemon uses. SQLite is the default; CI runs both as parallel jobs.
BACKEND=sqlite
while [[ $# -gt 0 ]]; do
  case "$1" in
    --backend)
      BACKEND="$2"
      shift 2
      ;;
    --backend=*)
      BACKEND="${1#*=}"
      shift
      ;;
    *)
      break
      ;;
  esac
done

case "$BACKEND" in
  sqlite)
    unset FLOW_E2E_PG_DSN
    ;;
  postgres)
    if [[ -z "${FLOW_E2E_PG_DSN:-}" ]]; then
      export FLOW_E2E_PG_DSN="postgres://postgres@127.0.0.1/flow_test?sslmode=disable"
    fi
    if ! psql "$FLOW_E2E_PG_DSN" -c 'SELECT 1' >/dev/null 2>&1; then
      echo "ERROR: FLOW_E2E_PG_DSN ($FLOW_E2E_PG_DSN) not reachable for postgres e2e backend." >&2
      exit 1
    fi
    ;;
  *)
    echo "unknown --backend: $BACKEND (want sqlite or postgres)" >&2
    exit 2
    ;;
esac

cd "$REPO_ROOT/tests/e2e"
exec go test -race -count=1 -backend "$BACKEND" "$@" ./...
```

The `-backend` flag isn't a stdlib `go test` flag, so wire it
through a `tests/e2e/main_test.go` `flag.String` that sets a package
variable read by `harness.NewEnv` defaults. Concretely, add
`tests/e2e/main_test.go`:

```go
// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"flag"
	"os"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

var backendFlag = flag.String("backend", "sqlite", "storage backend: sqlite | postgres")

func TestMain(m *testing.M) {
	flag.Parse()
	harness.SetDefaultBackend(*backendFlag)
	os.Exit(m.Run())
}
```

And in `tests/e2e/harness/env.go`, add a package-level default:

```go
var defaultBackend = "sqlite"

// SetDefaultBackend overrides the backend used when NewEnv is called
// without WithBackend. Wired from main_test.go's -backend flag.
func SetDefaultBackend(b string) { defaultBackend = b }
```

…and update `NewEnv` to use it:

```go
	cfg := &envCfg{backend: defaultBackend}
```

**Step 4: Update `.mise/tasks/ci` to chain `e2e`**

Replace the body of `.mise/tasks/ci`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run all CI checks (single-backend; CI workflow runs the matrix)"
#MISE depends=["lint", "test", "e2e"]
set -euo pipefail
```

The CI workflow handles dual-backend matrixing; `mise run ci`
locally is the developer-side single-backend safety net (default
SQLite for e2e, plus PG-on-127.0.0.1 for unit tests).

**Step 5: Update `.github/workflows/ci.yaml` to matrix e2e per
backend**

Replace `.github/workflows/ci.yaml`:

```yaml
# SPDX-License-Identifier: GPL-2.0-only
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  lint-and-unit:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:18
        env:
          POSTGRES_DB: flow_test
          POSTGRES_USER: postgres
          POSTGRES_HOST_AUTH_METHOD: trust
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: mise run lint
      - run: mise run test
        env:
          FLOW_DB: postgres://postgres@127.0.0.1:5432/flow_test?sslmode=disable

  e2e:
    needs: lint-and-unit
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        backend: [sqlite, postgres]
    services:
      postgres:
        image: postgres:18
        env:
          POSTGRES_DB: flow_test
          POSTGRES_USER: postgres
          POSTGRES_HOST_AUTH_METHOD: trust
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: mise run e2e --backend ${{ matrix.backend }}
        env:
          FLOW_E2E_PG_DSN: postgres://postgres@127.0.0.1:5432/flow_test?sslmode=disable
```

**Step 6: Verify locally**

Run: `mise run test`
Expected: PASS — local PG is reachable, every PG-tagged test runs.

Run: `mise run e2e --backend sqlite`
Expected: PASS — SQLite-backed daemon, all four e2e tests green.

Run: `mise run e2e --backend postgres`
Expected: PASS — Postgres-backed daemon, all four e2e tests green.

Run: `mise run ci`
Expected: PASS — chains lint + test + e2e (sqlite default).

**Step 7: Commit**

```
git add .mise/tasks/test .mise/tasks/e2e .mise/tasks/ci .github/workflows/ci.yaml internal/infra/postgres/store_test.go tests/e2e/main_test.go tests/e2e/harness/env.go
git commit -m "$(cat <<'EOF'
chore(mise): default test runner to local PG; chain e2e in ci

mise run test now sets FLOW_DB to the host's local PG by default and
fails loudly when PG isn't reachable, replacing the silent t.Skip
that hid every PG-only test from the safety net. mise run e2e gains
a --backend flag (sqlite | postgres) so the same task drives both
backends; mise run ci chains lint + test + e2e at the default
backend. The CI workflow matrixes e2e per backend as parallel jobs
— per feedback_e2e_dual_backend.md, the dual-backend chaining lives
in CI orchestration, not in the local ci task.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 15: Full green run

**Step 1: Local matrix**

Run: `mise run lint`
Expected: exits 0.

Run: `mise run test`
Expected: PASS — every unit and integration test, including PG-side.

Run: `mise run e2e --backend sqlite`
Expected: PASS.

Run: `mise run e2e --backend postgres`
Expected: PASS.

Run: `mise run ci`
Expected: PASS — chains lint + test + e2e.

Run: `mise run build:dev` and `mise run build:release`
Expected: both build cleanly.

**Step 2: No commit**

This task is verification only. If anything fails, the failure must
be fixed in the task that introduced it (per
`feedback_no_test_failures.md` — never file as "pre-existing").

---

## Verification checklist

Tick each before handing to TPM.

- [ ] `mise run lint` passes.
- [ ] `mise run test` passes — PG reachability is enforced; no
      silent skips. The runner exits non-zero with the "start PG"
      message when local PG isn't reachable.
- [ ] `mise run e2e --backend sqlite` passes — all four new e2e
      tests plus the existing health/auth tests.
- [ ] `mise run e2e --backend postgres` passes — same suite,
      Postgres-backed daemon.
- [ ] `mise run ci` passes — chains `lint + test + e2e` (default
      SQLite backend).
- [ ] `mise run build:dev` produces `build/flow`.
- [ ] `flow daemon` starts without panic when Hive is reachable via
      Pylon; log output shows the lease renewer started.
- [ ] `flow daemon` starts without panic when Hive is NOT reachable;
      log output shows "hive not found in Pylon", scheduler is nil,
      renewer is not started.
- [ ] `*hive.Adapter` satisfies both `domain.IdentityProvider` and
      `domain.HiveAgentClient` (compile-time assertion at the bottom
      of `internal/infra/hive/adapter.go`).
- [ ] `*stub.Driver` satisfies `domain.RuntimeDriver` (compile-time
      assertion in `stub/driver_test.go`).
- [ ] Every `AcquireAgent` writes one `agent_claimed` event; every
      `ReleaseAgent` writes one `agent_released` event; every
      successful renewer tick writes one `lease_renewed` event per
      active claim. Verified at unit level (Task 10) and end-to-end
      via `audit_events_test.go` (Task 13).
- [ ] LeaseRenewer test asserts an exact renew count using a hand-
      driven tick channel — no `time.Sleep`-based timing assertions
      that would flake under `-race`.
- [ ] `lease_expired_by_sweeper` is declared in code and accepted by
      both DB CHECK constraints; no Step 1 code path produces it.
- [ ] `tests/e2e/go.mod` does NOT import
      `github.com/Work-Fort/Hive/client` or
      `github.com/Work-Fort/Flow/internal/...` (per
      `feedback_e2e_harness_independence.md`).
- [ ] CI workflow runs e2e as a 2-job matrix (sqlite + postgres) in
      parallel — single-backend `mise run ci` only chains one of
      them locally.

## Completion criteria

- `Scheduler.AcquireAgent` + `Scheduler.ReleaseAgent` work against
  a real Hive daemon when one is reachable through Pylon, and fail
  gracefully (no panics, clear log) when Hive is not reachable.
- Claims survive Hive restarts as long as the Hive sweeper is
  enabled — the renewer re-establishes liveness on the next tick,
  and mismatched claims are dropped cleanly.
- Audit events for the four recorded types are queryable by
  `workflow_id` and by `agent_id`, ordered oldest-first, on both
  SQLite and Postgres.
- `RuntimeDriver` interface is stable enough that the next two
  plans (Nexus driver, k8s driver) each translate, not redesign.
  Each method's k8s mapping is documented inline; if either future
  plan forces a port change, that's a red flag the interface shape
  was wrong here.
- `mise run test` and `mise run ci` exercise PG and e2e by default
  on every developer run. CI matrixes e2e per backend.

Later plans take this foundation forward:

- **Step 2 — Project source master + work-item drives (Nexus):**
  implements `RuntimeDriver` against `nexusctl`. Removes the e2e
  harness's `FLOW_E2E_RUNTIME_STUB` env-var hook in favour of a
  build-tagged injection.
- **Step 3 — claude-cli VM lifecycle:** wires the scheduler into
  workflow transitions so acquire/release triggers
  `StartAgentRuntime` / `StopAgentRuntime`.
- **Step 4 — Per-project bot processes + bot vocabulary parser:**
  adds the Sharkfin side.
- **Step 5 — Combine integration + merge webhook →
  RefreshProjectMaster:** closes the loop.
- **Step 6 — k8s RuntimeDriver:** second concrete implementation;
  validates Step 1's interface shape against real k8s primitives.
- **Sweeper-side audit producer:** wires either a Hive webhook or a
  Flow-startup reconciliation pass that writes
  `lease_expired_by_sweeper` events. The enum value is already
  reserved.
