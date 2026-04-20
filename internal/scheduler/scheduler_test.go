// SPDX-License-Identifier: GPL-2.0-only
package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

// fakeHive implements domain.HiveAgentClient with scripted responses.
type fakeHive struct {
	mu sync.Mutex

	claimResponses []claimResp
	claimCalls     int
	releaseCalls   int
	renewCalls     int
	releaseErr     error
	renewErr       error
}

type claimResp struct {
	agent *domain.HiveAgent
	err   error
}

func (f *fakeHive) ClaimAgent(_ context.Context, _, _, _ string, _ int) (*domain.HiveAgent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.claimCalls
	f.claimCalls++
	if idx >= len(f.claimResponses) {
		return nil, errors.New("fakeHive: out of scripted responses")
	}
	r := f.claimResponses[idx]
	return r.agent, r.err
}

func (f *fakeHive) ReleaseAgent(_ context.Context, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	return f.releaseErr
}

func (f *fakeHive) RenewAgentLease(_ context.Context, _, _ string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewCalls++
	return f.renewErr
}

func (f *fakeHive) ListAgents(_ context.Context, _ domain.HiveAgentFilter) ([]domain.HiveAgentRecord, error) {
	return nil, nil
}

// noopAuditStore is a zero-cost domain.AuditEventStore for tests that
// exercise scheduler/renewer mechanics rather than audit semantics.
// RecordAuditEvent is a no-op; the List* methods return empty slices.
// Use this in place of newAuditStore(t) when the test does not read
// audit events back — it removes per-call SQLite I/O from the hot path.
type noopAuditStore struct{}

func (noopAuditStore) RecordAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	return nil
}

func (noopAuditStore) ListAuditEventsByWorkflow(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (noopAuditStore) ListAuditEventsByAgent(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (noopAuditStore) ListAuditEventsByProject(_ context.Context, _ string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (noopAuditStore) ListAuditEventsFiltered(_ context.Context, _ domain.AuditFilter) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func newAuditStore(t *testing.T) domain.AuditEventStore {
	t.Helper()
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestScheduler_AcquireReleaseRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	expiry := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_003", Name: "agent-3", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "reviewer", "flow", "wf-1", 2*time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}
	if claim.AgentID != "a_003" || claim.WorkflowID != "wf-1" {
		t.Errorf("claim: got %+v", claim)
	}
	if len(sch.ActiveClaims()) != 1 {
		t.Fatalf("ActiveClaims after acquire: got %d, want 1", len(sch.ActiveClaims()))
	}

	events, _ := audit.ListAuditEventsByWorkflow(ctx, "wf-1")
	if len(events) != 1 || events[0].Type != domain.AuditEventAgentClaimed {
		t.Errorf("audit after acquire: got %+v", events)
	}

	if err := sch.ReleaseAgent(ctx, claim); err != nil {
		t.Fatalf("ReleaseAgent: %v", err)
	}
	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims after release: should be empty")
	}
	if hive.releaseCalls != 1 {
		t.Errorf("hive release calls: got %d, want 1", hive.releaseCalls)
	}

	events, _ = audit.ListAuditEventsByWorkflow(ctx, "wf-1")
	if len(events) != 2 {
		t.Fatalf("audit after release: got %d events, want 2", len(events))
	}
	if events[1].Type != domain.AuditEventAgentReleased {
		t.Errorf("second event: got %q, want agent_released", events[1].Type)
	}
}

func TestScheduler_AcquireRetriesOnPoolExhausted(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	expiry := time.Now().UTC().Add(time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
			{agent: &domain.HiveAgent{ID: "a_007", Name: "agent-7", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-2", time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent should eventually succeed: %v", err)
	}
	if claim.AgentID != "a_007" {
		t.Errorf("claim: got %+v", claim)
	}
	if hive.claimCalls != 3 {
		t.Errorf("claim calls: got %d, want 3", hive.claimCalls)
	}
}

func TestScheduler_AcquireReturnsErrPoolExhaustedAfterAllRetries(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	hive := &fakeHive{
		claimResponses: []claimResp{
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
			{err: domain.ErrPoolExhausted},
		},
	}
	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		MaxClaimRetries: 3,
		BackoffBase:     1 * time.Millisecond,
	})

	_, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-3", time.Minute)
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
	if hive.claimCalls != 3 {
		t.Errorf("claim calls: got %d, want 3", hive.claimCalls)
	}
	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims should be empty after failed acquire")
	}
}

// --- dispatcher integration tests ---

type fakeProjectStore struct {
	byName map[string]*domain.Project
}

func (f *fakeProjectStore) GetProjectByName(_ context.Context, name string) (*domain.Project, error) {
	if p, ok := f.byName[name]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeProjectStore) CreateProject(_ context.Context, _ *domain.Project) error { return nil }
func (f *fakeProjectStore) GetProject(_ context.Context, _ string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeProjectStore) ListProjects(_ context.Context) ([]*domain.Project, error) {
	return nil, nil
}
func (f *fakeProjectStore) UpdateProject(_ context.Context, _ *domain.Project) error { return nil }
func (f *fakeProjectStore) DeleteProject(_ context.Context, _ string) error          { return nil }

type fakeVocabStore struct {
	byID map[string]*domain.Vocabulary
}

func (f *fakeVocabStore) GetVocabulary(_ context.Context, id string) (*domain.Vocabulary, error) {
	if v, ok := f.byID[id]; ok {
		return v, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeVocabStore) GetVocabularyByName(_ context.Context, _ string) (*domain.Vocabulary, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeVocabStore) CreateVocabulary(_ context.Context, _ *domain.Vocabulary) error { return nil }
func (f *fakeVocabStore) ListVocabularies(_ context.Context) ([]*domain.Vocabulary, error) {
	return nil, nil
}

type capturingDispatcher struct {
	calls []dispatchCall
}

type dispatchCall struct {
	projectID string
	eventType string
	ctx       domain.VocabularyContext
}

func (d *capturingDispatcher) Dispatch(_ context.Context, projectID, eventType string, ctx domain.VocabularyContext) error {
	d.calls = append(d.calls, dispatchCall{projectID: projectID, eventType: eventType, ctx: ctx})
	return nil
}

func TestScheduler_ClaimFiresTaskAssignedDispatch(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	vocab := &domain.Vocabulary{
		ID:           "voc_test",
		Name:         "test",
		ReleaseEvent: "task_completed",
		Events: []domain.VocabularyEvent{
			{EventType: "task_assigned", MessageTemplate: "assigned"},
		},
	}
	prj := &domain.Project{ID: "prj_1", Name: "my-project", VocabularyID: vocab.ID}
	projects := &fakeProjectStore{byName: map[string]*domain.Project{prj.Name: prj}}
	vocabs := &fakeVocabStore{byID: map[string]*domain.Vocabulary{vocab.ID: vocab}}
	disp := &capturingDispatcher{}

	expiry := time.Now().UTC().Add(time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_1", Name: "agent-1", LeaseExpiresAt: expiry}},
		},
	}
	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		Projects:        projects,
		Vocabularies:    vocabs,
		Dispatcher:      disp,
		MaxClaimRetries: 1,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "developer", "my-project", "wf-disp-1", time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("dispatch calls after claim: got %d, want 1", len(disp.calls))
	}
	if disp.calls[0].eventType != "task_assigned" {
		t.Errorf("event type = %q, want task_assigned", disp.calls[0].eventType)
	}
	if disp.calls[0].ctx.AgentName != "agent-1" {
		t.Errorf("agent name = %q, want agent-1", disp.calls[0].ctx.AgentName)
	}
	_ = claim
}

func TestScheduler_ReleaseFiresVocabReleaseEvent(t *testing.T) {
	ctx := context.Background()
	audit := newAuditStore(t)

	// Use a vocab whose ReleaseEvent is "bug_resolved", NOT "task_completed",
	// so the test catches a hard-coded SDLC fallback.
	vocab := &domain.Vocabulary{
		ID:           "voc_bug",
		Name:         "bug-tracker",
		ReleaseEvent: "bug_resolved",
		Events: []domain.VocabularyEvent{
			{EventType: "task_assigned", MessageTemplate: "assigned"},
			{EventType: "bug_resolved", MessageTemplate: "resolved"},
		},
	}
	prj := &domain.Project{ID: "prj_2", Name: "bug-project", VocabularyID: vocab.ID}
	projects := &fakeProjectStore{byName: map[string]*domain.Project{prj.Name: prj}}
	vocabs := &fakeVocabStore{byID: map[string]*domain.Vocabulary{vocab.ID: vocab}}
	disp := &capturingDispatcher{}

	expiry := time.Now().UTC().Add(time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_2", Name: "agent-2", LeaseExpiresAt: expiry}},
		},
	}
	sch := scheduler.New(scheduler.Config{
		Hive:            hive,
		Audit:           audit,
		Projects:        projects,
		Vocabularies:    vocabs,
		Dispatcher:      disp,
		MaxClaimRetries: 1,
		BackoffBase:     1 * time.Millisecond,
	})

	claim, err := sch.AcquireAgent(ctx, "developer", "bug-project", "wf-bug-1", time.Minute)
	if err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}
	if err := sch.ReleaseAgent(ctx, claim); err != nil {
		t.Fatalf("ReleaseAgent: %v", err)
	}

	// Two dispatches: task_assigned on claim, bug_resolved on release.
	if len(disp.calls) != 2 {
		t.Fatalf("dispatch calls: got %d, want 2 (claim+release)", len(disp.calls))
	}
	if disp.calls[1].eventType != "bug_resolved" {
		t.Errorf("release event type = %q, want bug_resolved (not task_completed)", disp.calls[1].eventType)
	}
}
