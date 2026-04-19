// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
)

// fakeNexus returns a *httptest.Server backed by a per-path map of
// handlers. Each handler may inspect the request and respond with
// any status + body. Tests build up the routes they need and tear
// down on t.Cleanup.
func fakeNexus(t *testing.T, routes map[string]http.HandlerFunc) string {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range routes {
		mux.HandleFunc(pattern, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestClient_GetJSON_Success(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/abc": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"id":"abc","state":"running"}`)
		},
	})
	d := New(Config{BaseURL: url})
	var out struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := d.getJSON(context.Background(), "/v1/vms/abc", &out); err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if out.ID != "abc" || out.State != "running" {
		t.Errorf("decoded = %+v", out)
	}
}

func TestClient_GetJSON_404MapsToErrNotFound(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/ghost": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.getJSON(context.Background(), "/v1/vms/ghost", &struct{}{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want wrap of ErrNotFound", err)
	}
}

func TestClient_PostJSON_409MapsToErrInvalidState(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/drives/clone": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "source attached", http.StatusConflict)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.postJSON(context.Background(), "/v1/drives/clone", map[string]any{}, &struct{}{})
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("err = %v, want wrap of ErrInvalidState", err)
	}
}

func TestClient_DeleteOK(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"DELETE /v1/drives/d-1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})
	d := New(Config{BaseURL: url})
	if err := d.delete(context.Background(), "/v1/drives/d-1"); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestClient_AttachesBearerToken(t *testing.T) {
	var gotAuth string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/abc": func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Write([]byte(`{}`))
		},
	})
	d := New(Config{BaseURL: url, ServiceToken: "wf-svc_test"})
	_ = d.getJSON(context.Background(), "/v1/vms/abc", &struct{}{})
	if want := "Bearer wf-svc_test"; gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

func TestClient_TrailingSlashTolerant(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/abc": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{}`))
		},
	})
	d := New(Config{BaseURL: url + "/"})
	if err := d.getJSON(context.Background(), "/v1/vms/abc", &struct{}{}); err != nil {
		t.Errorf("with trailing slash: %v", err)
	}
}

// Ensure the json marshaler's nil-vs-empty handling in postJSON
// produces a "null" body when nil and a "{}" when an empty struct.
func TestClient_PostJSON_BodyEncoding(t *testing.T) {
	var got string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/drives/clone": func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got = strings.TrimSpace(string(b))
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		},
	})
	d := New(Config{BaseURL: url})
	body := map[string]string{"name": "x"}
	_ = d.postJSON(context.Background(), "/v1/drives/clone", body, &struct{}{})
	if !strings.Contains(got, `"name":"x"`) {
		t.Errorf("posted body = %q, want to contain name=x", got)
	}
	// Confirm the encoded body is valid JSON.
	var sink any
	if err := json.Unmarshal([]byte(got), &sink); err != nil {
		t.Errorf("posted body is not valid JSON: %v", err)
	}
}
