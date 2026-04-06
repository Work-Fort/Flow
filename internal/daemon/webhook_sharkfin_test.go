// SPDX-License-Identifier: GPL-2.0-only
package daemon_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daemon "github.com/Work-Fort/Flow/internal/daemon"
)

func TestSharkfinWebhook_IgnoresPlainMessage(t *testing.T) {
	handler := daemon.HandleSharkfinWebhook(nil)
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   42,
		"channel_id":   1,
		"channel_name": "general",
		"channel_type": "public",
		"from":         "agent1",
		"from_type":    "user",
		"body":         "hello world",
		"metadata":     nil,
		"sent_at":      "2026-04-05T10:00:00Z",
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/sharkfin", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestSharkfinWebhook_ParsesFlowCommand(t *testing.T) {
	var received *daemon.FlowCommand
	handler := daemon.HandleSharkfinWebhook(func(cmd *daemon.FlowCommand) {
		received = cmd
	})

	meta := `{"event_type":"flow_command","event_payload":{"action":"status","work_item_id":"wi_123"}}`
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   99,
		"channel_id":   2,
		"channel_name": "ops",
		"channel_type": "public",
		"from":         "agent2",
		"from_type":    "service",
		"body":         "@flow status wi_123",
		"metadata":     meta,
		"sent_at":      "2026-04-05T10:00:00Z",
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/sharkfin", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if received == nil {
		t.Fatal("command handler not called")
	}
	if received.Action != "status" {
		t.Errorf("action = %q, want %q", received.Action, "status")
	}
	if received.WorkItemID != "wi_123" {
		t.Errorf("work_item_id = %q, want %q", received.WorkItemID, "wi_123")
	}
	if received.FromAgent != "agent2" {
		t.Errorf("from = %q, want %q", received.FromAgent, "agent2")
	}
	if received.Channel != "ops" {
		t.Errorf("channel = %q, want %q", received.Channel, "ops")
	}
	if received.MessageID != 99 {
		t.Errorf("message_id = %d, want 99", received.MessageID)
	}
}
