// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"fmt"
	"io/fs"
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

	botpkg "github.com/Work-Fort/Flow/internal/bot"
	"github.com/Work-Fort/Flow/internal/domain"
	hiveinfra "github.com/Work-Fort/Flow/internal/infra/hive"
	passportinfra "github.com/Work-Fort/Flow/internal/infra/passport"
	sharkfininfra "github.com/Work-Fort/Flow/internal/infra/sharkfin"
	"github.com/Work-Fort/Flow/internal/scheduler"
	"github.com/Work-Fort/Flow/internal/workflow"
	flowweb "github.com/Work-Fort/Flow/web"
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

	// BotKeysDir is the directory under which per-bot Passport API
	// key plaintexts are written (mode 0600). Empty disables bot
	// creation (the POST /v1/projects/{id}/bot handler returns 503).
	BotKeysDir string

	// Dispatcher is the optional vocabulary-driven message
	// dispatcher. When nil, scheduler claim/release and the Combine
	// webhook skip outbound chat posts and continue with audit-only
	// behaviour.
	Dispatcher domain.BotDispatcher

	// Chat is the optional ChatProvider injected for auto-provisioning
	// Sharkfin channels on project create. When nil, channel create is skipped.
	Chat domain.ChatProvider

	// Passport is the optional PassportProvider for bot API key lifecycle.
	// When nil, auto-mint is disabled and the create-bot handler returns 503.
	Passport domain.PassportProvider
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

	// Wire Passport client when a URL is configured and no override was injected.
	if cfg.Passport == nil && cfg.PassportURL != "" && cfg.ServiceToken != "" {
		cfg.Passport = passportinfra.New(cfg.PassportURL, cfg.ServiceToken)
	}

	svc := workflow.New(cfg.Store, identityProvider)
	if chatAdapter != nil {
		svc = svc.WithChat(chatAdapter)
	}

	// Wire the bot dispatcher when Sharkfin is available and no override was
	// injected by the caller (e.g. tests). The dispatcher is the only path
	// that couples Store + ChatProvider to vocabulary events.
	if cfg.Dispatcher == nil && chatAdapter != nil {
		cfg.Dispatcher = botpkg.New(cfg.Store, chatAdapter)
	}

	// Populate cfg.Chat from the discovered Sharkfin adapter when not already
	// injected (tests may inject their own ChatProvider via ServerConfig.Chat).
	if cfg.Chat == nil && chatAdapter != nil {
		cfg.Chat = chatAdapter
	}

	var sch *scheduler.Scheduler
	if hiveAgentClient != nil {
		sch = scheduler.New(scheduler.Config{
			Hive:         hiveAgentClient,
			Audit:        cfg.Store,
			Projects:     cfg.Store,
			Vocabularies: cfg.Store,
			Dispatcher:   cfg.Dispatcher,
		})
	}

	registerTemplateRoutes(api, cfg.Store)
	registerInstanceRoutes(api, cfg.Store)
	registerWorkItemRoutes(api, cfg.Store)
	registerTransitionRoutes(api, svc)
	registerApprovalRoutes(api, cfg.Store, svc)
	registerRuntimeDiagRoutes(api, cfg.Runtime)
	registerSchedulerAndAuditDiagRoutes(api, sch, cfg.Store)
	registerVocabularyRoutes(api, cfg.Store)
	registerProjectRoutes(api, cfg.Store, cfg.BotKeysDir, cfg.Chat)
	registerBotKeyRoutes(api, cfg.Store, cfg.BotKeysDir, cfg.Passport)
	registerAgentRoutes(api, hiveAgentClient)
	registerAuditRoutes(api, cfg.Store)

	// UI routes — /ui/health + /ui/* embedded SPA. Sub the //go:embed
	// "dist" subdir to root the file server. fs.Sub returns an error
	// only on invalid path; an empty embed.FS yields a valid (empty)
	// fsys that fileExists("remoteEntry.js") returns false for, which
	// is exactly the dev-mode signal Scope treats as "UI not built".
	uiFS, err := fs.Sub(flowweb.Dist, "dist")
	if err == nil {
		registerUIRoutes(mux, uiFS)
	}

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// Sharkfin webhook receiver.
	mux.Handle("POST /v1/webhooks/sharkfin", HandleSharkfinWebhook(nil))

	// Combine webhook receiver.
	mux.Handle("POST /v1/webhooks/combine", HandleCombineWebhook(cfg.Store, cfg.Store, cfg.Dispatcher))

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
