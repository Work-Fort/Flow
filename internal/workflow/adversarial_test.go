// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

// Adversarial tests for 4 critical/high findings:
//   1. Gate bypass via TransitionItem
//   2. Unanimous mode never auto-advances
//   3. Auto-advance wrong branch (transition order determines outcome)
//   4. Invalid RejectionStepID accepted without validation

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// --- Finding 1: Gate Bypass via TransitionItem ---

// TestGateBypass_TransitionItem_Blocked verifies that TransitionItem refuses to
// execute a transition whose from-step is a gate step. Without this check, an
// agent can skip the approval threshold by calling TransitionItem directly.
func TestGateBypass_TransitionItem_Blocked(t *testing.T) {
	f := seedGate(t)
	ctx := context.Background()

	// Place item at the gate step (approval not satisfied).
	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// Attempt to advance via direct transition — must be refused.
	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransGateToC,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrGateRequiresApproval) {
		t.Errorf("want ErrGateRequiresApproval, got %v", err)
	}

	// Item must remain at gate.
	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.GateStep {
		t.Errorf("item moved despite gate bypass attempt: got step %q, want gate %q", persisted.CurrentStepID, f.GateStep)
	}
}

// --- Finding 2: Unanimous Mode Never Auto-Advances ---

// seedUnanimousGate builds a 3-step template with a gate requiring unanimous
// approval from exactly 2 approvers (mode=unanimous, required=2).
//
//	StepA (task) --> GateUnanimous (gate, mode=unanimous, required=2) --> StepC (task)
type unanimousGateFixture struct {
	Store      domain.Store
	Svc        *workflow.Service
	TemplateID string
	StepA      string
	GateStep   string
	StepC      string
	InstanceID string
}

func seedUnanimousGate(t *testing.T) unanimousGateFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	gateStep := tid("stp")
	stepC := tid("stp")
	transAG := tid("tr")
	transGC := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "UnanimousGate", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{
				ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 1,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeUnanimous,
					RequiredApprovers: 2,
					ApproverRoleID:    "reviewer",
				},
			},
			{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transAG, TemplateID: tplID, Key: "a-to-gate", Name: "Submit", FromStepID: stepA, ToStepID: gateStep},
			{ID: transGC, TemplateID: tplID, Key: "gate-to-c", Name: "Approve", FromStepID: gateStep, ToStepID: stepC},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return unanimousGateFixture{
		Store: store, Svc: workflow.New(store),
		TemplateID: tplID, StepA: stepA, GateStep: gateStep, StepC: stepC,
		InstanceID: instID,
	}
}

// TestUnanimousMode_BelowThreshold_NoAdvance verifies that a unanimous gate
// does not advance when only 1 of 2 required approvers has approved.
func TestUnanimousMode_BelowThreshold_NoAdvance(t *testing.T) {
	f := seedUnanimousGate(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// One approval — below threshold.
	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	})
	if err != nil {
		t.Fatalf("ApproveItem: %v", err)
	}
	if updated.CurrentStepID != f.GateStep {
		t.Errorf("1 of 2 approvals on unanimous gate: should still be at gate, got %q", updated.CurrentStepID)
	}
}

// TestUnanimousMode_ThresholdMet_AutoAdvances verifies that a unanimous gate
// auto-advances when all required approvers have approved and none have rejected.
func TestUnanimousMode_ThresholdMet_AutoAdvances(t *testing.T) {
	f := seedUnanimousGate(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Second approval brings unanimous count to threshold — must auto-advance.
	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-bob",
	})
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if updated.CurrentStepID != f.StepC {
		t.Errorf("unanimous gate with 2/2 approvals: want StepC (%q), got %q", f.StepC, updated.CurrentStepID)
	}
}

// TestUnanimousMode_WithRejection_NoAdvance verifies that a unanimous gate does
// not advance when any approver has rejected, even if the approval count is met.
func TestUnanimousMode_WithRejection_NoAdvance(t *testing.T) {
	f := seedUnanimousGate(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// One approval, one rejection — should NOT advance (unanimous requires zero rejections).
	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	updated, err := f.Svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID, AgentID: "agent-bob", Comment: "not ready",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if updated.CurrentStepID != f.GateStep {
		t.Errorf("unanimous gate after rejection: want gate (%q), got %q", f.GateStep, updated.CurrentStepID)
	}
}

// --- Finding 3: Auto-Advance Wrong Branch (transition order sensitivity) ---

// TestAutoAdvance_WrongBranch_RejectionTransitionFirst verifies two cases:
//
//  1. With RejectionStepID set: the engine correctly skips the rejection
//     transition even when it is listed first in the template's transition
//     slice (SQLite returns them in insertion order).
//
//  2. Without RejectionStepID: a gate with two outgoing transitions and no
//     designated rejection path causes auto-advance to silently route to the
//     first transition found — which may be the wrong branch. The engine must
//     either error or have a deterministic rule for this ambiguous case.
func TestAutoAdvance_WrongBranch_RejectionTransitionFirst(t *testing.T) {
	ctx := context.Background()

	t.Run("with RejectionStepID set, rejection listed first — must skip to approval", func(t *testing.T) {
		store := openStore(t)

		tplID := tid("tpl")
		stepA := tid("stp")
		gateStep := tid("stp")
		stepC := tid("stp")
		transGA := tid("tr")
		transGC := tid("tr")

		tmpl := &domain.WorkflowTemplate{
			ID: tplID, Name: "WrongBranchGate", Version: 1,
			Steps: []domain.Step{
				{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
				{
					ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
					Type: domain.StepTypeGate, Position: 1,
					Approval: &domain.ApprovalConfig{
						Mode:              domain.ApprovalModeAny,
						RequiredApprovers: 1,
						ApproverRoleID:    "reviewer",
						RejectionStepID:   stepA, // explicitly identifies the rejection branch
					},
				},
				{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
			},
			// Adversarial ordering: rejection branch listed FIRST.
			Transitions: []domain.Transition{
				{ID: transGA, TemplateID: tplID, Key: "gate-to-a", Name: "SendBack", FromStepID: gateStep, ToStepID: stepA},
				{ID: transGC, TemplateID: tplID, Key: "gate-to-c", Name: "Approve", FromStepID: gateStep, ToStepID: stepC},
			},
		}
		instID := seedInstance(t, store, tmpl)
		svc := workflow.New(store)

		item := createItem(t, store, instID, gateStep)
		updated, err := svc.ApproveItem(ctx, workflow.ApproveRequest{
			WorkItemID: item.ID, AgentID: "agent-alice",
		})
		if err != nil {
			t.Fatalf("ApproveItem: %v", err)
		}
		// Engine must skip transGA (rejection branch) and advance to stepC.
		if updated.CurrentStepID != stepC {
			t.Errorf("auto-advance selected wrong branch: got %q, want stepC (%q) — rejection transition listed first caused wrong routing", updated.CurrentStepID, stepC)
		}
	})

	t.Run("without RejectionStepID, two outgoing transitions — first transition wins (order sensitivity bug)", func(t *testing.T) {
		store := openStore(t)

		tplID := tid("tpl")
		stepA := tid("stp")
		gateStep := tid("stp")
		stepC := tid("stp")
		transGA := tid("tr")
		transGC := tid("tr")

		// Gate with NO RejectionStepID — both outgoing transitions are candidates.
		// The "wrong" transition (gate→a) is listed first.
		tmpl := &domain.WorkflowTemplate{
			ID: tplID, Name: "AmbiguousGate", Version: 1,
			Steps: []domain.Step{
				{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
				{
					ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
					Type: domain.StepTypeGate, Position: 1,
					Approval: &domain.ApprovalConfig{
						Mode:              domain.ApprovalModeAny,
						RequiredApprovers: 1,
						ApproverRoleID:    "reviewer",
						// No RejectionStepID — ambiguous
					},
				},
				{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
			},
			// The "back/rejection-like" transition is listed FIRST.
			Transitions: []domain.Transition{
				{ID: transGA, TemplateID: tplID, Key: "gate-to-a", Name: "SendBack", FromStepID: gateStep, ToStepID: stepA},
				{ID: transGC, TemplateID: tplID, Key: "gate-to-c", Name: "Approve", FromStepID: gateStep, ToStepID: stepC},
			},
		}
		instID := seedInstance(t, store, tmpl)
		svc := workflow.New(store)

		item := createItem(t, store, instID, gateStep)
		updated, err := svc.ApproveItem(ctx, workflow.ApproveRequest{
			WorkItemID: item.ID, AgentID: "agent-alice",
		})
		if err != nil {
			t.Fatalf("ApproveItem: %v", err)
		}
		// With no RejectionStepID, the current engine picks the first outgoing
		// transition — which is gate→a (the wrong direction for an approval).
		// The fix must ensure this case either returns an error (ambiguity) or
		// uses a higher-position step as the "forward" destination.
		if updated.CurrentStepID == stepA {
			t.Errorf("auto-advance with no RejectionStepID routed to stepA (first transition) instead of stepC — order-sensitivity bug")
		}
		if updated.CurrentStepID != stepC {
			t.Errorf("auto-advance: want stepC (%q), got %q", stepC, updated.CurrentStepID)
		}
	})
}

// --- Finding 4: Invalid RejectionStepID Accepted Without Validation ---

// TestInvalidRejectionStepID_CreateTemplate verifies that creating a template
// with a gate step whose RejectionStepID refers to a step not in the same
// template is rejected with a validation error.
func TestInvalidRejectionStepID_CreateTemplate(t *testing.T) {
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	tplID := tid("tpl")
	gateStepID := tid("stp")
	nextStepID := tid("stp")
	bogusStepID := "stp_doesnotexist"

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "BadRejectionStep", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStepID, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 1,
					ApproverRoleID:    "reviewer",
					RejectionStepID:   bogusStepID, // not a step in this template
				},
			},
			{ID: nextStepID, TemplateID: tplID, Key: "next", Name: "Next", Type: domain.StepTypeTask, Position: 1},
		},
	}

	err = store.CreateTemplate(ctx, tmpl)
	if err == nil {
		t.Fatal("want error for invalid RejectionStepID, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidGuard) && !errors.Is(err, domain.ErrNotFound) {
		// Accept any domain error that indicates validation failure.
		// The exact error type will be determined when the fix is implemented.
		t.Logf("got validation error (non-domain type): %v", err)
	}
}

// TestInvalidRejectionStepID_RejectItem_CreateTemplate_Blocked verifies that
// the CreateTemplate validation introduced to fix the invalid-RejectionStepID
// bug prevents injection of the bad state entirely. A template with a gate step
// whose RejectionStepID points to a step not in the template must be rejected
// at creation time, so the runtime can never encounter corrupted routing state.
func TestInvalidRejectionStepID_RejectItem_CreateTemplate_Blocked(t *testing.T) {
	rawStore, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rawStore.Close()
	ctx := context.Background()

	tplID := tid("tpl")
	gateStepID := tid("stp")
	bogusStepID := "stp_doesnotexist"

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "BadRejectionRuntime", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStepID, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 1,
					ApproverRoleID:    "reviewer",
					RejectionStepID:   bogusStepID,
				},
			},
		},
	}

	err = rawStore.CreateTemplate(ctx, tmpl)
	if err == nil {
		t.Fatal("CreateTemplate must reject a gate step with a RejectionStepID that doesn't exist in the template")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound (validation error), got %T: %v", err, err)
	}
}
