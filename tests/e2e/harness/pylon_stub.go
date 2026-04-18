// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
)

// PylonService is the JSON shape the Pylon client expects per service.
// Field names match pylon/lead/client/go/types.go on the wire.
type PylonService struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Route     string   `json:"route"`
	BaseURL   string   `json:"base_url"`
	UI        bool     `json:"ui"`
	Connected bool     `json:"connected"`
	WSPaths   []string `json:"ws_paths"`
}

// StartPylonStub serves GET /api/services with the provided service list.
// The list is captured at start time; tests that need to vary the
// registry must restart the stub.
func StartPylonStub(services []PylonService) (addr string, stop func()) {
	body, _ := json.Marshal(map[string]any{"services": services})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("pylon_stub: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return ln.Addr().String(), func() { srv.Close() }
}
