// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"context"
	"time"
)

type Bot struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	PassportAPIKeyHash  string    `json:"-"`
	PassportAPIKeyID    string    `json:"passport_api_key_id"`
	HiveRoleAssignments []string  `json:"hive_role_assignments"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// BotDispatcher is the inward-facing port the scheduler + webhook
// handlers call to send vocabulary-rendered messages on a project's
// behalf. Implementations live in internal/bot/. Keeping the port in
// domain follows the same pattern as ChatProvider/IdentityProvider —
// no inward package imports the concrete implementation.
type BotDispatcher interface {
	Dispatch(ctx context.Context, projectID, eventType string, ctxData VocabularyContext) error
}
