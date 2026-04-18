// SPDX-License-Identifier: GPL-2.0-only

// Package sharkfin provides a Flow ChatProvider backed by the Sharkfin chat service.
package sharkfin

import (
	"context"
	"encoding/json"
	"fmt"

	sharkfinclient "github.com/Work-Fort/sharkfin/client/go"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.ChatProvider using the Sharkfin REST API.
//
// Flow receives chat events through the Sharkfin webhook receiver
// (internal/daemon/webhook_sharkfin.go), so it has no need for the
// WebSocket event stream. A REST-only client avoids the WS goroutine,
// reconnection state, and pending-request machinery.
type Adapter struct {
	client *sharkfinclient.RESTClient
}

// New constructs an Adapter. baseURL is the HTTP base URL returned by
// Pylon (e.g., "http://sharkfin:16000"). token is a Passport JWT or
// API key. No network I/O happens at construction time.
func New(baseURL, token string) *Adapter {
	return &Adapter{
		client: sharkfinclient.NewRESTClient(baseURL, sharkfinclient.WithToken(token)),
	}
}

// PostMessage sends content to channel and returns the message ID.
// metadata is attached as a JSON sidecar on the message. If metadata
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

// Close releases the underlying HTTP client. No-op for the REST
// client today; provided so callers can write symmetric setup/teardown.
func (a *Adapter) Close() error {
	return a.client.Close()
}

// Ensure Adapter satisfies domain.ChatProvider at compile time.
var _ domain.ChatProvider = (*Adapter)(nil)
