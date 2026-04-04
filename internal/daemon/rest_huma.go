// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"errors"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

// mapDomainErr converts a domain error to a Huma error with the appropriate
// HTTP status code. Returns nil if err is nil.
func mapDomainErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrHasDependencies):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidGuard):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrGuardDenied):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrNotAtGateStep):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrPermissionDenied):
		return huma.NewError(http.StatusForbidden, err.Error())
	default:
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
}

// --- response converters ---

func templateToResponse(t *domain.WorkflowTemplate) templateDetailResponse {
	steps := make([]stepResponse, len(t.Steps))
	for i, s := range t.Steps {
		sr := stepResponse{
			ID: s.ID, TemplateID: s.TemplateID, Key: s.Key,
			Name: s.Name, Type: string(s.Type), Position: s.Position,
		}
		if s.Approval != nil {
			sr.Approval = &stepApprovalResponse{
				Mode:              string(s.Approval.Mode),
				RequiredApprovers: s.Approval.RequiredApprovers,
				ApproverRoleID:    s.Approval.ApproverRoleID,
				RejectionStepID:   s.Approval.RejectionStepID,
			}
		}
		steps[i] = sr
	}
	transitions := make([]transitionResponse, len(t.Transitions))
	for i, tr := range t.Transitions {
		transitions[i] = transitionResponse{
			ID: tr.ID, TemplateID: tr.TemplateID, Key: tr.Key, Name: tr.Name,
			FromStepID: tr.FromStepID, ToStepID: tr.ToStepID,
			Guard: tr.Guard, RequiredRoleID: tr.RequiredRoleID,
		}
	}
	return templateDetailResponse{
		ID: t.ID, Name: t.Name, Description: t.Description, Version: t.Version,
		Steps: steps, Transitions: transitions,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func templateHeaderToResponse(t *domain.WorkflowTemplate) templateResponse {
	return templateResponse{
		ID: t.ID, Name: t.Name, Description: t.Description, Version: t.Version,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func instanceToResponse(i *domain.WorkflowInstance) instanceResponse {
	return instanceResponse{
		ID: i.ID, TemplateID: i.TemplateID, TemplateVersion: i.TemplateVersion,
		TeamID: i.TeamID, Name: i.Name, Status: string(i.Status),
		CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}

func workItemToResponse(w *domain.WorkItem) workItemResponse {
	return workItemResponse{
		ID: w.ID, InstanceID: w.InstanceID, Title: w.Title, Description: w.Description,
		CurrentStepID: w.CurrentStepID, AssignedAgentID: w.AssignedAgentID,
		Priority: string(w.Priority), Fields: w.Fields,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

func historyToResponse(h *domain.TransitionHistory) historyResponse {
	return historyResponse{
		ID: h.ID, WorkItemID: h.WorkItemID, FromStepID: h.FromStepID, ToStepID: h.ToStepID,
		TransitionID: h.TransitionID, TriggeredBy: h.TriggeredBy, Reason: h.Reason,
		Timestamp: h.Timestamp,
	}
}

func approvalToResponse(a *domain.Approval) approvalResponse {
	return approvalResponse{
		ID: a.ID, WorkItemID: a.WorkItemID, StepID: a.StepID, AgentID: a.AgentID,
		Decision: string(a.Decision), Comment: a.Comment, Timestamp: a.Timestamp,
	}
}

// --- template routes ---

func registerTemplateRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-templates",
		Method:      http.MethodGet,
		Path:        "/v1/templates",
		Summary:     "List workflow templates",
		Tags:        []string{"Templates"},
	}, func(ctx context.Context, input *struct{}) (*TemplateListOutput, error) {
		templates, err := store.ListTemplates(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]templateResponse, len(templates))
		for i, t := range templates {
			resp[i] = templateHeaderToResponse(t)
		}
		return &TemplateListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-template",
		Method:        http.MethodPost,
		Path:          "/v1/templates",
		Summary:       "Create a workflow template",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Templates"},
	}, func(ctx context.Context, input *CreateTemplateInput) (*TemplateOutput, error) {
		t := &domain.WorkflowTemplate{
			ID:          NewID("tpl"),
			Name:        input.Body.Name,
			Description: input.Body.Description,
			Version:     1,
		}
		if err := store.CreateTemplate(ctx, t); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetTemplate(ctx, t.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TemplateOutput{Body: templateToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-template",
		Method:      http.MethodGet,
		Path:        "/v1/templates/{id}",
		Summary:     "Get a workflow template",
		Tags:        []string{"Templates"},
	}, func(ctx context.Context, input *IDPathInput) (*TemplateOutput, error) {
		t, err := store.GetTemplate(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TemplateOutput{Body: templateToResponse(t)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-template",
		Method:      http.MethodPatch,
		Path:        "/v1/templates/{id}",
		Summary:     "Update a workflow template",
		Tags:        []string{"Templates"},
	}, func(ctx context.Context, input *PatchTemplateInput) (*TemplateOutput, error) {
		existing, err := store.GetTemplate(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Name != "" {
			existing.Name = input.Body.Name
		}
		if input.Body.Description != "" {
			existing.Description = input.Body.Description
		}
		if err := store.UpdateTemplate(ctx, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetTemplate(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TemplateOutput{Body: templateToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-template",
		Method:        http.MethodDelete,
		Path:          "/v1/templates/{id}",
		Summary:       "Delete a workflow template",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Templates"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteTemplate(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})
}

// --- instance routes ---

func registerInstanceRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-instances",
		Method:      http.MethodGet,
		Path:        "/v1/instances",
		Summary:     "List workflow instances",
		Tags:        []string{"Instances"},
	}, func(ctx context.Context, input *ListInstancesInput) (*InstanceListOutput, error) {
		instances, err := store.ListInstances(ctx, input.TeamID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]instanceResponse, len(instances))
		for i, inst := range instances {
			resp[i] = instanceToResponse(inst)
		}
		return &InstanceListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-instance",
		Method:        http.MethodPost,
		Path:          "/v1/instances",
		Summary:       "Create a workflow instance",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Instances"},
	}, func(ctx context.Context, input *CreateInstanceInput) (*InstanceOutput, error) {
		// Snapshot template version
		tmpl, err := store.GetTemplate(ctx, input.Body.TemplateID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		inst := &domain.WorkflowInstance{
			ID:              NewID("ins"),
			TemplateID:      input.Body.TemplateID,
			TemplateVersion: tmpl.Version,
			TeamID:          input.Body.TeamID,
			Name:            input.Body.Name,
			Status:          domain.InstanceStatusActive,
		}
		if err := store.CreateInstance(ctx, inst); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetInstance(ctx, inst.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &InstanceOutput{Body: instanceToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-instance",
		Method:      http.MethodGet,
		Path:        "/v1/instances/{id}",
		Summary:     "Get a workflow instance",
		Tags:        []string{"Instances"},
	}, func(ctx context.Context, input *IDPathInput) (*InstanceOutput, error) {
		inst, err := store.GetInstance(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &InstanceOutput{Body: instanceToResponse(inst)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-instance",
		Method:      http.MethodPatch,
		Path:        "/v1/instances/{id}",
		Summary:     "Update a workflow instance",
		Tags:        []string{"Instances"},
	}, func(ctx context.Context, input *PatchInstanceInput) (*InstanceOutput, error) {
		existing, err := store.GetInstance(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Name != "" {
			existing.Name = input.Body.Name
		}
		if input.Body.Status != "" {
			existing.Status = domain.InstanceStatus(input.Body.Status)
		}
		if err := store.UpdateInstance(ctx, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetInstance(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &InstanceOutput{Body: instanceToResponse(updated)}, nil
	})
}

// --- work item routes ---

func registerWorkItemRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID:   "create-work-item",
		Method:        http.MethodPost,
		Path:          "/v1/instances/{id}/items",
		Summary:       "Create a work item",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"WorkItems"},
	}, func(ctx context.Context, input *CreateWorkItemInput) (*WorkItemOutput, error) {
		// Verify instance exists
		inst, err := store.GetInstance(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		// Get template to find first step (lowest position)
		tmpl, err := store.GetTemplate(ctx, inst.TemplateID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if len(tmpl.Steps) == 0 {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "template has no steps")
		}
		firstStep := tmpl.Steps[0]
		for _, s := range tmpl.Steps {
			if s.Position < firstStep.Position {
				firstStep = s
			}
		}

		priority := domain.Priority(input.Body.Priority)
		if priority == "" {
			priority = domain.PriorityNormal
		}

		w := &domain.WorkItem{
			ID:              NewID("wi"),
			InstanceID:      input.ID,
			Title:           input.Body.Title,
			Description:     input.Body.Description,
			CurrentStepID:   firstStep.ID,
			AssignedAgentID: input.Body.AssignedAgentID,
			Priority:        priority,
		}
		if err := store.CreateWorkItem(ctx, w); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetWorkItem(ctx, w.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-work-items",
		Method:      http.MethodGet,
		Path:        "/v1/instances/{id}/items",
		Summary:     "List work items for an instance",
		Tags:        []string{"WorkItems"},
	}, func(ctx context.Context, input *ListWorkItemsInput) (*WorkItemListOutput, error) {
		items, err := store.ListWorkItems(ctx, input.ID, input.StepID, input.AgentID, domain.Priority(input.Priority))
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]workItemResponse, len(items))
		for i, w := range items {
			resp[i] = workItemToResponse(w)
		}
		return &WorkItemListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-work-item",
		Method:      http.MethodGet,
		Path:        "/v1/items/{id}",
		Summary:     "Get a work item",
		Tags:        []string{"WorkItems"},
	}, func(ctx context.Context, input *IDPathInput) (*WorkItemOutput, error) {
		w, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(w)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-work-item",
		Method:      http.MethodPatch,
		Path:        "/v1/items/{id}",
		Summary:     "Update a work item",
		Tags:        []string{"WorkItems"},
	}, func(ctx context.Context, input *PatchWorkItemInput) (*WorkItemOutput, error) {
		existing, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Title != "" {
			existing.Title = input.Body.Title
		}
		if input.Body.Description != "" {
			existing.Description = input.Body.Description
		}
		if input.Body.AssignedAgentID != nil {
			existing.AssignedAgentID = *input.Body.AssignedAgentID
		}
		if input.Body.Fields != nil {
			existing.Fields = input.Body.Fields
		}
		if err := store.UpdateWorkItem(ctx, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-work-item-history",
		Method:      http.MethodGet,
		Path:        "/v1/items/{id}/history",
		Summary:     "Get transition history for a work item",
		Tags:        []string{"WorkItems"},
	}, func(ctx context.Context, input *IDPathInput) (*WorkItemHistoryOutput, error) {
		history, err := store.GetTransitionHistory(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]historyResponse, len(history))
		for i, h := range history {
			resp[i] = historyToResponse(h)
		}
		return &WorkItemHistoryOutput{Body: resp}, nil
	})
}

// --- transition routes ---

func registerTransitionRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "transition-work-item",
		Method:      http.MethodPost,
		Path:        "/v1/items/{id}/transition",
		Summary:     "Transition a work item to a new step",
		Tags:        []string{"Transitions"},
	}, func(ctx context.Context, input *TransitionWorkItemInput) (*WorkItemOutput, error) {
		w, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		// Load template to find the transition
		inst, err := store.GetInstance(ctx, w.InstanceID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		tmpl, err := store.GetTemplate(ctx, inst.TemplateID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		var tr *domain.Transition
		for i := range tmpl.Transitions {
			if tmpl.Transitions[i].ID == input.Body.TransitionID {
				tr = &tmpl.Transitions[i]
				break
			}
		}
		if tr == nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, domain.ErrInvalidTransition.Error())
		}

		// Verify current step matches from_step_id
		if w.CurrentStepID != tr.FromStepID {
			return nil, huma.NewError(http.StatusUnprocessableEntity, domain.ErrInvalidTransition.Error())
		}

		// Build guard context and evaluate
		guardCtx := domain.GuardContext{
			Item: domain.GuardItem{
				Title:    w.Title,
				Priority: string(w.Priority),
				Step:     w.CurrentStepID,
			},
			Actor: domain.GuardActor{
				RoleID:  input.Body.ActorRoleID,
				AgentID: input.Body.ActorAgentID,
			},
		}
		if err := domain.EvaluateGuard(tr.Guard, guardCtx); err != nil {
			return nil, mapDomainErr(err)
		}

		// Update work item
		w.CurrentStepID = tr.ToStepID
		if err := store.UpdateWorkItem(ctx, w); err != nil {
			return nil, mapDomainErr(err)
		}

		// Record history
		h := &domain.TransitionHistory{
			ID:           NewID("th"),
			WorkItemID:   w.ID,
			FromStepID:   tr.FromStepID,
			ToStepID:     tr.ToStepID,
			TransitionID: tr.ID,
			TriggeredBy:  input.Body.ActorAgentID,
			Reason:       input.Body.Reason,
		}
		if err := store.RecordTransition(ctx, h); err != nil {
			return nil, mapDomainErr(err)
		}

		updated, err := store.GetWorkItem(ctx, w.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(updated)}, nil
	})
}

// --- approval routes ---

func registerApprovalRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "approve-work-item",
		Method:      http.MethodPost,
		Path:        "/v1/items/{id}/approve",
		Summary:     "Approve a work item at a gate step",
		Tags:        []string{"Approvals"},
	}, func(ctx context.Context, input *ApproveWorkItemInput) (*WorkItemOutput, error) {
		w, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		// Verify gate step
		inst, err := store.GetInstance(ctx, w.InstanceID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		tmpl, err := store.GetTemplate(ctx, inst.TemplateID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		var currentStep *domain.Step
		for i := range tmpl.Steps {
			if tmpl.Steps[i].ID == w.CurrentStepID {
				currentStep = &tmpl.Steps[i]
				break
			}
		}
		if currentStep == nil || currentStep.Type != domain.StepTypeGate {
			return nil, huma.NewError(http.StatusUnprocessableEntity, domain.ErrNotAtGateStep.Error())
		}

		// Record approval
		a := &domain.Approval{
			ID:         NewID("apr"),
			WorkItemID: w.ID,
			StepID:     w.CurrentStepID,
			AgentID:    input.Body.AgentID,
			Decision:   domain.ApprovalDecisionApproved,
			Comment:    input.Body.Comment,
		}
		if err := store.RecordApproval(ctx, a); err != nil {
			return nil, mapDomainErr(err)
		}

		// Auto-advance logic for mode=any
		if currentStep.Approval != nil {
			approvals, err := store.ListApprovals(ctx, w.ID, w.CurrentStepID)
			if err != nil {
				return nil, mapDomainErr(err)
			}
			approvedCount := 0
			for _, ap := range approvals {
				if ap.Decision == domain.ApprovalDecisionApproved {
					approvedCount++
				}
			}

			if currentStep.Approval.Mode == domain.ApprovalModeAny &&
				approvedCount >= currentStep.Approval.RequiredApprovers {
				// Find outgoing transition (not going to rejection step)
				for i := range tmpl.Transitions {
					tr := &tmpl.Transitions[i]
					if tr.FromStepID == w.CurrentStepID &&
						tr.ToStepID != currentStep.Approval.RejectionStepID {
						w.CurrentStepID = tr.ToStepID
						if err := store.UpdateWorkItem(ctx, w); err != nil {
							return nil, mapDomainErr(err)
						}
						h := &domain.TransitionHistory{
							ID:           NewID("th"),
							WorkItemID:   w.ID,
							FromStepID:   tr.FromStepID,
							ToStepID:     tr.ToStepID,
							TransitionID: tr.ID,
							TriggeredBy:  input.Body.AgentID,
							Reason:       "auto-advance on approval threshold",
						}
						if err := store.RecordTransition(ctx, h); err != nil {
							log.Warn("record auto-advance transition history", "err", err)
						}
						break
					}
				}
			}
		}

		updated, err := store.GetWorkItem(ctx, w.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reject-work-item",
		Method:      http.MethodPost,
		Path:        "/v1/items/{id}/reject",
		Summary:     "Reject a work item at a gate step",
		Tags:        []string{"Approvals"},
	}, func(ctx context.Context, input *RejectWorkItemInput) (*WorkItemOutput, error) {
		w, err := store.GetWorkItem(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		// Verify gate step
		inst, err := store.GetInstance(ctx, w.InstanceID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		tmpl, err := store.GetTemplate(ctx, inst.TemplateID)
		if err != nil {
			return nil, mapDomainErr(err)
		}

		var currentStep *domain.Step
		for i := range tmpl.Steps {
			if tmpl.Steps[i].ID == w.CurrentStepID {
				currentStep = &tmpl.Steps[i]
				break
			}
		}
		if currentStep == nil || currentStep.Type != domain.StepTypeGate {
			return nil, huma.NewError(http.StatusUnprocessableEntity, domain.ErrNotAtGateStep.Error())
		}

		// Record rejection
		a := &domain.Approval{
			ID:         NewID("apr"),
			WorkItemID: w.ID,
			StepID:     w.CurrentStepID,
			AgentID:    input.Body.AgentID,
			Decision:   domain.ApprovalDecisionRejected,
			Comment:    input.Body.Comment,
		}
		if err := store.RecordApproval(ctx, a); err != nil {
			return nil, mapDomainErr(err)
		}

		// Advance to rejection step if configured
		if currentStep.Approval != nil && currentStep.Approval.RejectionStepID != "" {
			fromStepID := w.CurrentStepID
			w.CurrentStepID = currentStep.Approval.RejectionStepID
			if err := store.UpdateWorkItem(ctx, w); err != nil {
				return nil, mapDomainErr(err)
			}
			// Find rejection transition
			for i := range tmpl.Transitions {
				tr := &tmpl.Transitions[i]
				if tr.FromStepID == fromStepID && tr.ToStepID == currentStep.Approval.RejectionStepID {
					h := &domain.TransitionHistory{
						ID:           NewID("th"),
						WorkItemID:   w.ID,
						FromStepID:   fromStepID,
						ToStepID:     currentStep.Approval.RejectionStepID,
						TransitionID: tr.ID,
						TriggeredBy:  input.Body.AgentID,
						Reason:       "rejected: " + input.Body.Comment,
					}
					if err := store.RecordTransition(ctx, h); err != nil {
						log.Warn("record rejection transition history", "err", err)
					}
					break
				}
			}
		}

		updated, err := store.GetWorkItem(ctx, w.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &WorkItemOutput{Body: workItemToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-approvals",
		Method:      http.MethodGet,
		Path:        "/v1/items/{id}/approvals",
		Summary:     "List approvals for a work item",
		Tags:        []string{"Approvals"},
	}, func(ctx context.Context, input *IDPathInput) (*ApprovalListOutput, error) {
		approvals, err := store.ListApprovals(ctx, input.ID, "")
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]approvalResponse, len(approvals))
		for i, a := range approvals {
			resp[i] = approvalToResponse(a)
		}
		return &ApprovalListOutput{Body: resp}, nil
	})
}
