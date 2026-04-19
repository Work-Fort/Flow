// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// Plan A's harness Client doesn't expose a way to set arbitrary
// headers per request; postCombineE2E builds a raw request to add
// X-SoftServe-Event. Plan B uses raw http.Client here rather than
// extending the Client API — single-use, single-test convenience.
func postCombineE2E(t *testing.T, env *harness.Env, tok, event string, payload any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, env.Daemon.BaseURL()+"/v1/webhooks/combine", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", event)
	req.Header.Set("X-SoftServe-Delivery", "del-e2e")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func TestCombineWebhook_PushAndMergeFlowsThroughDaemon(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-cw", "flow-cw", "Flow CW", "service")

	// Push.
	status, body := postCombineE2E(t, env, tok, "push", map[string]any{
		"repository": map[string]any{"name": "flow"},
		"ref":        "refs/heads/main",
		"before":     "abc",
		"after":      "def",
	})
	if status != http.StatusNoContent {
		t.Errorf("push status = %d, want 204; body=%s", status, body)
	}

	// Merge.
	status, body = postCombineE2E(t, env, tok, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 7, "target_branch": "main"},
		"sender":       map[string]any{"username": "agent-merge"},
	})
	if status != http.StatusNoContent {
		t.Errorf("merge status = %d, want 204; body=%s", status, body)
	}

	// Ignored event type.
	status, body = postCombineE2E(t, env, tok, "issue_opened", map[string]any{"number": 1})
	if status != http.StatusNoContent {
		t.Errorf("ignored event status = %d, want 204; body=%s", status, body)
	}
}
