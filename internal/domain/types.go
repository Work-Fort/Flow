// SPDX-License-Identifier: GPL-2.0-only

// Package domain defines core types and port interfaces for Flow.
// This package has no infrastructure dependencies.
package domain

import (
	"encoding/json"
	"time"
)

type StepType string
type ApprovalMode string
type ApprovalDecision string
type InstanceStatus string
type Priority string

const (
	StepTypeTask StepType = "task"
	StepTypeGate StepType = "gate"

	ApprovalModeAny       ApprovalMode = "any"
	ApprovalModeUnanimous ApprovalMode = "unanimous"

	ApprovalDecisionApproved ApprovalDecision = "approved"
	ApprovalDecisionRejected ApprovalDecision = "rejected"

	InstanceStatusActive    InstanceStatus = "active"
	InstanceStatusPaused    InstanceStatus = "paused"
	InstanceStatusCompleted InstanceStatus = "completed"
	InstanceStatusArchived  InstanceStatus = "archived"

	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityNormal   Priority = "normal"
	PriorityLow      Priority = "low"
)

type WorkflowTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Version          int               `json:"version"`
	Steps            []Step            `json:"steps"`
	Transitions      []Transition      `json:"transitions"`
	RoleMappings     []RoleMapping     `json:"role_mappings"`
	IntegrationHooks []IntegrationHook `json:"integration_hooks"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Step struct {
	ID         string          `json:"id"`
	TemplateID string          `json:"template_id"`
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Type       StepType        `json:"type"`
	Position   int             `json:"position"`
	Approval   *ApprovalConfig `json:"approval,omitempty"`
}

type ApprovalConfig struct {
	Mode              ApprovalMode `json:"mode"`
	RequiredApprovers int          `json:"required_approvers"`
	ApproverRoleID    string       `json:"approver_role_id"`
	RejectionStepID   string       `json:"rejection_step_id,omitempty"`
}

type Transition struct {
	ID             string `json:"id"`
	TemplateID     string `json:"template_id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	FromStepID     string `json:"from_step_id"`
	ToStepID       string `json:"to_step_id"`
	Guard          string `json:"guard,omitempty"`
	RequiredRoleID string `json:"required_role_id,omitempty"`
}

type RoleMapping struct {
	ID             string   `json:"id"`
	TemplateID     string   `json:"template_id"`
	StepID         string   `json:"step_id"`
	RoleID         string   `json:"role_id"`
	AllowedActions []string `json:"allowed_actions"`
}

type IntegrationHook struct {
	ID           string          `json:"id"`
	TemplateID   string          `json:"template_id"`
	TransitionID string          `json:"transition_id"`
	Event        string          `json:"event"`
	AdapterType  string          `json:"adapter_type"`
	Action       string          `json:"action"`
	Config       json.RawMessage `json:"config,omitempty"`
}

type WorkflowInstance struct {
	ID                 string              `json:"id"`
	TemplateID         string              `json:"template_id"`
	TemplateVersion    int                 `json:"template_version"`
	TeamID             string              `json:"team_id"`
	Name               string              `json:"name"`
	Status             InstanceStatus      `json:"status"`
	IntegrationConfigs []IntegrationConfig `json:"integration_configs,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type IntegrationConfig struct {
	ID          string          `json:"id"`
	InstanceID  string          `json:"instance_id"`
	AdapterType string          `json:"adapter_type"`
	Config      json.RawMessage `json:"config"`
}

type WorkItem struct {
	ID              string          `json:"id"`
	InstanceID      string          `json:"instance_id"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	CurrentStepID   string          `json:"current_step_id"`
	AssignedAgentID string          `json:"assigned_agent_id,omitempty"`
	Priority        Priority        `json:"priority"`
	Fields          json.RawMessage `json:"fields,omitempty"`
	ExternalLinks   []ExternalLink  `json:"external_links,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ExternalLink struct {
	ID          string `json:"id"`
	WorkItemID  string `json:"work_item_id"`
	ServiceType string `json:"service_type"`
	Adapter     string `json:"adapter"`
	ExternalID  string `json:"external_id"`
	URL         string `json:"url,omitempty"`
}

type TransitionHistory struct {
	ID           string    `json:"id"`
	WorkItemID   string    `json:"work_item_id"`
	FromStepID   string    `json:"from_step_id"`
	ToStepID     string    `json:"to_step_id"`
	TransitionID string    `json:"transition_id"`
	TriggeredBy  string    `json:"triggered_by"`
	Reason       string    `json:"reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type Approval struct {
	ID         string           `json:"id"`
	WorkItemID string           `json:"work_item_id"`
	StepID     string           `json:"step_id"`
	AgentID    string           `json:"agent_id"`
	Decision   ApprovalDecision `json:"decision"`
	Comment    string           `json:"comment,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
}

// IdentityAgent is Flow's view of a Hive agent — the fields Flow needs.
type IdentityAgent struct {
	ID     string
	Name   string
	TeamID string
	Roles  []IdentityRole
}

// IdentityRole is Flow's view of a Hive role.
type IdentityRole struct {
	ID       string
	Name     string
	ParentID string
}

// --- runtime driver value types ---

// VolumeRef identifies a volume/drive the runtime can attach to an agent
// runtime. It is deliberately opaque: the orchestrator passes whatever
// the driver returned from CloneWorkItemVolume or GetProjectMasterRef
// back into StartAgentRuntime. Drivers interpret the Ref themselves.
//
// Maps cleanly to k8s: Kind="k8s-pvc", ID=<PVC name in the agent
// namespace>. Maps to Nexus: Kind="nexus-drive", ID=<drive UUID>.
type VolumeRef struct {
	// Kind is driver-specific ("nexus-drive", "k8s-pvc", "stub", …).
	Kind string
	// ID is driver-specific (drive ID, PVC name, …).
	ID string
}

// --- audit events ---

type AuditEventType string

const (
	AuditEventAgentClaimed          AuditEventType = "agent_claimed"
	AuditEventAgentReleased         AuditEventType = "agent_released"
	AuditEventLeaseRenewed          AuditEventType = "lease_renewed"
	AuditEventLeaseExpiredBySweeper AuditEventType = "lease_expired_by_sweeper"
	AuditEventCombinePushReceived   AuditEventType = "combine_push_received"
	AuditEventCombineMergeReceived  AuditEventType = "combine_merge_received"
)

// AuditEvent is a single durable record of an agent-pool lifecycle
// transition. Flow is the legal audit primary; Sharkfin transcripts are
// a human-readable derivation.
//
// AuditEventLeaseExpiredBySweeper is reserved for a future plan that
// wires either a Hive-sweeper webhook into Flow or a Flow-startup
// reconciliation pass. No Step 1 code path produces this type.
type AuditEvent struct {
	ID             string
	OccurredAt     time.Time
	Type           AuditEventType
	AgentID        string
	AgentName      string
	WorkflowID     string
	Role           string
	Project        string
	LeaseExpiresAt time.Time       // zero-valued when not applicable.
	Payload        json.RawMessage // free-form context; may be nil.
}

// RuntimeHandle identifies a running agent runtime instance. The
// orchestrator retains the handle between StartAgentRuntime and
// StopAgentRuntime; drivers interpret it themselves.
//
// Maps to k8s: Kind="k8s-pod", ID=<pod name in the agent namespace>.
// Maps to Nexus: Kind="nexus-vm", ID=<VM ID>.
type RuntimeHandle struct {
	// Kind is driver-specific ("nexus-vm", "k8s-pod", "stub", …).
	Kind string
	// ID is driver-specific (VM ID, pod name, …).
	ID string
}
