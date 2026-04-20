// SPDX-License-Identifier: GPL-2.0-only
package postgres_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
)

func mustRecordAuditPG(t *testing.T, s domain.Store, e *domain.AuditEvent) {
	t.Helper()
	if err := s.RecordAuditEvent(context.Background(), e); err != nil {
		t.Fatalf("RecordAuditEvent: %v", err)
	}
}

func TestListAuditEventsFiltered_ByWorkItem_PG(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-1", Project: "p1"})
	mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentReleased, AgentID: "a1", WorkflowID: "wi-1", Project: "p1"})
	mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wi-2", Project: "p2"})

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{WorkflowID: "wi-1"})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
}

func TestListAuditEventsFiltered_ByProject_PG(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-1", Project: "proj-a"})
	mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wi-2", Project: "proj-b"})

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{Project: "proj-a"})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Project != "proj-a" {
		t.Errorf("project = %q, want proj-a", got[0].Project)
	}
}

func TestListAuditEventsFiltered_Limit_PG(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mustRecordAuditPG(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-limit-pg", Project: "p1"})
	}

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{WorkflowID: "wi-limit-pg", Limit: 3})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
}
