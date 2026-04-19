// SPDX-License-Identifier: GPL-2.0-only
package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// RenewerConfig configures a LeaseRenewer. Zero values for Interval and
// LeaseTTL get sensible defaults. Tick is optional — when nil, the
// renewer creates its own time.NewTicker(Interval). Tests pass a
// hand-driven channel to make timing deterministic.
type RenewerConfig struct {
	Scheduler *Scheduler             // required
	Hive      domain.HiveAgentClient // required
	Interval  time.Duration          // default 30s; ignored when Tick != nil
	LeaseTTL  time.Duration          // default 2m
	Tick      <-chan time.Time       // optional, for tests
}

const (
	defaultRenewInterval = 30 * time.Second
	defaultRenewTTL      = 2 * time.Minute
)

// LeaseRenewer is a background goroutine that keeps every claim the
// live Flow process holds alive by calling Hive's renew endpoint every
// Interval until its context is cancelled.
type LeaseRenewer struct {
	sch      *Scheduler
	hive     domain.HiveAgentClient
	interval time.Duration
	ttl      time.Duration
	tick     <-chan time.Time

	idleMu   sync.Mutex
	idleCond *sync.Cond
	ticks    uint64 // number of completed renewOnce calls; monotonic
}

// NewLeaseRenewer constructs a LeaseRenewer.
func NewLeaseRenewer(cfg RenewerConfig) *LeaseRenewer {
	if cfg.Scheduler == nil {
		panic("renewer: Scheduler is required")
	}
	if cfg.Hive == nil {
		panic("renewer: Hive is required")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultRenewInterval
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = defaultRenewTTL
	}
	r := &LeaseRenewer{
		sch:      cfg.Scheduler,
		hive:     cfg.Hive,
		interval: cfg.Interval,
		ttl:      cfg.LeaseTTL,
		tick:     cfg.Tick,
	}
	r.idleCond = sync.NewCond(&r.idleMu)
	return r
}

// Run ticks every Interval (or each value sent on the test Tick) and
// renews every claim currently held by the Scheduler. Returns when
// ctx is cancelled.
func (r *LeaseRenewer) Run(ctx context.Context) {
	tickCh := r.tick
	if tickCh == nil {
		t := time.NewTicker(r.interval)
		defer t.Stop()
		tickCh = t.C
	}
	for {
		select {
		case <-ctx.Done():
			log.Debug("renewer: shutting down")
			return
		case <-tickCh:
			r.renewOnce(ctx)
			r.signalIdle()
		}
	}
}

// WaitIdle blocks until the renewer completes at least one renewOnce
// after the call begins. Used by tests after sending a manual tick to
// wait for the renewer to drain it. Production code does not call this.
func (r *LeaseRenewer) WaitIdle() {
	r.idleMu.Lock()
	defer r.idleMu.Unlock()
	start := r.ticks
	for r.ticks == start {
		r.idleCond.Wait()
	}
}

func (r *LeaseRenewer) signalIdle() {
	r.idleMu.Lock()
	r.ticks++
	r.idleCond.Broadcast()
	r.idleMu.Unlock()
}

func (r *LeaseRenewer) renewOnce(ctx context.Context) {
	ttlSeconds := int(r.ttl.Round(time.Second).Seconds())
	newExpiry := time.Now().UTC().Add(r.ttl)
	for _, c := range r.sch.ActiveClaims() {
		err := r.hive.RenewAgentLease(ctx, c.AgentID, c.WorkflowID, ttlSeconds)
		switch {
		case err == nil:
			r.sch.UpdateLease(ctx, c.AgentID, c.WorkflowID, newExpiry)
		case errors.Is(err, domain.ErrWorkflowMismatch), errors.Is(err, domain.ErrNotFound):
			log.Warn("renewer: claim gone, dropping",
				"agent_id", c.AgentID, "workflow_id", c.WorkflowID, "err", err)
			claim := c
			r.sch.unregister(&claim)
		default:
			log.Warn("renewer: renew failed, will retry next tick",
				"agent_id", c.AgentID, "workflow_id", c.WorkflowID, "err", err)
		}
	}
}
