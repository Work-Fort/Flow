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

// --- identity test helpers ---

func newTestStore(t *testing.T) domain.Store {
	t.Helper()
	return openStore(t)
}

// roleTemplateFixture holds IDs from a template built for role-check tests.
type roleTemplateFixture struct {
	tmpl         *domain.WorkflowTemplate
	firstStepID  string
	secondStepID string
	transID      string
}

// buildTemplateWithRoleRequirement creates and stores a 2-step template whose
// only transition requires roleID.
func buildTemplateWithRoleRequirement(t *testing.T, store domain.Store, roleID string) roleTemplateFixture {
	t.Helper()
	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "RoleRequired", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "Step A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "Step B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance",
				FromStepID: stepA, ToStepID: stepB,
				RequiredRoleID: roleID,
			},
		},
	}
	ctx := context.Background()
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	return roleTemplateFixture{tmpl: tmpl, firstStepID: stepA, secondStepID: stepB, transID: transAB}
}

// gateTemplateFixture holds IDs from a gate template built for approver-role tests.
type gateTemplateFixture struct {
	tmpl        *domain.WorkflowTemplate
	taskStepID  string
	gateStepID  string
	doneStepID  string
}

// buildGateTemplateWithApproverRole creates and stores a 3-step template with
// a gate step whose ApprovalConfig.ApproverRoleID is set to roleID.
func buildGateTemplateWithApproverRole(t *testing.T, store domain.Store, roleID string) gateTemplateFixture {
	t.Helper()
	tplID := tid("tpl")
	taskStep := tid("stp")
	gateStep := tid("stp")
	doneStep := tid("stp")
	transTaskGate := tid("tr")
	transGateDone := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "GateApproverRole", Version: 1,
		Steps: []domain.Step{
			{ID: taskStep, TemplateID: tplID, Key: "task", Name: "Task", Type: domain.StepTypeTask, Position: 0},
			{
				ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 1,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 1,
					ApproverRoleID:    roleID,
				},
			},
			{ID: doneStep, TemplateID: tplID, Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transTaskGate, TemplateID: tplID, Key: "task-to-gate", Name: "Submit", FromStepID: taskStep, ToStepID: gateStep},
			{ID: transGateDone, TemplateID: tplID, Key: "gate-to-done", Name: "Approve", FromStepID: gateStep, ToStepID: doneStep},
		},
	}
	ctx := context.Background()
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	return gateTemplateFixture{tmpl: tmpl, taskStepID: taskStep, gateStepID: gateStep, doneStepID: doneStep}
}

func buildInstance(t *testing.T, store domain.Store, templateID string) *domain.WorkflowInstance {
	t.Helper()
	ctx := context.Background()
	inst := &domain.WorkflowInstance{
		ID:              tid("ins"),
		TemplateID:      templateID,
		TemplateVersion: 1,
		TeamID:          "team-test",
		Name:            "Test Instance",
		Status:          domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	return inst
}

func buildWorkItem(t *testing.T, store domain.Store, instanceID, stepID string) *domain.WorkItem {
	t.Helper()
	return createItem(t, store, instanceID, stepID)
}

func firstStepID(f roleTemplateFixture) string  { return f.firstStepID }
func firstTransitionID(f roleTemplateFixture) string { return f.transID }
func gateStepID(f gateTemplateFixture) string   { return f.gateStepID }

// --- tests ---

func TestTransitionItem_NilIdentity_SkipsRoleCheck(t *testing.T) {
	store := newTestStore(t)
	svc := workflow.New(store, nil) // nil identity: no checks

	f := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, f.tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(f))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(f),
		ActorAgentID: "agent-no-roles",
	})
	if err != nil {
		t.Fatalf("expected nil err with nil identity, got %v", err)
	}
}

func TestTransitionItem_MatchingRole_Allowed(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-reviewer": {"role-reviewer"},
	}}
	svc := workflow.New(store, identity)

	f := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, f.tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(f))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(f),
		ActorAgentID: "agent-reviewer",
	})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}

func TestTransitionItem_MissingRole_Denied(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-developer": {"role-developer"}, // has developer, not reviewer
	}}
	svc := workflow.New(store, identity)

	f := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, f.tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(f))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(f),
		ActorAgentID: "agent-developer",
	})
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestApproveItem_WrongRole_Denied(t *testing.T) {
	store := newTestStore(t)
	identity := &stubIdentity{roles: map[string][]string{
		"agent-developer": {"role-developer"},
	}}
	svc := workflow.New(store, identity)

	f := buildGateTemplateWithApproverRole(t, store, "role-pm")
	inst := buildInstance(t, store, f.tmpl.ID)
	// Place item at the gate step.
	item := buildWorkItem(t, store, inst.ID, gateStepID(f))

	_, err := svc.ApproveItem(context.Background(), workflow.ApproveRequest{
		WorkItemID: item.ID,
		AgentID:    "agent-developer",
	})
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestTransitionItem_HiveDown_PropagatesError(t *testing.T) {
	store := newTestStore(t)
	hiveErr := errors.New("connection refused")
	identity := &stubIdentity{err: hiveErr}
	svc := workflow.New(store, identity)

	f := buildTemplateWithRoleRequirement(t, store, "role-reviewer")
	inst := buildInstance(t, store, f.tmpl.ID)
	item := buildWorkItem(t, store, inst.ID, firstStepID(f))

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   item.ID,
		TransitionID: firstTransitionID(f),
		ActorAgentID: "agent-x",
	})
	if err == nil {
		t.Fatal("expected error when Hive is down")
	}
	if errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatal("Hive errors must not masquerade as ErrPermissionDenied")
	}
}
