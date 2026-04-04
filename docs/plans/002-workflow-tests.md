# Flow Phase 2 — Workflow Logic Test Suite

> **For agentic workers:** Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a comprehensive integration-style test suite for the Flow workflow engine that acts as a refactoring safety net. Tests cover transition logic, guard evaluation, approval flows, auto-advance, state validation, and lifecycle constraints.

**Test approach:** Integration-style tests spin up a real in-memory SQLite store (`sqlite.Open("")`), seed purpose-built templates, and exercise handler logic directly — not through HTTP. Each test creates its own isolated store instance. No mocking.

**Test location:** `internal/workflow/workflow_test.go` — new package that imports the store and handler logic. Handler functions (`registerTransitionRoutes`, `registerApprovalRoutes`, etc.) are not directly callable as plain functions since they register with `huma.API`. Extract the core logic into a thin `WorkflowService` type (or test via a real Huma test server) — see Task 1 for the decision.

**Reference:** `internal/daemon/rest_huma.go` — the transition handler (lines 459–532) and approval handlers (lines 544–731) are the code under test. Read them carefully before writing tests.

---

## Chunk 1: Test Infrastructure

### Task 1: Choose and build the test harness

**Files:** `internal/workflow/service.go`, `internal/workflow/workflow_test.go`

The transition and approval logic currently lives inline in Huma handler closures. Two options:

**Option A — Extract to a service layer.** Move the workflow logic (guard evaluation, step lookup, history recording, auto-advance) into a `WorkflowService` struct with plain methods. Handlers become thin wrappers. Tests call the service directly.

**Option B — Huma test server.** Spin up a real `huma.NewTestAPI` with a real store. Tests make HTTP calls via `humatest`. Logic stays in handlers.

**Recommendation:** Option A. Extracting the logic makes it independently testable, improves clarity, and is the right design regardless. The test plan drives the refactor.

- [ ] **Step 1: Create `internal/workflow/service.go`** — extract transition and approval logic from `rest_huma.go` into a `Service` struct:

```go
type Service struct {
    store domain.Store
}

func New(store domain.Store) *Service

func (s *Service) TransitionItem(ctx context.Context, req TransitionRequest) (*domain.WorkItem, error)
func (s *Service) ApproveItem(ctx context.Context, req ApproveRequest) (*domain.WorkItem, error)
func (s *Service) RejectItem(ctx context.Context, req RejectRequest) (*domain.WorkItem, error)
```

Where:
```go
type TransitionRequest struct {
    WorkItemID   string
    TransitionID string
    ActorAgentID string
    ActorRoleID  string
    Reason       string
}

type ApproveRequest struct {
    WorkItemID string
    AgentID    string
    Comment    string
}

type RejectRequest struct {
    WorkItemID string
    AgentID    string
    Comment    string
}
```

- [ ] **Step 2: Update `rest_huma.go` and `mcp_tools.go`** — both files contain the transition and approval logic and must both be updated:
  - `rest_huma.go`: handlers call `service.TransitionItem`, `service.ApproveItem`, `service.RejectItem`. Map returned errors via `mapDomainErr`. Handlers become thin wrappers.
  - `mcp_tools.go`: the `transition_work_item`, `approve_work_item`, and `reject_work_item` tools are a full duplicate of the same logic with the same bugs. They must also delegate to the service. Without this, the MCP path remains untested and carries the known guard-context bugs.
  - Both files share `server.go` wiring — pass the service to both `registerTools` and the Huma registration functions.

- [ ] **Step 3: Verify** — `mise run build` passes.

- [ ] **Step 4: Commit** — `refactor: extract workflow logic into Service for testability`

---

### Task 2: Test fixtures and helpers

**File:** `internal/workflow/fixtures_test.go`

- [ ] **Step 1: Create fixture helpers** — helpers that build and seed common test templates into a store:

```go
// openStore opens a fresh in-memory SQLite store. Fails test on error.
func openStore(t *testing.T) domain.Store

// newID generates a short prefixed ID (copy of daemon.NewID logic).
func newID(prefix string) string

// twoStepTemplate builds and seeds a minimal 2-step task template:
//   Step A (position=0) --[advance]--> Step B (position=1)
// Returns template + IDs.
type twoStepFixture struct {
    Store      domain.Store
    TemplateID string
    StepA      string
    StepB      string
    TransAtoB  string
    InstanceID string
}
func seedTwoStep(t *testing.T) twoStepFixture

// guardedTransitionFixture builds a 2-step template where the A->B transition
// has a CEL guard: item.priority == "high"
type guardedFixture struct {
    twoStepFixture
}
func seedGuarded(t *testing.T) guardedFixture

// gateFixture builds a 3-step template:
//   Step A (task, pos=0) --[submit]--> Gate (gate, pos=1) --[approve]--> Step C (task, pos=2)
//   Gate has RejectionStepID -> Step A
//   Gate has Mode=any, RequiredApprovers=2
type gateFixture struct {
    Store       domain.Store
    TemplateID  string
    StepA       string
    GateStep    string
    StepC       string
    TransAtoGate  string
    TransGateToC  string
    TransGateToA  string   // rejection transition
    InstanceID  string
}
func seedGate(t *testing.T) gateFixture

// cycleFixture builds a 3-step template with a cycle:
//   Step A --[forward]--> Step B --[forward]--> Step C
//   Step B --[back]--> Step A
type cycleFixture struct {
    Store      domain.Store
    TemplateID string
    StepA, StepB, StepC string
    TransAtoB, TransBtoC, TransBtoA string
    InstanceID string
}
func seedCycle(t *testing.T) cycleFixture

// multiTransFixture builds a 3-step template with two transitions from the same step:
//   Step A --[go-to-b]--> Step B
//   Step A --[go-to-c]--> Step C
type multiTransFixture struct {
    Store      domain.Store
    TemplateID string
    StepA, StepB, StepC string
    TransAtoB, TransAtoC string
    InstanceID string
}
func seedMultiTrans(t *testing.T) multiTransFixture

// createItem is a test helper that creates a work item at the given stepID.
func createItem(t *testing.T, store domain.Store, instanceID, stepID string) *domain.WorkItem
```

- [ ] **Step 2: Verify** — helpers compile and return non-nil stores/items.

- [ ] **Step 3: Commit** — `test: add workflow test fixtures and helpers`

---

## Chunk 2: Transition Logic Tests

### Task 3: Valid transition advances item and records history

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestTransition_ValidAdvances** — seed `twoStepFixture`, create item at StepA, call `service.TransitionItem` with `TransAtoB`. Assert:
  - Returned item `CurrentStepID == StepB`
  - `store.GetWorkItem` confirms the updated step
  - `store.GetTransitionHistory` returns 1 entry with correct `FromStepID`, `ToStepID`, `TransitionID`, `TriggeredBy`

- [ ] **Step 2: TestTransition_WrongCurrentStep** — seed `twoStepFixture`, create item at StepA, manually advance item to StepB in the store. Then call `TransitionItem` with `TransAtoB` (which expects FromStep=StepA). Assert error `domain.ErrInvalidTransition`.

- [ ] **Step 3: TestTransition_NonExistentTransitionID** — seed `twoStepFixture`, create item at StepA. Call `TransitionItem` with a random nonexistent transition ID. Assert error `domain.ErrInvalidTransition`.

- [ ] **Step 4: TestTransition_WorkItemNotFound** — call `TransitionItem` with a nonexistent work item ID. Assert error `domain.ErrNotFound`.

- [ ] **Step 5: Commit** — `test: transition valid path and error cases`

---

### Task 4: CEL guard evaluation on transitions

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestTransition_GuardPass** — seed `guardedFixture` (guard: `item.priority == "high"`), create item at StepA with priority=`high`. Call `TransitionItem`. Assert item advances to StepB.

- [ ] **Step 2: TestTransition_GuardFail** — seed `guardedFixture`, create item at StepA with priority=`normal`. Call `TransitionItem`. Assert error `domain.ErrGuardDenied`. Verify `store.GetWorkItem` shows item still at StepA (unchanged).

- [ ] **Step 3: TestTransition_GuardWithActorRole** — create a template with guard `actor.role_id == "developer"`. Call with matching role → succeeds. Call with non-matching role → `ErrGuardDenied`.

- [ ] **Step 4: TestTransition_GuardWithFields** — create a template with guard `item.fields.tests_passing == true`. Create a work item with `Fields = {"tests_passing": true}`. Call `TransitionItem`. Assert advances. Then repeat with `false` → `ErrGuardDenied`.

  _Gap (confirmed): `w.Fields` is a `json.RawMessage`. The original handler never unmarshalled it into `map[string]any` before populating `GuardItem.Fields`, so `item.fields` in CEL was always nil/empty. The service must `json.Unmarshal(w.Fields, &fields)` and assign the result to `guardCtx.Item.Fields` before calling `EvaluateGuard`._

- [ ] **Step 5: TestTransition_GuardWithApprovalCount** — create a template with guard `approval.count >= 1`. Seed an approval for the work item on the current step. Call `TransitionItem` — assert advances. Repeat with no approvals → `ErrGuardDenied`.

  _Gap (confirmed): The original handler left `GuardContext.Approval` zero-valued (`Count: 0`, `Rejections: 0`), so guards on `approval.count` always saw 0 and never passed. The service must call `store.ListApprovals(ctx, w.ID, w.CurrentStepID)` before calling `EvaluateGuard` and populate both `Count` and `Rejections`._

  _Both gaps (Fields unmarshalling and Approval population) must be fixed in the service before these tests can pass. They apply equally to `mcp_tools.go`._

- [ ] **Step 6: Commit** — `test: CEL guard evaluation on transitions`

---

### Task 5: Multiple transitions from the same step

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestTransition_MultipleOutgoing_ChooseB** — seed `multiTransFixture`, create item at StepA. Call `TransitionItem` with `TransAtoB`. Assert item is at StepB.

- [ ] **Step 2: TestTransition_MultipleOutgoing_ChooseC** — same fixture, create item at StepA. Call `TransitionItem` with `TransAtoC`. Assert item is at StepC.

- [ ] **Step 3: TestTransition_MultipleOutgoing_BothRecordedSeparately** — advance two different items from StepA: one to StepB, one to StepC. Verify each item's history shows the correct transition.

- [ ] **Step 4: Commit** — `test: multiple outgoing transitions from same step`

---

### Task 6: Cyclic transitions

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestTransition_CyclicPath_ForwardAndBack** — seed `cycleFixture`, create item at StepA. Advance A→B. Advance B→A (back transition). Verify item is back at StepA. Verify history has 2 entries.

- [ ] **Step 2: TestTransition_CyclicPath_MultipleLoops** — do the A→B→A cycle three times. Verify item returns to StepA after each cycle. Verify history has 6 entries. This confirms the engine doesn't reject repeated use of the same transition.

- [ ] **Step 3: Commit** — `test: cyclic transitions work across multiple loops`

---

## Chunk 3: Approval Logic Tests

### Task 7: Approval recording

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestApprove_AtGateStep_RecordsApproval** — seed `gateFixture`, create item at GateStep. Call `service.ApproveItem`. Assert:
  - Approval is recorded: `store.ListApprovals(workItemID, gateStep)` returns 1 entry with `Decision=approved`
  - Agent ID and comment are stored correctly

- [ ] **Step 2: TestApprove_AtTaskStep_ReturnsError** — seed `twoStepFixture`, create item at StepA (a task step). Call `service.ApproveItem`. Assert error `domain.ErrNotAtGateStep`.

- [ ] **Step 3: TestReject_AtGateStep_RecordsRejection** — seed `gateFixture`, create item at GateStep. Call `service.RejectItem`. Assert:
  - Approval is recorded with `Decision=rejected`
  - Comment is stored

- [ ] **Step 4: TestReject_AtTaskStep_ReturnsError** — seed `twoStepFixture`, create item at StepA. Call `service.RejectItem`. Assert error `domain.ErrNotAtGateStep`.

- [ ] **Step 5: Commit** — `test: approval recording and gate step validation`

---

### Task 8: Auto-advance after approval threshold

**File:** `internal/workflow/workflow_test.go`

The `gateFixture` has `Mode=any, RequiredApprovers=2`.

- [ ] **Step 1: TestApprove_BelowThreshold_NoAdvance** — seed `gateFixture`, create item at GateStep. Call `ApproveItem` once (1 of 2 required). Assert item is still at GateStep.

- [ ] **Step 2: TestApprove_ReachesThreshold_AutoAdvances** — seed `gateFixture`, create item at GateStep. Call `ApproveItem` twice (different agent IDs). Assert item has advanced to StepC. Assert transition history contains an auto-advance entry with reason `"auto-advance on approval threshold"`.

- [ ] **Step 3: TestApprove_ExceedsThreshold_StillAtAdvancedStep** — call `ApproveItem` three times. After the 2nd approval the item auto-advances to StepC (a task step). The 3rd `ApproveItem` call must return `domain.ErrNotAtGateStep` — the item is no longer at a gate step. Assert both: the error is `ErrNotAtGateStep`, AND the item remains at StepC (did not regress).

- [ ] **Step 4: TestApprove_MultipleAgents_EachCountedOnce** — add 2 approvals from `agent-alice` and `agent-bob`. Assert threshold met and item advanced. Verify approvals list has 2 entries, one per agent.

- [ ] **Step 5: Commit** — `test: auto-advance after approval threshold`

---

### Task 9: Rejection routing

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestReject_WithRejectionStep_AdvancesToRejectionStep** — seed `gateFixture` (has RejectionStepID=StepA), create item at GateStep. Call `RejectItem`. Assert:
  - Item `CurrentStepID == StepA`
  - Transition history has an entry from GateStep to StepA with `TriggeredBy = agentID`
  - History reason contains `"rejected:"`

- [ ] **Step 2: TestReject_WithoutRejectionStep_RecordsAndStays** — the existing `gateFixture` always has a `RejectionStepID`, so this test needs its own fixture. Use an inline template or a `seedNoRejectionGate` helper that creates a single gate step with no `RejectionStepID` (leave the field empty). Call `RejectItem`. Assert:
  - Rejection approval is recorded
  - Item stays at the gate step (no transition)
  - No transition history entry added

- [ ] **Step 3: Commit** — `test: rejection routing with and without rejection step`

---

## Chunk 4: State Validation Tests

### Task 10: Work item lifecycle

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestCreateWorkItem_StartsAtFirstStep** — seed `twoStepFixture`. StepA has position=0, StepB has position=1. Call `store.CreateWorkItem` directly (or via handler). Assert item `CurrentStepID == StepA`. Verify the "lowest position" logic handles steps defined in non-position order (add a step at position 5 and one at position 0; item should start at position 0).

- [ ] **Step 2: TestCreateWorkItem_TemplateWithNoSteps** — create a template with zero steps. Attempt to create a work item. Assert the create-work-item handler returns an error (HTTP 422 or `ErrInvalidTransition` equivalent). _Check the REST handler at line 332 — it returns HTTP 422 with "template has no steps"._

- [ ] **Step 3: TestTransition_ItemNotFound** — call `TransitionItem` with a work item ID that doesn't exist. Assert `ErrNotFound`.

- [ ] **Step 4: TestApprove_ItemNotFound** — call `ApproveItem` with a work item ID that doesn't exist. Assert `ErrNotFound`.

- [ ] **Step 5: Commit** — `test: work item lifecycle and not-found error paths`

---

### Task 11: Template and instance lifecycle

**File:** `internal/workflow/workflow_test.go`

- [ ] **Step 1: TestDeleteTemplate_WithActiveInstance_Blocked** — create a template, create an instance from it. Attempt to delete the template. Assert error `domain.ErrHasDependencies`. _Verify the SQLite store enforces this via foreign key constraints or explicit check._

- [ ] **Step 2: TestDeleteTemplate_NoInstances_Succeeds** — create a template, delete it. Assert `store.GetTemplate` returns `ErrNotFound`.

- [ ] **Step 3: TestCreateInstance_SnapshotsTemplateVersion** — create a template at version 1. Call `store.CreateInstance`. Retrieve the instance. Assert `TemplateVersion == 1`. Update the template (bumping version to 2). Create a second instance. Assert second instance has `TemplateVersion == 2` and first instance still has `TemplateVersion == 1`.

- [ ] **Step 4: Commit** — `test: template/instance lifecycle constraints`

---

## Chunk 5: Integration Hook Lookup (Stubbed)

### Task 12: Hook lookup verification

**File:** `internal/workflow/workflow_test.go`

Integration hooks have adapters that are out of scope for Phase 1. Tests verify the template can carry hooks and they are retrievable via the store — the "lookup would fire" contract.

- [ ] **Step 1: TestIntegrationHooks_StoredAndRetrievable** — create a template with one `IntegrationHook` on a transition (`on_transition`, adapter=`chat`, action=`post_message`). Retrieve the template. Assert:
  - `template.IntegrationHooks` has 1 entry
  - `TransitionID`, `AdapterType`, `Action`, `Config` round-trip correctly

- [ ] **Step 2: TestIntegrationHooks_MultipleHooksOnDifferentTransitions** — create a template with hooks on two different transitions. Retrieve the template. Assert both hooks are present and associated with the correct transition IDs.

- [ ] **Step 3: Commit** — `test: integration hook storage and retrieval`

---

## Chunk 6: Scenario Tests (End-to-End Flows)

### Task 13: SDLC-flavored scenario

**File:** `internal/workflow/scenario_test.go`

A single test that exercises a complete lifecycle through an SDLC-like template as a user/agent would experience it.

**Template:** 4 steps — `Backlog` (task, pos=0), `Planning` (task, pos=1), `Review` (gate, mode=any, required=1, rejection→Planning), `Done` (task, pos=3).

**Transitions:** Backlog→Planning (`triage`), Planning→Review (`submit`), Review→Done (approval auto-advance), Review→Planning (rejection routing).

- [ ] **Step 1: TestSDLCScenario_HappyPath** — full forward path:
  1. Create item → assert at Backlog
  2. `TransitionItem` with `triage` → assert at Planning
  3. `TransitionItem` with `submit` → assert at Review
  4. `ApproveItem` (1 approval, threshold=1) → assert auto-advanced to Done
  5. Retrieve history → assert 3 entries (triage, submit, auto-advance)

- [ ] **Step 2: TestSDLCScenario_RejectionAndResubmit** — rejection cycle:
  1. Create item, triage to Planning, submit to Review
  2. `RejectItem` → assert item back at Planning (rejection routing)
  3. `TransitionItem` with `submit` again → assert at Review
  4. `ApproveItem` → assert at Done
  5. Verify history has 5 entries

- [ ] **Step 3: Commit** — `test: SDLC scenario happy path and rejection cycle`

---

## Test Coverage Targets

After all chunks are complete, run `go test -cover ./internal/workflow/...` and verify:
- `internal/workflow/service.go`: > 90% coverage
- All error paths in `TransitionItem`, `ApproveItem`, `RejectItem` are hit

---

## Notes on Guard Context Population

**Two confirmed gaps must be fixed in `service.go` (and correspondingly in `mcp_tools.go`) before Task 4 Steps 4 and 5 can pass:**

**Gap 1 — `Item.Fields` never populated (Task 4, Step 4).** `w.Fields` is a `json.RawMessage`. Neither the original REST handler nor the MCP tool unmarshalled it into `map[string]any` before constructing `GuardItem.Fields`. As a result, `item.fields` in any CEL guard was always nil/empty — guards like `item.fields.tests_passing == true` always failed with a type error rather than evaluating the field value. Fix: in `TransitionItem`, call `json.Unmarshal(w.Fields, &fields)` and assign the result to `guardCtx.Item.Fields` before calling `EvaluateGuard`.

**Gap 2 — `Approval.Count` and `Approval.Rejections` never populated (Task 4, Step 5).** Both the original REST handler (`rest_huma.go` lines 492–505) and the MCP `transition_work_item` tool left `GuardContext.Approval` zero-valued. Guards using `approval.count >= N` always saw `0` and never passed. Fix: in `TransitionItem`, call `store.ListApprovals(ctx, w.ID, w.CurrentStepID)` before `EvaluateGuard` and populate both `Count` (approved decisions) and `Rejections` (rejected decisions).

Both fixes belong in `service.go`. The same bugs exist in `mcp_tools.go` and are eliminated when that file is refactored to delegate to the service (Task 1, Step 2).
