// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// filterMessages returns messages on the given channel name.
func filterMessages(msgs []harness.SharkfinMessage, channel string) []harness.SharkfinMessage {
	var out []harness.SharkfinMessage
	for _, m := range msgs {
		if m.Channel == channel {
			out = append(out, m)
		}
	}
	return out
}

// TestBotVocabulary_SDLCRoundTrip drives the canonical SDLC loop
// through the project + bot + vocabulary surface and asserts rendered
// messages appear on the correct channel with correct body + metadata.
func TestBotVocabulary_SDLCRoundTrip(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	env.Hive.SeedPoolAgent("a_sdlc_1", "agent-sdlc", "team-sdlc")
	tok := env.Daemon.SignJWT("svc-sdlc", "flow-sdlc", "Flow SDLC", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Create "flow" project with default SDLC vocabulary.
	var prj struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/projects",
		map[string]any{"name": "flow", "channel_name": "#flow"}, &prj); err != nil || status != 201 {
		t.Fatalf("create project: status=%d err=%v", status, err)
	}

	// Step 1: claim via scheduler diag.
	var claim struct {
		AgentID    string `json:"agent_id"`
		WorkflowID string `json:"workflow_id"`
	}
	if status, _, err := c.PostJSON("/v1/scheduler/_diag/claim", map[string]any{
		"role":              "developer",
		"project":           "flow",
		"workflow_id":       "wf-sdlc-1",
		"lease_ttl_seconds": 30,
	}, &claim); err != nil || status != 200 {
		t.Fatalf("claim: status=%d err=%v", status, err)
	}

	// Step 2: assert task_assigned was dispatched to #flow.
	msgs := filterMessages(env.Sharkfin.Messages(), "#flow")
	if len(msgs) < 1 {
		t.Fatalf("no message on #flow after claim; all messages = %+v", env.Sharkfin.Messages())
	}
	assignMsg := msgs[0]
	if !strings.Contains(assignMsg.Body, "Task assigned") {
		t.Errorf("claim message body = %q, want 'Task assigned'", assignMsg.Body)
	}
	if et, ok := assignMsg.Metadata["event_type"]; !ok || et != "task_assigned" {
		t.Errorf("claim message event_type = %v, want task_assigned", et)
	}

	// Step 3: POST a pull_request_merged for repo "flow".
	postCombineE2E(t, env, tok, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 99, "target_branch": "main"},
		"sender":       map[string]any{"username": "agent-sdlc"},
	})

	// Step 4: assert merged message.
	msgs = filterMessages(env.Sharkfin.Messages(), "#flow")
	var mergeMsg *harness.SharkfinMessage
	for i := range msgs {
		if strings.Contains(msgs[i].Body, "Merged PR #99") {
			mergeMsg = &msgs[i]
		}
	}
	if mergeMsg == nil {
		t.Fatalf("no 'Merged PR #99' message on #flow; messages = %+v", msgs)
	}
	if et, ok := mergeMsg.Metadata["event_type"]; !ok || et != "merged" {
		t.Errorf("merge message event_type = %v, want merged", et)
	}

	// Step 5: release (returns 204).
	if status, _, err := c.PostJSON("/v1/scheduler/_diag/release", map[string]any{
		"agent_id":    claim.AgentID,
		"workflow_id": claim.WorkflowID,
	}, nil); err != nil || (status != 200 && status != 204) {
		t.Fatalf("release: status=%d err=%v", status, err)
	}

	// Step 6: assert task_completed message.
	msgs = filterMessages(env.Sharkfin.Messages(), "#flow")
	var completedMsg *harness.SharkfinMessage
	for i := range msgs {
		if strings.Contains(msgs[i].Body, "Task completed") {
			completedMsg = &msgs[i]
		}
	}
	if completedMsg == nil {
		t.Fatalf("no 'Task completed' message on #flow; messages = %+v", msgs)
	}
	if et, ok := completedMsg.Metadata["event_type"]; !ok || et != "task_completed" {
		t.Errorf("release message event_type = %v, want task_completed", et)
	}

	// Step 7: per-project audit returns only flow-project events.
	var audit struct {
		Events []map[string]any `json:"events"`
	}
	if status, _, err := c.GetJSON("/v1/projects/"+prj.ID+"/audit", &audit); err != nil || status != 200 {
		t.Fatalf("get project audit: status=%d err=%v", status, err)
	}
	for _, e := range audit.Events {
		if proj, _ := e["project"].(string); proj != "flow" {
			t.Errorf("audit event project = %q, want 'flow'", proj)
		}
	}
}

// TestBotVocabulary_BugTrackerVocabPlugIn proves the per-workflow plug-in
// path: a project bound to bug-tracker vocab renders bug-tracker messages
// and never SDLC messages.
func TestBotVocabulary_BugTrackerVocabPlugIn(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	env.Hive.SeedPoolAgent("a_bug_1", "agent-bug", "team-bug")
	tok := env.Daemon.SignJWT("svc-bug", "flow-bug", "Flow Bug", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Step 1: find bug-tracker vocabulary ID.
	var vocabs []map[string]any
	if status, _, err := c.GetJSON("/v1/vocabularies", &vocabs); err != nil || status != 200 {
		t.Fatalf("list vocabularies: %d %v", status, err)
	}
	var bugVocID string
	for _, v := range vocabs {
		if v["name"] == "bug-tracker" {
			bugVocID, _ = v["id"].(string)
		}
	}
	if bugVocID == "" {
		t.Fatal("bug-tracker vocabulary not found")
	}

	// Step 2: create "tracker" project with bug-tracker vocab.
	var prj struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/projects",
		map[string]any{"name": "tracker", "channel_name": "#tracker", "vocabulary_id": bugVocID},
		&prj); err != nil || status != 201 {
		t.Fatalf("create project: status=%d err=%v", status, err)
	}

	// Step 3: claim for "tracker" — bug-tracker vocab has no task_assigned,
	// so no message should be posted to #tracker.
	var claim struct {
		AgentID    string `json:"agent_id"`
		WorkflowID string `json:"workflow_id"`
	}
	if status, _, err := c.PostJSON("/v1/scheduler/_diag/claim", map[string]any{
		"role":              "developer",
		"project":           "tracker",
		"workflow_id":       "wf-bug-1",
		"lease_ttl_seconds": 30,
	}, &claim); err != nil || status != 200 {
		t.Fatalf("claim: status=%d err=%v", status, err)
	}
	if n := len(filterMessages(env.Sharkfin.Messages(), "#tracker")); n != 0 {
		t.Errorf("claim on bug-tracker vocab: expected 0 messages, got %d", n)
	}

	// Step 4: POST a push to "tracker" repo — bug-tracker has no commit_landed.
	postCombineE2E(t, env, tok, "push", map[string]any{
		"repository": map[string]any{"name": "tracker"},
		"ref":        "refs/heads/main",
		"before":     "000",
		"after":      "111",
	})
	if n := len(filterMessages(env.Sharkfin.Messages(), "#tracker")); n != 0 {
		t.Errorf("push to bug-tracker project: expected 0 messages, got %d", n)
	}

	// Step 5: post bug lifecycle events directly via a dispatch diag endpoint.
	// Since we don't have a /v1/_diag/dispatch endpoint, we use the Combine
	// webhook with a fake Sharkfin-joined test bot. Instead, we verify the
	// bug lifecycle by calling the scheduler release, which fires bug_closed
	// (the vocab's release_event).
	if status, _, err := c.PostJSON("/v1/scheduler/_diag/release", map[string]any{
		"agent_id":    claim.AgentID,
		"workflow_id": claim.WorkflowID,
	}, nil); err != nil || (status != 200 && status != 204) {
		t.Fatalf("release: status=%d err=%v", status, err)
	}

	msgs := filterMessages(env.Sharkfin.Messages(), "#tracker")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (bug_closed on release), got %d: %+v", len(msgs), msgs)
	}
	releaseMsg := msgs[0]
	if !strings.Contains(releaseMsg.Body, "Closed:") {
		t.Errorf("release message body = %q, want 'Closed:'", releaseMsg.Body)
	}
	if et, ok := releaseMsg.Metadata["event_type"]; !ok || et != "bug_closed" {
		t.Errorf("release message event_type = %v, want bug_closed", et)
	}

	// Step 6: assert no SDLC templates leaked into #tracker messages.
	for _, m := range msgs {
		raw, _ := json.Marshal(m)
		if bytesContains(raw, "Task assigned") || bytesContains(raw, "Task completed") ||
			bytesContains(raw, "Merged PR") {
			t.Errorf("SDLC template text found in bug-tracker message: %s", raw)
		}
	}
}
