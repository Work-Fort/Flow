// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"net/http"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func TestProjects_CRUD(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	tok := env.Daemon.SignJWT("svc-p", "flow-p", "Flow P", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// List vocabularies — at least the SDLC seed must be present.
	var vocs []map[string]any
	if status, _, err := c.GetJSON("/v1/vocabularies", &vocs); err != nil || status != 200 {
		t.Fatalf("list vocab: status=%d err=%v", status, err)
	}
	var sdlcID string
	for _, v := range vocs {
		if v["name"] == "sdlc" {
			sdlcID = v["id"].(string)
		}
	}
	if sdlcID == "" {
		t.Fatal("SDLC vocab seed missing")
	}

	var created struct {
		ID string `json:"id"`
	}
	if status, _, err := c.PostJSON("/v1/projects", map[string]any{
		"name": "p-test", "channel_name": "#p-test", "vocabulary_id": sdlcID,
	}, &created); err != nil || status != http.StatusCreated {
		t.Fatalf("create project: status=%d err=%v", status, err)
	}
	if created.ID == "" {
		t.Fatal("missing id")
	}

	// Duplicate name conflicts.
	if status, _, err := c.PostJSON("/v1/projects", map[string]any{
		"name": "p-test", "channel_name": "#dup", "vocabulary_id": sdlcID,
	}, nil); err != nil || status != http.StatusConflict {
		t.Errorf("expected 409, got status=%d err=%v", status, err)
	}

	var got map[string]any
	if status, _, err := c.GetJSON("/v1/projects/"+created.ID, &got); err != nil || status != 200 {
		t.Fatalf("get project: status=%d err=%v", status, err)
	}
	if got["name"] != "p-test" {
		t.Errorf("name = %v", got["name"])
	}

	// Default-vocab path: omitting vocabulary_id resolves to the SDLC
	// seed via store.GetVocabularyByName(ctx, "sdlc").
	var defaulted struct {
		ID           string `json:"id"`
		VocabularyID string `json:"vocabulary_id"`
	}
	if status, _, err := c.PostJSON("/v1/projects", map[string]any{
		"name": "p-default", "channel_name": "#p-default",
	}, &defaulted); err != nil || status != http.StatusCreated {
		t.Fatalf("create default-vocab project: status=%d err=%v", status, err)
	}
	if defaulted.VocabularyID != sdlcID {
		t.Errorf("default vocabulary_id = %q, want SDLC seed %q",
			defaulted.VocabularyID, sdlcID)
	}
}
