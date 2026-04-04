// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// --- Chunk 2: Transition Logic ---

func TestTransition_ValidAdvances(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}
	if updated.CurrentStepID != f.StepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, f.StepB)
	}

	// Confirm store reflects the update.
	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.StepB {
		t.Errorf("persisted CurrentStepID: got %q, want %q", persisted.CurrentStepID, f.StepB)
	}

	// Check history.
	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(history))
	}
	h := history[0]
	if h.FromStepID != f.StepA {
		t.Errorf("history FromStepID: got %q, want %q", h.FromStepID, f.StepA)
	}
	if h.ToStepID != f.StepB {
		t.Errorf("history ToStepID: got %q, want %q", h.ToStepID, f.StepB)
	}
	if h.TransitionID != f.TransAtoB {
		t.Errorf("history TransitionID: got %q, want %q", h.TransitionID, f.TransAtoB)
	}
	if h.TriggeredBy != "agent-1" {
		t.Errorf("history TriggeredBy: got %q, want %q", h.TriggeredBy, "agent-1")
	}
}

func TestTransition_WrongCurrentStep(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	// Manually advance to StepB so the item is no longer at StepA.
	item.CurrentStepID = f.StepB
	if err := f.Store.UpdateWorkItem(ctx, item); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}

	// TransAtoB expects from=StepA, but item is at StepB.
	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
}

func TestTransition_NonExistentTransitionID(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: "tr_doesnotexist",
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
}

func TestTransition_WorkItemNotFound(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   "wi_doesnotexist",
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// --- Chunk 2: CEL Guard Evaluation ---

func TestTransition_GuardPass(t *testing.T) {
	f := seedGuarded(t) // guard: item.priority == "high"
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)
	// Set priority to high.
	item.Priority = domain.PriorityHigh
	if err := f.Store.UpdateWorkItem(ctx, item); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}

	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}
	if updated.CurrentStepID != f.StepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, f.StepB)
	}
}

func TestTransition_GuardFail(t *testing.T) {
	f := seedGuarded(t) // guard: item.priority == "high"
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)
	// Item has priority=normal (the default) — guard will fail.

	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrGuardDenied) {
		t.Errorf("want ErrGuardDenied, got %v", err)
	}

	// Item must be unchanged.
	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.StepA {
		t.Errorf("item should still be at StepA, got %q", persisted.CurrentStepID)
	}
}

func TestTransition_GuardWithActorRole(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "RoleGuard", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance",
				FromStepID: stepA, ToStepID: stepB,
				Guard: `actor.role_id == "developer"`,
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	svc := workflow.New(store)

	// Matching role — should advance.
	item1 := createItem(t, store, instID, stepA)
	updated, err := svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item1.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("matching role: %v", err)
	}
	if updated.CurrentStepID != stepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, stepB)
	}

	// Non-matching role — should deny.
	item2 := createItem(t, store, instID, stepA)
	_, err = svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item2.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-2",
		ActorRoleID:  "viewer",
	})
	if !errors.Is(err, domain.ErrGuardDenied) {
		t.Errorf("non-matching role: want ErrGuardDenied, got %v", err)
	}
}

func TestTransition_GuardWithFields(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "FieldGuard", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance",
				FromStepID: stepA, ToStepID: stepB,
				Guard: `item.fields.tests_passing == true`,
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	svc := workflow.New(store)

	// Item with tests_passing=true — should advance.
	item1 := createItem(t, store, instID, stepA)
	item1.Fields = json.RawMessage(`{"tests_passing": true}`)
	if err := store.UpdateWorkItem(ctx, item1); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}
	updated, err := svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item1.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("tests_passing=true: %v", err)
	}
	if updated.CurrentStepID != stepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, stepB)
	}

	// Item with tests_passing=false — should deny.
	item2 := createItem(t, store, instID, stepA)
	item2.Fields = json.RawMessage(`{"tests_passing": false}`)
	if err := store.UpdateWorkItem(ctx, item2); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}
	_, err = svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item2.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrGuardDenied) {
		t.Errorf("tests_passing=false: want ErrGuardDenied, got %v", err)
	}
}

func TestTransition_GuardWithApprovalCount(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "ApprovalCountGuard", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance",
				FromStepID: stepA, ToStepID: stepB,
				Guard: `approval.count >= 1`,
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	svc := workflow.New(store)

	// No approvals — guard must deny.
	item1 := createItem(t, store, instID, stepA)
	_, err := svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item1.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrGuardDenied) {
		t.Errorf("no approvals: want ErrGuardDenied, got %v", err)
	}

	// Seed 1 approval — guard must pass.
	item2 := createItem(t, store, instID, stepA)
	if err := store.RecordApproval(ctx, &domain.Approval{
		ID:         tid("apr"),
		WorkItemID: item2.ID,
		StepID:     stepA,
		AgentID:    "approver-1",
		Decision:   domain.ApprovalDecisionApproved,
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	updated, err := svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item2.ID,
		TransitionID: transAB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("with 1 approval: %v", err)
	}
	if updated.CurrentStepID != stepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, stepB)
	}
}

// --- Chunk 2: Multiple Transitions from the Same Step ---

func TestTransition_MultipleOutgoing_ChooseB(t *testing.T) {
	f := seedMultiTrans(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)
	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}
	if updated.CurrentStepID != f.StepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, f.StepB)
	}
}

func TestTransition_MultipleOutgoing_ChooseC(t *testing.T) {
	f := seedMultiTrans(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)
	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoC,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}
	if updated.CurrentStepID != f.StepC {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, f.StepC)
	}
}

func TestTransition_MultipleOutgoing_BothRecordedSeparately(t *testing.T) {
	f := seedMultiTrans(t)
	ctx := context.Background()

	itemToB := createItem(t, f.Store, f.InstanceID, f.StepA)
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   itemToB.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	}); err != nil {
		t.Fatalf("TransitionItem to B: %v", err)
	}

	itemToC := createItem(t, f.Store, f.InstanceID, f.StepA)
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   itemToC.ID,
		TransitionID: f.TransAtoC,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	}); err != nil {
		t.Fatalf("TransitionItem to C: %v", err)
	}

	histB, err := f.Store.GetTransitionHistory(ctx, itemToB.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory B: %v", err)
	}
	if len(histB) != 1 || histB[0].ToStepID != f.StepB {
		t.Errorf("item->B history: want ToStepID=%q, got %+v", f.StepB, histB)
	}

	histC, err := f.Store.GetTransitionHistory(ctx, itemToC.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory C: %v", err)
	}
	if len(histC) != 1 || histC[0].ToStepID != f.StepC {
		t.Errorf("item->C history: want ToStepID=%q, got %+v", f.StepC, histC)
	}
}

// --- Chunk 2: Cyclic Transitions ---

func TestTransition_CyclicPath_ForwardAndBack(t *testing.T) {
	f := seedCycle(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	// A → B
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1", ActorRoleID: "developer",
	}); err != nil {
		t.Fatalf("A→B: %v", err)
	}

	// B → A (cycle back)
	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.TransBtoA,
		ActorAgentID: "agent-1", ActorRoleID: "developer",
	})
	if err != nil {
		t.Fatalf("B→A: %v", err)
	}
	if updated.CurrentStepID != f.StepA {
		t.Errorf("CurrentStepID after cycle: got %q, want %q", updated.CurrentStepID, f.StepA)
	}

	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("want 2 history entries, got %d", len(history))
	}
}

func TestTransition_CyclicPath_MultipleLoops(t *testing.T) {
	f := seedCycle(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	// Cycle A→B→A three times.
	for i := 0; i < 3; i++ {
		if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
			WorkItemID: item.ID, TransitionID: f.TransAtoB,
			ActorAgentID: "agent-1", ActorRoleID: "developer",
		}); err != nil {
			t.Fatalf("loop %d A→B: %v", i, err)
		}
		if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
			WorkItemID: item.ID, TransitionID: f.TransBtoA,
			ActorAgentID: "agent-1", ActorRoleID: "developer",
		}); err != nil {
			t.Fatalf("loop %d B→A: %v", i, err)
		}
	}

	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.StepA {
		t.Errorf("CurrentStepID after 3 loops: got %q, want %q", persisted.CurrentStepID, f.StepA)
	}

	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 6 {
		t.Errorf("want 6 history entries, got %d", len(history))
	}
}

// --- Chunk 3: Approval Recording ---

func TestApprove_AtGateStep_RecordsApproval(t *testing.T) {
	f := seedGate(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	_, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID,
		AgentID:    "approver-alice",
		Comment:    "LGTM",
	})
	if err != nil {
		t.Fatalf("ApproveItem: %v", err)
	}

	approvals, err := f.Store.ListApprovals(ctx, item.ID, f.GateStep)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("want 1 approval, got %d", len(approvals))
	}
	a := approvals[0]
	if a.Decision != domain.ApprovalDecisionApproved {
		t.Errorf("Decision: got %q, want approved", a.Decision)
	}
	if a.AgentID != "approver-alice" {
		t.Errorf("AgentID: got %q, want approver-alice", a.AgentID)
	}
	if a.Comment != "LGTM" {
		t.Errorf("Comment: got %q, want LGTM", a.Comment)
	}
}

func TestApprove_AtTaskStep_ReturnsError(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA) // task step

	_, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID,
		AgentID:    "approver-1",
	})
	if !errors.Is(err, domain.ErrNotAtGateStep) {
		t.Errorf("want ErrNotAtGateStep, got %v", err)
	}
}

func TestReject_AtGateStep_RecordsRejection(t *testing.T) {
	f := seedGate(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	_, err := f.Svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID,
		AgentID:    "reviewer-bob",
		Comment:    "Needs more work",
	})
	if err != nil {
		t.Fatalf("RejectItem: %v", err)
	}

	approvals, err := f.Store.ListApprovals(ctx, item.ID, f.GateStep)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("want 1 approval record, got %d", len(approvals))
	}
	if approvals[0].Decision != domain.ApprovalDecisionRejected {
		t.Errorf("Decision: got %q, want rejected", approvals[0].Decision)
	}
	if approvals[0].Comment != "Needs more work" {
		t.Errorf("Comment: got %q, want %q", approvals[0].Comment, "Needs more work")
	}
}

func TestReject_AtTaskStep_ReturnsError(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)

	_, err := f.Svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID,
		AgentID:    "reviewer-1",
	})
	if !errors.Is(err, domain.ErrNotAtGateStep) {
		t.Errorf("want ErrNotAtGateStep, got %v", err)
	}
}

// --- Chunk 3: Auto-Advance ---

func TestApprove_BelowThreshold_NoAdvance(t *testing.T) {
	f := seedGate(t) // required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID,
		AgentID:    "approver-1",
	})
	if err != nil {
		t.Fatalf("ApproveItem: %v", err)
	}
	// 1 of 2 required — should remain at gate.
	if updated.CurrentStepID != f.GateStep {
		t.Errorf("CurrentStepID: got %q, want %q (gate)", updated.CurrentStepID, f.GateStep)
	}
}

func TestApprove_ReachesThreshold_AutoAdvances(t *testing.T) {
	f := seedGate(t) // required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// First approval — no advance.
	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "approver-alice",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Second approval — should auto-advance.
	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "approver-bob",
	})
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if updated.CurrentStepID != f.StepC {
		t.Errorf("CurrentStepID: got %q, want %q (StepC)", updated.CurrentStepID, f.StepC)
	}

	// History should contain the auto-advance entry: check step IDs, not the reason string.
	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	found := false
	for _, h := range history {
		if h.FromStepID == f.GateStep && h.ToStepID == f.StepC {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected auto-advance history entry from gate to StepC, got: %+v", history)
	}
}

func TestApprove_ExceedsThreshold_StillAtAdvancedStep(t *testing.T) {
	f := seedGate(t) // required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// First approval — below threshold, still at gate.
	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "approver-1",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Second approval — threshold met, auto-advances to StepC.
	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "approver-2",
	})
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if updated.CurrentStepID != f.StepC {
		t.Errorf("after 2nd approval: CurrentStepID=%q, want StepC (%q)", updated.CurrentStepID, f.StepC)
	}

	// Third approval: item is now at StepC (a task step), so approving it
	// returns ErrNotAtGateStep — the item did not regress.
	_, err = f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "approver-3",
	})
	if !errors.Is(err, domain.ErrNotAtGateStep) {
		t.Errorf("3rd approval on task step: want ErrNotAtGateStep, got %v", err)
	}

	// Confirm item is still at StepC.
	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.StepC {
		t.Errorf("item regressed: CurrentStepID=%q, want StepC", persisted.CurrentStepID)
	}
}

func TestApprove_MultipleAgents_BothCounted(t *testing.T) {
	f := seedGate(t) // required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	}); err != nil {
		t.Fatalf("alice approval: %v", err)
	}
	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-bob",
	}); err != nil {
		t.Fatalf("bob approval: %v", err)
	}

	approvals, err := f.Store.ListApprovals(ctx, item.ID, f.GateStep)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 2 {
		t.Errorf("want 2 approval records, got %d", len(approvals))
	}

	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.StepC {
		t.Errorf("CurrentStepID: got %q, want StepC", persisted.CurrentStepID)
	}
}

// --- Chunk 3: Rejection Routing ---

func TestReject_WithRejectionStep_AdvancesToRejectionStep(t *testing.T) {
	f := seedGate(t) // RejectionStepID = StepA
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	updated, err := f.Svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID,
		AgentID:    "reviewer-bob",
		Comment:    "not ready",
	})
	if err != nil {
		t.Fatalf("RejectItem: %v", err)
	}
	if updated.CurrentStepID != f.StepA {
		t.Errorf("CurrentStepID: got %q, want StepA (%q)", updated.CurrentStepID, f.StepA)
	}

	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(history))
	}
	h := history[0]
	if h.TriggeredBy != "reviewer-bob" {
		t.Errorf("TriggeredBy: got %q, want reviewer-bob", h.TriggeredBy)
	}
	if !strings.HasPrefix(h.Reason, "rejected:") {
		t.Errorf("Reason: got %q, want prefix 'rejected:'", h.Reason)
	}
	if h.ToStepID != f.StepA {
		t.Errorf("history ToStepID: got %q, want StepA", h.ToStepID)
	}
}

func TestReject_WithoutRejectionStep_RecordsAndStays(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	gateStep := tid("stp")

	// Gate step with NO RejectionStepID.
	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "NoRejectStep", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode: domain.ApprovalModeAny, RequiredApprovers: 1,
					ApproverRoleID: "reviewer",
					// RejectionStepID intentionally empty
				},
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	svc := workflow.New(store)

	item := createItem(t, store, instID, gateStep)

	updated, err := svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID,
		AgentID:    "reviewer-1",
		Comment:    "blocked",
	})
	if err != nil {
		t.Fatalf("RejectItem: %v", err)
	}
	// Item stays at gate.
	if updated.CurrentStepID != gateStep {
		t.Errorf("CurrentStepID: got %q, want gate (%q)", updated.CurrentStepID, gateStep)
	}

	// Rejection is recorded.
	approvals, err := store.ListApprovals(ctx, item.ID, gateStep)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Decision != domain.ApprovalDecisionRejected {
		t.Errorf("expected 1 rejection, got: %+v", approvals)
	}

	// No history entry (no transition happened).
	history, err := store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 history entries, got %d: %+v", len(history), history)
	}
}

// --- Chunk 4: State Validation ---

func TestCreateWorkItem_StartsAtFirstStep(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	// Define steps out of position order (pos=5 listed first) to verify
	// the "lowest position" selection works correctly.
	tplID := tid("tpl")
	step5 := tid("stp")
	step0 := tid("stp")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "PositionTest", Version: 1,
		Steps: []domain.Step{
			{ID: step5, TemplateID: tplID, Key: "s5", Name: "Step 5", Type: domain.StepTypeTask, Position: 5},
			{ID: step0, TemplateID: tplID, Key: "s0", Name: "Step 0", Type: domain.StepTypeTask, Position: 0},
		},
	}
	instID := seedInstance(t, store, tmpl)

	// The create-work-item logic (in the REST handler) finds the step with
	// the lowest position. We replicate that logic here and verify the item
	// starts at the right step.
	inst, err := store.GetInstance(ctx, instID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	loaded, err := store.GetTemplate(ctx, inst.TemplateID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	firstStep := loaded.Steps[0]
	for _, s := range loaded.Steps {
		if s.Position < firstStep.Position {
			firstStep = s
		}
	}
	if firstStep.ID != step0 {
		t.Errorf("first step: got %q (pos=%d), want step0 (pos=0)", firstStep.ID, firstStep.Position)
	}

	item := createItem(t, store, instID, firstStep.ID)
	if item.CurrentStepID != step0 {
		t.Errorf("item CurrentStepID: got %q, want step0 (%q)", item.CurrentStepID, step0)
	}
}

func TestTransition_ItemNotFound(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   "wi_ghost",
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestApprove_ItemNotFound(t *testing.T) {
	f := seedGate(t)
	ctx := context.Background()

	_, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: "wi_ghost",
		AgentID:    "approver-1",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// --- Chunk 4: Template and Instance Lifecycle ---

func TestDeleteTemplate_WithActiveInstance_Blocked(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	// Instance already exists from seed — delete should be blocked.
	err := f.Store.DeleteTemplate(ctx, f.TemplateID)
	if !errors.Is(err, domain.ErrHasDependencies) {
		t.Errorf("want ErrHasDependencies, got %v", err)
	}
}

func TestDeleteTemplate_NoInstances_Succeeds(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "ToDelete", Version: 1,
		Steps: []domain.Step{
			{ID: tid("stp"), TemplateID: tplID, Key: "s", Name: "S", Type: domain.StepTypeTask, Position: 0},
		},
	}
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if err := store.DeleteTemplate(ctx, tplID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	_, err := store.GetTemplate(ctx, tplID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestCreateInstance_SnapshotsTemplateVersion(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "Versioned", Version: 1,
		Steps: []domain.Step{
			{ID: tid("stp"), TemplateID: tplID, Key: "s", Name: "S", Type: domain.StepTypeTask, Position: 0},
		},
	}
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	// Instance 1 at version 1.
	inst1ID := tid("ins")
	inst1 := &domain.WorkflowInstance{
		ID: inst1ID, TemplateID: tplID, TemplateVersion: tmpl.Version,
		TeamID: "team-1", Name: "Inst1", Status: domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(ctx, inst1); err != nil {
		t.Fatalf("CreateInstance 1: %v", err)
	}
	got1, err := store.GetInstance(ctx, inst1ID)
	if err != nil {
		t.Fatalf("GetInstance 1: %v", err)
	}
	if got1.TemplateVersion != 1 {
		t.Errorf("inst1 TemplateVersion: got %d, want 1", got1.TemplateVersion)
	}

	// Bump template to version 2.
	tmpl.Version = 2
	if err := store.UpdateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}

	// Instance 2 at version 2.
	inst2ID := tid("ins")
	inst2 := &domain.WorkflowInstance{
		ID: inst2ID, TemplateID: tplID, TemplateVersion: 2,
		TeamID: "team-1", Name: "Inst2", Status: domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(ctx, inst2); err != nil {
		t.Fatalf("CreateInstance 2: %v", err)
	}
	got2, err := store.GetInstance(ctx, inst2ID)
	if err != nil {
		t.Fatalf("GetInstance 2: %v", err)
	}
	if got2.TemplateVersion != 2 {
		t.Errorf("inst2 TemplateVersion: got %d, want 2", got2.TemplateVersion)
	}

	// Instance 1's snapshot must be unchanged.
	got1Again, err := store.GetInstance(ctx, inst1ID)
	if err != nil {
		t.Fatalf("GetInstance 1 again: %v", err)
	}
	if got1Again.TemplateVersion != 1 {
		t.Errorf("inst1 TemplateVersion should still be 1, got %d", got1Again.TemplateVersion)
	}
}

// --- Chunk 5: Integration Hook Storage ---

func TestIntegrationHooks_StoredAndRetrievable(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transID := tid("tr")
	hookID := tid("hook")

	hookConfig := json.RawMessage(`{"channel": "general"}`)
	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "WithHook", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: transID, TemplateID: tplID, Key: "a-to-b", Name: "Advance", FromStepID: stepA, ToStepID: stepB},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{
				ID: hookID, TemplateID: tplID, TransitionID: transID,
				Event: "on_transition", AdapterType: "chat",
				Action: "post_message", Config: hookConfig,
			},
		},
	}
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := store.GetTemplate(ctx, tplID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if len(got.IntegrationHooks) != 1 {
		t.Fatalf("want 1 hook, got %d", len(got.IntegrationHooks))
	}
	h := got.IntegrationHooks[0]
	if h.TransitionID != transID {
		t.Errorf("TransitionID: got %q, want %q", h.TransitionID, transID)
	}
	if h.AdapterType != "chat" {
		t.Errorf("AdapterType: got %q, want chat", h.AdapterType)
	}
	if h.Action != "post_message" {
		t.Errorf("Action: got %q, want post_message", h.Action)
	}
	if string(h.Config) != string(hookConfig) {
		t.Errorf("Config: got %s, want %s", h.Config, hookConfig)
	}
}

func TestIntegrationHooks_MultipleHooksOnDifferentTransitions(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	stepC := tid("stp")
	transAB := tid("tr")
	transBC := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "MultiHook", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
			{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "AB", FromStepID: stepA, ToStepID: stepB},
			{ID: transBC, TemplateID: tplID, Key: "b-to-c", Name: "BC", FromStepID: stepB, ToStepID: stepC},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{ID: tid("hook"), TemplateID: tplID, TransitionID: transAB, Event: "on_transition", AdapterType: "chat", Action: "notify"},
			{ID: tid("hook"), TemplateID: tplID, TransitionID: transBC, Event: "on_transition", AdapterType: "git-forge", Action: "create_issue"},
		},
	}
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := store.GetTemplate(ctx, tplID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if len(got.IntegrationHooks) != 2 {
		t.Fatalf("want 2 hooks, got %d", len(got.IntegrationHooks))
	}

	byTrans := map[string]string{}
	for _, h := range got.IntegrationHooks {
		byTrans[h.TransitionID] = h.AdapterType
	}
	if byTrans[transAB] != "chat" {
		t.Errorf("transAB hook AdapterType: got %q, want chat", byTrans[transAB])
	}
	if byTrans[transBC] != "git-forge" {
		t.Errorf("transBC hook AdapterType: got %q, want git-forge", byTrans[transBC])
	}
}

// TestApprove_SameAgentTwice documents current behavior: the engine counts
// each ApproveItem call toward the threshold independently, so the same agent
// approving twice counts as two approvals. This is a known design decision
// that may need revisiting (deduplication by AgentID has not been implemented).
func TestApprove_SameAgentTwice(t *testing.T) {
	f := seedGate(t) // required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	// First approval from agent-alice.
	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Second approval from the same agent — current behavior: counts again and auto-advances.
	// NOTE: same agent counting twice is a known design decision — if deduplication
	// is added in the future this test will need updating.
	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	})
	if err != nil {
		t.Fatalf("second approval (same agent): %v", err)
	}
	// Current behavior: threshold met (2 records regardless of agent), item advances.
	if updated.CurrentStepID != f.StepC {
		t.Errorf("same-agent double approval: CurrentStepID=%q, want StepC (%q)", updated.CurrentStepID, f.StepC)
	}

	approvals, err := f.Store.ListApprovals(ctx, item.ID, f.GateStep)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	// Two records from the same agent.
	if len(approvals) != 2 {
		t.Errorf("want 2 approval records (same agent twice), got %d", len(approvals))
	}
}

// TestTransition_OnGateStep verifies that TransitionItem refuses to execute a
// transition out of a gate step, returning ErrGateRequiresApproval. Agents must
// go through ApproveItem/RejectItem to advance past a gate step.
func TestTransition_OnGateStep(t *testing.T) {
	f := seedGate(t) // GateStep --[TransGateToC]--> StepC
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	_, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransGateToC,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if !errors.Is(err, domain.ErrGateRequiresApproval) {
		t.Errorf("want ErrGateRequiresApproval, got %v", err)
	}

	// Item must remain at the gate step.
	persisted, err := f.Store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.CurrentStepID != f.GateStep {
		t.Errorf("item moved despite gate bypass attempt: got %q, want gate %q", persisted.CurrentStepID, f.GateStep)
	}
}

// --- Finding 2: Unanimous Mode ---

func TestApprove_UnanimousMode_AllApprove_AutoAdvances(t *testing.T) {
	f := seedUnanimousGate(t) // mode=unanimous, required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

	if _, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	updated, err := f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-bob",
	})
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if updated.CurrentStepID != f.StepC {
		t.Errorf("unanimous with 2/2 approvals: want StepC (%q), got %q", f.StepC, updated.CurrentStepID)
	}
}

func TestApprove_UnanimousMode_OneRejects_NoAdvance(t *testing.T) {
	f := seedUnanimousGate(t) // mode=unanimous, required=2
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.GateStep)

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
	// One rejection must block unanimous advance.
	if updated.CurrentStepID != f.GateStep {
		t.Errorf("unanimous after rejection: want gate (%q), got %q", f.GateStep, updated.CurrentStepID)
	}
}

// --- Finding 3: Auto-advance guard evaluation on multiple forward transitions ---

// TestApprove_MultipleForwardTransitions_GuardEvaluated verifies that when a
// gate step has two forward transitions each with a different CEL guard, the
// auto-advance selects the transition whose guard matches the work item's state.
func TestApprove_MultipleForwardTransitions_GuardEvaluated(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	gateStep := tid("stp")
	stepHigh := tid("stp") // destination for high-priority items
	stepNorm := tid("stp") // destination for normal-priority items
	transToHigh := tid("tr")
	transToNorm := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "GuardedAutoAdvance", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode: domain.ApprovalModeAny, RequiredApprovers: 1,
					ApproverRoleID: "reviewer",
				},
			},
			// stepHigh has higher position (2) than stepNorm (1) — without guard
			// evaluation the max-position selector would always pick stepHigh,
			// regardless of the item's actual priority.
			{ID: stepHigh, TemplateID: tplID, Key: "high-track", Name: "High Track", Type: domain.StepTypeTask, Position: 2},
			{ID: stepNorm, TemplateID: tplID, Key: "norm-track", Name: "Normal Track", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transToHigh, TemplateID: tplID, Key: "gate-to-high", Name: "High Path",
				FromStepID: gateStep, ToStepID: stepHigh,
				Guard: `item.priority == "high"`,
			},
			{
				ID: transToNorm, TemplateID: tplID, Key: "gate-to-norm", Name: "Normal Path",
				FromStepID: gateStep, ToStepID: stepNorm,
				Guard: `item.priority == "normal"`,
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	svc := workflow.New(store)

	// Item with priority=normal — must route to stepNorm, not stepHigh.
	item := createItem(t, store, instID, gateStep)
	// priority is set to normal by default in createItem.

	updated, err := svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "agent-alice",
	})
	if err != nil {
		t.Fatalf("ApproveItem: %v", err)
	}
	if updated.CurrentStepID != stepNorm {
		t.Errorf("normal-priority item: auto-advance should pick stepNorm (%q), got %q", stepNorm, updated.CurrentStepID)
	}
}

// --- Finding 4: Invalid RejectionStepID validation at template creation ---

func TestCreateTemplate_InvalidRejectionStepID(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tplID := tid("tpl")
	gateStepID := tid("stp")
	nextStepID := tid("stp")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "BadRejStep", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStepID, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 1,
					ApproverRoleID:    "reviewer",
					RejectionStepID:   "stp_doesnotexist", // not a step in this template
				},
			},
			{ID: nextStepID, TemplateID: tplID, Key: "next", Name: "Next", Type: domain.StepTypeTask, Position: 1},
		},
	}

	err := store.CreateTemplate(ctx, tmpl)
	if err == nil {
		t.Fatal("want error for invalid RejectionStepID, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %T: %v", err, err)
	}
}

func TestTransition_MalformedFieldsJSON(t *testing.T) {
	f := seedTwoStep(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.StepA)
	item.Fields = json.RawMessage("not valid json")
	if err := f.Store.UpdateWorkItem(ctx, item); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}

	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: f.TransAtoB,
		ActorAgentID: "agent-1",
		ActorRoleID:  "developer",
	})
	if err != nil {
		t.Fatalf("TransitionItem with malformed fields: %v", err)
	}
	if updated.CurrentStepID != f.StepB {
		t.Errorf("CurrentStepID: got %q, want %q", updated.CurrentStepID, f.StepB)
	}
}
