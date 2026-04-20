// SPDX-License-Identifier: GPL-2.0-only
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
)

const projectCols = "id, name, description, template_id, channel_name, vocabulary_id, created_at, updated_at"

func (s *Store) CreateProject(ctx context.Context, p *domain.Project) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (`+projectCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.Name, p.Description, p.TemplateID, p.ChannelName, p.VocabularyID,
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: project %q", domain.ErrAlreadyExists, p.Name)
		}
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *Store) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	return s.scanProject(s.db.QueryRowContext(ctx,
		`SELECT `+projectCols+` FROM projects WHERE id = $1`, id))
}

func (s *Store) GetProjectByName(ctx context.Context, name string) (*domain.Project, error) {
	return s.scanProject(s.db.QueryRowContext(ctx,
		`SELECT `+projectCols+` FROM projects WHERE name = $1`, name))
}

func (s *Store) ListProjects(ctx context.Context) ([]*domain.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+projectCols+` FROM projects ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []*domain.Project
	for rows.Next() {
		p, err := s.scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, p *domain.Project) error {
	p.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name=$1, description=$2, template_id=$3, channel_name=$4, vocabulary_id=$5, updated_at=$6 WHERE id=$7`,
		p.Name, p.Description, p.TemplateID, p.ChannelName, p.VocabularyID, p.UpdatedAt, p.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: project %q", domain.ErrAlreadyExists, p.Name)
		}
		return fmt.Errorf("update project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: project %s", domain.ErrNotFound, p.ID)
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	var hasBot int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bots WHERE project_id = $1`, id).Scan(&hasBot); err != nil {
		return fmt.Errorf("count bots: %w", err)
	}
	if hasBot > 0 {
		return fmt.Errorf("%w: project %s", domain.ErrProjectHasBot, id)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: project %s", domain.ErrNotFound, id)
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func (s *Store) scanProject(r rowScanner) (*domain.Project, error) {
	var p domain.Project
	var created, updated time.Time
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.TemplateID, &p.ChannelName,
		&p.VocabularyID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: project", domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.CreatedAt = created
	p.UpdatedAt = updated
	return &p, nil
}

func (s *Store) scanProjectRow(rows *sql.Rows) (*domain.Project, error) {
	return s.scanProject(rows)
}
