// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// sdlcFixture is a 4-step SDLC-flavored template:
//
//	Backlog (task, pos=0) --[triage]--> Planning (task, pos=1) --[submit]--> Review (gate, pos=2) --auto-advance--> Done (task, pos=3)
//	Review --[rejection routing]--> Planning
type sdlcFixture struct {
	Store      domain.Store
	Svc        *workflow.Service
	TemplateID string
	Backlog    string
	Planning   string
	Review     string
	Done       string
	Triage     string // Backlog → Planning
	Submit     string // Planning → Review
	ReviewDone string // Review → Done (used by auto-advance)
	ReviewPlan string // Review → Planning (rejection)
	InstanceID string
}

func seedSDLC(t *testing.T) sdlcFixture {
	t.Helper()
	store := openStore(t)

	tplID := tid("tpl")
	backlog := tid("stp")
	planning := tid("stp")
	review := tid("stp")
	done := tid("stp")
	triage := tid("tr")
	submit := tid("tr")
	reviewDone := tid("tr")
	reviewPlan := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "SDLC", Version: 1,
		Steps: []domain.Step{
			{ID: backlog, TemplateID: tplID, Key: "backlog", Name: "Backlog", Type: domain.StepTypeTask, Position: 0},
			{ID: planning, TemplateID: tplID, Key: "planning", Name: "Planning", Type: domain.StepTypeTask, Position: 1},
			{
				ID: review, TemplateID: tplID, Key: "review", Name: "Review",
				Type: domain.StepTypeGate, Position: 2,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 1,
					ApproverRoleID:    "reviewer",
					RejectionStepID:   planning,
				},
			},
			{ID: done, TemplateID: tplID, Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 3},
		},
		Transitions: []domain.Transition{
			{ID: triage, TemplateID: tplID, Key: "triage", Name: "Triage", FromStepID: backlog, ToStepID: planning},
			{ID: submit, TemplateID: tplID, Key: "submit", Name: "Submit", FromStepID: planning, ToStepID: review},
			{ID: reviewDone, TemplateID: tplID, Key: "review-done", Name: "Approve", FromStepID: review, ToStepID: done},
			{ID: reviewPlan, TemplateID: tplID, Key: "review-plan", Name: "Reject", FromStepID: review, ToStepID: planning},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return sdlcFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID,
		Backlog:    backlog, Planning: planning, Review: review, Done: done,
		Triage: triage, Submit: submit, ReviewDone: reviewDone, ReviewPlan: reviewPlan,
		InstanceID: instID,
	}
}

func TestSDLCScenario_HappyPath(t *testing.T) {
	f := seedSDLC(t)
	ctx := context.Background()

	// 1. Create item — should start at Backlog (lowest position=0).
	item := createItem(t, f.Store, f.InstanceID, f.Backlog)
	if item.CurrentStepID != f.Backlog {
		t.Fatalf("new item: want Backlog, got %q", item.CurrentStepID)
	}

	// 2. Triage: Backlog → Planning
	updated, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.Triage,
		ActorAgentID: "pm-agent", ActorRoleID: "product-manager",
		Reason: "ready to plan",
	})
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	if updated.CurrentStepID != f.Planning {
		t.Errorf("after triage: want Planning, got %q", updated.CurrentStepID)
	}

	// 3. Submit: Planning → Review
	updated, err = f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.Submit,
		ActorAgentID: "planner-agent", ActorRoleID: "planner",
		Reason: "plan complete",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if updated.CurrentStepID != f.Review {
		t.Errorf("after submit: want Review, got %q", updated.CurrentStepID)
	}

	// 4. Approve: 1 approval, threshold=1 → auto-advance to Done
	updated, err = f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "reviewer-agent", Comment: "approved",
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if updated.CurrentStepID != f.Done {
		t.Errorf("after approve: want Done, got %q", updated.CurrentStepID)
	}

	// 5. History: triage + submit + auto-advance = 3 entries
	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("want 3 history entries, got %d: %+v", len(history), history)
	}
}

func TestSDLCScenario_RejectionAndResubmit(t *testing.T) {
	f := seedSDLC(t)
	ctx := context.Background()

	item := createItem(t, f.Store, f.InstanceID, f.Backlog)

	// Triage → Planning
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.Triage,
		ActorAgentID: "pm-agent", ActorRoleID: "pm",
	}); err != nil {
		t.Fatalf("triage: %v", err)
	}

	// Submit → Review
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.Submit,
		ActorAgentID: "planner-agent", ActorRoleID: "planner",
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Reject → back to Planning
	updated, err := f.Svc.RejectItem(ctx, workflow.RejectRequest{
		WorkItemID: item.ID, AgentID: "reviewer-agent", Comment: "needs revision",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if updated.CurrentStepID != f.Planning {
		t.Errorf("after reject: want Planning, got %q", updated.CurrentStepID)
	}

	// Resubmit → Review
	if _, err := f.Svc.TransitionItem(ctx, workflow.TransitionRequest{
		WorkItemID: item.ID, TransitionID: f.Submit,
		ActorAgentID: "planner-agent", ActorRoleID: "planner",
	}); err != nil {
		t.Fatalf("resubmit: %v", err)
	}

	// Approve → Done
	updated, err = f.Svc.ApproveItem(ctx, workflow.ApproveRequest{
		WorkItemID: item.ID, AgentID: "reviewer-agent", Comment: "looks good now",
	})
	if err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if updated.CurrentStepID != f.Done {
		t.Errorf("after second approve: want Done, got %q", updated.CurrentStepID)
	}

	// History: triage + submit + rejection-route + resubmit + auto-advance = 5
	history, err := f.Store.GetTransitionHistory(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("want 5 history entries, got %d: %+v", len(history), history)
	}
}
