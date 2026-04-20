// SPDX-License-Identifier: GPL-2.0-only
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
)

func (s *Store) CreateInstance(ctx context.Context, i *domain.WorkflowInstance) error {
	now := time.Now().UTC()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	if i.UpdatedAt.IsZero() {
		i.UpdatedAt = now
	}
	if i.Status == "" {
		i.Status = domain.InstanceStatusActive
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_instances (id, template_id, template_version, team_id, project_id, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		i.ID, i.TemplateID, i.TemplateVersion, i.TeamID,
		sql.NullString{String: i.ProjectID, Valid: i.ProjectID != ""},
		i.Name, string(i.Status), i.CreatedAt.UTC(), i.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: instance %q", domain.ErrAlreadyExists, i.ID)
		}
		return fmt.Errorf("insert instance: %w", err)
	}

	for _, ic := range i.IntegrationConfigs {
		cfg := ic.Config
		if cfg == nil {
			cfg = json.RawMessage("{}")
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO integration_configs (id, instance_id, adapter_type, config) VALUES ($1, $2, $3, $4)`,
			ic.ID, ic.InstanceID, ic.AdapterType, string(cfg))
		if err != nil {
			return fmt.Errorf("insert integration_config: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetInstance(ctx context.Context, id string) (*domain.WorkflowInstance, error) {
	var i domain.WorkflowInstance
	var projectID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, template_id, template_version, team_id, project_id, name, status, created_at, updated_at
		FROM workflow_instances WHERE id = $1`, id,
	).Scan(&i.ID, &i.TemplateID, &i.TemplateVersion, &i.TeamID, &projectID, &i.Name, &i.Status, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: instance %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	i.ProjectID = projectID.String
	return &i, nil
}

func (s *Store) ListInstances(ctx context.Context, teamID string) ([]*domain.WorkflowInstance, error) {
	var rows *sql.Rows
	var err error
	if teamID == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, template_id, template_version, team_id, project_id, name, status, created_at, updated_at
			FROM workflow_instances ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, template_id, template_version, team_id, project_id, name, status, created_at, updated_at
			FROM workflow_instances WHERE team_id = $1 ORDER BY created_at DESC`, teamID)
	}
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	var instances []*domain.WorkflowInstance
	for rows.Next() {
		i, err := s.scanInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, i)
	}
	return instances, rows.Err()
}

func (s *Store) ListInstancesByProject(ctx context.Context, projectID string) ([]*domain.WorkflowInstance, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, template_id, template_version, team_id, project_id, name, status, created_at, updated_at
		FROM workflow_instances WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list instances by project: %w", err)
	}
	defer rows.Close()
	var out []*domain.WorkflowInstance
	for rows.Next() {
		i, err := s.scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

type instanceRowScanner interface{ Scan(...any) error }

func (s *Store) scanInstance(r instanceRowScanner) (*domain.WorkflowInstance, error) {
	var i domain.WorkflowInstance
	var projectID sql.NullString
	if err := r.Scan(&i.ID, &i.TemplateID, &i.TemplateVersion, &i.TeamID, &projectID,
		&i.Name, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan instance: %w", err)
	}
	i.ProjectID = projectID.String
	return &i, nil
}

func (s *Store) UpdateInstance(ctx context.Context, i *domain.WorkflowInstance) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE workflow_instances SET name = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		i.Name, string(i.Status), i.ID)
	if err != nil {
		return fmt.Errorf("update instance: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: instance %q", domain.ErrNotFound, i.ID)
	}
	return nil
}
