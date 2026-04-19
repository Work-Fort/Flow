// SPDX-License-Identifier: GPL-2.0-only
package harness_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// Verifies the MCP client envelopes a tools/call request as JSON-RPC
// 2.0, sends Authorization: ApiKey-v1 <key>, captures the
// Mcp-Session-Id header, and decodes a single-line application/json
// response into the result.
func TestMCPClient_ToolsCallRoundTrip(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "ApiKey-v1 test-key" {
			t.Errorf("Authorization = %q, want %q", got, "ApiKey-v1 test-key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["jsonrpc"] != "2.0" || body["method"] != "tools/call" {
			t.Errorf("unexpected body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "sess-abc")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"\"ok\""}]}}`))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")

	text, err := mc.Call("ping_tool", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if text != `"ok"` {
		t.Errorf("text = %q, want %q", text, `"ok"`)
	}
	if mc.SessionID() != "sess-abc" {
		t.Errorf("session = %q, want sess-abc", mc.SessionID())
	}

	// Second call should send the captured session header back.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Mcp-Session-Id"); got != "sess-abc" {
			t.Errorf("session header = %q, want sess-abc", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"\"ok2\""}]}}`))
	}))
	defer srv2.Close()
	mc2 := harness.NewMCPClient(srv2.URL+"/mcp", "test-key")
	mc2.SetSessionID("sess-abc")
	if _, err := mc2.Call("ping_tool", nil); err != nil {
		t.Fatalf("Call (session reuse): %v", err)
	}
}

// Verifies the client extracts the last data: line from an SSE
// response (matching the mcp-bridge readResponseBody convention).
func TestMCPClient_ParsesSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notification\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"\\\"sse-ok\\\"\"}]}}\n\n"))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")
	text, err := mc.Call("ping_tool", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(text, "sse-ok") {
		t.Errorf("text = %q, want substring sse-ok", text)
	}
}

// Verifies that an MCP-level error result surfaces as a Go error.
func TestMCPClient_ErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"boom"}]}}`))
	}))
	defer srv.Close()

	mc := harness.NewMCPClient(srv.URL+"/mcp", "test-key")
	_, err := mc.Call("ping_tool", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want containing 'boom'", err)
	}
}
