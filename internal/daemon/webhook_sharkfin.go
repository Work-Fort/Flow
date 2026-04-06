// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"encoding/json"
	"net/http"
)

// sharkfinWebhookPayload is the JSON body Sharkfin POSTs to Flow.
// Field names match WebhookPayload in Sharkfin's pkg/daemon/webhooks.go.
type sharkfinWebhookPayload struct {
	Event       string  `json:"event"`
	ChannelID   int64   `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	ChannelType string  `json:"channel_type"`
	From        string  `json:"from"`
	FromType    string  `json:"from_type"`
	MessageID   int64   `json:"message_id"`
	Body        string  `json:"body"`
	Metadata    *string `json:"metadata"` // JSON string or null
	SentAt      string  `json:"sent_at"`

	// Legacy field — may be empty on per-identity webhooks.
	Channel string `json:"channel,omitempty"`
}

// sharkfinMessageMeta is the sidecar metadata on messages sent with a Flow command.
type sharkfinMessageMeta struct {
	EventType    string          `json:"event_type"`
	EventPayload json.RawMessage `json:"event_payload"`
}

// sharkfinCommandPayload is the event_payload when event_type == "flow_command".
type sharkfinCommandPayload struct {
	Action     string `json:"action"`
	WorkItemID string `json:"work_item_id,omitempty"`
}

// FlowCommand is a parsed, dispatched command received from Sharkfin.
// Exported so that tests in package daemon_test can reference it.
type FlowCommand struct {
	Action     string
	WorkItemID string
	FromAgent  string
	Channel    string
	MessageID  int64
}

// CommandHandler is called when a valid Flow command is parsed from a webhook.
type CommandHandler func(cmd *FlowCommand)

// HandleSharkfinWebhook returns an http.Handler for POST /v1/webhooks/sharkfin.
// If handler is nil, commands are parsed but not dispatched (useful for testing
// parse-only behaviour). Always responds 204 No Content — Sharkfin does not
// use the response body.
func HandleSharkfinWebhook(handler CommandHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload sharkfinWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Only act on messages that carry a Flow command in metadata.
		if payload.Metadata != nil && *payload.Metadata != "" {
			var meta sharkfinMessageMeta
			if err := json.Unmarshal([]byte(*payload.Metadata), &meta); err == nil {
				if meta.EventType == "flow_command" {
					var cmdPayload sharkfinCommandPayload
					if err := json.Unmarshal(meta.EventPayload, &cmdPayload); err == nil && handler != nil {
						cmd := &FlowCommand{
							Action:     cmdPayload.Action,
							WorkItemID: cmdPayload.WorkItemID,
							FromAgent:  payload.From,
							Channel:    payload.ChannelName,
							MessageID:  payload.MessageID,
						}
						handler(cmd)
					}
				}
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
