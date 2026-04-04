// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
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
