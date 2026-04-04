// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	auth "github.com/Work-Fort/Passport/go/service-auth"
	"github.com/Work-Fort/Passport/go/service-auth/apikey"
	"github.com/Work-Fort/Passport/go/service-auth/jwt"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind        string
	Port        int
	PassportURL string
	Health      *HealthService
	Store       domain.Store
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Huma API — registers /openapi and /docs automatically on the mux.
	config := huma.DefaultConfig("Flow API", "1.0.0")
	api := humago.New(mux, config)

	// REST API routes via Huma
	svc := workflow.New(cfg.Store, nil)
	registerTemplateRoutes(api, cfg.Store)
	registerInstanceRoutes(api, cfg.Store)
	registerWorkItemRoutes(api, cfg.Store)
	registerTransitionRoutes(api, svc)
	registerApprovalRoutes(api, cfg.Store, svc)

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))
	mux.HandleFunc("GET /ui/health", HandleUIHealth())

	// MCP server — raw handler (JSON-RPC 2.0, not REST)
	mcpHandler := NewMCPHandler(MCPDeps{
		Store: cfg.Store,
		Svc:   svc,
	})
	mux.Handle("/mcp", mcpHandler)

	// Passport auth middleware — validates JWT and API key tokens.
	var handler http.Handler
	if cfg.PassportURL != "" {
		opts := auth.DefaultOptions(cfg.PassportURL)
		jwtV, err := jwt.New(context.Background(), opts.JWKSURL, opts.JWKSRefreshInterval)
		if err != nil {
			log.Warn("jwt validator init failed, falling back to API key only", "err", err)
		}

		var validators []auth.Validator
		if jwtV != nil {
			validators = append(validators, jwtV)
		}
		validators = append(validators, apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL))

		passportMW := auth.NewFromValidators(validators...)
		handler = publicPathSkip(passportMW(mux), mux)
	} else {
		handler = mux
	}

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ListenAndServe starts the server on the configured address.
func ListenAndServe(srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	fmt.Printf("Flow daemon listening on %s\n", ln.Addr())
	return srv.Serve(ln)
}
