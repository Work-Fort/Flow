// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

func bytesContains(b []byte, s string) bool {
	return bytes.Contains(b, []byte(s))
}

func TestBot_BindUnbind(t *testing.T) {
	env := harness.NewEnv(t)
	defer env.Cleanup(t)
	tok := env.Daemon.SignJWT("svc", "flow", "Flow", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	var prj struct {
		ID string `json:"id"`
	}
	_, _, _ = c.PostJSON("/v1/projects",
		map[string]any{"name": "bot-prj", "channel_name": "#bot-prj"}, &prj)

	var bot struct {
		ID                  string   `json:"id"`
		PassportAPIKeyID    string   `json:"passport_api_key_id"`
		HiveRoleAssignments []string `json:"hive_role_assignments"`
	}
	status, body, err := c.PostJSON("/v1/projects/"+prj.ID+"/bot", map[string]any{
		"passport_api_key":      "wf-svc_test_plaintext_xxxxxxxx",
		"passport_api_key_id":   "pak_test_001",
		"hive_role_assignments": []string{"developer", "reviewer"},
	}, &bot)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("bind bot: status=%d body=%s err=%v", status, body, err)
	}
	if bot.PassportAPIKeyID != "pak_test_001" || len(bot.HiveRoleAssignments) != 2 {
		t.Errorf("bot row missing fields: %+v", bot)
	}

	// Plaintext key must not appear in the response.
	if bytesContains(body, "wf-svc_test_plaintext") {
		t.Errorf("plaintext key leaked in response body: %s", body)
	}

	// Key file written to the bot-keys dir.
	keyPath := filepath.Join(env.Daemon.BotKeysDir(), bot.ID)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("key file not created at %s", keyPath)
	}

	// Idempotency: second create returns 409.
	if status, _, _ := c.PostJSON("/v1/projects/"+prj.ID+"/bot", map[string]any{
		"passport_api_key": "wf-svc_other", "passport_api_key_id": "pak_2",
	}, nil); status != http.StatusConflict {
		t.Errorf("expected 409 on duplicate bind, got %d", status)
	}

	// Unbind clears the row.
	if status, _, _ := c.DeleteJSON("/v1/projects/" + prj.ID + "/bot"); status != http.StatusNoContent {
		t.Errorf("delete bot: %d", status)
	}
	if status, _, _ := c.GetJSON("/v1/projects/"+prj.ID+"/bot", nil); status != http.StatusNotFound {
		t.Errorf("get bot after delete: status=%d, want 404", status)
	}

	// Key file removed after unbind.
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Errorf("key file not removed after unbind: %s", keyPath)
	}

	// Project delete now succeeds.
	if status, _, _ := c.DeleteJSON("/v1/projects/" + prj.ID); status != http.StatusNoContent {
		t.Errorf("delete project after unbind: %d", status)
	}
}
