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

const botCols = "id, project_id, passport_api_key_hash, passport_api_key_id, hive_role_assignments, created_at, updated_at"

func (s *Store) CreateBot(ctx context.Context, b *domain.Bot) error {
	now := time.Now().UTC()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	rolesJSON, _ := json.Marshal(b.HiveRoleAssignments)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bots (`+botCols+`) VALUES (?,?,?,?,?,?,?)`,
		b.ID, b.ProjectID, b.PassportAPIKeyHash, b.PassportAPIKeyID,
		string(rolesJSON), b.CreatedAt, b.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: bot for project %s", domain.ErrAlreadyExists, b.ProjectID)
		}
		return fmt.Errorf("insert bot: %w", err)
	}
	return nil
}

func (s *Store) GetBotByID(ctx context.Context, id string) (*domain.Bot, error) {
	var b domain.Bot
	var roles string
	var created, updated time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT `+botCols+` FROM bots WHERE id = ?`, id).Scan(
		&b.ID, &b.ProjectID, &b.PassportAPIKeyHash, &b.PassportAPIKeyID,
		&roles, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: bot %s", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get bot by id: %w", err)
	}
	_ = json.Unmarshal([]byte(roles), &b.HiveRoleAssignments)
	b.CreatedAt = created
	b.UpdatedAt = updated
	return &b, nil
}

func (s *Store) GetBotByProject(ctx context.Context, projectID string) (*domain.Bot, error) {
	var b domain.Bot
	var roles string
	var created, updated time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT `+botCols+` FROM bots WHERE project_id = ?`, projectID).Scan(
		&b.ID, &b.ProjectID, &b.PassportAPIKeyHash, &b.PassportAPIKeyID,
		&roles, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID)
	}
	if err != nil {
		return nil, fmt.Errorf("get bot: %w", err)
	}
	_ = json.Unmarshal([]byte(roles), &b.HiveRoleAssignments)
	b.CreatedAt = created
	b.UpdatedAt = updated
	return &b, nil
}

func (s *Store) UpdateBot(ctx context.Context, b *domain.Bot) error {
	b.UpdatedAt = time.Now().UTC()
	rolesJSON, _ := json.Marshal(b.HiveRoleAssignments)
	res, err := s.db.ExecContext(ctx,
		`UPDATE bots SET passport_api_key_hash=?, passport_api_key_id=?, hive_role_assignments=?, updated_at=? WHERE id=?`,
		b.PassportAPIKeyHash, b.PassportAPIKeyID, string(rolesJSON), b.UpdatedAt, b.ID)
	if err != nil {
		return fmt.Errorf("update bot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: bot %s", domain.ErrNotFound, b.ID)
	}
	return nil
}

func (s *Store) DeleteBotByProject(ctx context.Context, projectID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bots WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("delete bot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID)
	}
	return nil
}
