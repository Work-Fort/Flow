// SPDX-License-Identifier: GPL-2.0-only
package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/domain"
)

type captureAudit struct {
	events []*domain.AuditEvent
}

func (c *captureAudit) RecordAuditEvent(_ context.Context, e *domain.AuditEvent) error {
	c.events = append(c.events, e)
	return nil
}
func (c *captureAudit) ListAuditEventsByWorkflow(context.Context, string) ([]*domain.AuditEvent, error) {
	return nil, nil
}
func (c *captureAudit) ListAuditEventsByAgent(context.Context, string) ([]*domain.AuditEvent, error) {
	return nil, nil
}
func (c *captureAudit) ListAuditEventsByProject(context.Context, string) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func (c *captureAudit) ListAuditEventsFiltered(context.Context, domain.AuditFilter) ([]*domain.AuditEvent, error) {
	return nil, nil
}

func postCombine(t *testing.T, h http.Handler, event string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/combine", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SoftServe-Event", event)
	req.Header.Set("X-SoftServe-Delivery", "del-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCombineWebhook_PushAuditedAndAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit, nil, nil)
	w := postCombine(t, h, "push", map[string]any{
		"repository": map[string]any{"name": "flow"},
		"ref":        "refs/heads/main",
		"before":     "abc123",
		"after":      "def456",
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	if audit.events[0].Type != domain.AuditEventCombinePushReceived {
		t.Errorf("type = %q, want %q", audit.events[0].Type, domain.AuditEventCombinePushReceived)
	}
}

func TestCombineWebhook_MergeAuditedAndAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit, nil, nil)
	w := postCombine(t, h, "pull_request_merged", map[string]any{
		"repository":   map[string]any{"name": "flow"},
		"pull_request": map[string]any{"number": 42, "target_branch": "main"},
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	if audit.events[0].Type != domain.AuditEventCombineMergeReceived {
		t.Errorf("type = %q, want %q", audit.events[0].Type, domain.AuditEventCombineMergeReceived)
	}
}

func TestCombineWebhook_OtherEventsIgnoredButAcked(t *testing.T) {
	audit := &captureAudit{}
	h := daemon.HandleCombineWebhook(audit, nil, nil)
	w := postCombine(t, h, "issue_opened", map[string]any{"number": 1})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(audit.events) != 0 {
		t.Errorf("audit events = %d, want 0", len(audit.events))
	}
}

func TestCombineWebhook_NilAuditNeverPanics(t *testing.T) {
	h := daemon.HandleCombineWebhook(nil, nil, nil)
	w := postCombine(t, h, "push", map[string]any{})
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}
