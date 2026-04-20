// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

func registerProjectRoutes(api huma.API, store domain.Store, botKeysDir string, chat domain.ChatProvider) {
	huma.Register(api, huma.Operation{
		OperationID: "list-projects",
		Method:      http.MethodGet,
		Path:        "/v1/projects",
		Summary:     "List projects",
		Tags:        []string{"Projects"},
	}, func(ctx context.Context, input *struct{}) (*ProjectListOutput, error) {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]projectResponse, len(projects))
		for i, p := range projects {
			resp[i] = projectToResponse(p)
		}
		return &ProjectListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-project",
		Method:        http.MethodPost,
		Path:          "/v1/projects",
		Summary:       "Create a project",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Projects"},
	}, func(ctx context.Context, input *CreateProjectInput) (*ProjectOutput, error) {
		vocID := input.Body.VocabularyID
		if vocID == "" {
			sdlc, err := store.GetVocabularyByName(ctx, "sdlc")
			if err != nil {
				return nil, huma.NewError(http.StatusUnprocessableEntity, "SDLC vocabulary seed missing; seed vocabularies first")
			}
			vocID = sdlc.ID
		}
		p := &domain.Project{
			ID:            NewID("prj"),
			Name:          input.Body.Name,
			Description:   input.Body.Description,
			TemplateID:    input.Body.TemplateID,
			ChannelName:   input.Body.ChannelName,
			VocabularyID:  vocID,
			RetentionDays: input.Body.RetentionDays,
		}
		if err := store.CreateProject(ctx, p); err != nil {
			return nil, mapDomainErr(err)
		}
		// Auto-provision Sharkfin channel after the project row commits.
		if chat != nil && !input.Body.ChannelAlreadyExists {
			if err := chat.CreateChannel(ctx, input.Body.ChannelName, true); err != nil {
				log.Warn("sharkfin channel create failed; project committed, channel skipped",
					"project", p.ID, "channel", input.Body.ChannelName, "err", err)
				_ = store.RecordAuditEvent(ctx, &domain.AuditEvent{
					Type:    domain.AuditEventProjectChannelCreateFailed,
					Project: p.Name,
				})
			}
		}
		created, err := store.GetProject(ctx, p.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &ProjectOutput{Body: projectToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-project",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{id}",
		Summary:     "Get a project",
		Tags:        []string{"Projects"},
	}, func(ctx context.Context, input *IDPathInput) (*ProjectOutput, error) {
		p, err := store.GetProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &ProjectOutput{Body: projectToResponse(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-project",
		Method:      http.MethodPatch,
		Path:        "/v1/projects/{id}",
		Summary:     "Update a project",
		Tags:        []string{"Projects"},
	}, func(ctx context.Context, input *PatchProjectInput) (*ProjectOutput, error) {
		existing, err := store.GetProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Name != "" {
			existing.Name = input.Body.Name
		}
		if input.Body.Description != "" {
			existing.Description = input.Body.Description
		}
		if input.Body.TemplateID != "" {
			existing.TemplateID = input.Body.TemplateID
		}
		if input.Body.ChannelName != "" {
			existing.ChannelName = input.Body.ChannelName
		}
		if input.Body.VocabularyID != "" {
			existing.VocabularyID = input.Body.VocabularyID
		}
		if input.Body.RetentionDays != nil {
			existing.RetentionDays = input.Body.RetentionDays
		}
		if input.Body.ClearRetentionDays {
			existing.RetentionDays = nil
		}
		if err := store.UpdateProject(ctx, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &ProjectOutput{Body: projectToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-project",
		Method:        http.MethodDelete,
		Path:          "/v1/projects/{id}",
		Summary:       "Delete a project",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Projects"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteProject(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})

	// Per-project audit endpoint
	huma.Register(api, huma.Operation{
		OperationID: "get-project-audit",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{id}/audit",
		Summary:     "Get audit events for a project",
		Tags:        []string{"Projects"},
	}, func(ctx context.Context, input *IDPathInput) (*ProjectAuditOutput, error) {
		p, err := store.GetProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		events, err := store.ListAuditEventsByProject(ctx, p.Name)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]auditEventResponse, len(events))
		for i, e := range events {
			resp[i] = auditEventResponse{
				ID:         e.ID,
				OccurredAt: e.OccurredAt,
				Type:       string(e.Type),
				AgentID:    e.AgentID,
				AgentName:  e.AgentName,
				WorkflowID: e.WorkflowID,
				Role:       e.Role,
				Project:    e.Project,
			}
		}
		var out ProjectAuditOutput
		out.Body.Events = resp
		return &out, nil
	})
}

func projectToResponse(p *domain.Project) projectResponse {
	return projectResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		TemplateID:    p.TemplateID,
		ChannelName:   p.ChannelName,
		VocabularyID:  p.VocabularyID,
		RetentionDays: p.RetentionDays,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func botToResponse(b *domain.Bot) botResponse {
	roles := b.HiveRoleAssignments
	if roles == nil {
		roles = []string{}
	}
	return botResponse{
		ID:                  b.ID,
		ProjectID:           b.ProjectID,
		PassportAPIKeyID:    b.PassportAPIKeyID,
		HiveRoleAssignments: roles,
		CreatedAt:           b.CreatedAt,
		UpdatedAt:           b.UpdatedAt,
	}
}
