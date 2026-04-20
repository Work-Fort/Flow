// SPDX-License-Identifier: GPL-2.0-only
package bot_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Work-Fort/Flow/internal/bot"
	"github.com/Work-Fort/Flow/internal/domain"
)

// fakeStore is a minimal in-memory store for dispatcher tests.
// Only Project + Vocabulary queries are implemented; everything else panics.
type fakeStore struct {
	projects     map[string]*domain.Project
	vocabularies map[string]*domain.Vocabulary
}

func (f *fakeStore) GetProject(_ context.Context, id string) (*domain.Project, error) {
	if p, ok := f.projects[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
}

func (f *fakeStore) GetVocabulary(_ context.Context, id string) (*domain.Vocabulary, error) {
	if v, ok := f.vocabularies[id]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("%w: vocabulary %s", domain.ErrNotFound, id)
}

// Unimplemented store methods — panic if called unexpectedly.
func (f *fakeStore) CreateTemplate(_ context.Context, _ *domain.WorkflowTemplate) error {
	panic("unimplemented")
}
func (f *fakeStore) GetTemplate(_ context.Context, _ string) (*domain.WorkflowTemplate, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListTemplates(_ context.Context) ([]*domain.WorkflowTemplate, error) {
	panic("unimplemented")
}
func (f *fakeStore) UpdateTemplate(_ context.Context, _ *domain.WorkflowTemplate) error {
	panic("unimplemented")
}
func (f *fakeStore) DeleteTemplate(_ context.Context, _ string) error { panic("unimplemented") }
func (f *fakeStore) CreateInstance(_ context.Context, _ *domain.WorkflowInstance) error {
	panic("unimplemented")
}
func (f *fakeStore) GetInstance(_ context.Context, _ string) (*domain.WorkflowInstance, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListInstances(_ context.Context, _ string) ([]*domain.WorkflowInstance, error) {
	panic("unimplemented")
}
func (f *fakeStore) UpdateInstance(_ context.Context, _ *domain.WorkflowInstance) error {
	panic("unimplemented")
}
func (f *fakeStore) CreateWorkItem(_ context.Context, _ *domain.WorkItem) error {
	panic("unimplemented")
}
func (f *fakeStore) GetWorkItem(_ context.Context, _ string) (*domain.WorkItem, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListWorkItems(_ context.Context, _, _, _ string, _ domain.Priority) ([]*domain.WorkItem, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListWorkItemsByAgent(_ context.Context, _ string) ([]*domain.WorkItem, error) {
	panic("unimplemented")
}
func (f *fakeStore) UpdateWorkItem(_ context.Context, _ *domain.WorkItem) error {
	panic("unimplemented")
}
func (f *fakeStore) RecordTransition(_ context.Context, _ *domain.TransitionHistory) error {
	panic("unimplemented")
}
func (f *fakeStore) GetTransitionHistory(_ context.Context, _ string) ([]*domain.TransitionHistory, error) {
	panic("unimplemented")
}
func (f *fakeStore) RecordApproval(_ context.Context, _ *domain.Approval) error {
	panic("unimplemented")
}
func (f *fakeStore) ListApprovals(_ context.Context, _, _ string) ([]*domain.Approval, error) {
	panic("unimplemented")
}
func (f *fakeStore) RecordAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	panic("unimplemented")
}
func (f *fakeStore) ListAuditEventsByWorkflow(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListAuditEventsByAgent(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListAuditEventsByProject(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	panic("unimplemented")
}

func (f *fakeStore) ListAuditEventsFiltered(_ context.Context, _ domain.AuditFilter) ([]*domain.AuditEvent, error) {
	panic("unimplemented")
}
func (f *fakeStore) GetProjectByName(_ context.Context, _ string) (*domain.Project, error) {
	panic("unimplemented")
}
func (f *fakeStore) CreateProject(_ context.Context, _ *domain.Project) error { panic("unimplemented") }
func (f *fakeStore) ListProjects(_ context.Context) ([]*domain.Project, error) {
	panic("unimplemented")
}
func (f *fakeStore) UpdateProject(_ context.Context, _ *domain.Project) error { panic("unimplemented") }
func (f *fakeStore) DeleteProject(_ context.Context, _ string) error          { panic("unimplemented") }
func (f *fakeStore) CreateBot(_ context.Context, _ *domain.Bot) error         { panic("unimplemented") }
func (f *fakeStore) GetBotByID(_ context.Context, _ string) (*domain.Bot, error) {
	panic("unimplemented")
}
func (f *fakeStore) GetBotByProject(_ context.Context, _ string) (*domain.Bot, error) {
	panic("unimplemented")
}
func (f *fakeStore) DeleteBotByProject(_ context.Context, _ string) error { panic("unimplemented") }
func (f *fakeStore) CreateVocabulary(_ context.Context, _ *domain.Vocabulary) error {
	panic("unimplemented")
}
func (f *fakeStore) GetVocabularyByName(_ context.Context, _ string) (*domain.Vocabulary, error) {
	panic("unimplemented")
}
func (f *fakeStore) ListVocabularies(_ context.Context) ([]*domain.Vocabulary, error) {
	panic("unimplemented")
}
func (f *fakeStore) Ping(_ context.Context) error { return nil }
func (f *fakeStore) Close() error                 { return nil }

// stubChat records PostMessage calls.
type stubChat struct {
	messages []stubMsg
}

type stubMsg struct {
	Channel  string
	Content  string
	Metadata json.RawMessage
}

func (s *stubChat) PostMessage(_ context.Context, ch, content string, meta json.RawMessage) (int64, error) {
	s.messages = append(s.messages, stubMsg{Channel: ch, Content: content, Metadata: meta})
	return int64(len(s.messages)), nil
}

func (s *stubChat) CreateChannel(_ context.Context, _ string, _ bool) error { return nil }
func (s *stubChat) JoinChannel(_ context.Context, _ string) error           { return nil }

func newTestStore() (*fakeStore, *domain.Vocabulary) {
	voc := &domain.Vocabulary{
		ID:           "voc_test",
		Name:         "test-vocab",
		ReleaseEvent: "task_completed",
		Events: []domain.VocabularyEvent{
			{
				ID:              "ve_test_assigned",
				VocabularyID:    "voc_test",
				EventType:       "task_assigned",
				MessageTemplate: `Task assigned: {{.WorkItem.Title}} → {{.AgentName}} ({{.Role}})`,
				MetadataKeys:    []string{"agent_id", "role"},
			},
		},
	}
	prj := &domain.Project{
		ID:           "prj_test",
		Name:         "test-project",
		ChannelName:  "#test-channel",
		VocabularyID: voc.ID,
	}
	store := &fakeStore{
		projects:     map[string]*domain.Project{prj.ID: prj},
		vocabularies: map[string]*domain.Vocabulary{voc.ID: voc},
	}
	return store, voc
}

func TestDispatcher_KnownEvent_RendersAndPosts(t *testing.T) {
	store, _ := newTestStore()
	chat := &stubChat{}
	d := bot.New(store, chat)

	err := d.Dispatch(context.Background(), "prj_test", "task_assigned", domain.VocabularyContext{
		WorkItem:  &domain.WorkItem{Title: "Fix the bug"},
		AgentName: "agent-1",
		Role:      "developer",
		Payload:   map[string]any{"agent_id": "a_1", "role": "developer"},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(chat.messages) != 1 {
		t.Fatalf("expected 1 post, got %d", len(chat.messages))
	}
	msg := chat.messages[0]
	if msg.Channel != "#test-channel" {
		t.Errorf("channel = %q, want #test-channel", msg.Channel)
	}
	want := "Task assigned: Fix the bug → agent-1 (developer)"
	if msg.Content != want {
		t.Errorf("content = %q, want %q", msg.Content, want)
	}
	var meta map[string]any
	if err := json.Unmarshal(msg.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["agent_id"] != "a_1" {
		t.Errorf("metadata agent_id = %v", meta["agent_id"])
	}
}

func TestDispatcher_UnknownEvent_ReturnsErrEventNotInVocabulary(t *testing.T) {
	store, _ := newTestStore()
	chat := &stubChat{}
	d := bot.New(store, chat)

	err := d.Dispatch(context.Background(), "prj_test", "nonexistent_event", domain.VocabularyContext{})
	if !errors.Is(err, domain.ErrEventNotInVocabulary) {
		t.Errorf("err = %v, want ErrEventNotInVocabulary", err)
	}
}

func TestDispatcher_NilChat_ReturnsNil(t *testing.T) {
	store, _ := newTestStore()
	d := bot.New(store, nil)

	if err := d.Dispatch(context.Background(), "prj_test", "task_assigned", domain.VocabularyContext{}); err != nil {
		t.Errorf("expected nil with nil chat, got %v", err)
	}
}
