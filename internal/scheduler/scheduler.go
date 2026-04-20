// SPDX-License-Identifier: GPL-2.0-only

// Package scheduler implements the runtime-agnostic agent pool
// scheduler. Workflows call AcquireAgent / ReleaseAgent; the scheduler
// wraps Hive's claim/release endpoints, registers active claims for
// the lease renewer, and emits audit events.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Config holds dependencies and tuning knobs for a Scheduler. Zero
// values for tuning fields fall back to sensible defaults.
type Config struct {
	Hive  domain.HiveAgentClient // required
	Audit domain.AuditEventStore // required

	// Optional. All three nil → scheduler runs as today (audit-only).
	// All three non-nil → claim/release also fire vocabulary dispatch.
	Projects     domain.ProjectStore    // optional
	Vocabularies domain.VocabularyStore // optional, paired with Projects
	Dispatcher   domain.BotDispatcher   // optional, paired with the above

	// MaxClaimRetries is the number of ClaimAgent attempts before
	// AcquireAgent returns ErrPoolExhausted. Default 5.
	MaxClaimRetries int
	// BackoffBase is the base wait between claim retries. Actual wait is
	// BackoffBase * 2^(attempt-1) capped at BackoffMax. Default 100ms.
	BackoffBase time.Duration
	// BackoffMax caps exponential backoff. Default 5s.
	BackoffMax time.Duration
}

const (
	defaultMaxClaimRetries = 5
	defaultBackoffBase     = 100 * time.Millisecond
	defaultBackoffMax      = 5 * time.Second
)

// Scheduler is the concrete implementation of domain.Scheduler.
type Scheduler struct {
	hive         domain.HiveAgentClient
	audit        domain.AuditEventStore
	projects     domain.ProjectStore
	vocabularies domain.VocabularyStore
	dispatcher   domain.BotDispatcher

	maxRetries  int
	backoffBase time.Duration
	backoffMax  time.Duration

	mu     sync.RWMutex
	claims map[claimKey]*domain.AgentClaim
}

type claimKey struct {
	agentID    string
	workflowID string
}

// New constructs a Scheduler from Config. Panics if Hive or Audit is nil.
func New(cfg Config) *Scheduler {
	if cfg.Hive == nil {
		panic("scheduler: Hive is required")
	}
	if cfg.Audit == nil {
		panic("scheduler: Audit is required")
	}
	if cfg.MaxClaimRetries <= 0 {
		cfg.MaxClaimRetries = defaultMaxClaimRetries
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = defaultBackoffBase
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = defaultBackoffMax
	}
	return &Scheduler{
		hive:         cfg.Hive,
		audit:        cfg.Audit,
		projects:     cfg.Projects,
		vocabularies: cfg.Vocabularies,
		dispatcher:   cfg.Dispatcher,
		maxRetries:   cfg.MaxClaimRetries,
		backoffBase:  cfg.BackoffBase,
		backoffMax:   cfg.BackoffMax,
		claims:       make(map[claimKey]*domain.AgentClaim),
	}
}

// AcquireAgent implements domain.Scheduler.
func (s *Scheduler) AcquireAgent(ctx context.Context, role, project, workflowID string, leaseTTL time.Duration) (*domain.AgentClaim, error) {
	ttlSeconds := int(leaseTTL.Round(time.Second).Seconds())
	if ttlSeconds <= 0 {
		return nil, fmt.Errorf("scheduler: leaseTTL must be positive, got %v", leaseTTL)
	}

	var lastErr error
	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		ag, err := s.hive.ClaimAgent(ctx, role, project, workflowID, ttlSeconds)
		if err == nil {
			claim := &domain.AgentClaim{
				AgentID:        ag.ID,
				AgentName:      ag.Name,
				Role:           role,
				Project:        project,
				WorkflowID:     workflowID,
				LeaseExpiresAt: ag.LeaseExpiresAt,
			}
			s.register(claim)
			s.recordEvent(ctx, domain.AuditEventAgentClaimed, claim)
			s.postClaimDispatch(ctx, claim)
			log.Info("scheduler: claim succeeded",
				"agent_id", claim.AgentID, "workflow_id", workflowID,
				"role", role, "project", project)
			return claim, nil
		}
		if !errors.Is(err, domain.ErrPoolExhausted) {
			return nil, fmt.Errorf("scheduler: claim agent: %w", err)
		}
		lastErr = err
		log.Debug("scheduler: pool exhausted, backing off",
			"attempt", attempt, "role", role, "project", project)
		if attempt < s.maxRetries {
			if err := s.sleep(ctx, s.backoffFor(attempt)); err != nil {
				return nil, err
			}
		}
	}
	return nil, lastErr
}

// ReleaseAgent implements domain.Scheduler.
func (s *Scheduler) ReleaseAgent(ctx context.Context, claim *domain.AgentClaim) error {
	if claim == nil {
		return fmt.Errorf("scheduler: nil claim")
	}
	err := s.hive.ReleaseAgent(ctx, claim.AgentID, claim.WorkflowID)
	s.unregister(claim)
	if err != nil {
		log.Warn("scheduler: release failed",
			"agent_id", claim.AgentID, "workflow_id", claim.WorkflowID, "err", err)
		return fmt.Errorf("scheduler: release agent: %w", err)
	}
	s.recordEvent(ctx, domain.AuditEventAgentReleased, claim)
	s.postReleaseDispatch(ctx, claim)
	log.Info("scheduler: release succeeded",
		"agent_id", claim.AgentID, "workflow_id", claim.WorkflowID)
	return nil
}

// ActiveClaims implements domain.Scheduler.
func (s *Scheduler) ActiveClaims() []domain.AgentClaim {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.AgentClaim, 0, len(s.claims))
	for _, c := range s.claims {
		out = append(out, *c)
	}
	return out
}

// UpdateLease is called by the LeaseRenewer after a successful renew to
// keep the in-memory claim's LeaseExpiresAt current. Writes a
// lease_renewed audit event.
func (s *Scheduler) UpdateLease(ctx context.Context, agentID, workflowID string, newExpiry time.Time) {
	s.mu.Lock()
	c, ok := s.claims[claimKey{agentID: agentID, workflowID: workflowID}]
	if ok {
		c.LeaseExpiresAt = newExpiry
	}
	s.mu.Unlock()
	if ok {
		snap := *c
		s.recordEvent(ctx, domain.AuditEventLeaseRenewed, &snap)
	}
}

// HiveClient returns the HiveAgentClient the scheduler was built
// with. Used by the LeaseRenewer so daemon wiring doesn't need to
// thread the adapter through twice.
func (s *Scheduler) HiveClient() domain.HiveAgentClient { return s.hive }

// --- internal helpers ---

func (s *Scheduler) register(c *domain.AgentClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims[claimKey{agentID: c.AgentID, workflowID: c.WorkflowID}] = c
}

func (s *Scheduler) unregister(c *domain.AgentClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claims, claimKey{agentID: c.AgentID, workflowID: c.WorkflowID})
}

func (s *Scheduler) backoffFor(attempt int) time.Duration {
	d := s.backoffBase << (attempt - 1)
	if d <= 0 || d > s.backoffMax {
		return s.backoffMax
	}
	return d
}

func (s *Scheduler) sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Scheduler) recordEvent(ctx context.Context, ty domain.AuditEventType, c *domain.AgentClaim) {
	e := &domain.AuditEvent{
		Type:           ty,
		AgentID:        c.AgentID,
		AgentName:      c.AgentName,
		WorkflowID:     c.WorkflowID,
		Role:           c.Role,
		Project:        c.Project,
		LeaseExpiresAt: c.LeaseExpiresAt,
	}
	if err := s.audit.RecordAuditEvent(ctx, e); err != nil {
		// Audit failures are warnings, not errors — the scheduler's
		// primary duty (Hive CAS) already succeeded.
		log.Warn("scheduler: audit record failed",
			"event_type", ty, "agent_id", c.AgentID, "err", err)
	}
}

func (s *Scheduler) postClaimDispatch(ctx context.Context, claim *domain.AgentClaim) {
	if s.dispatcher == nil || s.projects == nil {
		return
	}
	p, err := s.projects.GetProjectByName(ctx, claim.Project)
	if err != nil {
		log.Debug("scheduler: no project for claim", "project", claim.Project, "err", err)
		return
	}
	if err := s.dispatcher.Dispatch(ctx, p.ID, "task_assigned", domain.VocabularyContext{
		AgentName: claim.AgentName,
		Role:      claim.Role,
		Payload: map[string]any{
			"agent_id":    claim.AgentID,
			"role":        claim.Role,
			"workflow_id": claim.WorkflowID,
		},
	}); err != nil {
		log.Warn("scheduler: dispatch task_assigned failed", "err", err)
	}
}

func (s *Scheduler) postReleaseDispatch(ctx context.Context, claim *domain.AgentClaim) {
	if s.dispatcher == nil || s.projects == nil || s.vocabularies == nil {
		return
	}
	p, err := s.projects.GetProjectByName(ctx, claim.Project)
	if err != nil {
		log.Debug("scheduler: no project for release", "project", claim.Project, "err", err)
		return
	}
	voc, err := s.vocabularies.GetVocabulary(ctx, p.VocabularyID)
	if err != nil {
		log.Warn("scheduler: vocab load failed for release", "vocab", p.VocabularyID, "err", err)
		return
	}
	eventType := voc.ReleaseEvent
	if eventType == "" {
		// DEFENSIVE FALLBACK ONLY — a vocabulary that declares
		// ReleaseEvent (every seeded one does) overrides this.
		// Do NOT add other event-name literals to scheduler code.
		eventType = "task_completed"
	}
	if err := s.dispatcher.Dispatch(ctx, p.ID, eventType, domain.VocabularyContext{
		AgentName: claim.AgentName,
		Role:      claim.Role,
		Payload: map[string]any{
			"agent_id":    claim.AgentID,
			"role":        claim.Role,
			"workflow_id": claim.WorkflowID,
		},
	}); err != nil {
		log.Warn("scheduler: dispatch release_event failed", "event", eventType, "err", err)
	}
}

// Compile-time assertion.
var _ domain.Scheduler = (*Scheduler)(nil)
