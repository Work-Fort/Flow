// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
)

func (s *Store) CreateWorkItem(ctx context.Context, w *domain.WorkItem) error {
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	if w.Priority == "" {
		w.Priority = domain.PriorityNormal
	}
	fields := w.Fields
	if fields == nil {
		fields = json.RawMessage("{}")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO work_items (id, instance_id, title, description, current_step_id, assigned_agent_id, priority, fields, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.InstanceID, w.Title, w.Description, w.CurrentStepID, w.AssignedAgentID, string(w.Priority), string(fields), w.CreatedAt.UTC(), w.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: work_item %q", domain.ErrAlreadyExists, w.ID)
		}
		return fmt.Errorf("insert work_item: %w", err)
	}
	return nil
}

func (s *Store) GetWorkItem(ctx context.Context, id string) (*domain.WorkItem, error) {
	var w domain.WorkItem
	var fieldsStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, instance_id, title, description, current_step_id, assigned_agent_id, priority, fields, created_at, updated_at
		FROM work_items WHERE id = ?`, id,
	).Scan(&w.ID, &w.InstanceID, &w.Title, &w.Description, &w.CurrentStepID, &w.AssignedAgentID, &w.Priority, &fieldsStr, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: work_item %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get work_item: %w", err)
	}
	w.Fields = json.RawMessage(fieldsStr)

	// External links
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, work_item_id, service_type, adapter, external_id, url FROM external_links WHERE work_item_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("list external_links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var link domain.ExternalLink
		if err := rows.Scan(&link.ID, &link.WorkItemID, &link.ServiceType, &link.Adapter, &link.ExternalID, &link.URL); err != nil {
			return nil, fmt.Errorf("scan external_link: %w", err)
		}
		w.ExternalLinks = append(w.ExternalLinks, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate external_links: %w", err)
	}

	return &w, nil
}

func (s *Store) ListWorkItems(ctx context.Context, instanceID, stepID, agentID string, priority domain.Priority) ([]*domain.WorkItem, error) {
	query := `SELECT id, instance_id, title, description, current_step_id, assigned_agent_id, priority, fields, created_at, updated_at
		FROM work_items WHERE instance_id = ?`
	args := []any{instanceID}

	if stepID != "" {
		query += " AND current_step_id = ?"
		args = append(args, stepID)
	}
	if agentID != "" {
		query += " AND assigned_agent_id = ?"
		args = append(args, agentID)
	}
	if priority != "" {
		query += " AND priority = ?"
		args = append(args, string(priority))
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list work_items: %w", err)
	}
	defer rows.Close()

	var items []*domain.WorkItem
	for rows.Next() {
		var w domain.WorkItem
		var fieldsStr string
		if err := rows.Scan(&w.ID, &w.InstanceID, &w.Title, &w.Description, &w.CurrentStepID, &w.AssignedAgentID, &w.Priority, &fieldsStr, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan work_item: %w", err)
		}
		w.Fields = json.RawMessage(fieldsStr)
		items = append(items, &w)
	}
	return items, rows.Err()
}

func (s *Store) ListWorkItemsByAgent(ctx context.Context, agentID string) ([]*domain.WorkItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, instance_id, title, description, current_step_id, assigned_agent_id, priority, fields, created_at, updated_at
		FROM work_items
		WHERE assigned_agent_id = ?
		ORDER BY updated_at DESC, id ASC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("query work_items by agent: %w", err)
	}
	defer rows.Close()
	return scanWorkItems(rows)
}

func scanWorkItems(rows *sql.Rows) ([]*domain.WorkItem, error) {
	var items []*domain.WorkItem
	for rows.Next() {
		var w domain.WorkItem
		var fieldsStr string
		if err := rows.Scan(&w.ID, &w.InstanceID, &w.Title, &w.Description, &w.CurrentStepID, &w.AssignedAgentID, &w.Priority, &fieldsStr, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan work_item: %w", err)
		}
		w.Fields = json.RawMessage(fieldsStr)
		items = append(items, &w)
	}
	return items, rows.Err()
}

func (s *Store) UpdateWorkItem(ctx context.Context, w *domain.WorkItem) error {
	fields := w.Fields
	if fields == nil {
		fields = json.RawMessage("{}")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE work_items SET title = ?, description = ?, current_step_id = ?, assigned_agent_id = ?, priority = ?, fields = ?, updated_at = datetime('now')
		WHERE id = ?`,
		w.Title, w.Description, w.CurrentStepID, w.AssignedAgentID, string(w.Priority), string(fields), w.ID)
	if err != nil {
		return fmt.Errorf("update work_item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: work_item %q", domain.ErrNotFound, w.ID)
	}
	return nil
}

func (s *Store) RecordTransition(ctx context.Context, h *domain.TransitionHistory) error {
	if h.Timestamp.IsZero() {
		h.Timestamp = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO transition_history (id, work_item_id, from_step_id, to_step_id, transition_id, triggered_by, reason, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.WorkItemID, h.FromStepID, h.ToStepID, h.TransitionID, h.TriggeredBy, h.Reason, h.Timestamp.UTC())
	if err != nil {
		return fmt.Errorf("record transition: %w", err)
	}
	return nil
}

func (s *Store) GetTransitionHistory(ctx context.Context, workItemID string) ([]*domain.TransitionHistory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, work_item_id, from_step_id, to_step_id, transition_id, triggered_by, reason, timestamp
		FROM transition_history WHERE work_item_id = ? ORDER BY timestamp ASC`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("get transition_history: %w", err)
	}
	defer rows.Close()

	var history []*domain.TransitionHistory
	for rows.Next() {
		var h domain.TransitionHistory
		if err := rows.Scan(&h.ID, &h.WorkItemID, &h.FromStepID, &h.ToStepID, &h.TransitionID, &h.TriggeredBy, &h.Reason, &h.Timestamp); err != nil {
			return nil, fmt.Errorf("scan transition_history: %w", err)
		}
		history = append(history, &h)
	}
	return history, rows.Err()
}
