// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"errors"
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

func TestListWorkItems_Filters(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Build a template with two task steps so items can be placed on different steps.
	templateID := newID("tpl")
	stepAlpha := newID("stp")
	stepBeta := newID("stp")
	instanceID := newID("ins")

	tmpl := &domain.WorkflowTemplate{
		ID: templateID, Name: "Filter Test", Version: 1,
		Steps: []domain.Step{
			{ID: stepAlpha, TemplateID: templateID, Key: "alpha", Name: "Alpha", Type: domain.StepTypeTask, Position: 0},
			{ID: stepBeta, TemplateID: templateID, Key: "beta", Name: "Beta", Type: domain.StepTypeTask, Position: 1},
		},
	}
	if err := s.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	inst := &domain.WorkflowInstance{
		ID: instanceID, TemplateID: templateID, TemplateVersion: 1,
		TeamID: "team1", Name: "Filter Instance", Status: domain.InstanceStatusActive,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	// Create 3 work items at different steps with different priorities/agents.
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
		for _, item := range items {
			if item.CurrentStepID != stepAlpha {
				t.Errorf("unexpected step: got %q, want %q", item.CurrentStepID, stepAlpha)
			}
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
		if len(items) > 0 && items[0].AssignedAgentID != "agent-y" {
			t.Errorf("AssignedAgentID: got %q, want agent-y", items[0].AssignedAgentID)
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
		if len(items) > 0 && items[0].Priority != domain.PriorityHigh {
			t.Errorf("Priority: got %q, want high", items[0].Priority)
		}
	})

	t.Run("combined filters narrow correctly", func(t *testing.T) {
		// agent-x at stepAlpha: matches wi1 and wi3 (not wi2 which is at stepBeta)
		items, err := s.ListWorkItems(ctx, instanceID, stepAlpha, "agent-x", "")
		if err != nil {
			t.Fatalf("ListWorkItems: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("want 2 items for agent-x at stepAlpha, got %d", len(items))
		}
		for _, item := range items {
			if item.CurrentStepID != stepAlpha {
				t.Errorf("step filter violated: got %q", item.CurrentStepID)
			}
			if item.AssignedAgentID != "agent-x" {
				t.Errorf("agent filter violated: got %q", item.AssignedAgentID)
			}
		}
	})
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

func openTestStore(t *testing.T) domain.Store {
	t.Helper()
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_ProjectCRUD(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	voc := &domain.Vocabulary{
		ID: "voc_t1", Name: "t1",
		Events: []domain.VocabularyEvent{
			{ID: "ve_1", VocabularyID: "voc_t1", EventType: "task_assigned", MessageTemplate: "x"},
		},
	}
	if err := store.CreateVocabulary(ctx, voc); err != nil {
		t.Fatalf("CreateVocabulary: %v", err)
	}

	p := &domain.Project{
		ID: "prj_t1", Name: "flow", ChannelName: "#flow", VocabularyID: voc.ID,
	}
	if err := store.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := store.GetProject(ctx, p.ID)
	if err != nil || got.Name != "flow" {
		t.Fatalf("GetProject: got=%v err=%v", got, err)
	}

	byName, err := store.GetProjectByName(ctx, "flow")
	if err != nil || byName.ID != p.ID {
		t.Fatalf("GetProjectByName: got=%v err=%v", byName, err)
	}

	// Duplicate name is rejected.
	if err := store.CreateProject(ctx, &domain.Project{
		ID: "prj_t2", Name: "flow", ChannelName: "#flow2", VocabularyID: voc.ID,
	}); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists for duplicate name, got %v", err)
	}
}

func TestStore_BotCRUD(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	voc := &domain.Vocabulary{ID: "voc_b1", Name: "b1"}
	_ = store.CreateVocabulary(ctx, voc)
	_ = store.CreateProject(ctx, &domain.Project{
		ID: "prj_b1", Name: "b1", ChannelName: "#b1", VocabularyID: voc.ID,
	})
	b := &domain.Bot{
		ID: "bot_b1", ProjectID: "prj_b1",
		PassportAPIKeyHash: "deadbeef", PassportAPIKeyID: "pak_1",
		HiveRoleAssignments: []string{"developer", "reviewer"},
	}
	if err := store.CreateBot(ctx, b); err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if err := store.CreateBot(ctx, &domain.Bot{
		ID: "bot_b2", ProjectID: "prj_b1", PassportAPIKeyHash: "x", PassportAPIKeyID: "pak_2",
	}); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("expected one-bot-per-project conflict, got %v", err)
	}
	got, err := store.GetBotByProject(ctx, "prj_b1")
	if err != nil || got.ID != "bot_b1" || len(got.HiveRoleAssignments) != 2 {
		t.Fatalf("GetBotByProject: %v %v", got, err)
	}
}

func TestStore_AuditByProject(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	_ = store.RecordAuditEvent(ctx, &domain.AuditEvent{
		Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wf1", Project: "p1",
	})
	_ = store.RecordAuditEvent(ctx, &domain.AuditEvent{
		Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wf2", Project: "p2",
	})
	got, err := store.ListAuditEventsByProject(ctx, "p1")
	if err != nil || len(got) != 1 || got[0].AgentID != "a1" {
		t.Fatalf("ListAuditEventsByProject: %v err=%v", got, err)
	}
}

func seedTwoInstancesOneAgent(t *testing.T, store domain.Store, agentID string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		tplID := newID("tpl")
		stepID := newID("stp")
		instID := newID("ins")
		wiID := newID("wi")
		_ = store.CreateTemplate(ctx, &domain.WorkflowTemplate{
			ID: tplID, Name: fmt.Sprintf("tpl-%d", i), Version: 1,
			Steps: []domain.Step{{ID: stepID, TemplateID: tplID, Key: "s", Name: "S", Type: domain.StepTypeTask, Position: 0}},
		})
		_ = store.CreateInstance(ctx, &domain.WorkflowInstance{
			ID: instID, TemplateID: tplID, TemplateVersion: 1,
			TeamID: "t", Name: fmt.Sprintf("inst-%d", i), Status: domain.InstanceStatusActive,
		})
		_ = store.CreateWorkItem(ctx, &domain.WorkItem{
			ID: wiID, InstanceID: instID, Title: fmt.Sprintf("item-%d", i),
			CurrentStepID: stepID, AssignedAgentID: agentID, Priority: domain.PriorityNormal,
		})
	}
}

func TestStore_ListWorkItemsByAgent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedTwoInstancesOneAgent(t, store, "agent-x")
	items, err := store.ListWorkItemsByAgent(ctx, "agent-x")
	if err != nil {
		t.Fatalf("ListWorkItemsByAgent: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items across 2 instances, got %d", len(items))
	}
	for _, it := range items {
		if it.AssignedAgentID != "agent-x" {
			t.Errorf("item %s assigned to %s, want agent-x", it.ID, it.AssignedAgentID)
		}
	}
}
