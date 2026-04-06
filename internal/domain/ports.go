// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"encoding/json"
	"io"
)

type TemplateStore interface {
	CreateTemplate(ctx context.Context, t *WorkflowTemplate) error
	GetTemplate(ctx context.Context, id string) (*WorkflowTemplate, error)
	ListTemplates(ctx context.Context) ([]*WorkflowTemplate, error)
	UpdateTemplate(ctx context.Context, t *WorkflowTemplate) error
	DeleteTemplate(ctx context.Context, id string) error
}

type InstanceStore interface {
	CreateInstance(ctx context.Context, i *WorkflowInstance) error
	GetInstance(ctx context.Context, id string) (*WorkflowInstance, error)
	ListInstances(ctx context.Context, teamID string) ([]*WorkflowInstance, error)
	UpdateInstance(ctx context.Context, i *WorkflowInstance) error
}

type WorkItemStore interface {
	CreateWorkItem(ctx context.Context, w *WorkItem) error
	GetWorkItem(ctx context.Context, id string) (*WorkItem, error)
	// ListWorkItems filters by instanceID (required), stepID, agentID, priority (all optional except instanceID).
	ListWorkItems(ctx context.Context, instanceID, stepID, agentID string, priority Priority) ([]*WorkItem, error)
	UpdateWorkItem(ctx context.Context, w *WorkItem) error

	RecordTransition(ctx context.Context, h *TransitionHistory) error
	GetTransitionHistory(ctx context.Context, workItemID string) ([]*TransitionHistory, error)
}

type ApprovalStore interface {
	RecordApproval(ctx context.Context, a *Approval) error
	ListApprovals(ctx context.Context, workItemID, stepID string) ([]*Approval, error)
}

// Store combines all storage interfaces.
type Store interface {
	TemplateStore
	InstanceStore
	WorkItemStore
	ApprovalStore
	Ping(ctx context.Context) error
	io.Closer
}

// ChatProvider posts messages and manages channels in an external chat service.
// It is an optional dependency — if nil, chat notifications are skipped.
type ChatProvider interface {
	// PostMessage sends a message to the named channel and returns the message ID.
	// metadata may be nil.
	PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error)

	// CreateChannel creates a channel with the given name and visibility.
	CreateChannel(ctx context.Context, name string, public bool) error

	// JoinChannel joins the named channel.
	JoinChannel(ctx context.Context, channel string) error
}

// IdentityProvider resolves agents and roles from an external identity service.
// It is an optional dependency — if nil, role checks are skipped.
type IdentityProvider interface {
	// ResolveAgent returns the agent with their current role assignments.
	ResolveAgent(ctx context.Context, agentID string) (*IdentityAgent, error)

	// ResolveRole returns the role record for the given role ID.
	ResolveRole(ctx context.Context, roleID string) (*IdentityRole, error)

	// GetTeamMembers returns all agents belonging to the given team.
	GetTeamMembers(ctx context.Context, teamID string) ([]IdentityAgent, error)

	// GetAgentRoles returns the roles assigned to the given agent.
	GetAgentRoles(ctx context.Context, agentID string) ([]IdentityRole, error)
}
