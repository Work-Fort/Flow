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
	"github.com/Work-Fort/Flow/internal/workflow"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind           string
	Port           int
	PassportURL    string
	ServiceToken   string
	HiveURL        string
	PylonURL       string
	WebhookBaseURL string
	Health         *HealthService
	Store          domain.Store
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Huma API — registers /openapi and /docs automatically on the mux.
	config := huma.DefaultConfig("Flow API", "1.0.0")
	api := humago.New(mux, config)

	var identityProvider domain.IdentityProvider
	if cfg.HiveURL != "" {
		identityProvider = hiveinfra.New(cfg.HiveURL, cfg.ServiceToken)
	}

	// Sharkfin chat adapter — discovered via Pylon. Optional: if Pylon URL is
	// not set or Sharkfin is not registered, chat notifications are skipped.
	var chatAdapter *sharkfininfra.Adapter
	if cfg.PylonURL != "" {
		pylonClient := pylonclient.New(cfg.PylonURL, cfg.ServiceToken)
		startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer startupCancel()
		if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, "sharkfin"); err == nil {
			if a, err := sharkfininfra.New(startupCtx, sharkfinSvc.BaseURL, cfg.ServiceToken); err == nil {
				chatAdapter = a
				// Bot lifecycle: register identity and webhook.
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
				log.Warn("sharkfin dial failed, chat disabled", "err", err)
			}
		} else {
			log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
		}
	}

	svc := workflow.New(cfg.Store, identityProvider)
	if chatAdapter != nil {
		svc = svc.WithChat(chatAdapter)
	}
	registerTemplateRoutes(api, cfg.Store)
	registerInstanceRoutes(api, cfg.Store)
	registerWorkItemRoutes(api, cfg.Store)
	registerTransitionRoutes(api, svc)
	registerApprovalRoutes(api, cfg.Store, svc)

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))
	mux.HandleFunc("GET /ui/health", HandleUIHealth())

	// Sharkfin webhook receiver.
	mux.Handle("POST /v1/webhooks/sharkfin", HandleSharkfinWebhook(nil))

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
