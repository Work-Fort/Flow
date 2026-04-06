// SPDX-License-Identifier: GPL-2.0-only

// Package sharkfin provides a Flow ChatProvider backed by the Sharkfin chat service.
package sharkfin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sharkfinclient "github.com/Work-Fort/sharkfin/client/go"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.ChatProvider using the Sharkfin WebSocket client.
type Adapter struct {
	client *sharkfinclient.Client
}

// New dials the Sharkfin server and returns an Adapter. baseURL is the HTTP
// base URL returned by Pylon (e.g., "http://sharkfin:16000"). token is a
// Passport JWT or API key.
//
// The WebSocket URL is derived by replacing the http(s) scheme with ws(s)
// and appending "/ws".
func New(ctx context.Context, baseURL, token string) (*Adapter, error) {
	wsURL := httpToWS(baseURL) + "/ws"
	c, err := sharkfinclient.Dial(ctx, wsURL, sharkfinclient.WithToken(token))
	if err != nil {
		return nil, fmt.Errorf("sharkfin dial %s: %w", wsURL, err)
	}
	return &Adapter{client: c}, nil
}

// httpToWS converts an HTTP base URL to the WebSocket equivalent.
func httpToWS(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasPrefix(u, "https://") {
		return "wss://" + u[8:]
	}
	if strings.HasPrefix(u, "http://") {
		return "ws://" + u[7:]
	}
	return u
}

// PostMessage sends content to channel and returns the message ID.
// metadata is attached as a JSON string sidecar on the message. If metadata
// is nil or empty, no metadata is attached.
func (a *Adapter) PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error) {
	var opts *sharkfinclient.SendOpts
	if len(metadata) > 0 {
		s := string(metadata)
		opts = &sharkfinclient.SendOpts{Metadata: &s}
	}
	id, err := a.client.SendMessage(ctx, channel, content, opts)
	if err != nil {
		return 0, fmt.Errorf("sharkfin post message to %s: %w", channel, err)
	}
	return id, nil
}

// CreateChannel creates a channel in Sharkfin.
func (a *Adapter) CreateChannel(ctx context.Context, name string, public bool) error {
	if err := a.client.CreateChannel(ctx, name, public); err != nil {
		return fmt.Errorf("sharkfin create channel %s: %w", name, err)
	}
	return nil
}

// JoinChannel joins the named channel.
func (a *Adapter) JoinChannel(ctx context.Context, channel string) error {
	if err := a.client.JoinChannel(ctx, channel); err != nil {
		return fmt.Errorf("sharkfin join channel %s: %w", channel, err)
	}
	return nil
}

// Register registers Flow's identity as a service bot with Sharkfin.
func (a *Adapter) Register(ctx context.Context) error {
	if err := a.client.Register(ctx); err != nil {
		return fmt.Errorf("sharkfin register: %w", err)
	}
	return nil
}

// RegisterWebhook registers a webhook callback URL. Returns the webhook ID.
func (a *Adapter) RegisterWebhook(ctx context.Context, callbackURL string) (string, error) {
	id, err := a.client.RegisterWebhook(ctx, callbackURL)
	if err != nil {
		return "", fmt.Errorf("sharkfin register webhook: %w", err)
	}
	return id, nil
}

// ListWebhooks returns all registered webhooks for this identity.
func (a *Adapter) ListWebhooks(ctx context.Context) ([]sharkfinclient.Webhook, error) {
	whs, err := a.client.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("sharkfin list webhooks: %w", err)
	}
	return whs, nil
}

// Close cleanly shuts down the WebSocket connection.
func (a *Adapter) Close() error {
	return a.client.Close()
}

// Ensure Adapter satisfies domain.ChatProvider at compile time.
var _ domain.ChatProvider = (*Adapter)(nil)
