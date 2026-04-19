// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"net/http"
	"strings"
)

// publicPathSkip routes public paths (health, OpenAPI docs) to the
// unprotected handler, and all other paths through the Passport auth
// middleware.
func publicPathSkip(passport http.Handler, unprotected http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/health",
			r.URL.Path == "/ui/health",
			r.URL.Path == "/openapi",
			strings.HasPrefix(r.URL.Path, "/docs"):
			unprotected.ServeHTTP(w, r)
		default:
			passport.ServeHTTP(w, r)
		}
	})
}
