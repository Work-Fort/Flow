// SPDX-License-Identifier: GPL-2.0-only
package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
)

// RoleIndex maps semantic role names to Hive role IDs.
// In Phase 1, pass nil — role names are used as-is.
type RoleIndex map[string]string

// TemplateFile mirrors the JSON schema for workflow templates.
type TemplateFile struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Version       string            `json:"version"`
	Steps         []StepFile        `json:"steps"`
	Transitions   []TransitionFile  `json:"transitions"`
	RoleMappings  []RoleMappingFile `json:"role_mappings"`
	Hooks         []HookFile        `json:"integration_hooks"`
}

type StepFile struct {
	Key      string        `json:"key"`
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Position int           `json:"position"`
	Approval *ApprovalFile `json:"approval,omitempty"`
}

type ApprovalFile struct {
	Mode              string `json:"mode"`
	RequiredApprovers int    `json:"required_approvers"`
	ApproverRole      string `json:"approver_role"`
	RejectionStep     string `json:"rejection_step"`
}

type TransitionFile struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	From         string `json:"from"`
	To           string `json:"to"`
	Guard        string `json:"guard,omitempty"`
	RequiredRole string `json:"required_role,omitempty"`
}

type RoleMappingFile struct {
	Role    string   `json:"role"`
	Step    string   `json:"step"`
	Actions []string `json:"actions"`
}

type HookFile struct {
	Transition string          `json:"transition"`
	Adapter    string          `json:"adapter"`
	Action     string          `json:"action"`
	Config     json.RawMessage `json:"config,omitempty"`
}

// ImportTemplate reads a JSON file at path and creates the template in the store.
func ImportTemplate(ctx context.Context, store domain.Store, path string, roles RoleIndex) (*domain.WorkflowTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template file: %w", err)
	}
	var tf TemplateFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse template file: %w", err)
	}
	return ImportTemplateFromFile(ctx, store, &tf, roles)
}

// ImportTemplateFromFile converts a TemplateFile to domain entities and creates it in the store.
func ImportTemplateFromFile(ctx context.Context, store domain.Store, tf *TemplateFile, roles RoleIndex) (*domain.WorkflowTemplate, error) {
	// Generate template UUID
	templateID := newID("tpl")

	// Assign UUIDs to steps (key → UUID map)
	stepIDs := make(map[string]string, len(tf.Steps))
	for _, s := range tf.Steps {
		stepIDs[s.Key] = newID("stp")
	}

	// Assign UUIDs to transitions (key → UUID map)
	transitionIDs := make(map[string]string, len(tf.Transitions))
	for _, tr := range tf.Transitions {
		transitionIDs[tr.Key] = newID("tr")
	}

	// Resolve role name: roles[name] if present, else use name as-is
	resolveRole := func(name string) string {
		if roles == nil {
			return name
		}
		if id, ok := roles[name]; ok {
			return id
		}
		return name
	}

	// Build steps
	steps := make([]domain.Step, 0, len(tf.Steps))
	for _, sf := range tf.Steps {
		step := domain.Step{
			ID:         stepIDs[sf.Key],
			TemplateID: templateID,
			Key:        sf.Key,
			Name:       sf.Name,
			Type:       domain.StepType(sf.Type),
			Position:   sf.Position,
		}
		if sf.Approval != nil {
			rejectionStepID := ""
			if sf.Approval.RejectionStep != "" {
				sid, ok := stepIDs[sf.Approval.RejectionStep]
				if !ok {
					return nil, fmt.Errorf("rejection_step %q not found for step %q", sf.Approval.RejectionStep, sf.Key)
				}
				rejectionStepID = sid
			}
			step.Approval = &domain.ApprovalConfig{
				Mode:              domain.ApprovalMode(sf.Approval.Mode),
				RequiredApprovers: sf.Approval.RequiredApprovers,
				ApproverRoleID:    resolveRole(sf.Approval.ApproverRole),
				RejectionStepID:   rejectionStepID,
			}
		}
		steps = append(steps, step)
	}

	// Validate guard expressions in transitions
	for _, tr := range tf.Transitions {
		if err := domain.ValidateGuard(tr.Guard); err != nil {
			return nil, fmt.Errorf("invalid guard on transition %q: %w", tr.Key, err)
		}
	}

	// Build transitions
	transitions := make([]domain.Transition, 0, len(tf.Transitions))
	for _, tr := range tf.Transitions {
		fromID, ok := stepIDs[tr.From]
		if !ok {
			return nil, fmt.Errorf("from step %q not found for transition %q", tr.From, tr.Key)
		}
		toID, ok := stepIDs[tr.To]
		if !ok {
			return nil, fmt.Errorf("to step %q not found for transition %q", tr.To, tr.Key)
		}
		transitions = append(transitions, domain.Transition{
			ID:             transitionIDs[tr.Key],
			TemplateID:     templateID,
			Key:            tr.Key,
			Name:           tr.Name,
			FromStepID:     fromID,
			ToStepID:       toID,
			Guard:          tr.Guard,
			RequiredRoleID: resolveRole(tr.RequiredRole),
		})
	}

	// Build role mappings
	roleMappings := make([]domain.RoleMapping, 0, len(tf.RoleMappings))
	for _, rm := range tf.RoleMappings {
		stepID, ok := stepIDs[rm.Step]
		if !ok {
			return nil, fmt.Errorf("step %q not found for role_mapping", rm.Step)
		}
		roleMappings = append(roleMappings, domain.RoleMapping{
			ID:             newID("rm"),
			TemplateID:     templateID,
			StepID:         stepID,
			RoleID:         resolveRole(rm.Role),
			AllowedActions: rm.Actions,
		})
	}

	// Build integration hooks
	hooks := make([]domain.IntegrationHook, 0, len(tf.Hooks))
	for _, h := range tf.Hooks {
		transitionID, ok := transitionIDs[h.Transition]
		if !ok {
			return nil, fmt.Errorf("transition %q not found for hook", h.Transition)
		}
		cfg := h.Config
		if cfg == nil {
			cfg = json.RawMessage("{}")
		}
		hooks = append(hooks, domain.IntegrationHook{
			ID:           newID("hk"),
			TemplateID:   templateID,
			TransitionID: transitionID,
			Event:        "on_transition",
			AdapterType:  h.Adapter,
			Action:       h.Action,
			Config:       cfg,
		})
	}

	// Derive version from template file version string
	version := 1
	if tf.Version != "" {
		// Parse major version from e.g. "1.0.0"
		parts := strings.SplitN(tf.Version, ".", 2)
		if len(parts) > 0 {
			fmt.Sscanf(parts[0], "%d", &version) //nolint:errcheck
		}
	}

	t := &domain.WorkflowTemplate{
		ID:               templateID,
		Name:             tf.Name,
		Description:      tf.Description,
		Version:          version,
		Steps:            steps,
		Transitions:      transitions,
		RoleMappings:     roleMappings,
		IntegrationHooks: hooks,
	}

	if err := store.CreateTemplate(ctx, t); err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	return store.GetTemplate(ctx, t.ID)
}

// newID generates a prefixed short ID.
func newID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}
