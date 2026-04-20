// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func mustRecordAuditSQLite(t *testing.T, s *sqlite.Store, e *domain.AuditEvent) {
	t.Helper()
	if err := s.RecordAuditEvent(context.Background(), e); err != nil {
		t.Fatalf("RecordAuditEvent: %v", err)
	}
}

func TestListAuditEventsFiltered_ByWorkItem_SQLite(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-1", Project: "p1"})
	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentReleased, AgentID: "a1", WorkflowID: "wi-1", Project: "p1"})
	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wi-2", Project: "p2"})

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{WorkflowID: "wi-1"})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
}

func TestListAuditEventsFiltered_ByProject_SQLite(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-1", Project: "proj-a"})
	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a2", WorkflowID: "wi-2", Project: "proj-b"})

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

func TestListAuditEventsFiltered_Limit_SQLite(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-limit", Project: "p1"})
	}

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{WorkflowID: "wi-limit", Limit: 3})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
}

func TestListAuditEventsFiltered_NoFilter_ReturnsAll_SQLite(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentClaimed, AgentID: "a1", WorkflowID: "wi-a", Project: "p1"})
	mustRecordAuditSQLite(t, s, &domain.AuditEvent{Type: domain.AuditEventAgentReleased, AgentID: "a2", WorkflowID: "wi-b", Project: "p2"})

	got, err := s.ListAuditEventsFiltered(ctx, domain.AuditFilter{})
	if err != nil {
		t.Fatalf("ListAuditEventsFiltered: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("got %d events, want >= 2", len(got))
	}
}
