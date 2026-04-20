// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"encoding/json"
	"io"
	"time"
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
	// ListWorkItemsByAgent returns all work items assigned to the given agent across all instances.
	ListWorkItemsByAgent(ctx context.Context, agentID string) ([]*WorkItem, error)
	UpdateWorkItem(ctx context.Context, w *WorkItem) error

	RecordTransition(ctx context.Context, h *TransitionHistory) error
	GetTransitionHistory(ctx context.Context, workItemID string) ([]*TransitionHistory, error)
}

type ApprovalStore interface {
	RecordApproval(ctx context.Context, a *Approval) error
	ListApprovals(ctx context.Context, workItemID, stepID string) ([]*Approval, error)
}

type ProjectStore interface {
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id string) error
}

type BotStore interface {
	CreateBot(ctx context.Context, b *Bot) error
	GetBotByID(ctx context.Context, id string) (*Bot, error)
	GetBotByProject(ctx context.Context, projectID string) (*Bot, error)
	DeleteBotByProject(ctx context.Context, projectID string) error
	UpdateBot(ctx context.Context, b *Bot) error
}

type VocabularyStore interface {
	CreateVocabulary(ctx context.Context, v *Vocabulary) error
	GetVocabulary(ctx context.Context, id string) (*Vocabulary, error)
	GetVocabularyByName(ctx context.Context, name string) (*Vocabulary, error)
	ListVocabularies(ctx context.Context) ([]*Vocabulary, error)
}

// Store combines all storage interfaces.
type Store interface {
	TemplateStore
	InstanceStore
	WorkItemStore
	ApprovalStore
	AuditEventStore
	ProjectStore
	BotStore
	VocabularyStore
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

// AuditFilter is the combined filter for ListAuditEventsFiltered.
// Empty/zero-valued fields are ignored. Limit 0 = no limit (handler
// caps at 500). Offset supports pagination.
type AuditFilter struct {
	Project    string
	WorkflowID string
	AgentID    string
	EventType  string    // matches AuditEvent.Type; empty = all types
	Since      time.Time // zero = no lower bound
	Until      time.Time // zero = no upper bound
	Limit      int       // 0 = no limit (handler caps at 500)
	Offset     int       // pagination offset, 0 = start
}

// AuditEventStore persists AuditEvent records. Every scheduler claim,
// release, and renewal writes one event.
type AuditEventStore interface {
	// RecordAuditEvent writes a new event. The store assigns ID and
	// OccurredAt if either is zero-valued.
	RecordAuditEvent(ctx context.Context, e *AuditEvent) error

	// ListAuditEventsByWorkflow returns every event for a workflow ID,
	// oldest first.
	ListAuditEventsByWorkflow(ctx context.Context, workflowID string) ([]*AuditEvent, error)

	// ListAuditEventsByAgent returns every event for an agent, oldest
	// first.
	ListAuditEventsByAgent(ctx context.Context, agentID string) ([]*AuditEvent, error)

	// ListAuditEventsByProject returns every event for a project (matched
	// by the project's name field on AgentClaim), oldest first.
	ListAuditEventsByProject(ctx context.Context, project string) ([]*AuditEvent, error)

	// ListAuditEventsFiltered returns events matching all non-zero fields
	// of f, oldest first. Supports pagination via Limit + Offset.
	ListAuditEventsFiltered(ctx context.Context, f AuditFilter) ([]*AuditEvent, error)
}

// PassportProvider manages API key lifecycle for bot identities.
type PassportProvider interface {
	// MintAPIKey creates a new API key in Passport. Returns the plaintext
	// (only available at creation time) and the key ID for future operations.
	MintAPIKey(ctx context.Context, name string) (plaintext, keyID string, err error)

	// RevokeAPIKey revokes an existing key by ID. Best-effort — callers
	// should log failures but not abort their primary operation.
	RevokeAPIKey(ctx context.Context, keyID string) error
}

// RuntimeDriver abstracts the runtime that actually executes an agent's
// adjutant loop. Today there is one concrete driver (Nexus VMs, future
// plan); tomorrow the long-term primary will be k8s pods + CSI
// VolumeSnapshot. Per AGENT-POOL-REMAINING-WORK.md "Load-bearing
// decisions": every method must map 1:1 to k8s primitives (Pods, PVCs,
// CSI snapshots) so the k8s driver is a translation layer, not a
// re-architecture.
//
// Seven methods. Resist bloating it.
type RuntimeDriver interface {
	// StartAgentRuntime brings an agent's runtime online, attaching the
	// per-agent credentials volume and the per-work-item working volume.
	// Returns a handle the caller must pass back to StopAgentRuntime.
	//
	// k8s mapping: create a Pod from a per-role PodTemplate with two
	// volume mounts whose PVCs are creds.ID and work.ID; return
	// {Kind:"k8s-pod", ID: pod.Name}. Nexus mapping: pick a free VM from
	// the pool, attach drives creds.ID + work.ID, start the VM.
	StartAgentRuntime(ctx context.Context, agentID string, creds, work VolumeRef) (RuntimeHandle, error)

	// StopAgentRuntime shuts down a runtime previously started with
	// StartAgentRuntime and detaches its volumes. Idempotent on already-
	// stopped handles.
	//
	// k8s mapping: kubectl delete pod h.ID (with grace period). Nexus
	// mapping: stop VM h.ID, detach drives, return VM to pool.
	StopAgentRuntime(ctx context.Context, h RuntimeHandle) error

	// IsRuntimeAlive returns true when the runtime at h is still
	// executing. Used by higher-level liveness checks; MUST NOT block
	// indefinitely — drivers should cap internal timeouts at ctx's
	// deadline or ~2 s, whichever is smaller.
	//
	// k8s mapping: Pod.Status.Phase == "Running". Nexus mapping: VM
	// status query.
	IsRuntimeAlive(ctx context.Context, h RuntimeHandle) (bool, error)

	// CloneWorkItemVolume forks the project master into a new volume
	// dedicated to `workItemID`. Returns a VolumeRef the caller passes
	// to StartAgentRuntime.
	//
	// k8s mapping: create a VolumeSnapshot of projectMaster.ID, then a
	// PVC dataSourceRef'd at the snapshot (CSI clone-from-snapshot).
	// Nexus mapping: btrfs subvolume snapshot of the master drive into
	// a new drive named work-item-<workItemID>.
	CloneWorkItemVolume(ctx context.Context, projectMaster VolumeRef, workItemID string) (VolumeRef, error)

	// DeleteVolume destroys a volume previously returned from
	// CloneWorkItemVolume. Idempotent.
	//
	// k8s mapping: delete the PVC (CSI driver handles the snapshot/
	// volume reclaim). Nexus mapping: delete the drive.
	DeleteVolume(ctx context.Context, v VolumeRef) error

	// RefreshProjectMaster pulls the given git ref into the project
	// master volume for `projectID`, running whatever warming steps
	// (build, install) the project configures. Creates the volume on
	// first call.
	//
	// k8s mapping: launch a one-shot Job that mounts the master PVC,
	// runs `git pull` + warming script, then exits; CSI snapshot of
	// the resulting PVC becomes the next clone source. Nexus mapping:
	// run an ephemeral VM with the master drive attached, run the
	// warming script, snapshot the result.
	RefreshProjectMaster(ctx context.Context, projectID string, gitRef string) error

	// GetProjectMasterRef returns the VolumeRef for `projectID`'s
	// current master, or a zero-value VolumeRef when the project has no
	// master yet (caller should RefreshProjectMaster first).
	//
	// k8s mapping: return the per-project master PVC name. Nexus
	// mapping: return the project's master drive UUID.
	GetProjectMasterRef(projectID string) VolumeRef
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
