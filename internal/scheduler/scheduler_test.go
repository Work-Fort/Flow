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
