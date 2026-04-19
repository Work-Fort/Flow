// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	auth "github.com/Work-Fort/Passport/go/service-auth"
	"github.com/Work-Fort/Passport/go/service-auth/apikey"
	"github.com/Work-Fort/Passport/go/service-auth/jwt"

	pylonclient "github.com/Work-Fort/Pylon/client/go"

	"github.com/Work-Fort/Flow/internal/domain"
	hiveinfra "github.com/Work-Fort/Flow/internal/infra/hive"
	sharkfininfra "github.com/Work-Fort/Flow/internal/infra/sharkfin"
	"github.com/Work-Fort/Flow/internal/scheduler"
	"github.com/Work-Fort/Flow/internal/workflow"
)

// PylonServices holds the Pylon-registered service names for each downstream dependency.
// Values are passed to pylonClient.ServiceByName — override in config if your
// service is registered under a non-default name (e.g. "borg" instead of "hive").
type PylonServices struct {
	Hive     string
	Sharkfin string
}

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind           string
	Port           int
	PassportURL    string
	ServiceToken   string
	PylonURL       string
	PylonServices  PylonServices
	WebhookBaseURL string
	Health         *HealthService
	Store          domain.Store
	// Runtime is the runtime driver bound to /v1/runtime/_diag/*. nil
	// in production until a real driver lands; the e2e harness injects
	// stub.Driver so the diag endpoint exercises the interface.
	Runtime domain.RuntimeDriver
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) (*http.Server, *scheduler.Scheduler) {
	mux := http.NewServeMux()

	// Huma API — registers /openapi and /docs automatically on the mux.
	config := huma.DefaultConfig("Flow API", "1.0.0")
	api := humago.New(mux, config)

	var identityProvider domain.IdentityProvider
	var hiveAgentClient domain.HiveAgentClient
	var chatAdapter *sharkfininfra.Adapter

	if cfg.PylonURL != "" {
		pylonClient := pylonclient.New(cfg.PylonURL, cfg.ServiceToken)
		startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer startupCancel()

		// Discover Hive via Pylon.
		if hiveSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Hive); err == nil {
			adapter := hiveinfra.New(hiveSvc.BaseURL, cfg.ServiceToken)
			identityProvider = adapter
			hiveAgentClient = adapter
		} else {
			log.Warn("hive not found in Pylon, identity checks disabled", "err", err)
		}

		// Discover Sharkfin via Pylon.
		if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Sharkfin); err == nil {
			a := sharkfininfra.New(sharkfinSvc.BaseURL, cfg.ServiceToken)
			chatAdapter = a
			if err := a.Register(startupCtx); err != nil {
				log.Warn("sharkfin register failed", "err", err)
			}
			if cfg.WebhookBaseURL != "" {
				callbackURL := strings.TrimRight(cfg.WebhookBaseURL, "/") + "/v1/webhooks/sharkfin"
				if _, err := a.RegisterWebhook(startupCtx, callbackURL); err != nil {
					log.Warn("sharkfin register webhook failed", "err", err)
				}
			}
		} else {
			log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
		}
	}

	svc := workflow.New(cfg.Store, identityProvider)
	if chatAdapter != nil {
		svc = svc.WithChat(chatAdapter)
	}

	var sch *scheduler.Scheduler
	if hiveAgentClient != nil {
		sch = scheduler.New(scheduler.Config{
			Hive:  hiveAgentClient,
			Audit: cfg.Store,
		})
	}

	registerTemplateRoutes(api, cfg.Store)
	registerInstanceRoutes(api, cfg.Store)
	registerWorkItemRoutes(api, cfg.Store)
	registerTransitionRoutes(api, svc)
	registerApprovalRoutes(api, cfg.Store, svc)
	registerRuntimeDiagRoutes(api, cfg.Runtime)
	registerSchedulerAndAuditDiagRoutes(api, sch, cfg.Store)

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))
	mux.HandleFunc("GET /ui/health", HandleUIHealth())

	// Sharkfin webhook receiver.
	mux.Handle("POST /v1/webhooks/sharkfin", HandleSharkfinWebhook(nil))

	// Combine webhook receiver.
	mux.Handle("POST /v1/webhooks/combine", HandleCombineWebhook(cfg.Store))

	// MCP server — raw handler (JSON-RPC 2.0, not REST)
	mcpHandler := NewMCPHandler(MCPDeps{
		Store: cfg.Store,
		Svc:   svc,
	})
	mux.Handle("/mcp", mcpHandler)

	// Passport auth middleware — routes Bearer JWTs to the JWT validator and
	// ApiKey-v1 tokens to the API-key validator. NewSchemeDispatch requires both
	// validators non-nil; if JWKS init fails at startup (logged below) we
	// substitute the upstream AlwaysFail stub so the API-key path keeps working.
	var handler http.Handler
	if cfg.PassportURL != "" {
		opts := auth.DefaultOptions(cfg.PassportURL)
		jwtV, err := jwt.New(context.Background(), opts.JWKSURL, opts.JWKSRefreshInterval)
		if err != nil {
			log.Warn("jwt validator init failed, falling back to API key only", "err", err)
		}

		apiKeyV := apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL)

		// NewSchemeDispatch requires both validators non-nil. If JWKS init
		// failed at startup (logged a warning above and left jwtV == nil),
		// substitute the fail-closed stub exported by service-auth so the
		// API-key path keeps working. Use the upstream helper rather than
		// reimplementing it locally — single source of truth.
		var jwtForDispatch auth.Validator
		if jwtV != nil {
			jwtForDispatch = jwtV
		} else {
			jwtForDispatch = auth.AlwaysFail(fmt.Errorf("jwt validator unavailable (jwks init failed)"))
		}

		passportMW := auth.NewSchemeDispatch(jwtForDispatch, apiKeyV)
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
	}, sch
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
