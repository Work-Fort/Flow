// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func TestAuditEvent_RoundTrip(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	lease := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)

	e := &domain.AuditEvent{
		Type:           domain.AuditEventAgentClaimed,
		AgentID:        "a_003",
		AgentName:      "agent-3",
		WorkflowID:     "wf-117",
		Role:           "reviewer",
		Project:        "flow",
		LeaseExpiresAt: lease,
	}
	if err := s.RecordAuditEvent(ctx, e); err != nil {
		t.Fatalf("RecordAuditEvent: %v", err)
	}
	if e.ID == "" {
		t.Errorf("RecordAuditEvent should populate ID, got empty")
	}
	if e.OccurredAt.IsZero() {
		t.Errorf("RecordAuditEvent should populate OccurredAt, got zero")
	}

	byWF, err := s.ListAuditEventsByWorkflow(ctx, "wf-117")
	if err != nil {
		t.Fatalf("ListAuditEventsByWorkflow: %v", err)
	}
	if len(byWF) != 1 {
		t.Fatalf("want 1 event by workflow, got %d", len(byWF))
	}
	if byWF[0].Type != domain.AuditEventAgentClaimed {
		t.Errorf("Type: got %q, want agent_claimed", byWF[0].Type)
	}
	if !byWF[0].LeaseExpiresAt.Equal(lease) {
		t.Errorf("LeaseExpiresAt round-trip: got %v, want %v", byWF[0].LeaseExpiresAt, lease)
	}

	byAgent, err := s.ListAuditEventsByAgent(ctx, "a_003")
	if err != nil {
		t.Fatalf("ListAuditEventsByAgent: %v", err)
	}
	if len(byAgent) != 1 {
		t.Errorf("want 1 event by agent, got %d", len(byAgent))
	}
}

func TestAuditEvent_OrderedByOccurredAt(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i, ty := range []domain.AuditEventType{
		domain.AuditEventAgentClaimed,
		domain.AuditEventLeaseRenewed,
		domain.AuditEventAgentReleased,
	} {
		e := &domain.AuditEvent{
			OccurredAt: base.Add(time.Duration(i) * time.Second),
			Type:       ty,
			AgentID:    "a_003", WorkflowID: "wf-200",
		}
		if err := s.RecordAuditEvent(ctx, e); err != nil {
			t.Fatalf("RecordAuditEvent %s: %v", ty, err)
		}
	}

	events, _ := s.ListAuditEventsByWorkflow(ctx, "wf-200")
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	if events[0].Type != domain.AuditEventAgentClaimed ||
		events[1].Type != domain.AuditEventLeaseRenewed ||
		events[2].Type != domain.AuditEventAgentReleased {
		t.Errorf("wrong order: got %v / %v / %v",
			events[0].Type, events[1].Type, events[2].Type)
	}
}
