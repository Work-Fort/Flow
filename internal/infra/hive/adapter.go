// SPDX-License-Identifier: GPL-2.0-only

// Package hive provides a Flow IdentityProvider backed by the Hive identity service.
package hive

import (
	"context"
	"errors"
	"fmt"

	hiveclient "github.com/Work-Fort/Hive/client"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.IdentityProvider using the Hive REST API.
type Adapter struct {
	client *hiveclient.Client
}

// New creates a new Adapter. baseURL is the Hive daemon URL (e.g.,
// "http://127.0.0.1:17000"). token is a Passport JWT or API key.
func New(baseURL, token string) *Adapter {
	return &Adapter{client: hiveclient.New(baseURL, token)}
}

// ResolveAgent fetches the agent and their role assignments from Hive, and
// returns a Flow domain IdentityAgent. Returns domain.ErrNotFound if Hive
// returns 404.
func (a *Adapter) ResolveAgent(ctx context.Context, agentID string) (*domain.IdentityAgent, error) {
	awr, err := a.client.GetAgent(ctx, agentID)
	if err != nil {
		return nil, mapHiveError(err, agentID, "agent")
	}
	roles := make([]domain.IdentityRole, 0, len(awr.Roles))
	for _, ar := range awr.Roles {
		roles = append(roles, domain.IdentityRole{ID: ar.RoleID})
	}
	return &domain.IdentityAgent{
		ID:     awr.ID,
		Name:   awr.Name,
		TeamID: awr.TeamID,
		Roles:  roles,
	}, nil
}

// ResolveRole fetches a single role by ID. Returns domain.ErrNotFound if Hive
// returns 404.
func (a *Adapter) ResolveRole(ctx context.Context, roleID string) (*domain.IdentityRole, error) {
	r, err := a.client.GetRole(ctx, roleID)
	if err != nil {
		return nil, mapHiveError(err, roleID, "role")
	}
	return &domain.IdentityRole{
		ID:       r.ID,
		Name:     r.Name,
		ParentID: r.ParentID,
	}, nil
}

// GetTeamMembers returns all agents in the given team. Each agent has no roles
// populated — use ResolveAgent for role-aware lookups.
func (a *Adapter) GetTeamMembers(ctx context.Context, teamID string) ([]domain.IdentityAgent, error) {
	agents, err := a.client.ListAgents(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("hive list agents for team %s: %w", teamID, err)
	}
	out := make([]domain.IdentityAgent, 0, len(agents))
	for _, ag := range agents {
		out = append(out, domain.IdentityAgent{
			ID:     ag.ID,
			Name:   ag.Name,
			TeamID: ag.TeamID,
		})
	}
	return out, nil
}

// GetAgentRoles returns the roles assigned to the agent. Unlike ResolveAgent,
// role Name and ParentID are not populated — only IDs are returned, which is
// sufficient for permission checks.
func (a *Adapter) GetAgentRoles(ctx context.Context, agentID string) ([]domain.IdentityRole, error) {
	awr, err := a.client.GetAgent(ctx, agentID)
	if err != nil {
		return nil, mapHiveError(err, agentID, "agent")
	}
	roles := make([]domain.IdentityRole, 0, len(awr.Roles))
	for _, ar := range awr.Roles {
		roles = append(roles, domain.IdentityRole{ID: ar.RoleID})
	}
	return roles, nil
}

// ClaimAgent implements domain.HiveAgentClient — delegates to the Hive
// client. Maps hiveclient.ErrConflict to domain.ErrPoolExhausted so the
// scheduler can retry-vs-surface on a single sentinel.
//
// Per-endpoint disambiguation: Hive's /claim only returns 409 on
// pool-exhausted; /release and /renew only return 409 on workflow-id
// mismatch (see hive/lead/internal/daemon/rest_huma.go:34-37). This
// adapter exploits that to map ErrConflict differently per method.
func (a *Adapter) ClaimAgent(ctx context.Context, role, project, workflowID string, ttlSeconds int) (*domain.HiveAgent, error) {
	ag, err := a.client.ClaimAgent(ctx, role, project, workflowID, ttlSeconds)
	if err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return nil, domain.ErrPoolExhausted
		}
		return nil, fmt.Errorf("hive claim agent: %w", err)
	}
	return &domain.HiveAgent{
		ID:             ag.ID,
		Name:           ag.Name,
		LeaseExpiresAt: ag.LeaseExpiresAt,
	}, nil
}

// ReleaseAgent implements domain.HiveAgentClient — maps 409 to
// domain.ErrWorkflowMismatch (the only 409 case at /release).
func (a *Adapter) ReleaseAgent(ctx context.Context, id, workflowID string) error {
	if err := a.client.ReleaseAgent(ctx, id, workflowID); err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return domain.ErrWorkflowMismatch
		}
		if errors.Is(err, hiveclient.ErrNotFound) {
			return fmt.Errorf("agent %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("hive release agent %s: %w", id, err)
	}
	return nil
}

// RenewAgentLease implements domain.HiveAgentClient.
func (a *Adapter) RenewAgentLease(ctx context.Context, id, workflowID string, ttlSeconds int) error {
	if err := a.client.RenewAgentLease(ctx, id, workflowID, ttlSeconds); err != nil {
		if errors.Is(err, hiveclient.ErrConflict) {
			return domain.ErrWorkflowMismatch
		}
		if errors.Is(err, hiveclient.ErrNotFound) {
			return fmt.Errorf("agent %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("hive renew lease %s: %w", id, err)
	}
	return nil
}

// Compile-time assertions.
var _ domain.IdentityProvider = (*Adapter)(nil)
var _ domain.HiveAgentClient = (*Adapter)(nil)

// mapHiveError converts a Hive client error to a domain error where the
// mapping is well-defined (404 → ErrNotFound). Other errors pass through
// wrapped with context.
//
// The Hive client uses sentinel errors via Unwrap — errors.Is is correct here.
// Do not inspect APIError.StatusCode directly.
func mapHiveError(err error, id, kind string) error {
	if errors.Is(err, hiveclient.ErrNotFound) {
		return fmt.Errorf("%s %s: %w", kind, id, domain.ErrNotFound)
	}
	return fmt.Errorf("hive %s %s: %w", kind, id, err)
}
