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
