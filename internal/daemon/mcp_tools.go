// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// jsonResult serializes v to indented JSON and returns it as an MCP text result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// RegisterTools registers every MCP tool the adjutant needs;
// count is asserted in TestRegisterTools_Count.
func RegisterTools(s *server.MCPServer, deps MCPDeps) {
	// list_templates
	s.AddTool(
		mcp.NewTool("list_templates",
			mcp.WithDescription("List all workflow templates."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			templates, err := deps.Store.ListTemplates(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list templates failed: %v", err)), nil
			}
			return jsonResult(templates)
		},
	)

	// get_template
	s.AddTool(
		mcp.NewTool("get_template",
			mcp.WithDescription("Get a workflow template by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Template ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			tmpl, err := deps.Store.GetTemplate(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get template failed: %v", err)), nil
			}
			return jsonResult(tmpl)
		},
	)

	// create_instance
	s.AddTool(
		mcp.NewTool("create_instance",
			mcp.WithDescription("Create a workflow instance from a template."),
			mcp.WithString("template_id", mcp.Required(), mcp.Description("Template ID")),
			mcp.WithString("team_id", mcp.Required(), mcp.Description("Team ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Instance name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			templateID := req.GetString("template_id", "")
			teamID := req.GetString("team_id", "")
			name := req.GetString("name", "")

			tmpl, err := deps.Store.GetTemplate(ctx, templateID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get template failed: %v", err)), nil
			}
			inst := &domain.WorkflowInstance{
				ID:              NewID("ins"),
				TemplateID:      templateID,
				TemplateVersion: tmpl.Version,
				TeamID:          teamID,
				Name:            name,
				Status:          domain.InstanceStatusActive,
			}
			if err := deps.Store.CreateInstance(ctx, inst); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create instance failed: %v", err)), nil
			}
			created, err := deps.Store.GetInstance(ctx, inst.ID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get instance failed: %v", err)), nil
			}
			return jsonResult(created)
		},
	)

	// list_instances
	s.AddTool(
		mcp.NewTool("list_instances",
			mcp.WithDescription("List workflow instances."),
			mcp.WithString("team_id", mcp.Description("Filter by team ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			teamID := req.GetString("team_id", "")
			instances, err := deps.Store.ListInstances(ctx, teamID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list instances failed: %v", err)), nil
			}
			return jsonResult(instances)
		},
	)

	// create_work_item
	s.AddTool(
		mcp.NewTool("create_work_item",
			mcp.WithDescription("Create a work item in a workflow instance."),
			mcp.WithString("instance_id", mcp.Required(), mcp.Description("Instance ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Work item title")),
			mcp.WithString("description", mcp.Description("Work item description")),
			mcp.WithString("assigned_agent_id", mcp.Description("Assigned agent ID")),
			mcp.WithString("priority", mcp.Description("Priority: critical, high, normal, low")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			instanceID := req.GetString("instance_id", "")
			title := req.GetString("title", "")
			description := req.GetString("description", "")
			assignedAgentID := req.GetString("assigned_agent_id", "")
			priorityStr := req.GetString("priority", "")

			inst, err := deps.Store.GetInstance(ctx, instanceID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get instance failed: %v", err)), nil
			}
			tmpl, err := deps.Store.GetTemplate(ctx, inst.TemplateID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get template failed: %v", err)), nil
			}
			if len(tmpl.Steps) == 0 {
				return mcp.NewToolResultError("template has no steps"), nil
			}
			firstStep := tmpl.Steps[0]
			for _, s := range tmpl.Steps {
				if s.Position < firstStep.Position {
					firstStep = s
				}
			}

			priority := domain.Priority(priorityStr)
			if priority == "" {
				priority = domain.PriorityNormal
			}

			w := &domain.WorkItem{
				ID:              NewID("wi"),
				InstanceID:      instanceID,
				Title:           title,
				Description:     description,
				CurrentStepID:   firstStep.ID,
				AssignedAgentID: assignedAgentID,
				Priority:        priority,
			}
			if err := deps.Store.CreateWorkItem(ctx, w); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create work item failed: %v", err)), nil
			}
			created, err := deps.Store.GetWorkItem(ctx, w.ID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get work item failed: %v", err)), nil
			}
			return jsonResult(created)
		},
	)

	// list_work_items
	s.AddTool(
		mcp.NewTool("list_work_items",
			mcp.WithDescription("List work items for an instance."),
			mcp.WithString("instance_id", mcp.Required(), mcp.Description("Instance ID")),
			mcp.WithString("step_id", mcp.Description("Filter by step ID")),
			mcp.WithString("agent_id", mcp.Description("Filter by agent ID")),
			mcp.WithString("priority", mcp.Description("Filter by priority")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			instanceID := req.GetString("instance_id", "")
			stepID := req.GetString("step_id", "")
			agentID := req.GetString("agent_id", "")
			priorityStr := req.GetString("priority", "")

			items, err := deps.Store.ListWorkItems(ctx, instanceID, stepID, agentID, domain.Priority(priorityStr))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list work items failed: %v", err)), nil
			}
			return jsonResult(items)
		},
	)

	// get_work_item
	s.AddTool(
		mcp.NewTool("get_work_item",
			mcp.WithDescription("Get a work item by ID including transition history."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Work item ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			w, err := deps.Store.GetWorkItem(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get work item failed: %v", err)), nil
			}
			history, err := deps.Store.GetTransitionHistory(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get history failed: %v", err)), nil
			}
			result := map[string]any{
				"work_item": w,
				"history":   history,
			}
			return jsonResult(result)
		},
	)

	// transition_work_item
	s.AddTool(
		mcp.NewTool("transition_work_item",
			mcp.WithDescription("Transition a work item to a new step."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Work item ID")),
			mcp.WithString("transition_id", mcp.Required(), mcp.Description("Transition ID")),
			mcp.WithString("actor_agent_id", mcp.Required(), mcp.Description("Actor agent ID")),
			mcp.WithString("actor_role_id", mcp.Required(), mcp.Description("Actor role ID")),
			mcp.WithString("reason", mcp.Description("Reason for transition")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			transitionID := req.GetString("transition_id", "")
			actorAgentID := req.GetString("actor_agent_id", "")
			actorRoleID := req.GetString("actor_role_id", "")
			reason := req.GetString("reason", "")

			updated, err := deps.Svc.TransitionItem(ctx, workflow.TransitionRequest{
				WorkItemID:   id,
				TransitionID: transitionID,
				ActorAgentID: actorAgentID,
				ActorRoleID:  actorRoleID,
				Reason:       reason,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(updated)
		},
	)

	// approve_work_item
	s.AddTool(
		mcp.NewTool("approve_work_item",
			mcp.WithDescription("Approve a work item at a gate step."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Work item ID")),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
			mcp.WithString("comment", mcp.Description("Approval comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			agentID := req.GetString("agent_id", "")
			comment := req.GetString("comment", "")

			updated, err := deps.Svc.ApproveItem(ctx, workflow.ApproveRequest{
				WorkItemID: id,
				AgentID:    agentID,
				Comment:    comment,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(updated)
		},
	)

	// reject_work_item
	s.AddTool(
		mcp.NewTool("reject_work_item",
			mcp.WithDescription("Reject a work item at a gate step."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Work item ID")),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
			mcp.WithString("comment", mcp.Description("Rejection comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			agentID := req.GetString("agent_id", "")
			comment := req.GetString("comment", "")

			updated, err := deps.Svc.RejectItem(ctx, workflow.RejectRequest{
				WorkItemID: id,
				AgentID:    agentID,
				Comment:    comment,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(updated)
		},
	)

	// assign_work_item
	s.AddTool(
		mcp.NewTool("assign_work_item",
			mcp.WithDescription("Assign a work item to an agent."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Work item ID")),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID to assign")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			agentID := req.GetString("agent_id", "")

			w, err := deps.Store.GetWorkItem(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get work item failed: %v", err)), nil
			}
			w.AssignedAgentID = agentID
			if err := deps.Store.UpdateWorkItem(ctx, w); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("update work item failed: %v", err)), nil
			}
			updated, err := deps.Store.GetWorkItem(ctx, w.ID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get work item failed: %v", err)), nil
			}
			return jsonResult(updated)
		},
	)

	// get_instance_status
	s.AddTool(
		mcp.NewTool("get_instance_status",
			mcp.WithDescription("Get instance status with work items grouped by step."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Instance ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			inst, err := deps.Store.GetInstance(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get instance failed: %v", err)), nil
			}
			items, err := deps.Store.ListWorkItems(ctx, id, "", "", "")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list work items failed: %v", err)), nil
			}
			byStep := make(map[string][]*domain.WorkItem)
			for _, item := range items {
				byStep[item.CurrentStepID] = append(byStep[item.CurrentStepID], item)
			}
			result := map[string]any{
				"instance": inst,
				"by_step":  byStep,
			}
			return jsonResult(result)
		},
	)

	// get_my_project: resolves via audit log for the most recent unreleased claim.
	s.AddTool(
		mcp.NewTool("get_my_project",
			mcp.WithDescription("Look up the project the agent is currently claimed for."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			agentID := req.GetString("agent_id", "")
			events, err := deps.Store.ListAuditEventsByAgent(ctx, agentID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var projectName string
			for i := len(events) - 1; i >= 0; i-- {
				if events[i].Type == domain.AuditEventAgentClaimed {
					projectName = events[i].Project
					break
				}
				if events[i].Type == domain.AuditEventAgentReleased {
					return mcp.NewToolResultError("agent has no active claim"), nil
				}
			}
			if projectName == "" {
				return mcp.NewToolResultError("agent has no claim history"), nil
			}
			p, err := deps.Store.GetProjectByName(ctx, projectName)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			v, err := deps.Store.GetVocabulary(ctx, p.VocabularyID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"project": p, "vocabulary": v})
		},
	)

	// list_my_work_items: indexed by work_items_assigned_agent_idx.
	s.AddTool(
		mcp.NewTool("list_my_work_items",
			mcp.WithDescription("List work items currently assigned to the agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			agentID := req.GetString("agent_id", "")
			items, err := deps.Store.ListWorkItemsByAgent(ctx, agentID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(items)
		},
	)

	// get_vocabulary
	s.AddTool(
		mcp.NewTool("get_vocabulary",
			mcp.WithDescription("Get a vocabulary by ID, including its event catalogue."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Vocabulary ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			v, err := deps.Store.GetVocabulary(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(v)
		},
	)
}
