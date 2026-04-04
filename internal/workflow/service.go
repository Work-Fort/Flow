// SPDX-License-Identifier: GPL-2.0-only

// Package workflow contains the core workflow engine business logic.
// The Service type owns the transition and approval logic that was
// previously inlined in the Huma handler closures.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Service executes workflow operations against a domain.Store.
type Service struct {
	store    domain.Store
	identity domain.IdentityProvider
}

// New creates a new Service. identity may be nil — if so, role checks are
// skipped (backwards-compatible, open-access behaviour).
func New(store domain.Store, identity domain.IdentityProvider) *Service {
	return &Service{store: store, identity: identity}
}

// TransitionRequest holds all parameters for a work item transition.
type TransitionRequest struct {
	WorkItemID   string
	TransitionID string
	ActorAgentID string
	ActorRoleID  string
	Reason       string
}

// ApproveRequest holds all parameters for an approval decision.
type ApproveRequest struct {
	WorkItemID string
	AgentID    string
	Comment    string
}

// RejectRequest holds all parameters for a rejection decision.
type RejectRequest struct {
	WorkItemID string
	AgentID    string
	Comment    string
}

// newID generates a prefixed short ID.
func newID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}

// TransitionItem moves a work item to a new step via the named transition.
// Returns the updated work item on success. Errors:
//   - domain.ErrNotFound if the work item does not exist
//   - domain.ErrInvalidTransition if the transition ID is unknown or the item
//     is not currently at the transition's from-step
//   - domain.ErrGuardDenied if the transition guard expression evaluates false
//   - domain.ErrInvalidGuard if the guard expression is malformed
func (s *Service) TransitionItem(ctx context.Context, req TransitionRequest) (*domain.WorkItem, error) {
	w, err := s.store.GetWorkItem(ctx, req.WorkItemID)
	if err != nil {
		return nil, err
	}

	inst, err := s.store.GetInstance(ctx, w.InstanceID)
	if err != nil {
		return nil, err
	}
	tmpl, err := s.store.GetTemplate(ctx, inst.TemplateID)
	if err != nil {
		return nil, err
	}

	var tr *domain.Transition
	for i := range tmpl.Transitions {
		if tmpl.Transitions[i].ID == req.TransitionID {
			tr = &tmpl.Transitions[i]
			break
		}
	}
	if tr == nil {
		return nil, domain.ErrInvalidTransition
	}

	if w.CurrentStepID != tr.FromStepID {
		return nil, domain.ErrInvalidTransition
	}

	// Refuse to directly transition out of a gate step. Callers must use
	// ApproveItem/RejectItem to satisfy the approval threshold first.
	for i := range tmpl.Steps {
		if tmpl.Steps[i].ID == w.CurrentStepID && tmpl.Steps[i].Type == domain.StepTypeGate {
			return nil, domain.ErrGateRequiresApproval
		}
	}

	// Role check: if the transition requires a role and identity is configured,
	// verify the actor holds that role.
	if tr.RequiredRoleID != "" && s.identity != nil {
		if err := s.checkActorHasRole(ctx, req.ActorAgentID, tr.RequiredRoleID); err != nil {
			return nil, err
		}
	}

	// Build guard context. Populate approval counts so that guards using
	// approval.count work correctly. The original handler left Approval
	// zero-valued, meaning guards on approval.count always saw 0.
	approvals, err := s.store.ListApprovals(ctx, w.ID, w.CurrentStepID)
	if err != nil {
		return nil, err
	}
	approvedCount, rejectionCount := 0, 0
	for _, a := range approvals {
		if a.Decision == domain.ApprovalDecisionApproved {
			approvedCount++
		} else {
			rejectionCount++
		}
	}

	guardCtx := domain.GuardContext{
		Item: domain.GuardItem{
			Title:    w.Title,
			Priority: string(w.Priority),
			Step:     w.CurrentStepID,
		},
		Actor: domain.GuardActor{
			RoleID:  req.ActorRoleID,
			AgentID: req.ActorAgentID,
		},
		Approval: domain.GuardApproval{
			Count:      approvedCount,
			Rejections: rejectionCount,
		},
	}
	if len(w.Fields) > 0 {
		var fields map[string]any
		if err := json.Unmarshal(w.Fields, &fields); err == nil {
			guardCtx.Item.Fields = fields
		}
	}

	if err := domain.EvaluateGuard(tr.Guard, guardCtx); err != nil {
		return nil, err
	}

	w.CurrentStepID = tr.ToStepID
	if err := s.store.UpdateWorkItem(ctx, w); err != nil {
		return nil, err
	}

	h := &domain.TransitionHistory{
		ID:           newID("th"),
		WorkItemID:   w.ID,
		FromStepID:   tr.FromStepID,
		ToStepID:     tr.ToStepID,
		TransitionID: tr.ID,
		TriggeredBy:  req.ActorAgentID,
		Reason:       req.Reason,
	}
	if err := s.store.RecordTransition(ctx, h); err != nil {
		return nil, err
	}

	return s.store.GetWorkItem(ctx, w.ID)
}

// ApproveItem records an approval for a work item at a gate step and
// auto-advances the item when the approval threshold is met (mode=any).
// Returns the updated work item. Errors:
//   - domain.ErrNotFound if the work item does not exist
//   - domain.ErrNotAtGateStep if the item is not currently at a gate step
func (s *Service) ApproveItem(ctx context.Context, req ApproveRequest) (*domain.WorkItem, error) {
	w, err := s.store.GetWorkItem(ctx, req.WorkItemID)
	if err != nil {
		return nil, err
	}

	currentStep, tmpl, err := s.loadCurrentStep(ctx, w)
	if err != nil {
		return nil, err
	}
	if currentStep == nil || currentStep.Type != domain.StepTypeGate {
		return nil, domain.ErrNotAtGateStep
	}

	// Approver role check.
	if currentStep.Approval != nil && currentStep.Approval.ApproverRoleID != "" && s.identity != nil {
		if err := s.checkActorHasRole(ctx, req.AgentID, currentStep.Approval.ApproverRoleID); err != nil {
			return nil, err
		}
	}

	a := &domain.Approval{
		ID:         newID("apr"),
		WorkItemID: w.ID,
		StepID:     w.CurrentStepID,
		AgentID:    req.AgentID,
		Decision:   domain.ApprovalDecisionApproved,
		Comment:    req.Comment,
	}
	if err := s.store.RecordApproval(ctx, a); err != nil {
		return nil, err
	}

	// Auto-advance when approval threshold is met.
	// Handles both mode=any and mode=unanimous.
	if currentStep.Approval != nil {
		approvals, err := s.store.ListApprovals(ctx, w.ID, w.CurrentStepID)
		if err != nil {
			return nil, err
		}
		approvedCount, rejectionCount := 0, 0
		for _, ap := range approvals {
			if ap.Decision == domain.ApprovalDecisionApproved {
				approvedCount++
			} else {
				rejectionCount++
			}
		}

		shouldAdvance := false
		switch currentStep.Approval.Mode {
		case domain.ApprovalModeAny:
			shouldAdvance = approvedCount >= currentStep.Approval.RequiredApprovers
		case domain.ApprovalModeUnanimous:
			// All required approvers must approve and no one may have rejected.
			shouldAdvance = approvedCount >= currentStep.Approval.RequiredApprovers && rejectionCount == 0
		}

		if shouldAdvance {
			// Select the forward (approval) transition.
			// Skip the designated rejection branch (if configured).
			// Among remaining candidates, prefer the first one whose guard passes.
			// Fall back to highest-position destination when no guards are set.
			rejectionStepID := currentStep.Approval.RejectionStepID

			// Build guard context for evaluating transition guards.
			guardCtx := domain.GuardContext{
				Item: domain.GuardItem{
					Title:    w.Title,
					Priority: string(w.Priority),
					Step:     w.CurrentStepID,
				},
				Approval: domain.GuardApproval{
					Count:      approvedCount,
					Rejections: rejectionCount,
				},
			}
			if len(w.Fields) > 0 {
				var fields map[string]any
				if err := json.Unmarshal(w.Fields, &fields); err == nil {
					guardCtx.Item.Fields = fields
				}
			}

			// Build a step position index for fallback tie-breaking.
			stepPos := make(map[string]int, len(tmpl.Steps))
			for i := range tmpl.Steps {
				stepPos[tmpl.Steps[i].ID] = tmpl.Steps[i].Position
			}

			var chosen *domain.Transition
			for i := range tmpl.Transitions {
				tr := &tmpl.Transitions[i]
				if tr.FromStepID != w.CurrentStepID {
					continue
				}
				if rejectionStepID != "" && tr.ToStepID == rejectionStepID {
					continue
				}
				if tr.Guard != "" {
					if err := domain.EvaluateGuard(tr.Guard, guardCtx); err != nil {
						// Guard fails or is invalid — skip this candidate.
						continue
					}
					// Guard passes: prefer guard-matched transitions over unguarded ones.
					chosen = tr
					break
				}
				// Unguarded fallback: pick highest-position destination.
				if chosen == nil || (chosen.Guard == "" && stepPos[tr.ToStepID] > stepPos[chosen.ToStepID]) {
					chosen = tr
				}
			}

			if chosen != nil {
				fromStepID := w.CurrentStepID
				w.CurrentStepID = chosen.ToStepID
				if err := s.store.UpdateWorkItem(ctx, w); err != nil {
					return nil, err
				}
				h := &domain.TransitionHistory{
					ID:           newID("th"),
					WorkItemID:   w.ID,
					FromStepID:   fromStepID,
					ToStepID:     chosen.ToStepID,
					TransitionID: chosen.ID,
					TriggeredBy:  req.AgentID,
					Reason:       "auto-advance on approval threshold",
				}
				if err := s.store.RecordTransition(ctx, h); err != nil {
					return nil, err
				}
			}
		}
	}

	return s.store.GetWorkItem(ctx, w.ID)
}

// RejectItem records a rejection for a work item at a gate step and routes
// the item to the configured rejection step (if any). Returns the updated
// work item. Errors:
//   - domain.ErrNotFound if the work item does not exist
//   - domain.ErrNotAtGateStep if the item is not currently at a gate step
func (s *Service) RejectItem(ctx context.Context, req RejectRequest) (*domain.WorkItem, error) {
	w, err := s.store.GetWorkItem(ctx, req.WorkItemID)
	if err != nil {
		return nil, err
	}

	currentStep, tmpl, err := s.loadCurrentStep(ctx, w)
	if err != nil {
		return nil, err
	}
	if currentStep == nil || currentStep.Type != domain.StepTypeGate {
		return nil, domain.ErrNotAtGateStep
	}

	// Approver role check.
	if currentStep.Approval != nil && currentStep.Approval.ApproverRoleID != "" && s.identity != nil {
		if err := s.checkActorHasRole(ctx, req.AgentID, currentStep.Approval.ApproverRoleID); err != nil {
			return nil, err
		}
	}

	a := &domain.Approval{
		ID:         newID("apr"),
		WorkItemID: w.ID,
		StepID:     w.CurrentStepID,
		AgentID:    req.AgentID,
		Decision:   domain.ApprovalDecisionRejected,
		Comment:    req.Comment,
	}
	if err := s.store.RecordApproval(ctx, a); err != nil {
		return nil, err
	}

	// Route to rejection step if configured.
	if currentStep.Approval != nil && currentStep.Approval.RejectionStepID != "" {
		fromStepID := w.CurrentStepID
		w.CurrentStepID = currentStep.Approval.RejectionStepID
		if err := s.store.UpdateWorkItem(ctx, w); err != nil {
			return nil, err
		}
		for i := range tmpl.Transitions {
			tr := &tmpl.Transitions[i]
			if tr.FromStepID == fromStepID && tr.ToStepID == currentStep.Approval.RejectionStepID {
				h := &domain.TransitionHistory{
					ID:           newID("th"),
					WorkItemID:   w.ID,
					FromStepID:   fromStepID,
					ToStepID:     currentStep.Approval.RejectionStepID,
					TransitionID: tr.ID,
					TriggeredBy:  req.AgentID,
					Reason:       "rejected: " + req.Comment,
				}
				if err := s.store.RecordTransition(ctx, h); err != nil {
					return nil, err
				}
				break
			}
		}
	}

	return s.store.GetWorkItem(ctx, w.ID)
}

// loadCurrentStep returns the Step the work item is currently at and the
// full template it belongs to.
func (s *Service) loadCurrentStep(ctx context.Context, w *domain.WorkItem) (*domain.Step, *domain.WorkflowTemplate, error) {
	inst, err := s.store.GetInstance(ctx, w.InstanceID)
	if err != nil {
		return nil, nil, err
	}
	tmpl, err := s.store.GetTemplate(ctx, inst.TemplateID)
	if err != nil {
		return nil, nil, err
	}
	for i := range tmpl.Steps {
		if tmpl.Steps[i].ID == w.CurrentStepID {
			return &tmpl.Steps[i], tmpl, nil
		}
	}
	return nil, tmpl, nil
}

// checkActorHasRole returns nil if agentID has roleID, domain.ErrPermissionDenied
// if they do not, or a wrapped error if Hive is unreachable.
func (s *Service) checkActorHasRole(ctx context.Context, agentID, roleID string) error {
	roles, err := s.identity.GetAgentRoles(ctx, agentID)
	if err != nil {
		return fmt.Errorf("resolve actor roles: %w", err)
	}
	for _, r := range roles {
		if r.ID == roleID {
			return nil
		}
	}
	return domain.ErrPermissionDenied
}
