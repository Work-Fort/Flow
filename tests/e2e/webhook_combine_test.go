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

func TestCombineWebhook_MergeDispatchesToProjectChannel(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-cw2", "flow-cw2", "Flow CW2", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Create a project whose name matches the Combine repo name.
	// Default vocab is SDLC which defines the "merged" event.
	var prj struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/projects",
		map[string]any{"name": "flow", "channel_name": "#flow"}, &prj); err != nil || status != 201 {
		t.Fatalf("create project: status=%d err=%v", status, err)
	}

	// POST a pull_request_merged for repo "flow".
	status, body := postCombineE2E(t, env, tok, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 42, "target_branch": "main"},
		"sender":       map[string]any{"username": "agent-1"},
	})
	if status != http.StatusNoContent {
		t.Fatalf("merge status = %d, want 204; body=%s", status, body)
	}

	msgs := env.Sharkfin.Messages()
	var found bool
	for _, m := range msgs {
		if m.Channel == "#flow" {
			found = true
			if !bytesContains([]byte(m.Body), "Merged PR #42") {
				t.Errorf("merge message body = %q, want to contain 'Merged PR #42'", m.Body)
			}
			if et, ok := m.Metadata["event_type"]; !ok || et != "merged" {
				t.Errorf("metadata event_type = %v, want 'merged'", et)
			}
		}
	}
	if !found {
		t.Errorf("no message posted to #flow channel; messages = %+v", msgs)
	}
}
