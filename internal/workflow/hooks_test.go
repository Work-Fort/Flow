// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

func TestTransitionItem_FiresChatHook(t *testing.T) {
	store := newTestStore(t)
	chat := &stubChat{}
	svc := workflow.New(store, nil).WithChat(chat)

	tmpl := &domain.WorkflowTemplate{
		ID:   "tmpl_hook",
		Name: "Hook Test",
		Steps: []domain.Step{
			{ID: "s1", TemplateID: "tmpl_hook", Key: "open", Name: "Open", Type: domain.StepTypeTask, Position: 0},
			{ID: "s2", TemplateID: "tmpl_hook", Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: "tr1", TemplateID: "tmpl_hook", Key: "close", Name: "Close", FromStepID: "s1", ToStepID: "s2"},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{
				ID:           "hook1",
				TemplateID:   "tmpl_hook",
				TransitionID: "tr1",
				Event:        "transition",
				AdapterType:  "chat",
				Action:       "post_message",
				Config:       json.RawMessage(`{"channel":"general","template":"{{item.title}} moved to Done"}`),
			},
		},
	}
	if err := store.CreateTemplate(context.Background(), tmpl); err != nil {
		t.Fatal(err)
	}

	inst := &domain.WorkflowInstance{
		ID:         "inst_hook",
		TemplateID: "tmpl_hook",
		TeamID:     "team1",
		Name:       "Hook Instance",
		Status:     domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}

	item := &domain.WorkItem{
		ID:            "wi_hook",
		InstanceID:    "inst_hook",
		Title:         "My Task",
		CurrentStepID: "s1",
		Priority:      domain.PriorityNormal,
	}
	if err := store.CreateWorkItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   "wi_hook",
		TransitionID: "tr1",
		ActorAgentID: "agent1",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}

	if len(chat.messages) != 1 {
		t.Fatalf("expected 1 chat message, got %d", len(chat.messages))
	}
	if chat.messages[0].Channel != "general" {
		t.Errorf("channel = %q, want %q", chat.messages[0].Channel, "general")
	}
	if chat.messages[0].Content != "My Task moved to Done" {
		t.Errorf("content = %q, want %q", chat.messages[0].Content, "My Task moved to Done")
	}
}

func TestTransitionItem_NilChat_NoNotification(t *testing.T) {
	store := newTestStore(t)
	// No chat provider — must not panic.
	svc := workflow.New(store, nil)

	tmpl := &domain.WorkflowTemplate{
		ID:   "tmpl_nilchat",
		Name: "Nil Chat Test",
		Steps: []domain.Step{
			{ID: "s1", TemplateID: "tmpl_nilchat", Key: "open", Name: "Open", Type: domain.StepTypeTask, Position: 0},
			{ID: "s2", TemplateID: "tmpl_nilchat", Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: "tr1", TemplateID: "tmpl_nilchat", Key: "close", Name: "Close", FromStepID: "s1", ToStepID: "s2"},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{
				ID:           "hook1",
				TemplateID:   "tmpl_nilchat",
				TransitionID: "tr1",
				Event:        "transition",
				AdapterType:  "chat",
				Action:       "post_message",
				Config:       json.RawMessage(`{"channel":"general","template":"{{item.title}} moved"}`),
			},
		},
	}
	if err := store.CreateTemplate(context.Background(), tmpl); err != nil {
		t.Fatal(err)
	}
	inst := &domain.WorkflowInstance{
		ID: "inst_nilchat", TemplateID: "tmpl_nilchat", TeamID: "team1",
		Name: "Nil Chat Instance", Status: domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	item := &domain.WorkItem{
		ID: "wi_nilchat", InstanceID: "inst_nilchat", Title: "A Task",
		CurrentStepID: "s1", Priority: domain.PriorityNormal,
	}
	if err := store.CreateWorkItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   "wi_nilchat",
		TransitionID: "tr1",
		ActorAgentID: "agent1",
	})
	if err != nil {
		t.Fatalf("TransitionItem with nil chat: %v", err)
	}
}
