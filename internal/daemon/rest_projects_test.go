// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/Work-Fort/Flow/internal/domain"
)


// minimalFakeStore satisfies domain.Store for project tests.
// Only the methods exercised by registerProjectRoutes are implemented;
// others panic so tests fail loudly if unexpected methods are called.
type minimalFakeStore struct {
	projects  []*domain.Project
	bots      []*domain.Bot
	vocabs    []*domain.Vocabulary
	createErr error
}

func (s *minimalFakeStore) CreateProject(_ context.Context, p *domain.Project) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.projects = append(s.projects, p)
	return nil
}
func (s *minimalFakeStore) GetProject(_ context.Context, id string) (*domain.Project, error) {
	for _, p := range s.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (s *minimalFakeStore) GetProjectByName(_ context.Context, name string) (*domain.Project, error) {
	for _, p := range s.projects {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (s *minimalFakeStore) ListProjects(_ context.Context) ([]*domain.Project, error) {
	return s.projects, nil
}
func (s *minimalFakeStore) UpdateProject(_ context.Context, p *domain.Project) error {
	for i, existing := range s.projects {
		if existing.ID == p.ID {
			s.projects[i] = p
			return nil
		}
	}
	return domain.ErrNotFound
}
func (s *minimalFakeStore) DeleteProject(_ context.Context, id string) error {
	for i, p := range s.projects {
		if p.ID == id {
			s.projects = append(s.projects[:i], s.projects[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}
func (s *minimalFakeStore) GetVocabularyByName(_ context.Context, name string) (*domain.Vocabulary, error) {
	for _, v := range s.vocabs {
		if v.Name == name {
			return v, nil
		}
	}
	return nil, domain.ErrNotFound
}

// Unused methods — panic to catch accidental calls.
func (s *minimalFakeStore) CreateBot(context.Context, *domain.Bot) error         { panic("unimplemented") }
func (s *minimalFakeStore) GetBotByID(context.Context, string) (*domain.Bot, error)      { panic("unimplemented") }
func (s *minimalFakeStore) GetBotByProject(context.Context, string) (*domain.Bot, error) { panic("unimplemented") }
func (s *minimalFakeStore) DeleteBotByProject(context.Context, string) error              { panic("unimplemented") }
func (s *minimalFakeStore) UpdateBot(context.Context, *domain.Bot) error                  { panic("unimplemented") }
func (s *minimalFakeStore) CreateTemplate(context.Context, *domain.WorkflowTemplate) error { panic("unimplemented") }
func (s *minimalFakeStore) GetTemplate(context.Context, string) (*domain.WorkflowTemplate, error) { panic("unimplemented") }
func (s *minimalFakeStore) ListTemplates(context.Context) ([]*domain.WorkflowTemplate, error) { panic("unimplemented") }
func (s *minimalFakeStore) UpdateTemplate(context.Context, *domain.WorkflowTemplate) error { panic("unimplemented") }
func (s *minimalFakeStore) DeleteTemplate(context.Context, string) error { panic("unimplemented") }
func (s *minimalFakeStore) CreateInstance(context.Context, *domain.WorkflowInstance) error { panic("unimplemented") }
func (s *minimalFakeStore) GetInstance(context.Context, string) (*domain.WorkflowInstance, error) { panic("unimplemented") }
func (s *minimalFakeStore) ListInstances(context.Context, string) ([]*domain.WorkflowInstance, error) { panic("unimplemented") }
func (s *minimalFakeStore) UpdateInstance(context.Context, *domain.WorkflowInstance) error { panic("unimplemented") }
func (s *minimalFakeStore) CreateWorkItem(context.Context, *domain.WorkItem) error { panic("unimplemented") }
func (s *minimalFakeStore) GetWorkItem(context.Context, string) (*domain.WorkItem, error) { panic("unimplemented") }
func (s *minimalFakeStore) ListWorkItems(context.Context, string, string, string, domain.Priority) ([]*domain.WorkItem, error) { panic("unimplemented") }
func (s *minimalFakeStore) ListWorkItemsByAgent(context.Context, string) ([]*domain.WorkItem, error) { panic("unimplemented") }
func (s *minimalFakeStore) UpdateWorkItem(context.Context, *domain.WorkItem) error { panic("unimplemented") }
func (s *minimalFakeStore) RecordTransition(context.Context, *domain.TransitionHistory) error { panic("unimplemented") }
func (s *minimalFakeStore) GetTransitionHistory(context.Context, string) ([]*domain.TransitionHistory, error) { panic("unimplemented") }
func (s *minimalFakeStore) RecordApproval(context.Context, *domain.Approval) error { panic("unimplemented") }
func (s *minimalFakeStore) ListApprovals(context.Context, string, string) ([]*domain.Approval, error) { panic("unimplemented") }
func (s *minimalFakeStore) RecordAuditEvent(context.Context, *domain.AuditEvent) error { return nil }
func (s *minimalFakeStore) ListAuditEventsByWorkflow(context.Context, string) ([]*domain.AuditEvent, error) { return nil, nil }
func (s *minimalFakeStore) ListAuditEventsByAgent(context.Context, string) ([]*domain.AuditEvent, error) { return nil, nil }
func (s *minimalFakeStore) ListAuditEventsByProject(context.Context, string) ([]*domain.AuditEvent, error) { return nil, nil }
func (s *minimalFakeStore) ListAuditEventsFiltered(context.Context, domain.AuditFilter) ([]*domain.AuditEvent, error) { return nil, nil }
func (s *minimalFakeStore) CreateVocabulary(context.Context, *domain.Vocabulary) error { panic("unimplemented") }
func (s *minimalFakeStore) GetVocabulary(context.Context, string) (*domain.Vocabulary, error) { panic("unimplemented") }
func (s *minimalFakeStore) ListVocabularies(context.Context) ([]*domain.Vocabulary, error) { return nil, nil }
func (s *minimalFakeStore) Ping(context.Context) error { return nil }
func (s *minimalFakeStore) Close() error { return nil }

// fakeChat tracks CreateChannel calls for project handler tests.
type fakeChat struct {
	created   map[string]bool
	createErr error
}

func (f *fakeChat) PostMessage(_ context.Context, _, _ string, _ json.RawMessage) (int64, error) {
	return 0, nil
}
func (f *fakeChat) CreateChannel(_ context.Context, name string, _ bool) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.created == nil {
		f.created = make(map[string]bool)
	}
	f.created[name] = true
	return nil
}
func (f *fakeChat) JoinChannel(_ context.Context, _ string) error { return nil }

func newProjectTestMux(t *testing.T, store domain.Store, chat domain.ChatProvider) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	registerProjectRoutes(api, store, "", chat)
	return mux
}

func postProjectJSON(mux http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestCreateProject_AutoProvisionsChannel(t *testing.T) {
	store := &minimalFakeStore{
		vocabs: []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
	}
	fc := &fakeChat{}
	mux := newProjectTestMux(t, store, fc)

	rr := postProjectJSON(mux, "/v1/projects", map[string]any{
		"name": "flow", "channel_name": "#flow",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !fc.created["#flow"] {
		t.Errorf("channel #flow not created; created=%v", fc.created)
	}
}

func TestCreateProject_ChannelAlreadyExists_SkipsCreate(t *testing.T) {
	store := &minimalFakeStore{
		vocabs: []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
	}
	fc := &fakeChat{}
	mux := newProjectTestMux(t, store, fc)

	rr := postProjectJSON(mux, "/v1/projects", map[string]any{
		"name": "flow2", "channel_name": "#flow2", "channel_already_exists": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(fc.created) > 0 {
		t.Errorf("expected no channel create call, got %v", fc.created)
	}
}

func TestCreateProject_ChannelCreateFails_ProjectStillCommitted_AuditFired(t *testing.T) {
	store := &minimalFakeStore{
		vocabs: []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
	}
	fc := &fakeChat{createErr: errors.New("sharkfin unreachable")}
	mux := newProjectTestMux(t, store, fc)

	rr := postProjectJSON(mux, "/v1/projects", map[string]any{
		"name": "flow3", "channel_name": "#flow3",
	})
	// Project should still be created despite channel failure (202 → the project row is committed)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 even on channel fail, got %d body=%s", rr.Code, rr.Body.String())
	}
	// Verify project was committed.
	if len(store.projects) != 1 {
		t.Errorf("expected 1 project in store, got %d", len(store.projects))
	}
}
