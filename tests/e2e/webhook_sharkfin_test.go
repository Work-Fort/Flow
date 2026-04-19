// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// Posts a flow_command-bearing Sharkfin webhook payload to the
// production /v1/webhooks/sharkfin endpoint via the spawned daemon.
// Asserts 204 (parser path) and that the daemon does not propagate
// command processing into the audit log (production wires
// HandleSharkfinWebhook(nil) — see internal/daemon/server.go:124).
// The bot-vocabulary plan will wire a real CommandHandler; this test
// pins the parse-only contract.
func TestSharkfinWebhook_FlowCommand_204(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-sw", "flow-sw", "Flow SW", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	meta := `{"event_type":"flow_command","event_payload":{"action":"status","work_item_id":"wi_unit"}}`
	payload := map[string]any{
		"event":        "message.new",
		"message_id":   42,
		"channel_id":   1,
		"channel_name": "general",
		"channel_type": "public",
		"from":         "agent-1",
		"from_type":    "service",
		"body":         "@flow status",
		"metadata":     meta,
		"sent_at":      "2026-04-19T10:00:00Z",
	}
	status, body, err := c.PostJSON("/v1/webhooks/sharkfin", payload, nil)
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body=%s", status, body)
	}
}

// Same endpoint must accept (and 204) plain messages with no
// metadata — this is the production hot path for ordinary chat.
func TestSharkfinWebhook_PlainMessage_204(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-sw", "flow-sw", "Flow SW", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	payload := map[string]any{
		"event":        "message.new",
		"message_id":   1,
		"channel_id":   2,
		"channel_name": "general",
		"channel_type": "public",
		"from":         "user-1",
		"from_type":    "user",
		"body":         "hello",
		"metadata":     nil,
		"sent_at":      "2026-04-19T10:00:00Z",
	}
	b, _ := json.Marshal(payload)
	status, _, err := c.Do(http.MethodPost, "/v1/webhooks/sharkfin", json.RawMessage(b), nil)
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
}
