// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

// uiManifest is the JSON envelope the Scope shell decodes from
// /ui/health. Mirrors the Sharkfin shape so the shell handles both
// services with one ServiceTracker code path.
type uiManifest struct {
	Status  string   `json:"status"`
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Route   string   `json:"route"`
	WSPaths []string `json:"ws_paths"`
}

const (
	uiName  = "flow"
	uiLabel = "Flow"
	uiRoute = "/flow"
)

// registerUIRoutes mounts /ui/health and the static file server at
// /ui/* against fsys. fsys must be rooted at the Vite output dir
// (the caller passes fs.Sub(web.Dist, "dist") for the embedded
// case, or a testing fs in tests). When fsys has no remoteEntry.js,
// /ui/health returns 503 — Scope's ServiceTracker treats that as
// "service reachable, UI not built".
func registerUIRoutes(mux *http.ServeMux, fsys fs.FS) {
	hasRemoteEntry := fileExists(fsys, "remoteEntry.js")

	mux.HandleFunc("GET /ui/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m := uiManifest{Name: uiName, Label: uiLabel, Route: uiRoute, WSPaths: []string{}}
		if hasRemoteEntry {
			m.Status = "ok"
			w.WriteHeader(http.StatusOK)
		} else {
			m.Status = "unavailable"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(m)
	})

	fileServer := http.StripPrefix("/ui/", http.FileServer(http.FS(fsys)))
	mux.Handle("/ui/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui/")
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	}))
}

func fileExists(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
