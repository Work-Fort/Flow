# Flow Phase 3 — Hive Identity Adapter

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Hive identity into Flow's role permission checks. Add an `IdentityProvider` port to the domain layer, a Hive-backed adapter in the infra layer, and enforce `RequiredRoleID` on transitions and `ApproverRoleID` on gate steps. Identity is optional — nil provider preserves current open-access behaviour.

**Prerequisite:** Phase 1 (001-core-engine.md) complete. Phase 2 tests (002-workflow-tests.md) passing.

**Hive client package:** `github.com/Work-Fort/Hive/client` at `/home/kazw/Work/WorkFort/hive/lead/client/`. Read this before implementing — do not guess at method signatures.

**Key facts about the Hive client:**
- `GetAgent(ctx, id) -> (*AgentWithRoles, error)` — returns agent plus its `[]AgentRole` (each has `RoleID`).
- `GetRole(ctx, id) -> (*Role, error)` — returns role record (ID, Name, ParentID).
- `ListAgents(ctx, teamID) -> ([]Agent, error)` — all agents for a team.
- No `GetTeamMembers` method exists — use `ListAgents(ctx, teamID)`.
- `AgentWithRoles.Roles` is `[]AgentRole`, not `[]Role`. Each `AgentRole` has `.RoleID`, not a full `Role`.

**Architecture rule:** Domain types must not import Hive client types. The adapter maps Hive client types to Flow domain identity types defined in `internal/domain/`.

---

## Chunk 1: Domain Identity Types and Port

### Task 1: Identity types in `internal/domain/types.go`

**Files:** `internal/domain/types.go`

Define lightweight domain-side identity types. These exist solely so the domain layer has concrete types to return — they carry only what Flow needs (IDs, names, role list).

- [ ] **Step 1: Append to `internal/domain/types.go`**

```go
// IdentityAgent is Flow's view of a Hive agent — the fields Flow needs.
type IdentityAgent struct {
	ID     string
	Name   string
	TeamID string
	Roles  []IdentityRole
}

// IdentityRole is Flow's view of a Hive role.
type IdentityRole struct {
	ID       string
	Name     string
	ParentID string
}
```

- [ ] **Step 2: Verify** — `go build ./internal/domain/...` exits 0.

---

### Task 2: `IdentityProvider` port in `internal/domain/ports.go`

**Files:** `internal/domain/ports.go`

Add the port interface. The domain layer references only its own types.

- [ ] **Step 1: Add `IdentityProvider` to `internal/domain/ports.go`**

```go
// IdentityProvider resolves agents and roles from an external identity service.
// It is an optional dependency — if nil, role checks are skipped.
type IdentityProvider interface {
	// ResolveAgent returns the agent with their current role assignments.
	ResolveAgent(ctx context.Context, agentID string) (*IdentityAgent, error)

	// ResolveRole returns the role record for the given role ID.
	ResolveRole(ctx context.Context, roleID string) (*IdentityRole, error)

	// GetTeamMembers returns all agents belonging to the given team.
	GetTeamMembers(ctx context.Context, teamID string) ([]IdentityAgent, error)

	// GetAgentRoles returns the roles assigned to the given agent.
	GetAgentRoles(ctx context.Context, agentID string) ([]IdentityRole, error)
}
```

Note: `context` is already imported in `ports.go`.

- [ ] **Step 2: Verify** — `go build ./internal/domain/...` exits 0.

---

## Chunk 2: Hive Adapter

### Task 3: Hive adapter in `internal/infra/hive/`

**Files:** `internal/infra/hive/adapter.go`

The adapter wraps the Hive client and maps client types to domain types. It must not be imported from the domain layer.

- [ ] **Step 1: Create `internal/infra/hive/adapter.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package hive provides a Flow IdentityProvider backed by the Hive identity service.
package hive

import (
	"context"
	"errors"
	"fmt"

	hiveclient "github.com/Work-Fort/Hive/client"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.IdentityProvider using the Hive REST API.
type Adapter struct {
	client *hiveclient.Client
}

// New creates a new Adapter. baseURL is the Hive daemon URL (e.g.,
// "http://127.0.0.1:17000"). token is a Passport JWT or API key.
func New(baseURL, token string) *Adapter {
	return &Adapter{client: hiveclient.New(baseURL, token)}
}

// ResolveAgent fetches the agent and their role assignments from Hive, and
// returns a Flow domain IdentityAgent. Returns domain.ErrNotFound if Hive
// returns 404.
func (a *Adapter) ResolveAgent(ctx context.Context, agentID string) (*domain.IdentityAgent, error) {
	awr, err := a.client.GetAgent(ctx, agentID)
	if err != nil {
		return nil, mapHiveError(err, agentID, "agent")
	}
	roles := make([]domain.IdentityRole, 0, len(awr.Roles))
	for _, ar := range awr.Roles {
		roles = append(roles, domain.IdentityRole{ID: ar.RoleID})
	}
	return &domain.IdentityAgent{
		ID:     awr.ID,
		Name:   awr.Name,
		TeamID: awr.TeamID,
		Roles:  roles,
	}, nil
}

// ResolveRole fetches a single role by ID. Returns domain.ErrNotFound if Hive
// returns 404.
func (a *Adapter) ResolveRole(ctx context.Context, roleID string) (*domain.IdentityRole, error) {
	r, err := a.client.GetRole(ctx, roleID)
	if err != nil {
		return nil, mapHiveError(err, roleID, "role")
	}
	return &domain.IdentityRole{
		ID:       r.ID,
		Name:     r.Name,
		ParentID: r.ParentID,
	}, nil
}

// GetTeamMembers returns all agents in the given team. Each agent has no roles
// populated — use ResolveAgent for role-aware lookups.
func (a *Adapter) GetTeamMembers(ctx context.Context, teamID string) ([]domain.IdentityAgent, error) {
	agents, err := a.client.ListAgents(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("hive list agents for team %s: %w", teamID, err)
	}
	out := make([]domain.IdentityAgent, 0, len(agents))
	for _, ag := range agents {
		out = append(out, domain.IdentityAgent{
			ID:     ag.ID,
			Name:   ag.Name,
			TeamID: ag.TeamID,
		})
	}
	return out, nil
}

// GetAgentRoles returns the roles assigned to the agent. Unlike ResolveAgent,
// role Name and ParentID are not populated — only IDs are returned, which is
// sufficient for permission checks.
func (a *Adapter) GetAgentRoles(ctx context.Context, agentID string) ([]domain.IdentityRole, error) {
	awr, err := a.client.GetAgent(ctx, agentID)
	if err != nil {
		return nil, mapHiveError(err, agentID, "agent")
	}
	roles := make([]domain.IdentityRole, 0, len(awr.Roles))
	for _, ar := range awr.Roles {
		roles = append(roles, domain.IdentityRole{ID: ar.RoleID})
	}
	return roles, nil
}

// mapHiveError converts a Hive client error to a domain error where the
// mapping is well-defined (404 → ErrNotFound). Other errors pass through
// wrapped with context.
//
// The Hive client uses sentinel errors via Unwrap — errors.Is is correct here.
// Do not inspect APIError.StatusCode directly.
func mapHiveError(err error, id, kind string) error {
	if errors.Is(err, hiveclient.ErrNotFound) {
		return fmt.Errorf("%s %s: %w", kind, id, domain.ErrNotFound)
	}
	return fmt.Errorf("hive %s %s: %w", kind, id, err)
}
```

- [ ] **Step 2: Verify** — `go build ./internal/infra/hive/...` exits 0. Fix any import path issues (the Hive client module path must match `go.mod`).

  To find the exact module path: `head -1 /home/kazw/Work/WorkFort/hive/lead/go.mod` and check whether it is already in Flow's `go.mod` as a `replace` directive.

- [ ] **Step 3: Commit** — `feat(infra): add Hive identity adapter`

---

## Chunk 3: Wire Identity into WorkflowService

### Task 4: Add `IdentityProvider` to `Service`

**Files:** `internal/workflow/service.go`

The service takes an optional identity provider. When nil, all role checks are skipped. This preserves current behaviour for standalone deployments.

- [ ] **Step 1: Update `Service` struct and constructor**

Replace:
```go
type Service struct {
	store domain.Store
}

func New(store domain.Store) *Service {
	return &Service{store: store}
}
```

With:
```go
type Service struct {
	store    domain.Store
	identity domain.IdentityProvider
}

// New creates a new Service. identity may be nil — if so, role checks are
// skipped (backwards-compatible, open-access behaviour).
func New(store domain.Store, identity domain.IdentityProvider) *Service {
	return &Service{store: store, identity: identity}
}
```

- [ ] **Step 2: Fix all call sites** — `server.go` calls `workflow.New(cfg.Store)`. Update to `workflow.New(cfg.Store, nil)` for now (will be fixed in Chunk 4).

- [ ] **Step 3: Verify** — `go build ./...` exits 0.

---

### Task 5: Enforce `RequiredRoleID` in `TransitionItem`

**Files:** `internal/workflow/service.go`

Insert the role check after the transition is found and the from-step is validated, but before the guard is evaluated.

- [ ] **Step 1: Add role check block to `TransitionItem`**

Insert immediately after the gate check block (after line 101 in the current `service.go`, the `return nil, domain.ErrGateRequiresApproval` line) and before the `s.store.ListApprovals` call that builds the guard context. The ordering is: find transition → validate from-step → gate check → **role check** → list approvals → evaluate guard → update.

```go
// Role check: if the transition requires a role and identity is configured,
// verify the actor holds that role.
if tr.RequiredRoleID != "" && s.identity != nil {
	if err := s.checkActorHasRole(ctx, req.ActorAgentID, tr.RequiredRoleID); err != nil {
		return nil, err
	}
}
```

- [ ] **Step 2: Add the `checkActorHasRole` helper to `service.go`**

```go
// checkActorHasRole returns nil if agentID has roleID, domain.ErrPermissionDenied
// if they do not, or a wrapped error if Hive is unreachable.
func (s *Service) checkActorHasRole(ctx context.Context, agentID, roleID string) error {
	roles, err := s.identity.GetAgentRoles(ctx, agentID)
	if err != nil {
		return fmt.Errorf("resolve actor roles: %w", err)
	}
	for _, r := range roles {
		if r.ID == roleID {
			return nil
		}
	}
	return domain.ErrPermissionDenied
}
```

- [ ] **Step 3: Verify** — `go build ./...` exits 0.

---

### Task 6: Enforce `ApproverRoleID` in `ApproveItem` and `RejectItem`

**Files:** `internal/workflow/service.go`

Gate steps have an `ApproverRoleID` in `ApprovalConfig`. Check it before recording the decision.

- [ ] **Step 1: Add approver role check to `ApproveItem`**

After the `ErrNotAtGateStep` check (before `RecordApproval`), add:

```go
// Approver role check.
if currentStep.Approval != nil && currentStep.Approval.ApproverRoleID != "" && s.identity != nil {
	if err := s.checkActorHasRole(ctx, req.AgentID, currentStep.Approval.ApproverRoleID); err != nil {
		return nil, err
	}
}
```

- [ ] **Step 2: Add identical approver role check to `RejectItem`**

Same position in `RejectItem` — after the `ErrNotAtGateStep` check, before `RecordApproval`. Same code block, substituting `req.AgentID`.

- [ ] **Step 3: Verify** — `go build ./...` exits 0.

- [ ] **Step 4: Commit** — `feat(workflow): enforce RequiredRoleID and ApproverRoleID via IdentityProvider`

---

## Chunk 4: Config and Server Wiring

### Task 7: Add Hive config to viper and `config.go`

**Files:** `internal/config/config.go`

- [ ] **Step 1: Add defaults to `InitViper`**

```go
viper.SetDefault("hive-url", "")
viper.SetDefault("hive-token", "")
```

- [ ] **Step 2: Verify** — `go build ./internal/config/...` exits 0.

---

### Task 8: Thread Hive URL/token through `ServerConfig` and daemon command

**Files:** `internal/daemon/server.go`, `cmd/daemon/daemon.go`

- [ ] **Step 1: Add `HiveURL` and `HiveToken` fields to `ServerConfig`**

```go
type ServerConfig struct {
	Bind        string
	Port        int
	PassportURL string
	HiveURL     string
	HiveToken   string
	Health      *HealthService
	Store       domain.Store
}
```

- [ ] **Step 2: Construct the identity adapter in `NewServer`**

Replace `svc := workflow.New(cfg.Store)` with:

```go
var identityProvider domain.IdentityProvider
if cfg.HiveURL != "" {
	identityProvider = hiveinfra.New(cfg.HiveURL, cfg.HiveToken)
}
svc := workflow.New(cfg.Store, identityProvider)
```

Add the import: `hiveinfra "github.com/Work-Fort/Flow/internal/infra/hive"`.

- [ ] **Step 3: Add `--hive-url` and `--hive-token` flags to `cmd/daemon/daemon.go`**

Follow the existing flag pattern exactly:

```go
var hiveURL string
var hiveToken string
```

In `NewCmd`:
```go
cmd.Flags().StringVar(&hiveURL, "hive-url", "", "Hive identity service URL")
cmd.Flags().StringVar(&hiveToken, "hive-token", "", "Hive API token")
```

In `RunE`, add viper fallbacks:
```go
if !cmd.Flags().Changed("hive-url") {
	hiveURL = viper.GetString("hive-url")
}
if !cmd.Flags().Changed("hive-token") {
	hiveToken = viper.GetString("hive-token")
}
return run(bind, port, db, passportURL, hiveURL, hiveToken)
```

Update `run` signature:
```go
func run(bind string, port int, db, passportURL, hiveURL, hiveToken string) error {
```

Pass into `ServerConfig`:
```go
srv := flowDaemon.NewServer(flowDaemon.ServerConfig{
	Bind:        bind,
	Port:        port,
	PassportURL: passportURL,
	HiveURL:     hiveURL,
	HiveToken:   hiveToken,
	Health:      health,
	Store:       store,
})
```

- [ ] **Step 4: Verify** — `go build ./...` exits 0.

- [ ] **Step 5: Commit** — `feat(config): add hive-url and hive-token daemon flags`

---

## Chunk 5: Unit Tests

### Task 9: Mock `IdentityProvider` and service role-check tests

**Files:** `internal/workflow/service_identity_test.go`

Tests use an in-process stub — no real Hive client, no HTTP. The stub is defined inline in the test file (no shared mock package).

- [ ] **Step 1: Create `internal/workflow/service_identity_test.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// stubIdentity is a minimal IdentityProvider for testing.
type stubIdentity struct {
	// roles maps agentID -> []roleID
	roles map[string][]string
	// err is returned from all calls if non-nil (simulates Hive being down).
	err error
}

func (s *stubIdentity) ResolveAgent(_ context.Context, agentID string) (*domain.IdentityAgent, error) {
	if s.err != nil {
		return nil, s.err
	}
	roleIDs := s.roles[agentID]
	roles := make([]domain.IdentityRole, 0, len(roleIDs))
	for _, id := range roleIDs {
		roles = append(roles, domain.IdentityRole{ID: id})
	}
	return &domain.IdentityAgent{ID: agentID, Roles: roles}, nil
}

func (s *stubIdentity) ResolveRole(_ context.Context, roleID string) (*domain.IdentityRole, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.IdentityRole{ID: roleID}, nil
}

func (s *stubIdentity) GetTeamMembers(_ context.Context, _ string) ([]domain.IdentityAgent, error) {
	return nil, s.err
}

func (s *stubIdentity) GetAgentRoles(_ context.Context, agentID string) ([]domain.IdentityRole, error) {
	if s.err != nil {
		return nil, s.err
	}
	roleIDs := s.roles[agentID]
	roles := make([]domain.IdentityRole, 0, len(roleIDs))
	for _, id := range roleIDs {
		roles = append(roles, domain.IdentityRole{ID: id})
	}
	return roles, nil
}
```

- [ ] **Step 2: Add test for nil identity provider — skips role check**

Test that when the identity provider is nil, a transition with a `RequiredRoleID` still succeeds (current open-access behaviour preserved).

```go
func TestTransitionItem_NilIdentity_SkipsRoleCheck(t *testing.T) {
	store := newTestStore(t)
	svc := workflow.New(store, nil) // nil identity: no checks

	tmpl := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(tmpl))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(tmpl),
		ActorAgentID: "agent-no-roles",
	})
	if err != nil {
		t.Fatalf("expected nil err with nil identity, got %v", err)
	}
}
```

- [ ] **Step 3: Add test for matching role — allows transition**

```go
func TestTransitionItem_MatchingRole_Allowed(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-reviewer": {"role-reviewer"},
	}}
	svc := workflow.New(store, identity)

	tmpl := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(tmpl))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(tmpl),
		ActorAgentID: "agent-reviewer",
	})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}
```

- [ ] **Step 4: Add test for missing role — returns `ErrPermissionDenied`**

```go
func TestTransitionItem_MissingRole_Denied(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-developer": {"role-developer"}, // has developer, not reviewer
	}}
	svc := workflow.New(store, identity)

	tmpl := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(tmpl))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(tmpl),
		ActorAgentID: "agent-developer",
	})
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}
```

- [ ] **Step 5: Add test for approver role check at gate step**

```go
func TestApproveItem_WrongRole_Denied(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-developer": {"role-developer"},
	}}
	svc := workflow.New(store, identity)

	tmpl := buildGateTemplateWithApproverRole(t, store, "role-pm")
	inst := buildInstance(t, store, tmpl.ID)
	// Place item at the gate step.
	item := buildWorkItem(t, store, inst.ID, gateStepID(tmpl))

	_, err := svc.ApproveItem(context.Background(), workflow.ApproveRequest{
		WorkItemID: item.ID,
		AgentID:    "agent-developer",
	})
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}
```

- [ ] **Step 6: Add test for Hive unreachable — propagates wrapped error (not permission denied)**

```go
func TestTransitionItem_HiveDown_PropagatesError(t *testing.T) {
	store := newTestStore(t)
	hiveErr := errors.New("connection refused")
	identity := &stubIdentity{err: hiveErr}
	svc := workflow.New(store, identity)

	tmpl := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(tmpl))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(tmpl),
		ActorAgentID: "agent-x",
	})
	if err == nil {
		t.Fatal("expected error when Hive is down")
	}
	if errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatal("Hive errors must not masquerade as ErrPermissionDenied")
	}
}
```

- [ ] **Step 7: Implement test helpers**

The tests above reference `buildTemplateWithRoleRequirement`, `buildGateTemplateWithApproverRole`, `buildInstance`, `buildWorkItem`, `firstStepID`, `firstTransitionID`, `gateStepID`, and `newTestStore`. These helpers are in-process and must:

  - `newTestStore` — use the in-memory SQLite store (`:memory:`) via `infra.Open`. Follow the pattern in existing `workflow_test.go` if present, otherwise use `infra.Open(":memory:")`.
  - `buildTemplateWithRoleRequirement(t, store, roleID)` — creates and stores a template with at least one task step, one transition from that step with `RequiredRoleID = roleID`, and returns the template.
  - `buildGateTemplateWithApproverRole(t, store, roleID)` — template with a gate step whose `ApprovalConfig.ApproverRoleID = roleID`.
  - `buildInstance`, `buildWorkItem` — minimal records (no integration configs needed).
  - `firstStepID`, `firstTransitionID`, `gateStepID` — extract IDs from the template.

  Check `internal/workflow/` for existing test helpers before adding new ones; reuse if already present.

- [ ] **Step 8: Run tests** — `mise run test` passes. Address any compilation errors before treating failures as logic bugs.

- [ ] **Step 9: Commit** — `test(workflow): add identity role check tests`

---

## Verification Checklist

Before marking this plan complete, confirm:

- [ ] `go build ./...` exits 0 from repo root.
- [ ] `go vet ./...` exits 0.
- [ ] `mise run test` passes (all packages, no skipped identity tests).
- [ ] `flow daemon --help` shows `--hive-url` and `--hive-token` flags.
- [ ] Starting `flow daemon` without `--hive-url` runs without error (nil identity, open-access mode).
- [ ] Starting `flow daemon --hive-url http://127.0.0.1:17000 --hive-token <tok>` starts without error (even if Hive is unreachable — role checks fire only on transition/approve requests, not at startup).

---

## Non-Goals (Phase 4+)

- `SyncRoles` endpoint — role-change propagation / cache invalidation.
- CEL guard access to resolved role names (currently guards use the caller-supplied `actor.role_id` string from the request, not the Hive-verified one). Phase 4 could replace this with the verified role from the identity check.
- Role hierarchy traversal — `ApproverRoleID` checks an exact role ID match. Inherited roles (via `ParentID`) are not traversed.
- Hive health check in daemon health endpoint.
