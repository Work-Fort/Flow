// SPDX-License-Identifier: GPL-2.0-only
package hive_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	hiveinfra "github.com/Work-Fort/Flow/internal/infra/hive"
)

func newAdapter(t *testing.T, h http.Handler) *hiveinfra.Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return hiveinfra.New(srv.URL, "test-token")
}

func TestAdapter_ClaimAgent_PoolExhausted(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/claim" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"agent pool exhausted"}`))
	}))

	_, err := a.ClaimAgent(context.Background(), "developer", "flow", "wf-1", 60)
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Fatalf("want ErrPoolExhausted, got %v", err)
	}
}

func TestAdapter_ReleaseAgent_WorkflowMismatch(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/release") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"workflow id mismatch"}`))
	}))

	err := a.ReleaseAgent(context.Background(), "a_003", "wf-x")
	if !errors.Is(err, domain.ErrWorkflowMismatch) {
		t.Fatalf("want ErrWorkflowMismatch, got %v", err)
	}
}

func TestAdapter_ReleaseAgent_NotFound(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail":"agent not found"}`))
	}))

	err := a.ReleaseAgent(context.Background(), "a_missing", "wf-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "a_missing") {
		t.Errorf("expected error to mention agent ID, got %v", err)
	}
}

func TestAdapter_RenewAgentLease_WorkflowMismatch(t *testing.T) {
	a := newAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"detail":"workflow id mismatch"}`))
	}))

	err := a.RenewAgentLease(context.Background(), "a_003", "wf-x", 60)
	if !errors.Is(err, domain.ErrWorkflowMismatch) {
		t.Fatalf("want ErrWorkflowMismatch, got %v", err)
	}
}

func TestAdapter_SatisfiesHiveAgentClient(t *testing.T) {
	var _ domain.HiveAgentClient = (*hiveinfra.Adapter)(nil)
}
