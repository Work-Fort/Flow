// SPDX-License-Identifier: GPL-2.0-only

// Package pylon contains construction-side wire-format tests for the Pylon
// client that Flow uses for service discovery. Flow uses pylonclient.New
// directly in daemon/server.go — there is no adapter struct in this package;
// the tests live here to keep the wire-format assertion co-located with its
// upstream boundary.
package pylon_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pylonclient "github.com/Work-Fort/Pylon/client/go"
)

// TestPylonClient_OutboundIsApiKeyV1 asserts that the Pylon client sends the
// service token under the ApiKey-v1 Authorization scheme. This is the
// construction-side wire-format guard for the Cluster 3b scheme-split:
// Pylon's client now sends ApiKey-v1 (parameter renamed token → apiKey,
// signature unchanged). Any future drift in the upstream wire format is
// caught here at Flow's boundary rather than at deploy time.
func TestPylonClient_OutboundIsApiKeyV1(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		// Return a minimal services response so pylonclient doesn't error.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"services":[]}`))
	}))
	defer srv.Close()

	c := pylonclient.New(srv.URL, "wf-svc_secret")
	_, _ = c.Services(context.Background())

	want := "ApiKey-v1 wf-svc_secret"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}
