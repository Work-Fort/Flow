// SPDX-License-Identifier: GPL-2.0-only
package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

func TestRenewer_RenewsEveryActiveClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)

	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
			{agent: &domain.HiveAgent{ID: "a_002", Name: "agent-2", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})

	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-a", time.Minute); err != nil {
		t.Fatalf("AcquireAgent wf-a: %v", err)
	}
	if _, err := sch.AcquireAgent(ctx, "reviewer", "flow", "wf-b", time.Minute); err != nil {
		t.Fatalf("AcquireAgent wf-b: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch,
		Hive:      hive,
		Tick:      tickCh,
		LeaseTTL:  time.Minute,
	})
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	// Send three explicit ticks; assert exactly 2 (claims) * 3 (ticks)
	// = 6 renew calls. Deterministic regardless of CI scheduling.
	for i := 0; i < 3; i++ {
		tickCh <- time.Now()
	}
	// Give the renewer one scheduling slice to drain the last tick
	// before we cancel; this is bounded, not racy — the renewer's
	// processing loop is synchronous within a tick.
	r.WaitIdle()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewer did not exit within 1 s of cancel")
	}

	hive.mu.Lock()
	defer hive.mu.Unlock()
	if hive.renewCalls != 6 {
		t.Errorf("renew calls: got %d, want 6", hive.renewCalls)
	}
}

func TestRenewer_DropsClaimOnMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
		},
		renewErr: domain.ErrWorkflowMismatch,
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})
	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-1", time.Minute); err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch, Hive: hive, Tick: tickCh, LeaseTTL: time.Minute,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); r.Run(ctx) }()

	tickCh <- time.Now()
	r.WaitIdle()
	cancel()
	wg.Wait()

	if len(sch.ActiveClaims()) != 0 {
		t.Errorf("ActiveClaims after mismatch: got %d, want 0", len(sch.ActiveClaims()))
	}
}

// TestRenewer_WaitIdleRaceUnderLoad exercises the WaitIdle/signalIdle
// pairing under repeated tick-then-wait cycles. With the original
// close-and-replace idleCh implementation, this test would hang for
// the test timeout when signalIdle ran before WaitIdle snapshotted the
// channel. The counter-based implementation must complete deterministically.
func TestRenewer_WaitIdleRaceUnderLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audit := newAuditStore(t)
	expiry := time.Now().UTC().Add(2 * time.Minute)
	hive := &fakeHive{
		claimResponses: []claimResp{
			{agent: &domain.HiveAgent{ID: "a_001", Name: "agent-1", LeaseExpiresAt: expiry}},
		},
	}

	sch := scheduler.New(scheduler.Config{Hive: hive, Audit: audit})
	if _, err := sch.AcquireAgent(ctx, "developer", "flow", "wf-load", time.Minute); err != nil {
		t.Fatalf("AcquireAgent: %v", err)
	}

	tickCh := make(chan time.Time)
	r := scheduler.NewLeaseRenewer(scheduler.RenewerConfig{
		Scheduler: sch, Hive: hive, Tick: tickCh, LeaseTTL: time.Minute,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); r.Run(ctx) }()

	const iterations = 200
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < iterations; i++ {
			tickCh <- time.Now()
			r.WaitIdle()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tick/WaitIdle loop hung — WaitIdle race regression")
	}

	cancel()
	wg.Wait()

	hive.mu.Lock()
	defer hive.mu.Unlock()
	if hive.renewCalls != iterations {
		t.Errorf("renew calls: got %d, want %d", hive.renewCalls, iterations)
	}
}
