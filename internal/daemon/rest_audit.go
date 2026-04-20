// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

type AuditFilterInput struct {
	Project    string    `query:"project_id"`
	WorkflowID string    `query:"workflow_id"`
	AgentID    string    `query:"agent_id"`
	EventType  string    `query:"event_type"`
	Since      time.Time `query:"since"`
	Until      time.Time `query:"until"`
	Limit      int       `query:"limit" minimum:"0" maximum:"500"`
	Offset     int       `query:"offset" minimum:"0"`
}

type AuditFilteredOutput struct {
	Body struct {
		Events []auditEventResponse `json:"events"`
	}
}

func registerAuditRoutes(api huma.API, store domain.AuditEventStore) {
	huma.Register(api, huma.Operation{
		OperationID: "list-audit-filtered",
		Method:      http.MethodGet,
		Path:        "/v1/audit",
		Summary:     "List audit events with optional filters",
		Tags:        []string{"Audit"},
	}, func(ctx context.Context, in *AuditFilterInput) (*AuditFilteredOutput, error) {
		lim := in.Limit
		if lim == 0 {
			lim = 500
		}
		events, err := store.ListAuditEventsFiltered(ctx, domain.AuditFilter{
			Project:    in.Project,
			WorkflowID: in.WorkflowID,
			AgentID:    in.AgentID,
			EventType:  in.EventType,
			Since:      in.Since,
			Until:      in.Until,
			Limit:      lim,
			Offset:     in.Offset,
		})
		if err != nil {
			return nil, mapDomainErr(err)
		}
		out := &AuditFilteredOutput{}
		out.Body.Events = make([]auditEventResponse, 0, len(events))
		for _, e := range events {
			out.Body.Events = append(out.Body.Events, auditEventResponse{
				ID: e.ID, OccurredAt: e.OccurredAt, Type: string(e.Type),
				AgentID: e.AgentID, AgentName: e.AgentName,
				WorkflowID: e.WorkflowID, Role: e.Role, Project: e.Project,
			})
		}
		return out, nil
	})
}
