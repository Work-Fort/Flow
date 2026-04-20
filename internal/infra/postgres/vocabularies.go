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

func (s *Store) CreateVocabulary(ctx context.Context, v *domain.Vocabulary) error {
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO vocabularies (id, name, description, release_event, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		v.ID, v.Name, v.Description, v.ReleaseEvent, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: vocabulary %q", domain.ErrAlreadyExists, v.Name)
		}
		return fmt.Errorf("insert vocabulary: %w", err)
	}
	for _, e := range v.Events {
		keysJSON, _ := json.Marshal(e.MetadataKeys)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO vocabulary_events (id, vocabulary_id, event_type, message_template, metadata_keys) VALUES ($1,$2,$3,$4,$5::jsonb)`,
			e.ID, v.ID, e.EventType, e.MessageTemplate, string(keysJSON)); err != nil {
			return fmt.Errorf("insert vocabulary_event %q: %w", e.EventType, err)
		}
	}
	return tx.Commit()
}

func (s *Store) GetVocabulary(ctx context.Context, id string) (*domain.Vocabulary, error) {
	return s.loadVocabulary(ctx, "id", id)
}

func (s *Store) GetVocabularyByName(ctx context.Context, name string) (*domain.Vocabulary, error) {
	return s.loadVocabulary(ctx, "name", name)
}

func (s *Store) loadVocabulary(ctx context.Context, col, val string) (*domain.Vocabulary, error) {
	var v domain.Vocabulary
	var created, updated time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, release_event, created_at, updated_at FROM vocabularies WHERE `+col+` = $1`, val).Scan(
		&v.ID, &v.Name, &v.Description, &v.ReleaseEvent, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: vocabulary", domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get vocabulary: %w", err)
	}
	v.CreatedAt = created
	v.UpdatedAt = updated

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, vocabulary_id, event_type, message_template, metadata_keys FROM vocabulary_events WHERE vocabulary_id = $1 ORDER BY event_type`, v.ID)
	if err != nil {
		return nil, fmt.Errorf("list vocab events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e domain.VocabularyEvent
		var keys string
		if err := rows.Scan(&e.ID, &e.VocabularyID, &e.EventType, &e.MessageTemplate, &keys); err != nil {
			return nil, fmt.Errorf("scan vocab event: %w", err)
		}
		_ = json.Unmarshal([]byte(keys), &e.MetadataKeys)
		v.Events = append(v.Events, e)
	}
	return &v, rows.Err()
}

// ListVocabularies loads every vocabulary + its events in a single
// pass, grouped in-memory to avoid N+1 queries.
func (s *Store) ListVocabularies(ctx context.Context) ([]*domain.Vocabulary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, release_event, created_at, updated_at FROM vocabularies ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list vocabularies: %w", err)
	}
	defer rows.Close()
	byID := map[string]*domain.Vocabulary{}
	var out []*domain.Vocabulary
	for rows.Next() {
		var v domain.Vocabulary
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.ReleaseEvent, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan vocab: %w", err)
		}
		vp := &v
		byID[v.ID] = vp
		out = append(out, vp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	eventRows, err := s.db.QueryContext(ctx,
		`SELECT id, vocabulary_id, event_type, message_template, metadata_keys FROM vocabulary_events ORDER BY vocabulary_id, event_type`)
	if err != nil {
		return nil, fmt.Errorf("list all vocab events: %w", err)
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var e domain.VocabularyEvent
		var keys string
		if err := eventRows.Scan(&e.ID, &e.VocabularyID, &e.EventType, &e.MessageTemplate, &keys); err != nil {
			return nil, fmt.Errorf("scan vocab event: %w", err)
		}
		_ = json.Unmarshal([]byte(keys), &e.MetadataKeys)
		if vp := byID[e.VocabularyID]; vp != nil {
			vp.Events = append(vp.Events, e)
		}
	}
	return out, eventRows.Err()
}
