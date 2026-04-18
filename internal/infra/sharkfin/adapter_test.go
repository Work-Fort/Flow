// SPDX-License-Identifier: GPL-2.0-only
package sharkfin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestAdapter returns an Adapter pointing at the given httptest.Server.
func newTestAdapter(t *testing.T, srv *httptest.Server) *Adapter {
	t.Helper()
	return New(srv.URL, "test-token")
}

// readJSON decodes the request body into out, or fails the test.
func readJSON(t *testing.T, r *http.Request, out any) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("unmarshal body %q: %v", string(body), err)
	}
}

func TestAdapter_Register_PostsAuthRegister(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/register" {
			hits.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 register call, got %d", hits.Load())
	}
}

func TestAdapter_CreateChannel_SendsNameAndPublic(t *testing.T) {
	var got struct {
		Name   string `json:"name"`
		Public bool   `json:"public"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/channels" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		readJSON(t, r, &got)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": got.Name, "public": got.Public})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.CreateChannel(context.Background(), "general", true); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if got.Name != "general" || got.Public != true {
		t.Fatalf("got name=%q public=%v", got.Name, got.Public)
	}
}

func TestAdapter_JoinChannel_PostsToJoin(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	if err := a.JoinChannel(context.Background(), "general"); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if want := "/api/v1/channels/general/join"; path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
}

func TestAdapter_PostMessage_NoMetadata(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages") {
			http.NotFound(w, r)
			return
		}
		readJSON(t, r, &body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"id": 42})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	id, err := a.PostMessage(context.Background(), "general", "hello", nil)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if id != 42 {
		t.Fatalf("id=%d want 42", id)
	}
	if body["body"] != "hello" {
		t.Fatalf("body[body]=%v", body["body"])
	}
	if _, present := body["metadata"]; present {
		t.Fatalf("metadata should be absent, got %v", body["metadata"])
	}
}

func TestAdapter_PostMessage_WithMetadata(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readJSON(t, r, &body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"id": 7})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	meta := json.RawMessage(`{"event_type":"flow_command","event_payload":{"action":"approve"}}`)
	id, err := a.PostMessage(context.Background(), "ops", "approve please", meta)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if id != 7 {
		t.Fatalf("id=%d want 7", id)
	}
	// Server-side, the RESTClient parses metadata into a map.
	mm, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata is not a map: %T %v", body["metadata"], body["metadata"])
	}
	if mm["event_type"] != "flow_command" {
		t.Fatalf("metadata.event_type=%v", mm["event_type"])
	}
}

func TestAdapter_RegisterWebhook_ReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/webhooks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "wh-abc"})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	id, err := a.RegisterWebhook(context.Background(), "http://flow/v1/webhooks/sharkfin")
	if err != nil {
		t.Fatalf("RegisterWebhook: %v", err)
	}
	if id != "wh-abc" {
		t.Fatalf("id=%q want wh-abc", id)
	}
}

func TestAdapter_ListWebhooks_ReturnsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/webhooks" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "wh-1", "url": "http://a", "active": true},
			{"id": "wh-2", "url": "http://b", "active": false},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv)
	whs, err := a.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(whs) != 2 {
		t.Fatalf("len=%d want 2", len(whs))
	}
}

func TestAdapter_Close_NoOp(t *testing.T) {
	a := New("http://unused", "")
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
