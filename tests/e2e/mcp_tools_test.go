// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// mcpFixture stands up Env, signs an API-key client, builds an
// MCPClient, and seeds one workflow template (2 steps, 1 transition)
// so every MCP tool has something to act on. Per-test instances and
// work items are created lazily inside each test.
type mcpFixture struct {
	env        *harness.Env
	mcp        *harness.MCPClient
	templateID string
	instanceID string
	workItemID string
	devStepID  string
	revStepID  string
	devToRev   string // transition ID
}

func setupMCP(t *testing.T) *mcpFixture {
	t.Helper()
	env := harness.NewEnv(t)
	t.Cleanup(func() { env.Cleanup(t) })

	tok := env.Daemon.SignJWT("svc-mcp", "flow-mcp", "Flow MCP", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	// Production CreateTemplateInput accepts only {name, description}
	// (internal/daemon/rest_types.go:73-78). Steps and transitions are
	// added via PATCH afterwards (mirrors tests/e2e/templates_test.go's
	// seedThreeStepTemplate at lines 279-309). Step + transition IDs
	// are client-supplied so the test can reference them without an
	// extra GET.
	const (
		stpDev = "stp_mcp_dev"
		stpRev = "stp_mcp_rev"
		trnD2R = "trn_mcp_d2r"
	)
	var tmplResp struct {
		ID string `json:"id"`
	}
	if status, body, err := c.PostJSON("/v1/templates",
		map[string]any{"name": "mcp-tmpl", "description": "tmpl for MCP e2e"},
		&tmplResp); err != nil || status != http.StatusCreated {
		t.Fatalf("create template: status=%d body=%s err=%v", status, body, err)
	}
	patch := map[string]any{
		"steps": []map[string]any{
			{"id": stpDev, "key": "dev", "name": "Dev", "type": "task", "position": 1},
			{"id": stpRev, "key": "rev", "name": "Review", "type": "task", "position": 2},
		},
		"transitions": []map[string]any{
			{"id": trnD2R, "key": "dev_to_rev", "name": "Dev to Review",
				"from_step_id": stpDev, "to_step_id": stpRev},
		},
	}
	if status, body, err := c.PatchJSON("/v1/templates/"+tmplResp.ID, patch, nil); err != nil ||
		status != http.StatusOK {
		t.Fatalf("seed template: status=%d body=%s err=%v", status, body, err)
	}

	f := &mcpFixture{
		env:        env,
		templateID: tmplResp.ID,
		devStepID:  stpDev,
		revStepID:  stpRev,
		devToRev:   trnD2R,
	}
	f.mcp = harness.NewMCPClient(env.Daemon.BaseURL()+"/mcp", "harness-service-token")
	if err := f.mcp.Initialize(); err != nil {
		t.Fatalf("mcp initialize: %v", err)
	}
	return f
}

// callDecode invokes the named tool and JSON-decodes the result text.
func (f *mcpFixture) callDecode(t *testing.T, tool string, args map[string]any, out any) {
	t.Helper()
	text, err := f.mcp.Call(tool, args)
	if err != nil {
		t.Fatalf("mcp.Call(%s): %v", tool, err)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(text), out); err != nil {
			t.Fatalf("decode %s result: %v (text=%s)", tool, err, text)
		}
	}
}

func TestMCP_ListAndGetTemplate(t *testing.T) {
	f := setupMCP(t)

	var list []map[string]any
	f.callDecode(t, "list_templates", nil, &list)
	if len(list) != 1 {
		t.Fatalf("list_templates: got %d, want 1", len(list))
	}

	var got map[string]any
	f.callDecode(t, "get_template", map[string]any{"id": f.templateID}, &got)
	if got["id"] != f.templateID {
		t.Errorf("get_template id = %v, want %v", got["id"], f.templateID)
	}
}

func TestMCP_InstanceLifecycle(t *testing.T) {
	f := setupMCP(t)

	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID,
		"team_id":     "team-mcp",
		"name":        "inst-mcp",
	}, &inst)
	instanceID, _ := inst["id"].(string)
	if instanceID == "" {
		t.Fatalf("create_instance returned no id: %v", inst)
	}

	var list []map[string]any
	f.callDecode(t, "list_instances", map[string]any{"team_id": "team-mcp"}, &list)
	if len(list) != 1 {
		t.Errorf("list_instances: got %d, want 1", len(list))
	}

	var status map[string]any
	f.callDecode(t, "get_instance_status", map[string]any{"id": instanceID}, &status)
	if status["instance"] == nil {
		t.Errorf("get_instance_status missing instance: %v", status)
	}
}

func TestMCP_WorkItemCRUDAndAssign(t *testing.T) {
	f := setupMCP(t)

	// Need an instance to host the work item.
	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID, "team_id": "team-mcp", "name": "inst",
	}, &inst)
	instanceID, _ := inst["id"].(string)

	var wi map[string]any
	f.callDecode(t, "create_work_item", map[string]any{
		"instance_id": instanceID,
		"title":       "wi via mcp",
		"description": "desc",
		"priority":    "high",
	}, &wi)
	wiID, _ := wi["id"].(string)
	if wiID == "" {
		t.Fatalf("create_work_item returned no id: %v", wi)
	}

	var listed []map[string]any
	f.callDecode(t, "list_work_items", map[string]any{
		"instance_id": instanceID,
	}, &listed)
	if len(listed) != 1 {
		t.Errorf("list_work_items: got %d, want 1", len(listed))
	}

	var got map[string]any
	f.callDecode(t, "get_work_item", map[string]any{"id": wiID}, &got)
	if got["work_item"] == nil {
		t.Errorf("get_work_item missing work_item: %v", got)
	}

	var assigned map[string]any
	f.callDecode(t, "assign_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x",
	}, &assigned)
	if assigned["assigned_agent_id"] != "agent-x" {
		t.Errorf("assigned_agent_id = %v, want agent-x", assigned["assigned_agent_id"])
	}
}

func TestMCP_TransitionApproveReject(t *testing.T) {
	f := setupMCP(t)

	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID, "team_id": "team-mcp", "name": "inst",
	}, &inst)
	instanceID, _ := inst["id"].(string)

	var wi map[string]any
	f.callDecode(t, "create_work_item", map[string]any{
		"instance_id": instanceID, "title": "wi",
	}, &wi)
	wiID, _ := wi["id"].(string)

	// Transition dev -> rev.
	var trans map[string]any
	f.callDecode(t, "transition_work_item", map[string]any{
		"id":             wiID,
		"transition_id":  f.devToRev,
		"actor_agent_id": "agent-x",
		"actor_role_id":  "role-developer",
		"reason":         "done",
	}, &trans)
	if trans["current_step_id"] != f.revStepID {
		t.Errorf("current_step_id = %v, want %v", trans["current_step_id"], f.revStepID)
	}

	// approve_work_item should fail when the step is not a gate; assert
	// the error path surfaces. The Dev step is not a gate, so we use it
	// against approve to exercise the error route.
	if _, err := f.mcp.Call("approve_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x", "comment": "lgtm",
	}); err == nil {
		t.Errorf("approve_work_item on non-gate step: expected error, got nil")
	} else if !strings.Contains(err.Error(), "gate") &&
		!strings.Contains(err.Error(), "approval") {
		t.Errorf("approve error = %v, want containing 'gate' or 'approval'", err)
	}

	// reject_work_item — same expectation.
	if _, err := f.mcp.Call("reject_work_item", map[string]any{
		"id": wiID, "agent_id": "agent-x", "comment": "no",
	}); err == nil {
		t.Errorf("reject_work_item on non-gate step: expected error, got nil")
	}
}

func TestMCP_ListMyWorkItems(t *testing.T) {
	f := setupMCP(t)

	var inst map[string]any
	f.callDecode(t, "create_instance", map[string]any{
		"template_id": f.templateID, "team_id": "team-mcp", "name": "inst-my-wi",
	}, &inst)
	instanceID, _ := inst["id"].(string)

	f.callDecode(t, "create_work_item", map[string]any{
		"instance_id": instanceID, "title": "my wi",
		"assigned_agent_id": "agent-me",
	}, nil)

	var items []map[string]any
	f.callDecode(t, "list_my_work_items", map[string]any{"agent_id": "agent-me"}, &items)
	if len(items) != 1 {
		t.Errorf("list_my_work_items: got %d, want 1", len(items))
	}
}

func TestMCP_GetVocabulary(t *testing.T) {
	f := setupMCP(t)

	// First get the SDLC vocabulary ID from the list.
	var vocabs []map[string]any
	c := harness.NewClient(f.env.Daemon.BaseURL(),
		f.env.Daemon.SignJWT("svc-v", "flow-v", "V", "service"))
	if status, _, err := c.GetJSON("/v1/vocabularies", &vocabs); err != nil || status != 200 {
		t.Fatalf("list vocabularies: %d %v", status, err)
	}
	var sdlcID string
	for _, v := range vocabs {
		if v["name"] == "sdlc" {
			sdlcID, _ = v["id"].(string)
		}
	}
	if sdlcID == "" {
		t.Fatal("SDLC vocabulary not found")
	}

	var voc map[string]any
	f.callDecode(t, "get_vocabulary", map[string]any{"id": sdlcID}, &voc)
	if voc["name"] != "sdlc" {
		t.Errorf("get_vocabulary name = %v, want sdlc", voc["name"])
	}
	events, _ := voc["events"].([]any)
	if len(events) == 0 {
		t.Errorf("get_vocabulary events: got %d, want > 0", len(events))
	}
}

func TestMCP_GetMyProject_NoClaimReturnsError(t *testing.T) {
	f := setupMCP(t)

	// An agent with no audit history should get an error.
	if _, err := f.mcp.Call("get_my_project", map[string]any{
		"agent_id": "agent-unclaimed",
	}); err == nil {
		t.Error("get_my_project with no claim: expected error, got nil")
	}
}
