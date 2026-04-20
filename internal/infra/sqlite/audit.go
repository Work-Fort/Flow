// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
)

const auditCols = "id, occurred_at, event_type, agent_id, agent_name, workflow_id, role, project, lease_expires_at, payload"

func newAuditID() string {
	return "ae_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
}

// RecordAuditEvent inserts a new audit event. Populates ID and
// OccurredAt on the caller's struct when either is zero-valued.
func (s *Store) RecordAuditEvent(ctx context.Context, e *domain.AuditEvent) error {
	if e.ID == "" {
		e.ID = newAuditID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	var lease sql.NullTime
	if !e.LeaseExpiresAt.IsZero() {
		lease = sql.NullTime{Time: e.LeaseExpiresAt.UTC(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (`+auditCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.OccurredAt.UTC(), string(e.Type),
		e.AgentID, e.AgentName, e.WorkflowID, e.Role, e.Project,
		lease, string(payload))
	if err != nil {
		return fmt.Errorf("insert audit_events: %w", err)
	}
	return nil
}

// ListAuditEventsByWorkflow returns events for a workflow, oldest first.
func (s *Store) ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditCols+`
		FROM audit_events
		WHERE workflow_id = ?
		ORDER BY occurred_at ASC, id ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("query audit_events by workflow: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

// ListAuditEventsByProject returns events for a project (by project name), oldest first.
func (s *Store) ListAuditEventsByProject(ctx context.Context, project string) ([]*domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditCols+`
		FROM audit_events
		WHERE project = ?
		ORDER BY occurred_at ASC, id ASC`, project)
	if err != nil {
		return nil, fmt.Errorf("query audit_events by project: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

// ListAuditEventsByAgent returns events for an agent, oldest first.
func (s *Store) ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditCols+`
		FROM audit_events
		WHERE agent_id = ?
		ORDER BY occurred_at ASC, id ASC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("query audit_events by agent: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

// ListAuditEventsFiltered returns events matching all non-zero fields in f.
// Builds the WHERE clause incrementally; the planner picks the most-selective
// single-column index. Limit 0 = no limit.
func (s *Store) ListAuditEventsFiltered(ctx context.Context, f domain.AuditFilter) ([]*domain.AuditEvent, error) {
	query := "SELECT " + auditCols + " FROM audit_events WHERE 1=1"
	var args []any
	if f.Project != "" {
		query += " AND project = ?"
		args = append(args, f.Project)
	}
	if f.WorkflowID != "" {
		query += " AND workflow_id = ?"
		args = append(args, f.WorkflowID)
	}
	if f.AgentID != "" {
		query += " AND agent_id = ?"
		args = append(args, f.AgentID)
	}
	if f.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, f.EventType)
	}
	if !f.Since.IsZero() {
		query += " AND occurred_at >= ?"
		args = append(args, f.Since.UTC())
	}
	if !f.Until.IsZero() {
		query += " AND occurred_at <= ?"
		args = append(args, f.Until.UTC())
	}
	query += " ORDER BY occurred_at ASC, id ASC"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, f.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit_events filtered: %w", err)
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func scanAuditEvents(rows *sql.Rows) ([]*domain.AuditEvent, error) {
	var out []*domain.AuditEvent
	for rows.Next() {
		var (
			e       domain.AuditEvent
			typ     string
			lease   sql.NullTime
			payload string
		)
		if err := rows.Scan(
			&e.ID, &e.OccurredAt, &typ,
			&e.AgentID, &e.AgentName, &e.WorkflowID, &e.Role, &e.Project,
			&lease, &payload,
		); err != nil {
			return nil, fmt.Errorf("scan audit_events: %w", err)
		}
		e.Type = domain.AuditEventType(typ)
		if lease.Valid {
			e.LeaseExpiresAt = lease.Time
		}
		if payload != "" && payload != "{}" {
			e.Payload = json.RawMessage(payload)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
