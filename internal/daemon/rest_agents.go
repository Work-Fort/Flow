// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

func registerAgentRoutes(api huma.API, hive domain.HiveAgentClient) {
	if hive == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "list-agents",
		Method:      http.MethodGet,
		Path:        "/v1/agents",
		Summary:     "List Hive pool agents (proxied)",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, in *AgentFilterInput) (*AgentListOutput, error) {
		records, err := hive.ListAgents(ctx, domain.HiveAgentFilter{
			TeamID:     in.TeamID,
			Assigned:   in.Assigned,
			WorkflowID: in.WorkflowID,
			Role:       in.Role,
			Project:    in.Project,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusBadGateway, "hive: "+err.Error())
		}
		out := &AgentListOutput{}
		out.Body.Agents = make([]agentResponse, 0, len(records))
		for _, r := range records {
			out.Body.Agents = append(out.Body.Agents, agentResponse{
				ID:                r.ID,
				Name:              r.Name,
				TeamID:            r.TeamID,
				Model:             r.Model,
				Runtime:           r.Runtime,
				CurrentRole:       r.CurrentRole,
				CurrentProject:    r.CurrentProject,
				CurrentWorkflowID: r.CurrentWorkflowID,
				LeaseExpiresAt:    r.LeaseExpiresAt,
			})
		}
		return out, nil
	})
}
