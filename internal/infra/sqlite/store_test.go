// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func newID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}

func TestStoreOpen(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	templateID := newID("tpl")
	step1ID := newID("stp")
	step2ID := newID("stp")
	transID := newID("tr")

	tmpl := &domain.WorkflowTemplate{
		ID:          templateID,
		Name:        "Test Template",
		Description: "A test template",
		Version:     1,
		Steps: []domain.Step{
			{ID: step1ID, TemplateID: templateID, Key: "start", Name: "Start", Type: domain.StepTypeTask, Position: 0},
			{ID: step2ID, TemplateID: templateID, Key: "end", Name: "End", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transID, TemplateID: templateID,
				Key: "start_to_end", Name: "Start to End",
				FromStepID: step1ID, ToStepID: step2ID,
			},
		},
	}

	if err := s.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.Name != tmpl.Name {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, tmpl.Name)
	}
	if len(got.Steps) != 2 {
		t.Errorf("want 2 steps, got %d", len(got.Steps))
	}
	if len(got.Transitions) != 1 {
		t.Errorf("want 1 transition, got %d", len(got.Transitions))
	}

	list, err := s.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("want 1 template, got %d", len(list))
	}

	if err := s.DeleteTemplate(ctx, templateID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	list2, err := s.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates after delete: %v", err)
	}
	if len(list2) != 0 {
		t.Errorf("want 0 templates after delete, got %d", len(list2))
	}
}

func TestWorkItemTransitionFlow(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	templateID := newID("tpl")
	step1ID := newID("stp")
	step2ID := newID("stp")
	transID := newID("tr")
	instanceID := newID("ins")
	workItemID := newID("wi")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "Flow Test", Version: 1,
		Steps: []domain.Step{
			{ID: step1ID, TemplateID: templateID, Key: "s1", Name: "Step 1", Type: domain.StepTypeTask, Position: 0},
			{ID: step2ID, TemplateID: templateID, Key: "s2", Name: "Step 2", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: transID, TemplateID: templateID, Key: "s1_s2", Name: "S1 to S2", FromStepID: step1ID, ToStepID: step2ID},
		},
	}
	if err := s.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "Test Instance", Status: domain.InstanceStatusActive,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	wi := &domain.WorkItem{
		ID: workItemID, InstanceID: instanceID, Title: "Feature A",
		CurrentStepID: step1ID, Priority: domain.PriorityNormal,
	}
	if err := s.CreateWorkItem(ctx, wi); err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}

	// Transition to step 2
	wi.CurrentStepID = step2ID
	if err := s.UpdateWorkItem(ctx, wi); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}

	h := &domain.TransitionHistory{
		ID: newID("th"), WorkItemID: workItemID,
		FromStepID: step1ID, ToStepID: step2ID, TransitionID: transID,
		TriggeredBy: "agent1", Timestamp: time.Now().UTC(),
	}
	if err := s.RecordTransition(ctx, h); err != nil {
		t.Fatalf("RecordTransition: %v", err)
	}

	history, err := s.GetTransitionHistory(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetTransitionHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("want 1 history entry, got %d", len(history))
	}
	if history[0].ToStepID != step2ID {
		t.Errorf("ToStepID mismatch: got %q, want %q", history[0].ToStepID, step2ID)
	}
}

func TestApprovalFlow(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	templateID := newID("tpl")
	gateStepID := newID("stp")
	nextStepID := newID("stp")
	instanceID := newID("ins")
	workItemID := newID("wi")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "Approval Test", Version: 1,
		Steps: []domain.Step{
			{
				ID: gateStepID, TemplateID: templateID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 0,
				Approval: &domain.ApprovalConfig{
					Mode: domain.ApprovalModeAny, RequiredApprovers: 1,
					ApproverRoleID: "reviewer",
				},
			},
			{ID: nextStepID, TemplateID: templateID, Key: "next", Name: "Next", Type: domain.StepTypeTask, Position: 1},
		},
	}
	if err := s.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "Test", Status: domain.InstanceStatusActive,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	wi := &domain.WorkItem{
		ID: workItemID, InstanceID: instanceID, Title: "Needs Approval",
		CurrentStepID: gateStepID, Priority: domain.PriorityNormal,
	}
	if err := s.CreateWorkItem(ctx, wi); err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}

	a := &domain.Approval{
		ID: newID("apr"), WorkItemID: workItemID, StepID: gateStepID,
		AgentID: "agent1", Decision: domain.ApprovalDecisionApproved,
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordApproval(ctx, a); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}

	approvals, err := s.ListApprovals(ctx, workItemID, gateStepID)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 1 {
		t.Errorf("want 1 approval, got %d", len(approvals))
	}
	if approvals[0].Decision != domain.ApprovalDecisionApproved {
		t.Errorf("Decision mismatch: got %q, want approved", approvals[0].Decision)
	}
}
