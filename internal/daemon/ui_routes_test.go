// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestUIHealth_ServiceUnavailable_NoEmbed(t *testing.T) {
	mux := http.NewServeMux()
	registerUIRoutes(mux, fstest.MapFS{})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/health", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	var body struct {
		Status, Name, Label, Route string
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "flow" || body.Label != "Flow" || body.Route != "/flow" {
		t.Errorf("manifest = %+v", body)
	}
}

func TestUIHealth_OK_WhenRemoteEntryPresent(t *testing.T) {
	fsys := fstest.MapFS{
		"remoteEntry.js": &fstest.MapFile{Data: []byte("/* MF entry */")},
	}
	mux := http.NewServeMux()
	registerUIRoutes(mux, mustSub(t, fsys))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/health", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestUI_RemoteEntryServed(t *testing.T) {
	fsys := fstest.MapFS{
		"remoteEntry.js": &fstest.MapFile{Data: []byte("ENTRYBODY")},
	}
	mux := http.NewServeMux()
	registerUIRoutes(mux, mustSub(t, fsys))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/remoteEntry.js", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "ENTRYBODY" {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache-control = %q, want no-cache", got)
	}
}

func TestUI_AssetsCacheImmutable(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/app-abc.js": &fstest.MapFile{Data: []byte("X")},
	}
	mux := http.NewServeMux()
	registerUIRoutes(mux, mustSub(t, fsys))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/app-abc.js", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("cache-control = %q", got)
	}
}

func mustSub(t *testing.T, fsys fs.FS) fs.FS {
	t.Helper()
	return fsys
}
