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
		// Steps, when non-nil, REPLACES the template's step set.
		// Each Step's TemplateID is overwritten with the URL path's id.
		// Mirrors store.UpdateTemplate's replace-on-write semantics.
		Steps []stepInput `json:"steps,omitempty"`
		// Transitions, when non-nil, REPLACES the template's transition
		// set. Each Transition's TemplateID is overwritten with the
		// URL path's id.
		Transitions []transitionInput `json:"transitions,omitempty"`
	}
}

// stepInput is the request-shape of Step.
type stepInput struct {
	ID       string             `json:"id" doc:"Step ID (client-supplied)"`
	Key      string             `json:"key" doc:"Step key (unique within template)"`
	Name     string             `json:"name" doc:"Step display name"`
	Type     string             `json:"type" enum:"task,gate" doc:"Step type"`
	Position int                `json:"position" doc:"Display order"`
	Approval *stepApprovalInput `json:"approval,omitempty"`
}

type stepApprovalInput struct {
	Mode              string `json:"mode" enum:"any,unanimous"`
	RequiredApprovers int    `json:"required_approvers"`
	ApproverRoleID    string `json:"approver_role_id"`
	RejectionStepID   string `json:"rejection_step_id,omitempty"`
}

type transitionInput struct {
	ID             string `json:"id" doc:"Transition ID (client-supplied)"`
	Key            string `json:"key" doc:"Transition key"`
	Name           string `json:"name" doc:"Display name"`
	FromStepID     string `json:"from_step_id" doc:"Source step ID"`
	ToStepID       string `json:"to_step_id" doc:"Destination step ID"`
	Guard          string `json:"guard,omitempty" doc:"CEL guard expression"`
	RequiredRoleID string `json:"required_role_id,omitempty"`
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
		AssignedAgentID *string         `json:"assigned_agent_id,omitempty" doc:"Assigned agent ID (empty string to unassign)"`
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

// --- projects ---

type projectResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	TemplateID   string    `json:"template_id,omitempty"`
	ChannelName  string    `json:"channel_name"`
	VocabularyID string    `json:"vocabulary_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProjectOutput struct {
	Body projectResponse
}

type ProjectListOutput struct {
	Body []projectResponse
}

type CreateProjectInput struct {
	Body struct {
		Name                 string `json:"name" doc:"Project name" minLength:"1"`
		Description          string `json:"description,omitempty" doc:"Project description"`
		TemplateID           string `json:"template_id,omitempty" doc:"Workflow template ID"`
		ChannelName          string `json:"channel_name" doc:"Sharkfin channel name" minLength:"1"`
		VocabularyID         string `json:"vocabulary_id,omitempty" doc:"Vocabulary ID (defaults to SDLC)"`
		ChannelAlreadyExists bool   `json:"channel_already_exists,omitempty" doc:"Skip CreateChannel call (use when channel was already created)"`
	}
}

type PatchProjectInput struct {
	ID   string `path:"id"`
	Body struct {
		Name         string `json:"name,omitempty" doc:"Project name"`
		Description  string `json:"description,omitempty" doc:"Project description"`
		TemplateID   string `json:"template_id,omitempty" doc:"Workflow template ID"`
		ChannelName  string `json:"channel_name,omitempty" doc:"Sharkfin channel name"`
		VocabularyID string `json:"vocabulary_id,omitempty" doc:"Vocabulary ID"`
	}
}

// --- bots ---

type botResponse struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	PassportAPIKeyID    string    `json:"passport_api_key_id"`
	HiveRoleAssignments []string  `json:"hive_role_assignments"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type BotOutput struct {
	Body botResponse
}

type CreateBotInput struct {
	ID   string `path:"id" doc:"Project ID"`
	Body struct {
		PassportAPIKey      string   `json:"passport_api_key" doc:"Passport API key plaintext (stored hashed; not returned)" minLength:"1"`
		PassportAPIKeyID    string   `json:"passport_api_key_id" doc:"Passport key ID" minLength:"1"`
		HiveRoleAssignments []string `json:"hive_role_assignments,omitempty" doc:"Hive role IDs the bot may claim for"`
	}
}

// --- vocabularies ---

type vocabularyEventResponse struct {
	ID              string   `json:"id"`
	VocabularyID    string   `json:"vocabulary_id"`
	EventType       string   `json:"event_type"`
	MessageTemplate string   `json:"message_template"`
	MetadataKeys    []string `json:"metadata_keys,omitempty"`
}

type vocabularyResponse struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	ReleaseEvent string                    `json:"release_event,omitempty"`
	Events       []vocabularyEventResponse `json:"events,omitempty"`
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
}

type VocabularyOutput struct {
	Body vocabularyResponse
}

type VocabularyListOutput struct {
	Body []vocabularyResponse
}

// --- audit ---

type auditEventResponse struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Type       string    `json:"type"`
	AgentID    string    `json:"agent_id"`
	AgentName  string    `json:"agent_name,omitempty"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	Role       string    `json:"role,omitempty"`
	Project    string    `json:"project,omitempty"`
}

type ProjectAuditOutput struct {
	Body struct {
		Events []auditEventResponse `json:"events"`
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

// --- agents ---

type AgentFilterInput struct {
	TeamID     string `query:"team_id"`
	Assigned   string `query:"assigned" enum:"true,false"`
	WorkflowID string `query:"workflow_id"`
	Role       string `query:"role"`
	Project    string `query:"project"`
}

// agentResponse uses *time.Time for LeaseExpiresAt because
// encoding/json does not honour omitempty on a zero time.Time.
// Idle agents serialise LeaseExpiresAt as omitted; active agents
// serialise it as an RFC3339 string.
type agentResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	TeamID            string     `json:"team_id,omitempty"`
	Model             string     `json:"model,omitempty"`
	Runtime           string     `json:"runtime,omitempty"`
	CurrentRole       string     `json:"current_role,omitempty"`
	CurrentProject    string     `json:"current_project,omitempty"`
	CurrentWorkflowID string     `json:"current_workflow_id,omitempty"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at,omitempty"`
}

type AgentListOutput struct {
	Body struct {
		Agents []agentResponse `json:"agents"`
	}
}
