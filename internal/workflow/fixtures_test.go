// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// openStore opens a fresh in-memory SQLite store and registers a cleanup
// to close it when the test ends.
func openStore(t *testing.T) domain.Store {
	t.Helper()
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newSvc creates a workflow.Service backed by a fresh store.
func newSvc(t *testing.T) (*workflow.Service, domain.Store) {
	t.Helper()
	store := openStore(t)
	return workflow.New(store, nil), store
}

// tid generates a short prefixed test ID.
func tid(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}

// seedInstance creates a template+instance and returns their IDs. The
// template is created without steps/transitions — callers add those first.
func seedInstance(t *testing.T, store domain.Store, tmpl *domain.WorkflowTemplate) string {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateTemplate(ctx, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	instID := tid("ins")
	inst := &domain.WorkflowInstance{
		ID:              instID,
		TemplateID:      tmpl.ID,
		TemplateVersion: tmpl.Version,
		TeamID:          "team-test",
		Name:            "Test Instance",
		Status:          domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	return instID
}

// createItem creates a work item at the given step and returns it.
func createItem(t *testing.T, store domain.Store, instanceID, stepID string) *domain.WorkItem {
	t.Helper()
	ctx := context.Background()
	w := &domain.WorkItem{
		ID:            tid("wi"),
		InstanceID:    instanceID,
		Title:         "Test Item",
		CurrentStepID: stepID,
		Priority:      domain.PriorityNormal,
	}
	if err := store.CreateWorkItem(ctx, w); err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	got, err := store.GetWorkItem(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorkItem after create: %v", err)
	}
	return got
}

// --- fixture types ---

// twoStepFixture is a minimal 2-step task template:
//
//	StepA (pos=0) --[TransAtoB]--> StepB (pos=1)
type twoStepFixture struct {
	Store      domain.Store
	Svc        *workflow.Service
	TemplateID string
	StepA      string
	StepB      string
	TransAtoB  string
	InstanceID string
}

func seedTwoStep(t *testing.T) twoStepFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "TwoStep", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "step-a", Name: "Step A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "step-b", Name: "Step B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance", FromStepID: stepA, ToStepID: stepB},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return twoStepFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID, StepA: stepA, StepB: stepB,
		TransAtoB: transAB, InstanceID: instID,
	}
}

// guardedFixture is a 2-step template where the A→B transition has a CEL
// guard: item.priority == "high"
type guardedFixture struct {
	twoStepFixture
}

func seedGuarded(t *testing.T) guardedFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	transAB := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "Guarded", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "step-a", Name: "Step A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "step-b", Name: "Step B", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{
				ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Advance",
				FromStepID: stepA, ToStepID: stepB,
				Guard: `item.priority == "high"`,
			},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return guardedFixture{twoStepFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID, StepA: stepA, StepB: stepB,
		TransAtoB: transAB, InstanceID: instID,
	}}
}

// gateFixture is a 3-step template:
//
//	StepA (task, pos=0) --[TransAtoGate]--> GateStep (gate, pos=1, mode=any, required=2)
//	GateStep --[TransGateToC]--> StepC (task, pos=2)
//	GateStep --[TransGateToA]--> StepA  (rejection path)
type gateFixture struct {
	Store       domain.Store
	Svc         *workflow.Service
	TemplateID  string
	StepA       string
	GateStep    string
	StepC       string
	TransAtoGate  string
	TransGateToC  string
	TransGateToA  string
	InstanceID  string
}

func seedGate(t *testing.T) gateFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	gateStep := tid("stp")
	stepC := tid("stp")
	transAG := tid("tr")
	transGC := tid("tr")
	transGA := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "Gate", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "step-a", Name: "Step A", Type: domain.StepTypeTask, Position: 0},
			{
				ID: gateStep, TemplateID: tplID, Key: "gate", Name: "Gate",
				Type: domain.StepTypeGate, Position: 1,
				Approval: &domain.ApprovalConfig{
					Mode:              domain.ApprovalModeAny,
					RequiredApprovers: 2,
					ApproverRoleID:    "reviewer",
					RejectionStepID:   stepA,
				},
			},
			{ID: stepC, TemplateID: tplID, Key: "step-c", Name: "Step C", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transAG, TemplateID: tplID, Key: "a-to-gate", Name: "Submit", FromStepID: stepA, ToStepID: gateStep},
			{ID: transGC, TemplateID: tplID, Key: "gate-to-c", Name: "Approve", FromStepID: gateStep, ToStepID: stepC},
			{ID: transGA, TemplateID: tplID, Key: "gate-to-a", Name: "Reject", FromStepID: gateStep, ToStepID: stepA},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return gateFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID, StepA: stepA, GateStep: gateStep, StepC: stepC,
		TransAtoGate: transAG, TransGateToC: transGC, TransGateToA: transGA,
		InstanceID: instID,
	}
}

// cycleFixture is a 3-step template with a backward cycle:
//
//	StepA --[TransAtoB]--> StepB --[TransBtoC]--> StepC
//	StepB --[TransBtoA]--> StepA
type cycleFixture struct {
	Store                   domain.Store
	Svc                     *workflow.Service
	TemplateID              string
	StepA, StepB, StepC    string
	TransAtoB, TransBtoC, TransBtoA string
	InstanceID              string
}

func seedCycle(t *testing.T) cycleFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	stepC := tid("stp")
	transAB := tid("tr")
	transBC := tid("tr")
	transBA := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "Cycle", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
			{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Forward", FromStepID: stepA, ToStepID: stepB},
			{ID: transBC, TemplateID: tplID, Key: "b-to-c", Name: "Forward", FromStepID: stepB, ToStepID: stepC},
			{ID: transBA, TemplateID: tplID, Key: "b-to-a", Name: "Back", FromStepID: stepB, ToStepID: stepA},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return cycleFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID,
		StepA: stepA, StepB: stepB, StepC: stepC,
		TransAtoB: transAB, TransBtoC: transBC, TransBtoA: transBA,
		InstanceID: instID,
	}
}

// multiTransFixture is a 3-step template with two outgoing transitions from
// StepA:
//
//	StepA --[TransAtoB]--> StepB
//	StepA --[TransAtoC]--> StepC
type multiTransFixture struct {
	Store                domain.Store
	Svc                  *workflow.Service
	TemplateID           string
	StepA, StepB, StepC string
	TransAtoB, TransAtoC string
	InstanceID           string
}

func seedMultiTrans(t *testing.T) multiTransFixture {
	t.Helper()
	store := openStore(t)
	tplID := tid("tpl")
	stepA := tid("stp")
	stepB := tid("stp")
	stepC := tid("stp")
	transAB := tid("tr")
	transAC := tid("tr")

	tmpl := &domain.WorkflowTemplate{
		ID: tplID, Name: "MultiTrans", Version: 1,
		Steps: []domain.Step{
			{ID: stepA, TemplateID: tplID, Key: "a", Name: "A", Type: domain.StepTypeTask, Position: 0},
			{ID: stepB, TemplateID: tplID, Key: "b", Name: "B", Type: domain.StepTypeTask, Position: 1},
			{ID: stepC, TemplateID: tplID, Key: "c", Name: "C", Type: domain.StepTypeTask, Position: 2},
		},
		Transitions: []domain.Transition{
			{ID: transAB, TemplateID: tplID, Key: "a-to-b", Name: "Go to B", FromStepID: stepA, ToStepID: stepB},
			{ID: transAC, TemplateID: tplID, Key: "a-to-c", Name: "Go to C", FromStepID: stepA, ToStepID: stepC},
		},
	}
	instID := seedInstance(t, store, tmpl)
	return multiTransFixture{
		Store: store, Svc: workflow.New(store, nil),
		TemplateID: tplID,
		StepA: stepA, StepB: stepB, StepC: stepC,
		TransAtoB: transAB, TransAtoC: transAC,
		InstanceID: instID,
	}
}
