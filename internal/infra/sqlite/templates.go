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

func (s *Store) CreateTemplate(ctx context.Context, t *domain.WorkflowTemplate) error {
	// Validate that all RejectionStepIDs in gate steps refer to steps in this template.
	stepIDs := make(map[string]struct{}, len(t.Steps))
	for i := range t.Steps {
		stepIDs[t.Steps[i].ID] = struct{}{}
	}
	for i := range t.Steps {
		step := &t.Steps[i]
		if step.Approval != nil && step.Approval.RejectionStepID != "" {
			if _, ok := stepIDs[step.Approval.RejectionStepID]; !ok {
				return fmt.Errorf("%w: rejection_step_id %q on step %q is not a step in this template",
					domain.ErrNotFound, step.Approval.RejectionStepID, step.Key)
			}
		}
	}

	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		"INSERT INTO workflow_templates (id, name, description, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.ID, t.Name, t.Description, t.Version, t.CreatedAt.UTC(), t.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: template %q", domain.ErrAlreadyExists, t.Name)
		}
		return fmt.Errorf("insert template: %w", err)
	}

	for _, step := range t.Steps {
		var approvalMode sql.NullString
		var approvalRequired sql.NullInt64
		var approvalApproverRoleID sql.NullString
		var approvalRejectionStepID sql.NullString

		if step.Approval != nil {
			approvalMode = sql.NullString{String: string(step.Approval.Mode), Valid: true}
			approvalRequired = sql.NullInt64{Int64: int64(step.Approval.RequiredApprovers), Valid: true}
			approvalApproverRoleID = sql.NullString{String: step.Approval.ApproverRoleID, Valid: true}
			if step.Approval.RejectionStepID != "" {
				approvalRejectionStepID = sql.NullString{String: step.Approval.RejectionStepID, Valid: true}
			}
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO steps (id, template_id, key, name, type, position,
			approval_mode, approval_required, approval_approver_role_id, approval_rejection_step_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			step.ID, step.TemplateID, step.Key, step.Name, string(step.Type), step.Position,
			approvalMode, approvalRequired, approvalApproverRoleID, approvalRejectionStepID)
		if err != nil {
			return fmt.Errorf("insert step %q: %w", step.Key, err)
		}
	}

	for _, tr := range t.Transitions {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO transitions (id, template_id, key, name, from_step_id, to_step_id, guard, required_role_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			tr.ID, tr.TemplateID, tr.Key, tr.Name, tr.FromStepID, tr.ToStepID, tr.Guard, tr.RequiredRoleID)
		if err != nil {
			return fmt.Errorf("insert transition %q: %w", tr.Key, err)
		}
	}

	for _, rm := range t.RoleMappings {
		actionsJSON, _ := json.Marshal(rm.AllowedActions)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO role_mappings (id, template_id, step_id, role_id, allowed_actions)
			VALUES (?, ?, ?, ?, ?)`,
			rm.ID, rm.TemplateID, rm.StepID, rm.RoleID, string(actionsJSON))
		if err != nil {
			return fmt.Errorf("insert role_mapping: %w", err)
		}
	}

	for _, h := range t.IntegrationHooks {
		cfg := h.Config
		if cfg == nil {
			cfg = json.RawMessage("{}")
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO integration_hooks (id, template_id, transition_id, event, adapter_type, action, config)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			h.ID, h.TemplateID, h.TransitionID, h.Event, h.AdapterType, h.Action, string(cfg))
		if err != nil {
			return fmt.Errorf("insert integration_hook: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetTemplate(ctx context.Context, id string) (*domain.WorkflowTemplate, error) {
	var t domain.WorkflowTemplate
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, description, version, created_at, updated_at FROM workflow_templates WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: template %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}

	// Steps
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, template_id, key, name, type, position,
		approval_mode, approval_required, approval_approver_role_id, approval_rejection_step_id
		FROM steps WHERE template_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var step domain.Step
		var approvalMode sql.NullString
		var approvalRequired sql.NullInt64
		var approvalApproverRoleID sql.NullString
		var approvalRejectionStepID sql.NullString
		if err := rows.Scan(&step.ID, &step.TemplateID, &step.Key, &step.Name, &step.Type, &step.Position,
			&approvalMode, &approvalRequired, &approvalApproverRoleID, &approvalRejectionStepID); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		if approvalMode.Valid {
			step.Approval = &domain.ApprovalConfig{
				Mode:              domain.ApprovalMode(approvalMode.String),
				RequiredApprovers: int(approvalRequired.Int64),
				ApproverRoleID:    approvalApproverRoleID.String,
				RejectionStepID:   approvalRejectionStepID.String,
			}
		}
		t.Steps = append(t.Steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate steps: %w", err)
	}

	// Transitions
	trows, err := s.db.QueryContext(ctx,
		`SELECT id, template_id, key, name, from_step_id, to_step_id, guard, required_role_id
		FROM transitions WHERE template_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("list transitions: %w", err)
	}
	defer trows.Close()
	for trows.Next() {
		var tr domain.Transition
		if err := trows.Scan(&tr.ID, &tr.TemplateID, &tr.Key, &tr.Name, &tr.FromStepID, &tr.ToStepID, &tr.Guard, &tr.RequiredRoleID); err != nil {
			return nil, fmt.Errorf("scan transition: %w", err)
		}
		t.Transitions = append(t.Transitions, tr)
	}
	if err := trows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transitions: %w", err)
	}

	// Role mappings
	rmrows, err := s.db.QueryContext(ctx,
		`SELECT id, template_id, step_id, role_id, allowed_actions FROM role_mappings WHERE template_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("list role_mappings: %w", err)
	}
	defer rmrows.Close()
	for rmrows.Next() {
		var rm domain.RoleMapping
		var actionsJSON string
		if err := rmrows.Scan(&rm.ID, &rm.TemplateID, &rm.StepID, &rm.RoleID, &actionsJSON); err != nil {
			return nil, fmt.Errorf("scan role_mapping: %w", err)
		}
		json.Unmarshal([]byte(actionsJSON), &rm.AllowedActions) //nolint:errcheck
		t.RoleMappings = append(t.RoleMappings, rm)
	}
	if err := rmrows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role_mappings: %w", err)
	}

	// Integration hooks
	hrows, err := s.db.QueryContext(ctx,
		`SELECT id, template_id, transition_id, event, adapter_type, action, config
		FROM integration_hooks WHERE template_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("list integration_hooks: %w", err)
	}
	defer hrows.Close()
	for hrows.Next() {
		var h domain.IntegrationHook
		var cfgStr string
		if err := hrows.Scan(&h.ID, &h.TemplateID, &h.TransitionID, &h.Event, &h.AdapterType, &h.Action, &cfgStr); err != nil {
			return nil, fmt.Errorf("scan integration_hook: %w", err)
		}
		h.Config = json.RawMessage(cfgStr)
		t.IntegrationHooks = append(t.IntegrationHooks, h)
	}
	if err := hrows.Err(); err != nil {
		return nil, fmt.Errorf("iterate integration_hooks: %w", err)
	}

	return &t, nil
}

func (s *Store) ListTemplates(ctx context.Context) ([]*domain.WorkflowTemplate, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, description, version, created_at, updated_at FROM workflow_templates ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []*domain.WorkflowTemplate
	for rows.Next() {
		var t domain.WorkflowTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, &t)
	}
	return templates, rows.Err()
}

func (s *Store) UpdateTemplate(ctx context.Context, t *domain.WorkflowTemplate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		"UPDATE workflow_templates SET name = ?, description = ?, version = ?, updated_at = datetime('now') WHERE id = ?",
		t.Name, t.Description, t.Version, t.ID)
	if err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: template %q", domain.ErrNotFound, t.ID)
	}

	// Replace all sub-collections. DELETEs run in FK-safe reverse-dependency
	// order (integration_hooks → role_mappings → transitions → steps) so
	// FK constraints with foreign_keys=ON are not violated.
	if _, err := tx.ExecContext(ctx, "DELETE FROM integration_hooks WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete integration_hooks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_mappings WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete role_mappings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM transitions WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete transitions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM steps WHERE template_id = ?", t.ID); err != nil {
		return fmt.Errorf("delete steps: %w", err)
	}

	for _, step := range t.Steps {
		var approvalMode sql.NullString
		var approvalRequired sql.NullInt64
		var approvalApproverRoleID sql.NullString
		var approvalRejectionStepID sql.NullString

		if step.Approval != nil {
			approvalMode = sql.NullString{String: string(step.Approval.Mode), Valid: true}
			approvalRequired = sql.NullInt64{Int64: int64(step.Approval.RequiredApprovers), Valid: true}
			approvalApproverRoleID = sql.NullString{String: step.Approval.ApproverRoleID, Valid: true}
			if step.Approval.RejectionStepID != "" {
				approvalRejectionStepID = sql.NullString{String: step.Approval.RejectionStepID, Valid: true}
			}
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO steps (id, template_id, key, name, type, position,
			approval_mode, approval_required, approval_approver_role_id, approval_rejection_step_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			step.ID, step.TemplateID, step.Key, step.Name, string(step.Type), step.Position,
			approvalMode, approvalRequired, approvalApproverRoleID, approvalRejectionStepID)
		if err != nil {
			return fmt.Errorf("insert step %q: %w", step.Key, err)
		}
	}

	for _, tr := range t.Transitions {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO transitions (id, template_id, key, name, from_step_id, to_step_id, guard, required_role_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			tr.ID, tr.TemplateID, tr.Key, tr.Name, tr.FromStepID, tr.ToStepID, tr.Guard, tr.RequiredRoleID)
		if err != nil {
			return fmt.Errorf("insert transition %q: %w", tr.Key, err)
		}
	}

	for _, rm := range t.RoleMappings {
		actionsJSON, _ := json.Marshal(rm.AllowedActions)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO role_mappings (id, template_id, step_id, role_id, allowed_actions)
			VALUES (?, ?, ?, ?, ?)`,
			rm.ID, rm.TemplateID, rm.StepID, rm.RoleID, string(actionsJSON))
		if err != nil {
			return fmt.Errorf("insert role_mapping: %w", err)
		}
	}

	for _, h := range t.IntegrationHooks {
		cfg := h.Config
		if cfg == nil {
			cfg = json.RawMessage("{}")
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO integration_hooks (id, template_id, transition_id, event, adapter_type, action, config)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			h.ID, h.TemplateID, h.TransitionID, h.Event, h.AdapterType, h.Action, string(cfg))
		if err != nil {
			return fmt.Errorf("insert integration_hook: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteTemplate(ctx context.Context, id string) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workflow_instances WHERE template_id = ?", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count instances: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: template has %d instances", domain.ErrHasDependencies, count)
	}

	res, err := s.db.ExecContext(ctx, "DELETE FROM workflow_templates WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: template %q", domain.ErrNotFound, id)
	}
	return nil
}
