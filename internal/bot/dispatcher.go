// SPDX-License-Identifier: GPL-2.0-only

// Package bot owns the per-project bot dispatch surface — the thing
// that takes a workflow event, looks up the project's vocabulary,
// and routes the rendered message through the ChatProvider port.
//
// It is the only consumer of Vocabulary.RenderEvent + ChatProvider.PostMessage
// in production code (the integration-hook path in internal/workflow/hooks.go
// remains as a less-flexible legacy path until templates migrate to
// vocabulary-driven posting).
package bot

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Flow/internal/domain"
)

type Dispatcher struct {
	store domain.Store
	chat  domain.ChatProvider
}

func New(store domain.Store, chat domain.ChatProvider) *Dispatcher {
	return &Dispatcher{store: store, chat: chat}
}

// Dispatch renders an event for the given project and posts it to
// the project's Sharkfin channel. Returns nil if the chat provider
// is nil (chat disabled) so callers don't need to nil-check.
func (d *Dispatcher) Dispatch(ctx context.Context, projectID, eventType string, ctxData domain.VocabularyContext) error {
	if d.chat == nil {
		return nil
	}
	p, err := d.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project %s: %w", projectID, err)
	}
	v, err := d.store.GetVocabulary(ctx, p.VocabularyID)
	if err != nil {
		return fmt.Errorf("load vocabulary %s: %w", p.VocabularyID, err)
	}
	ctxData.Project = p
	text, meta, err := v.RenderEvent(eventType, ctxData)
	if err != nil {
		return err
	}
	if _, err := d.chat.PostMessage(ctx, p.ChannelName, text, meta); err != nil {
		return fmt.Errorf("chat post for project %s: %w", projectID, err)
	}
	return nil
}
