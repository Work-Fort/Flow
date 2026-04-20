// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"time"
)

// AgentClaim is the value returned from Scheduler.AcquireAgent. The
// caller owns the claim until Scheduler.ReleaseAgent is invoked (or
// the Flow process exits, in which case Hive's sweeper will eventually
// clear the lease).
type AgentClaim struct {
	AgentID        string    // Passport agent ID (stable across claims).
	AgentName      string    // Human-friendly display name.
	Role           string    // Role the agent will fill for this claim.
	Project        string    // Project scope for this claim.
	WorkflowID     string    // Flow workflow ID that owns the lease.
	LeaseExpiresAt time.Time // Absolute expiry — renew before this.
}

// Scheduler manages the per-Flow-process agent-pool lifecycle. All
// public methods are safe for concurrent use.
//
// The interface intentionally exposes only the workflow-facing surface
// (Acquire, Release, ActiveClaims). Lease-renewal hooks
// (UpdateLease, HiveClient) are concrete-only on *scheduler.Scheduler;
// daemon wiring uses the concrete type so this interface stays a
// minimal public contract.
type Scheduler interface {
	// AcquireAgent asks Hive for a free agent matching (role, project),
	// sets its current assignment to workflowID with a lease of leaseTTL,
	// registers the claim with the lease renewer, and writes an
	// `agent_claimed` audit event. Returns ErrPoolExhausted after all
	// retries fail.
	AcquireAgent(ctx context.Context, role, project, workflowID string, leaseTTL time.Duration) (*AgentClaim, error)

	// ReleaseAgent clears the claim in Hive, de-registers it from the
	// lease renewer, and writes an `agent_released` audit event.
	ReleaseAgent(ctx context.Context, claim *AgentClaim) error

	// ActiveClaims returns a snapshot of every claim currently held by
	// this Flow process. Used by the lease renewer and by diagnostics.
	ActiveClaims() []AgentClaim
}

// HiveAgentFilter mirrors the optional query params on Hive's
// /v1/agents endpoint. Empty values omit the corresponding filter.
type HiveAgentFilter struct {
	TeamID     string
	Assigned   string // "" | "true" | "false"
	WorkflowID string
	Role       string
	Project    string
}

// HiveAgentRecord is a flattened view of a Hive Agent + its current
// pool assignment, sufficient for the agent-pool view. Field names
// mirror Hive client.Agent v0.3.0 verbatim (CurrentRole not
// AssignedRole — the published module uses CurrentRole).
type HiveAgentRecord struct {
	ID                string
	Name              string
	TeamID            string
	Model             string
	Runtime           string
	CurrentRole       string
	CurrentProject    string
	CurrentWorkflowID string
	// nil when idle; *time.Time because encoding/json does not honour
	// omitempty on a zero time.Time (zero serialises as "0001-01-01T00:00:00Z").
	LeaseExpiresAt *time.Time
}

// HiveAgentClient is the slice of the Hive Go client the scheduler
// depends on. Declared as an interface here so scheduler tests can
// substitute a fake without importing the Hive client package.
type HiveAgentClient interface {
	ClaimAgent(ctx context.Context, role, project, workflowID string, ttlSeconds int) (*HiveAgent, error)
	ReleaseAgent(ctx context.Context, id, workflowID string) error
	RenewAgentLease(ctx context.Context, id, workflowID string, ttlSeconds int) error
	ListAgents(ctx context.Context, filter HiveAgentFilter) ([]HiveAgentRecord, error)
}

// HiveAgent mirrors the fields of github.com/Work-Fort/Hive/client.Agent
// the scheduler reads. Declaring it here keeps domain free of any
// hive-client dependency; the adapter layer translates.
type HiveAgent struct {
	ID             string
	Name           string
	LeaseExpiresAt time.Time
}
