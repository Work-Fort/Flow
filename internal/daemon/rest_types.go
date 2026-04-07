// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"encoding/json"
	"time"
)

// --- shared input ---

type IDPathInput struct {
	ID string `path:"id" doc:"Resource ID"`
}

// --- templates ---

type templateResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type stepResponse struct {
	ID         string                `json:"id"`
	TemplateID string                `json:"template_id"`
	Key        string                `json:"key"`
	Name       string                `json:"name"`
	Type       string                `json:"type"`
	Position   int                   `json:"position"`
	Approval   *stepApprovalResponse `json:"approval,omitempty"`
}

type stepApprovalResponse struct {
	Mode              string `json:"mode"`
	RequiredApprovers int    `json:"required_approvers"`
	ApproverRoleID    string `json:"approver_role_id"`
	RejectionStepID   string `json:"rejection_step_id,omitempty"`
}

type transitionResponse struct {
	ID             string `json:"id"`
	TemplateID     string `json:"template_id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	FromStepID     string `json:"from_step_id"`
	ToStepID       string `json:"to_step_id"`
	Guard          string `json:"guard,omitempty"`
	RequiredRoleID string `json:"required_role_id,omitempty"`
}

type templateDetailResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Version     int                  `json:"version"`
	Steps       []stepResponse       `json:"steps"`
	Transitions []transitionResponse `json:"transitions"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

type TemplateOutput struct {
	Body templateDetailResponse
}

type TemplateListOutput struct {
	Body []templateResponse
}

type CreateTemplateInput struct {
	Body struct {
		Name        string `json:"name" doc:"Template name" minLength:"1"`
		Description string `json:"description,omitempty" doc:"Template description"`
	}
}

type PatchTemplateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name        string `json:"name,omitempty" doc:"Template name"`
		Description string `json:"description,omitempty" doc:"Template description"`
	}
}

// --- instances ---

type instanceResponse struct {
	ID              string    `json:"id"`
	TemplateID      string    `json:"template_id"`
	TemplateVersion int       `json:"template_version"`
	TeamID          string    `json:"team_id"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type InstanceOutput struct {
	Body instanceResponse
}

type InstanceListOutput struct {
	Body []instanceResponse
}

type ListInstancesInput struct {
	TeamID string `query:"team_id" doc:"Filter by team ID"`
}

type CreateInstanceInput struct {
	Body struct {
		TemplateID string `json:"template_id" doc:"Template ID" minLength:"1"`
		TeamID     string `json:"team_id" doc:"Team ID" minLength:"1"`
		Name       string `json:"name" doc:"Instance name" minLength:"1"`
	}
}

type PatchInstanceInput struct {
	ID   string `path:"id"`
	Body struct {
		Name   string `json:"name,omitempty" doc:"Instance name"`
		Status string `json:"status,omitempty" doc:"Status" enum:"active,paused,completed,archived"`
	}
}

// --- work items ---

type workItemResponse struct {
	ID              string          `json:"id"`
	InstanceID      string          `json:"instance_id"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	CurrentStepID   string          `json:"current_step_id"`
	AssignedAgentID string          `json:"assigned_agent_id,omitempty"`
	Priority        string          `json:"priority"`
	Fields          json.RawMessage `json:"fields,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type historyResponse struct {
	ID           string    `json:"id"`
	WorkItemID   string    `json:"work_item_id"`
	FromStepID   string    `json:"from_step_id"`
	ToStepID     string    `json:"to_step_id"`
	TransitionID string    `json:"transition_id"`
	TriggeredBy  string    `json:"triggered_by"`
	Reason       string    `json:"reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type workItemDetailResponse struct {
	workItemResponse
	History []historyResponse `json:"history,omitempty"`
}

type WorkItemOutput struct {
	Body workItemResponse
}

type WorkItemListOutput struct {
	Body []workItemResponse
}

type WorkItemDetailOutput struct {
	Body workItemDetailResponse
}

type WorkItemHistoryOutput struct {
	Body []historyResponse
}

type ListWorkItemsInput struct {
	ID       string `path:"id" doc:"Instance ID"`
	StepID   string `query:"step_id" doc:"Filter by step ID"`
	AgentID  string `query:"agent_id" doc:"Filter by agent ID"`
	Priority string `query:"priority" doc:"Filter by priority"`
}

type CreateWorkItemInput struct {
	ID   string `path:"id" doc:"Instance ID"`
	Body struct {
		Title           string `json:"title" doc:"Work item title" minLength:"1"`
		Description     string `json:"description,omitempty" doc:"Work item description"`
		AssignedAgentID string `json:"assigned_agent_id,omitempty" doc:"Assigned agent ID"`
		Priority        string `json:"priority,omitempty" doc:"Priority" enum:"critical,high,normal,low"`
	}
}

type PatchWorkItemInput struct {
	ID   string `path:"id" doc:"Work item ID"`
	Body struct {
		Title           string          `json:"title,omitempty" doc:"Work item title"`
		Description     string          `json:"description,omitempty" doc:"Work item description"`
		AssignedAgentID *string         `json:"assigned_agent_id" doc:"Assigned agent ID (empty string to unassign)"`
		Fields          json.RawMessage `json:"fields,omitempty" doc:"Custom fields"`
	}
}

type TransitionWorkItemInput struct {
	ID   string `path:"id" doc:"Work item ID"`
	Body struct {
		TransitionID string `json:"transition_id" doc:"Transition ID" minLength:"1"`
		ActorAgentID string `json:"actor_agent_id" doc:"Actor agent ID" minLength:"1"`
		ActorRoleID  string `json:"actor_role_id" doc:"Actor role ID" minLength:"1"`
		Reason       string `json:"reason,omitempty" doc:"Reason for transition"`
	}
}

type ApproveWorkItemInput struct {
	ID   string `path:"id" doc:"Work item ID"`
	Body struct {
		AgentID string `json:"agent_id" doc:"Agent ID" minLength:"1"`
		Comment string `json:"comment,omitempty" doc:"Approval comment"`
	}
}

type RejectWorkItemInput struct {
	ID   string `path:"id" doc:"Work item ID"`
	Body struct {
		AgentID string `json:"agent_id" doc:"Agent ID" minLength:"1"`
		Comment string `json:"comment,omitempty" doc:"Rejection comment"`
	}
}

// --- approvals ---

type approvalResponse struct {
	ID         string    `json:"id"`
	WorkItemID string    `json:"work_item_id"`
	StepID     string    `json:"step_id"`
	AgentID    string    `json:"agent_id"`
	Decision   string    `json:"decision"`
	Comment    string    `json:"comment,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type ApprovalListOutput struct {
	Body []approvalResponse
}
