// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"errors"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

func registerBotKeyRoutes(api huma.API, store domain.Store, botKeysDir string, passport domain.PassportProvider) {
	huma.Register(api, huma.Operation{
		OperationID:   "create-bot",
		Method:        http.MethodPost,
		Path:          "/v1/projects/{id}/bot",
		Summary:       "Bind a bot identity to a project",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Bots"},
	}, func(ctx context.Context, input *CreateBotInput) (*BotPlaintextOutput, error) {
		if botKeysDir == "" {
			return nil, huma.NewError(http.StatusServiceUnavailable, "bot keys directory not configured")
		}
		if _, err := store.GetProject(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}

		var plaintext, keyID string
		if input.Body.BringYourOwnKey {
			plaintext = input.Body.PassportAPIKey
			keyID = input.Body.PassportAPIKeyID
		} else {
			if passport == nil {
				return nil, huma.NewError(http.StatusServiceUnavailable, "passport provider not configured")
			}
			var err error
			plaintext, keyID, err = passport.MintAPIKey(ctx, "bot-"+input.ID)
			if err != nil {
				return nil, huma.NewError(http.StatusBadGateway, "failed to mint API key: "+err.Error())
			}
		}

		b := &domain.Bot{
			ID:                  NewID("bot"),
			ProjectID:           input.ID,
			PassportAPIKeyHash:  hashAPIKey(plaintext),
			PassportAPIKeyID:    keyID,
			HiveRoleAssignments: input.Body.HiveRoleAssignments,
		}
		if b.HiveRoleAssignments == nil {
			b.HiveRoleAssignments = []string{}
		}
		if err := store.CreateBot(ctx, b); err != nil {
			return nil, mapDomainErr(err)
		}
		if err := writeBotKeyFile(botKeysDir, b.ID, plaintext); err != nil {
			log.Warn("bot key file write failed; row committed, key recoverable via rotate",
				"bot", b.ID, "err", err)
		}
		created, err := store.GetBotByProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		var out BotPlaintextOutput
		out.Body.Bot = botToResponse(created)
		if !input.Body.BringYourOwnKey {
			out.Body.PlaintextAPIKey = plaintext
		}
		return &out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-bot",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{id}/bot",
		Summary:     "Get the bot for a project",
		Tags:        []string{"Bots"},
	}, func(ctx context.Context, input *IDPathInput) (*BotOutput, error) {
		b, err := store.GetBotByProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &BotOutput{Body: botToResponse(b)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-bot",
		Method:        http.MethodDelete,
		Path:          "/v1/projects/{id}/bot",
		Summary:       "Unbind and delete the project bot",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Bots"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		b, err := store.GetBotByProject(ctx, input.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, huma.NewError(http.StatusNotFound, err.Error())
			}
			return nil, mapDomainErr(err)
		}
		if botKeysDir != "" {
			removeBotKeyFile(botKeysDir, b.ID)
		}
		if err := store.DeleteBotByProject(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "rotate-bot-key",
		Method:        http.MethodPost,
		Path:          "/v1/projects/{id}/bot/rotate-key",
		Summary:       "Rotate the Passport API key for a project bot",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Bots"},
	}, func(ctx context.Context, input *IDPathInput) (*BotPlaintextOutput, error) {
		if botKeysDir == "" {
			return nil, huma.NewError(http.StatusServiceUnavailable, "bot keys directory not configured")
		}
		if passport == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "passport provider not configured")
		}
		b, err := store.GetBotByProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		oldKeyID := b.PassportAPIKeyID

		plaintext, keyID, err := passport.MintAPIKey(ctx, "bot-"+input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusBadGateway, "failed to mint new API key: "+err.Error())
		}

		b.PassportAPIKeyHash = hashAPIKey(plaintext)
		b.PassportAPIKeyID = keyID
		if err := store.UpdateBot(ctx, b); err != nil {
			// New key was minted but not saved — revoke it best-effort so it's not dangling.
			if rErr := passport.RevokeAPIKey(ctx, keyID); rErr != nil {
				log.Warn("rotate-key: update failed and new key revocation also failed",
					"bot", b.ID, "new_key_id", keyID, "revoke_err", rErr)
			}
			return nil, mapDomainErr(err)
		}
		if err := writeBotKeyFile(botKeysDir, b.ID, plaintext); err != nil {
			log.Warn("rotate-key: key file write failed; row updated, file stale",
				"bot", b.ID, "err", err)
		}
		// Revoke old key best-effort after the row is committed.
		if oldKeyID != "" {
			if rErr := passport.RevokeAPIKey(ctx, oldKeyID); rErr != nil {
				log.Warn("rotate-key: old key revocation failed (best-effort)",
					"bot", b.ID, "old_key_id", oldKeyID, "err", rErr)
			}
		}

		updated, err := store.GetBotByProject(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		var out BotPlaintextOutput
		out.Body.Bot = botToResponse(updated)
		out.Body.PlaintextAPIKey = plaintext
		return &out, nil
	})
}
