// SPDX-License-Identifier: GPL-2.0-only
package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/postgres"
)

func dsn(t *testing.T) string {
	t.Helper()
	v := os.Getenv("FLOW_DB")
	if v == "" {
		t.Skip("FLOW_DB not set — skipping PostgreSQL tests")
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

func newID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}

func TestStoreOpen(t *testing.T) {
	s := openTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	templateID := newID("tpl")
	step1ID := newID("stp")
	step2ID := newID("stp")
	transID := newID("tr")

	tmpl := &domain.WorkflowTemplate{
		ID:          templateID,
		Name:        "PG Test Template",
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
	found := false
	for _, tl := range list {
		if tl.ID == templateID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("template %q not found in list", templateID)
	}

	if err := s.DeleteTemplate(ctx, templateID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
}

func TestWorkItemTransitionFlow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	templateID := newID("tpl")
	step1ID := newID("stp")
	step2ID := newID("stp")
	transID := newID("tr")
	instanceID := newID("ins")
	workItemID := newID("wi")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "PG Flow Test", Version: 1,
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
	t.Cleanup(func() { s.DeleteTemplate(ctx, templateID) })

	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "PG Test Instance", Status: domain.InstanceStatusActive,
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

func TestListWorkItems_Filters(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	templateID := newID("tpl")
	stepAlpha := newID("stp")
	stepBeta := newID("stp")
	instanceID := newID("ins")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "PG Filter Test", Version: 1,
		Steps: []domain.Step{
			{ID: stepAlpha, TemplateID: templateID, Key: "alpha", Name: "Alpha", Type: domain.StepTypeTask, Position: 0},
			{ID: stepBeta, TemplateID: templateID, Key: "beta", Name: "Beta", Type: domain.StepTypeTask, Position: 1},
		},
	}
	if err := s.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	t.Cleanup(func() { s.DeleteTemplate(ctx, templateID) })

	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "PG Filter Instance", Status: domain.InstanceStatusActive,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	wi1 := &domain.WorkItem{
		ID: newID("wi"), InstanceID: instanceID, Title: "Item 1",
		CurrentStepID: stepAlpha, AssignedAgentID: "agent-x", Priority: domain.PriorityHigh,
	}
	wi2 := &domain.WorkItem{
		ID: newID("wi"), InstanceID: instanceID, Title: "Item 2",
		CurrentStepID: stepBeta, AssignedAgentID: "agent-y", Priority: domain.PriorityNormal,
	}
	wi3 := &domain.WorkItem{
		ID: newID("wi"), InstanceID: instanceID, Title: "Item 3",
		CurrentStepID: stepAlpha, AssignedAgentID: "agent-x", Priority: domain.PriorityCritical,
	}
	for _, wi := range []*domain.WorkItem{wi1, wi2, wi3} {
		if err := s.CreateWorkItem(ctx, wi); err != nil {
			t.Fatalf("CreateWorkItem %q: %v", wi.Title, err)
		}
	}

	t.Run("no filters returns all", func(t *testing.T) {
		items, err := s.ListWorkItems(ctx, instanceID, "", "", "")
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("want 3 items, got %d", len(items))
		}
	})

	t.Run("filter by stepID", func(t *testing.T) {
		items, err := s.ListWorkItems(ctx, instanceID, stepAlpha, "", "")
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("want 2 items at stepAlpha, got %d", len(items))
		}
	})

	t.Run("filter by agentID", func(t *testing.T) {
		items, err := s.ListWorkItems(ctx, instanceID, "", "agent-y", "")
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("want 1 item for agent-y, got %d", len(items))
		}
	})

	t.Run("filter by priority", func(t *testing.T) {
		items, err := s.ListWorkItems(ctx, instanceID, "", "", domain.PriorityHigh)
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("want 1 high-priority item, got %d", len(items))
		}
	})

	t.Run("combined filters narrow correctly", func(t *testing.T) {
		items, err := s.ListWorkItems(ctx, instanceID, stepAlpha, "agent-x", "")
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("want 2 items for agent-x at stepAlpha, got %d", len(items))
		}
	})
}

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

func TestApprovalFlow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	templateID := newID("tpl")
	gateStepID := newID("stp")
	nextStepID := newID("stp")
	instanceID := newID("ins")
	workItemID := newID("wi")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "PG Approval Test", Version: 1,
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
	t.Cleanup(func() { s.DeleteTemplate(ctx, templateID) })

	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "PG Approval Instance", Status: domain.InstanceStatusActive,
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
