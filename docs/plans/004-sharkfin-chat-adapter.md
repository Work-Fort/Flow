---
type: plan
step: "4"
status: approved
codebase: flow
---

# Flow Phase 4 — Sharkfin Chat Adapter

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Sharkfin chat into Flow. Add a `ChatProvider` port to the domain layer, a Sharkfin-backed adapter in the infra layer, a webhook receiver for inbound commands, and hook execution in the workflow service. The adapter is optional — nil provider skips notifications (same pattern as `IdentityProvider`).

**Prerequisite:** Phase 3 (003-hive-identity-adapter.md) complete and tests passing.

**Sharkfin client package:** `github.com/Work-Fort/sharkfin/client/go` at `/home/kazw/Work/WorkFort/sharkfin/lead/client/go/`. Read this before implementing — do not guess at method signatures.

**Pylon client package:** `github.com/Work-Fort/Pylon/client/go` at `/home/kazw/Work/WorkFort/pylon/lead/client/go/`. Read this before implementing.

**Key facts about the Sharkfin client:**
- The client is **WebSocket-based**. All channel and message operations (`SendMessage`, `CreateChannel`, `JoinChannel`) use WebSocket via the `request` method.
- Constructor: `client.Dial(ctx context.Context, url string, opts ...Option) (*Client, error)` — takes a WebSocket URL (e.g., `ws://sharkfin:16000/ws`), returns `(*Client, error)`.
- Authentication options: `client.WithToken(t string)` (JWT) or `client.WithAPIKey(k string)`.
- `SendMessage(ctx, channel, body string, opts *SendOpts) (int64, error)` — returns the message ID. `SendOpts.Metadata` is `*string` (JSON string), `SendOpts.ThreadID` is `*int64`.
- `CreateChannel(ctx, name string, public bool) error`
- `JoinChannel(ctx, channel string) error`
- REST methods (HTTP, not WebSocket): `Register(ctx) error`, `RegisterWebhook(ctx, url string) (string, error)`, `UnregisterWebhook(ctx, id string) error`, `ListWebhooks(ctx) ([]Webhook, error)`.
- Sentinel errors: `client.ErrNotConnected`, `client.ErrClosed`, `client.ErrTimeout`. Non-200 HTTP responses return `*client.ServerError`.
- WebSocket URL is derived by appending `/ws` to the HTTP base URL: if Pylon returns `http://sharkfin:16000`, the WS dial URL is `ws://sharkfin:16000/ws`.

**Key facts about the Pylon client:**
- Constructor: `client.New(pylonURL, token string) *Client` — synchronous, no error.
- `ServiceByName(ctx context.Context, name string) (*Service, error)` — fetches all services and finds by name. Returns `client.ErrNotFound` if missing.
- `Service.BaseURL string` — the HTTP base URL of the discovered service.

**Architecture rule:** Domain types must not import Sharkfin or Pylon client types. Adapters in `internal/infra/` map client types to domain types.

---

## Chunk 1: Domain Chat Port

### Task 1: `ChatProvider` port in `internal/domain/ports.go`

**Files:** `internal/domain/ports.go`

Add the chat port interface. The domain layer references only `context` and `encoding/json`.

- [ ] **Step 1: Add `ChatProvider` to `internal/domain/ports.go`**

Append to the existing file (after `IdentityProvider`):

```go
// ChatProvider posts messages and manages channels in an external chat service.
// It is an optional dependency — if nil, chat notifications are skipped.
type ChatProvider interface {
	// PostMessage sends a message to the named channel and returns the message ID.
	// metadata may be nil.
	PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error)

	// CreateChannel creates a channel with the given name and visibility.
	CreateChannel(ctx context.Context, name string, public bool) error

	// JoinChannel joins the named channel.
	JoinChannel(ctx context.Context, channel string) error
}
```

Add `"encoding/json"` to the import block in `ports.go` (it currently only imports `"context"` and `"io"`).

- [ ] **Step 2: Verify** — `go build ./internal/domain/...` exits 0.

- [ ] **Step 3: Commit**
```
git commit -m "feat(domain): add ChatProvider port interface"
```

---

## Chunk 2: Sharkfin Adapter

### Task 2: Add Sharkfin and Pylon dependencies

**Files:** `go.mod`, `go.sum`

Both modules are local — they are not published to a public registry. Add `replace` directives to `go.mod` first, following the same pattern as the existing Hive replace directive.

- [ ] **Step 1: Add `replace` directives to `go.mod`**

Open `go.mod` and append the following two lines after the existing `replace github.com/Work-Fort/Hive => ...` line:

```
replace github.com/Work-Fort/sharkfin/client/go => /home/kazw/Work/WorkFort/sharkfin/lead/client/go
replace github.com/Work-Fort/Pylon/client/go => /home/kazw/Work/WorkFort/pylon/lead/client/go
```

- [ ] **Step 2: Add the Sharkfin client module**

```bash
cd /home/kazw/Work/WorkFort/flow/lead && go get github.com/Work-Fort/sharkfin/client/go
```

- [ ] **Step 3: Add the Pylon client module**

```bash
cd /home/kazw/Work/WorkFort/flow/lead && go get github.com/Work-Fort/Pylon/client/go
```

- [ ] **Step 4: Verify** — `go build ./...` exits 0.

- [ ] **Step 5: Commit**
```
git commit -m "chore(deps): add Sharkfin and Pylon client dependencies"
```

---

### Task 3: Sharkfin adapter in `internal/infra/sharkfin/adapter.go`

**Files:** `internal/infra/sharkfin/adapter.go`

The adapter wraps the Sharkfin WebSocket client and implements `domain.ChatProvider`. Because `client.Dial` is a network operation, the constructor accepts a `context.Context` and returns an error.

- [ ] **Step 1: Create `internal/infra/sharkfin/adapter.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only

// Package sharkfin provides a Flow ChatProvider backed by the Sharkfin chat service.
package sharkfin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sharkfinclient "github.com/Work-Fort/sharkfin/client/go"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Adapter implements domain.ChatProvider using the Sharkfin WebSocket client.
type Adapter struct {
	client *sharkfinclient.Client
}

// New dials the Sharkfin server and returns an Adapter. baseURL is the HTTP
// base URL returned by Pylon (e.g., "http://sharkfin:16000"). token is a
// Passport JWT or API key.
//
// The WebSocket URL is derived by replacing the http(s) scheme with ws(s)
// and appending "/ws".
func New(ctx context.Context, baseURL, token string) (*Adapter, error) {
	wsURL := httpToWS(baseURL) + "/ws"
	c, err := sharkfinclient.Dial(ctx, wsURL, sharkfinclient.WithToken(token))
	if err != nil {
		return nil, fmt.Errorf("sharkfin dial %s: %w", wsURL, err)
	}
	return &Adapter{client: c}, nil
}

// httpToWS converts an HTTP base URL to the WebSocket equivalent.
func httpToWS(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasPrefix(u, "https://") {
		return "wss://" + u[8:]
	}
	if strings.HasPrefix(u, "http://") {
		return "ws://" + u[7:]
	}
	return u
}

// PostMessage sends content to channel and returns the message ID.
// metadata is attached as a JSON string sidecar on the message. If metadata
// is nil or empty, no metadata is attached.
func (a *Adapter) PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error) {
	var opts *sharkfinclient.SendOpts
	if len(metadata) > 0 {
		s := string(metadata)
		opts = &sharkfinclient.SendOpts{Metadata: &s}
	}
	id, err := a.client.SendMessage(ctx, channel, content, opts)
	if err != nil {
		return 0, fmt.Errorf("sharkfin post message to %s: %w", channel, err)
	}
	return id, nil
}

// CreateChannel creates a channel in Sharkfin.
func (a *Adapter) CreateChannel(ctx context.Context, name string, public bool) error {
	if err := a.client.CreateChannel(ctx, name, public); err != nil {
		return fmt.Errorf("sharkfin create channel %s: %w", name, err)
	}
	return nil
}

// JoinChannel joins the named channel.
func (a *Adapter) JoinChannel(ctx context.Context, channel string) error {
	if err := a.client.JoinChannel(ctx, channel); err != nil {
		return fmt.Errorf("sharkfin join channel %s: %w", channel, err)
	}
	return nil
}

// Register registers Flow's identity as a service bot with Sharkfin.
func (a *Adapter) Register(ctx context.Context) error {
	if err := a.client.Register(ctx); err != nil {
		return fmt.Errorf("sharkfin register: %w", err)
	}
	return nil
}

// RegisterWebhook registers a webhook callback URL. Returns the webhook ID.
func (a *Adapter) RegisterWebhook(ctx context.Context, callbackURL string) (string, error) {
	id, err := a.client.RegisterWebhook(ctx, callbackURL)
	if err != nil {
		return "", fmt.Errorf("sharkfin register webhook: %w", err)
	}
	return id, nil
}

// ListWebhooks returns all registered webhooks for this identity.
func (a *Adapter) ListWebhooks(ctx context.Context) ([]sharkfinclient.Webhook, error) {
	whs, err := a.client.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("sharkfin list webhooks: %w", err)
	}
	return whs, nil
}

// Close cleanly shuts down the WebSocket connection.
func (a *Adapter) Close() error {
	return a.client.Close()
}

// Ensure Adapter satisfies domain.ChatProvider at compile time.
var _ domain.ChatProvider = (*Adapter)(nil)
```

- [ ] **Step 2: Verify** — `go build ./internal/infra/sharkfin/...` exits 0.

- [ ] **Step 3: Commit**
```
git commit -m "feat(infra): add Sharkfin chat adapter"
```

---

### Task 4: Stub ChatProvider for tests

**Files:** `internal/workflow/stub_chat_test.go`

A minimal stub used by workflow service tests.

- [ ] **Step 1: Create `internal/workflow/stub_chat_test.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"encoding/json"
)

// stubChat records PostMessage calls for assertions.
type stubChat struct {
	messages []stubChatMessage
}

type stubChatMessage struct {
	Channel  string
	Content  string
	Metadata json.RawMessage
}

func (s *stubChat) PostMessage(ctx context.Context, channel, content string, metadata json.RawMessage) (int64, error) {
	s.messages = append(s.messages, stubChatMessage{
		Channel:  channel,
		Content:  content,
		Metadata: metadata,
	})
	return int64(len(s.messages)), nil
}

func (s *stubChat) CreateChannel(ctx context.Context, name string, public bool) error {
	return nil
}

func (s *stubChat) JoinChannel(ctx context.Context, channel string) error {
	return nil
}
```

- [ ] **Step 2: Verify** — `go build ./internal/workflow/...` exits 0.

---

## Chunk 3: Workflow Service Chat Integration

### Task 5: Wire ChatProvider into workflow.Service

**Files:** `internal/workflow/service.go`

Add the optional `chat domain.ChatProvider` field and a `WithChat` functional option or a second constructor. Use the same optional-nil pattern as `identity`.

- [ ] **Step 1: Add `chat` field and update `New`**

Change the `Service` struct and constructor:

```go
// Service executes workflow operations against a domain.Store.
type Service struct {
	store    domain.Store
	identity domain.IdentityProvider
	chat     domain.ChatProvider
}

// New creates a new Service. identity and chat may each be nil — if so,
// role checks and chat notifications are skipped respectively.
func New(store domain.Store, identity domain.IdentityProvider) *Service {
	return &Service{store: store, identity: identity}
}

// WithChat sets the optional ChatProvider. Returns s for chaining.
func (s *Service) WithChat(chat domain.ChatProvider) *Service {
	s.chat = chat
	return s
}
```

- [ ] **Step 2: Verify** — `go build ./internal/workflow/...` exits 0.

---

### Task 6: Hook execution after transitions

**Files:** `internal/workflow/service.go`, `internal/workflow/hooks.go`

After a successful transition in `TransitionItem`, fire any `integration_hooks` whose `adapter_type == "chat"` and `action == "post_message"`.

- [ ] **Step 1: Write failing test in `internal/workflow/hooks_test.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

func TestTransitionItem_FiresChatHook(t *testing.T) {
	store := newTestStore()
	chat := &stubChat{}
	svc := workflow.New(store, nil).WithChat(chat)

	tmpl := &domain.WorkflowTemplate{
		ID:   "tmpl_hook",
		Name: "Hook Test",
		Steps: []domain.Step{
			{ID: "s1", TemplateID: "tmpl_hook", Key: "open", Name: "Open", Type: domain.StepTypeTask, Position: 0},
			{ID: "s2", TemplateID: "tmpl_hook", Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: "tr1", TemplateID: "tmpl_hook", Key: "close", Name: "Close", FromStepID: "s1", ToStepID: "s2"},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{
				ID:           "hook1",
				TemplateID:   "tmpl_hook",
				TransitionID: "tr1",
				Event:        "transition",
				AdapterType:  "chat",
				Action:       "post_message",
				Config:       json.RawMessage(`{"channel":"general","template":"{{item.title}} moved to Done"}`),
			},
		},
	}
	if err := store.CreateTemplate(context.Background(), tmpl); err != nil {
		t.Fatal(err)
	}

	inst := &domain.WorkflowInstance{
		ID:         "inst_hook",
		TemplateID: "tmpl_hook",
		TeamID:     "team1",
		Name:       "Hook Instance",
		Status:     domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}

	item := &domain.WorkItem{
		ID:            "wi_hook",
		InstanceID:    "inst_hook",
		Title:         "My Task",
		CurrentStepID: "s1",
		Priority:      domain.PriorityNormal,
	}
	if err := store.CreateWorkItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   "wi_hook",
		TransitionID: "tr1",
		ActorAgentID: "agent1",
	})
	if err != nil {
		t.Fatalf("TransitionItem: %v", err)
	}

	if len(chat.messages) != 1 {
		t.Fatalf("expected 1 chat message, got %d", len(chat.messages))
	}
	if chat.messages[0].Channel != "general" {
		t.Errorf("channel = %q, want %q", chat.messages[0].Channel, "general")
	}
	if chat.messages[0].Content != "My Task moved to Done" {
		t.Errorf("content = %q, want %q", chat.messages[0].Content, "My Task moved to Done")
	}
}

func TestTransitionItem_NilChat_NoNotification(t *testing.T) {
	store := newTestStore()
	// No chat provider — must not panic.
	svc := workflow.New(store, nil)

	tmpl := &domain.WorkflowTemplate{
		ID:   "tmpl_nilchat",
		Name: "Nil Chat Test",
		Steps: []domain.Step{
			{ID: "s1", TemplateID: "tmpl_nilchat", Key: "open", Name: "Open", Type: domain.StepTypeTask, Position: 0},
			{ID: "s2", TemplateID: "tmpl_nilchat", Key: "done", Name: "Done", Type: domain.StepTypeTask, Position: 1},
		},
		Transitions: []domain.Transition{
			{ID: "tr1", TemplateID: "tmpl_nilchat", Key: "close", Name: "Close", FromStepID: "s1", ToStepID: "s2"},
		},
		IntegrationHooks: []domain.IntegrationHook{
			{
				ID:           "hook1",
				TemplateID:   "tmpl_nilchat",
				TransitionID: "tr1",
				Event:        "transition",
				AdapterType:  "chat",
				Action:       "post_message",
				Config:       json.RawMessage(`{"channel":"general","template":"{{item.title}} moved"}`),
			},
		},
	}
	if err := store.CreateTemplate(context.Background(), tmpl); err != nil {
		t.Fatal(err)
	}
	inst := &domain.WorkflowInstance{
		ID: "inst_nilchat", TemplateID: "tmpl_nilchat", TeamID: "team1",
		Name: "Nil Chat Instance", Status: domain.InstanceStatusActive,
	}
	if err := store.CreateInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	item := &domain.WorkItem{
		ID: "wi_nilchat", InstanceID: "inst_nilchat", Title: "A Task",
		CurrentStepID: "s1", Priority: domain.PriorityNormal,
	}
	if err := store.CreateWorkItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	_, err := svc.TransitionItem(context.Background(), workflow.TransitionRequest{
		WorkItemID:   "wi_nilchat",
		TransitionID: "tr1",
		ActorAgentID: "agent1",
	})
	if err != nil {
		t.Fatalf("TransitionItem with nil chat: %v", err)
	}
}
```

- [ ] **Step 2: Verify tests fail** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./internal/workflow/... -run "TestTransitionItem_FiresChatHook|TestTransitionItem_NilChat_NoNotification" 2>&1 | head -20
```
Expected: compile error or FAIL (hook execution not yet implemented).

- [ ] **Step 3: Create `internal/workflow/hooks.go`**

```go
// SPDX-License-Identifier: GPL-2.0-only
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/Work-Fort/Flow/internal/domain"
)

// hookConfig is the JSON shape stored in IntegrationHook.Config for
// adapter_type=="chat" / action=="post_message".
type hookConfig struct {
	Channel  string `json:"channel"`
	Template string `json:"template"`
}

// fireTransitionHooks fires all integration hooks for the given transition.
// Hook execution errors are logged but do not fail the transition — the
// transition is already committed when this is called.
func (s *Service) fireTransitionHooks(ctx context.Context, tmpl *domain.WorkflowTemplate, item *domain.WorkItem, transitionID string) {
	if s.chat == nil {
		return
	}
	for _, h := range tmpl.IntegrationHooks {
		if h.TransitionID != transitionID {
			continue
		}
		if h.AdapterType != "chat" || h.Action != "post_message" {
			continue
		}
		s.fireChatPostMessage(ctx, h, item)
	}
}

// fireChatPostMessage executes a single chat post_message hook.
func (s *Service) fireChatPostMessage(ctx context.Context, h domain.IntegrationHook, item *domain.WorkItem) {
	var cfg hookConfig
	if err := json.Unmarshal(h.Config, &cfg); err != nil {
		return // malformed config — skip silently
	}
	if cfg.Channel == "" || cfg.Template == "" {
		return
	}

	content := renderTemplate(cfg.Template, item)

	// No metadata for plain post_message hooks.
	s.chat.PostMessage(ctx, cfg.Channel, content, nil) //nolint:errcheck
}

// renderTemplate replaces {{item.title}} and {{item.priority}} placeholders
// in the template string using Go's text/template package.
func renderTemplate(tmplStr string, item *domain.WorkItem) string {
	// Replace {{item.x}} with {{.Item.x}} for Go template compatibility.
	tmplStr = strings.ReplaceAll(tmplStr, "{{item.", "{{.Item.")

	t, err := template.New("hook").Parse(tmplStr)
	if err != nil {
		return tmplStr // return raw on parse error
	}

	data := struct {
		Item struct {
			Title    string
			Priority string
		}
	}{}
	data.Item.Title = item.Title
	data.Item.Priority = string(item.Priority)

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmplStr
	}
	return buf.String()
}
```

- [ ] **Step 4: Call `fireTransitionHooks` at the end of `TransitionItem`**

In `internal/workflow/service.go`, at the end of `TransitionItem` (after `RecordTransition` succeeds, before the final `GetWorkItem` return), add:

```go
	s.fireTransitionHooks(ctx, tmpl, w, tr.ID)

	return s.store.GetWorkItem(ctx, w.ID)
```

Replace the existing final return:
```go
	return s.store.GetWorkItem(ctx, w.ID)
```
with:
```go
	s.fireTransitionHooks(ctx, tmpl, w, tr.ID)

	return s.store.GetWorkItem(ctx, w.ID)
```

- [ ] **Step 5: Verify tests pass** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./internal/workflow/... -run "TestTransitionItem_FiresChatHook|TestTransitionItem_NilChat_NoNotification"
```
Expected: PASS.

- [ ] **Step 6: Verify all workflow tests still pass** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./internal/workflow/...
```

- [ ] **Step 7: Commit**
```
git commit -m "feat(workflow): fire chat integration hooks on transitions"
```

---

## Chunk 4: Webhook Receiver

### Task 7: Webhook payload types

**Files:** `internal/daemon/webhook_sharkfin.go`

Define the inbound webhook payload types and the HTTP handler. Sharkfin POSTs a JSON payload to `POST /v1/webhooks/sharkfin` when a message arrives in a joined channel.

- [ ] **Step 1: Write failing test in `internal/daemon/webhook_sharkfin_test.go`**

```go
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
```

- [ ] **Step 2: Verify tests fail** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./internal/daemon/... -run "TestSharkfinWebhook" 2>&1 | head -20
```
Expected: compile error (types/handler not yet defined).

- [ ] **Step 3: Create `internal/daemon/webhook_sharkfin.go`**

```go
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
			w.WriteHeader(http.StatusBadRequest)
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
```

- [ ] **Step 4: Verify tests pass** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./internal/daemon/... -run "TestSharkfinWebhook"
```
Expected: PASS.

- [ ] **Step 5: Commit**
```
git commit -m "feat(daemon): add Sharkfin webhook receiver and command parser"
```

---

## Chunk 5: Config and Server Wiring

### Task 8: Add `--pylon-url` and `--webhook-base-url` config flags

**Files:** `internal/config/config.go`, `cmd/daemon/daemon.go`

- [ ] **Step 1: Add defaults to `internal/config/config.go`**

In `InitViper`, add after the existing `viper.SetDefault("hive-url", "")` line:

```go
	viper.SetDefault("pylon-url", "")
	viper.SetDefault("webhook-base-url", "")
```

- [ ] **Step 2: Verify** — `go build ./internal/config/...` exits 0.

- [ ] **Step 3: Add flags and plumbing in `cmd/daemon/daemon.go`**

Add `pylonURL` and `webhookBaseURL` variables alongside the existing flag variables:

```go
	var pylonURL string
	var webhookBaseURL string
```

Register the flags in `NewCmd` alongside the existing `hiveURL` flag:

```go
	cmd.Flags().StringVar(&pylonURL, "pylon-url", "", "Pylon service registry URL")
	cmd.Flags().StringVar(&webhookBaseURL, "webhook-base-url", "", "Flow's externally reachable base URL for webhook callbacks")
```

Viper fallback in the `RunE` closure (alongside existing `hive-url` fallback):

```go
			if !cmd.Flags().Changed("pylon-url") {
				pylonURL = viper.GetString("pylon-url")
			}
			if !cmd.Flags().Changed("webhook-base-url") {
				webhookBaseURL = viper.GetString("webhook-base-url")
			}
```

Pass the new values to `run`:

```go
			return run(bind, port, db, passportURL, serviceToken, hiveURL, pylonURL, webhookBaseURL)
```

Update the `run` function signature:

```go
func run(bind string, port int, db, passportURL, serviceToken, hiveURL, pylonURL, webhookBaseURL string) error {
```

- [ ] **Step 4: Verify** — `go build ./cmd/daemon/...` exits 0.

- [ ] **Step 5: Commit**
```
git commit -m "feat(config): add pylon-url and webhook-base-url daemon flags"
```

---

### Task 9: Wire Sharkfin adapter into `ServerConfig` and `NewServer`

**Files:** `internal/daemon/server.go`, `cmd/daemon/daemon.go`

- [ ] **Step 1: Extend `ServerConfig` in `internal/daemon/server.go`**

Add `PylonURL`, `WebhookBaseURL` fields to `ServerConfig`:

```go
// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind           string
	Port           int
	PassportURL    string
	ServiceToken   string
	HiveURL        string
	PylonURL       string
	WebhookBaseURL string
	Health         *HealthService
	Store          domain.Store
}
```

- [ ] **Step 2: Add Sharkfin construction and webhook route in `NewServer`**

Add imports at the top of `server.go`:

```go
	"context"
	"strings"

	pylonclient "github.com/Work-Fort/Pylon/client/go"
	sharkfininfra "github.com/Work-Fort/Flow/internal/infra/sharkfin"
```

In `NewServer`, after the `identityProvider` block and before `svc := workflow.New(...)`:

```go
	// Sharkfin chat adapter — discovered via Pylon. Optional: if Pylon URL is
	// not set or Sharkfin is not registered, chat notifications are skipped.
	var chatAdapter *sharkfininfra.Adapter
	if cfg.PylonURL != "" {
		pylonClient := pylonclient.New(cfg.PylonURL, cfg.ServiceToken)
		startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer startupCancel()
		if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, "sharkfin"); err == nil {
			if a, err := sharkfininfra.New(startupCtx, sharkfinSvc.BaseURL, cfg.ServiceToken); err == nil {
				chatAdapter = a
				// Bot lifecycle: register identity and webhook.
				if err := a.Register(startupCtx); err != nil {
					log.Warn("sharkfin register failed", "err", err)
				}
				if cfg.WebhookBaseURL != "" {
					callbackURL := strings.TrimRight(cfg.WebhookBaseURL, "/") + "/v1/webhooks/sharkfin"
					if _, err := a.RegisterWebhook(startupCtx, callbackURL); err != nil {
						log.Warn("sharkfin register webhook failed", "err", err)
					}
				}
			} else {
				log.Warn("sharkfin dial failed, chat disabled", "err", err)
			}
		} else {
			log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
		}
	}
```

Change the service construction to wire chat:

```go
	svc := workflow.New(cfg.Store, identityProvider)
	if chatAdapter != nil {
		svc = svc.WithChat(chatAdapter)
	}
```

Register the webhook route on the mux (after the existing health routes):

```go
	mux.Handle("POST /v1/webhooks/sharkfin", HandleSharkfinWebhook(nil))
```

Note: The command handler is `nil` for now — command dispatch wiring is out of scope for this plan. The route accepts and acknowledges messages; actual command dispatch is a follow-on.

- [ ] **Step 3: Pass new fields from `run` in `cmd/daemon/daemon.go`**

Update the `flowDaemon.NewServer(...)` call to include the new fields:

```go
	srv := flowDaemon.NewServer(flowDaemon.ServerConfig{
		Bind:           bind,
		Port:           port,
		PassportURL:    passportURL,
		HiveURL:        hiveURL,
		ServiceToken:   serviceToken,
		PylonURL:       pylonURL,
		WebhookBaseURL: webhookBaseURL,
		Health:         health,
		Store:          store,
	})
```

- [ ] **Step 4: Verify** — `go build ./...` exits 0.

- [ ] **Step 5: Verify all tests pass** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && go test ./...
```

- [ ] **Step 6: Commit**
```
git commit -m "feat(daemon): wire Sharkfin adapter via Pylon discovery"
```

---

## Chunk 6: Final Verification

### Task 10: Full build and test pass

- [ ] **Step 1: Full build** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && mise run build
```

- [ ] **Step 2: Full test suite** —
```bash
cd /home/kazw/Work/WorkFort/flow/lead && mise run test
```

All tests must pass.

- [ ] **Step 3: Verify webhook route is registered** — start the daemon in dry-run (build only) or check that `POST /v1/webhooks/sharkfin` is present in the mux by grep:
```bash
grep -r "webhooks/sharkfin" /home/kazw/Work/WorkFort/flow/lead/internal/daemon/
```

---

## QA Checklist

The following must be verified end-to-end before marking this plan complete:

1. **ChatProvider port compiles** — `go build ./internal/domain/...` exits 0.
2. **Sharkfin adapter builds** — `go build ./internal/infra/sharkfin/...` exits 0.
3. **Chat hook tests pass** — `go test ./internal/workflow/... -run "TestTransitionItem_FiresChatHook|TestTransitionItem_NilChat_NoNotification"` PASS.
4. **Webhook handler tests pass** — `go test ./internal/daemon/... -run "TestSharkfinWebhook"` PASS.
5. **Full test suite passes** — `mise run test` exits 0.
6. **Nil chat provider** — daemon starts and transitions work normally when `--pylon-url` is not set.
7. **Config flags registered** — `flow daemon --help` lists `--pylon-url` and `--webhook-base-url`.
8. **Webhook route** — `POST /v1/webhooks/sharkfin` returns 204 (verified by curl or test).
