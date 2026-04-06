// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"encoding/json"
)

// stubChat records PostMessage calls for assertions.
type stubChat struct {
	messages []stubChatMessage
}

type stubChatMessage struct {
	Channel  string
	Content  string
	Metadata json.RawMessage
}

func (s *stubChat) PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error) {
	s.messages = append(s.messages, stubChatMessage{
		Channel:  channel,
		Content:  content,
		Metadata: metadata,
	})
	return int64(len(s.messages)), nil
}

func (s *stubChat) CreateChannel(ctx context.Context, name string, public bool) error {
	return nil
}

func (s *stubChat) JoinChannel(ctx context.Context, channel string) error {
	return nil
}
