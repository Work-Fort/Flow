// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
)

func (s *Store) RecordApproval(ctx context.Context, a *domain.Approval) error {
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals (id, work_item_id, step_id, agent_id, decision, comment, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.WorkItemID, a.StepID, a.AgentID, string(a.Decision), a.Comment, a.Timestamp.UTC())
	if err != nil {
		return fmt.Errorf("record approval: %w", err)
	}
	return nil
}

func (s *Store) ListApprovals(ctx context.Context, workItemID, stepID string) ([]*domain.Approval, error) {
	query := `SELECT id, work_item_id, step_id, agent_id, decision, comment, timestamp
		FROM approvals WHERE work_item_id = ?`
	args := []any{workItemID}

	if stepID != "" {
		query += " AND step_id = ?"
		args = append(args, stepID)
	}
	query += " ORDER BY timestamp ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*domain.Approval
	for rows.Next() {
		var a domain.Approval
		if err := rows.Scan(&a.ID, &a.WorkItemID, &a.StepID, &a.AgentID, &a.Decision, &a.Comment, &a.Timestamp); err != nil {
			return nil, fmt.Errorf("scan approval: %w", err)
		}
		approvals = append(approvals, &a)
	}
	return approvals, rows.Err()
}
