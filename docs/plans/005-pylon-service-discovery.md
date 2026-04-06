---
type: plan
step: "5"
status: approved
codebase: flow
---

# Flow Phase 5 — Pylon Service Discovery and Config Restructure

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify all external service discovery through Pylon. Replace the flat `--hive-url` / `pylon-url` CLI flags and config keys with a nested `pylon:` YAML block. Hive is discovered via Pylon the same way Sharkfin is — no hardcoded URL flag.

**Prerequisites:** Phase 3 (003-hive-identity-adapter.md) and Phase 4 (004-sharkfin-chat-adapter.md) complete.

**Pylon client package:** `github.com/Work-Fort/Pylon/client/go` at `/home/kazw/Work/WorkFort/pylon/lead/client/go/`.

**Key facts about the Pylon client:**
- `New(pylonURL, token string) *Client` — constructs a client.
- `ServiceByName(ctx, name string) (*Service, error)` — looks up a service by its registered name; returns `ErrNotFound` sentinel if not registered.
- `Service.BaseURL string` — the discovered service's base URL.

**Config target — nested YAML:**
```yaml
pylon:
  url: http://pylon:17300
  services:
    hive: hive
    sharkfin: sharkfin
```

**Env vars after this plan:**
- `FLOW_PYLON_URL`
- `FLOW_PYLON_SERVICES_HIVE`
- `FLOW_PYLON_SERVICES_SHARKFIN`

**Auth doc to update:** `docs/2026-04-05-auth-config-fix.md` references `FLOW_HIVE_URL` — update to reflect Pylon-based discovery.

---

## Chunk 1: Config restructure

### Task 1: Update `internal/config/config.go`

**Files:** `internal/config/config.go`

Replace the flat `hive-url` and `pylon-url` viper defaults with nested `pylon.*` keys. Remove `hive-url` entirely.

- [ ] **Step 1: Update `InitViper` defaults**

Replace:
```go
viper.SetDefault("hive-url", "")
viper.SetDefault("pylon-url", "")
```

With:
```go
viper.SetDefault("pylon.url", "")
viper.SetDefault("pylon.services.hive", "hive")
viper.SetDefault("pylon.services.sharkfin", "sharkfin")
```

Viper's `AutomaticEnv()` handles nested key → env var mapping natively: `pylon.url` resolves to `FLOW_PYLON_URL`, `pylon.services.hive` resolves to `FLOW_PYLON_SERVICES_HIVE`. The `NewReplacer` is unchanged — keep it as `strings.NewReplacer("-", "_")`.

Full updated `InitViper` for reference:
```go
func InitViper() {
	viper.SetDefault("bind", DefaultBind)
	viper.SetDefault("port", DefaultPort)
	viper.SetDefault("log-level", "debug")
	viper.SetDefault("db", "")
	viper.SetDefault("passport-url", "")
	viper.SetDefault("service-token", "")
	viper.SetDefault("pylon.url", "")
	viper.SetDefault("pylon.services.hive", "hive")
	viper.SetDefault("pylon.services.sharkfin", "sharkfin")
	viper.SetDefault("webhook-base-url", "")

	viper.SetConfigName(ConfigFileName)
	viper.SetConfigType(ConfigType)
	viper.AddConfigPath(GlobalPaths.ConfigDir)

	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
```

**Breaking config change:** existing `config.yaml` files using the flat key `pylon-url:` must be updated to the nested form `pylon: { url: ... }`. There is no automatic migration — users must update their config files before upgrading.

- [ ] **Step 2: Verify** — `go build ./internal/config/...` exits 0.

- [ ] **Step 3: Commit** — `chore(config): replace flat hive-url/pylon-url with nested pylon.* keys`

  > **Breaking change note:** include in commit message body: "BREAKING: `pylon-url` flat config key is removed. Update config.yaml to use nested `pylon: { url: ... }` form. `hive-url` is removed entirely — Hive is now discovered via Pylon."

---

### Task 2: Update `cmd/daemon/daemon.go`

**Files:** `cmd/daemon/daemon.go`

Remove `--hive-url` flag. Replace flat `pylonURL` flag with `--pylon-url`. Read service name overrides from Viper (config/env only — no CLI flags for those).

- [ ] **Step 1: Rewrite `NewCmd` and `run` signature**

```go
func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var passportURL string
	var serviceToken string
	var pylonURL string
	var webhookBaseURL string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Flow daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("bind") {
				bind = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("db") {
				db = viper.GetString("db")
			}
			if !cmd.Flags().Changed("passport-url") {
				passportURL = viper.GetString("passport-url")
			}
			if !cmd.Flags().Changed("service-token") {
				serviceToken = viper.GetString("service-token")
			}
			if !cmd.Flags().Changed("pylon-url") {
				pylonURL = viper.GetString("pylon.url")
			}
			if !cmd.Flags().Changed("webhook-base-url") {
				webhookBaseURL = viper.GetString("webhook-base-url")
			}

			hiveSvcName := viper.GetString("pylon.services.hive")
			sharkfinSvcName := viper.GetString("pylon.services.sharkfin")

			return run(bind, port, db, passportURL, serviceToken, pylonURL, webhookBaseURL, hiveSvcName, sharkfinSvcName)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17200, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database file path (empty = default location)")
	cmd.Flags().StringVar(&passportURL, "passport-url", "", "Passport auth service URL")
	cmd.Flags().StringVar(&serviceToken, "service-token", "", "Service identity token (Passport API key)")
	cmd.Flags().StringVar(&pylonURL, "pylon-url", "", "Pylon service registry URL")
	cmd.Flags().StringVar(&webhookBaseURL, "webhook-base-url", "", "Flow's externally reachable base URL for webhook callbacks")

	return cmd
}
```

- [ ] **Step 2: Rewrite `run` to use new signature and pass `PylonServices` struct**

```go
func run(bind string, port int, db, passportURL, serviceToken, pylonURL, webhookBaseURL, hiveSvcName, sharkfinSvcName string) error {
	health := flowDaemon.NewHealthService()

	dsn := db
	if dsn == "" {
		dsn = filepath.Join(config.GlobalPaths.StateDir, "flow.db")
	}

	store, err := infra.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	health.RegisterPeriodicCheck("db", func(ctx context.Context) flowDaemon.CheckResult {
		if err := store.Ping(ctx); err != nil {
			return flowDaemon.CheckResult{Severity: flowDaemon.SeverityError, Message: err.Error()}
		}
		return flowDaemon.CheckResult{Severity: flowDaemon.SeverityOK}
	})

	srv := flowDaemon.NewServer(flowDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		PassportURL:  passportURL,
		ServiceToken: serviceToken,
		PylonURL:     pylonURL,
		PylonServices: flowDaemon.PylonServices{
			Hive:     hiveSvcName,
			Sharkfin: sharkfinSvcName,
		},
		WebhookBaseURL: webhookBaseURL,
		Health:         health,
		Store:          store,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := flowDaemon.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go health.StartPeriodic(healthCtx, 30*time.Second)

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
```

- [ ] **Step 3: Verify** — `go build ./cmd/...` exits 0.

- [ ] **Step 4: Commit** — `feat(daemon): remove --hive-url, add pylon service name config`

---

## Chunk 2: Server wiring — discover Hive via Pylon

### Task 3: Update `ServerConfig` and `NewServer` in `internal/daemon/server.go`

**Files:** `internal/daemon/server.go`

Replace `HiveURL string` with `PylonServices PylonServices`. Discover Hive through Pylon at startup using the same pattern as Sharkfin.

- [ ] **Step 1: Add `PylonServices` struct and update `ServerConfig`**

Replace:
```go
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
```

With:
```go
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
}
```

- [ ] **Step 2: Rewrite Pylon discovery block in `NewServer`**

Replace the existing identity provider and Pylon blocks:

```go
var identityProvider domain.IdentityProvider
if cfg.HiveURL != "" {
	identityProvider = hiveinfra.New(cfg.HiveURL, cfg.ServiceToken)
}

var chatAdapter *sharkfininfra.Adapter
if cfg.PylonURL != "" {
	pylonClient := pylonclient.New(cfg.PylonURL, cfg.ServiceToken)
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()
	if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, "sharkfin"); err == nil {
		...
	}
}
```

With a unified discovery block:

```go
var identityProvider domain.IdentityProvider
var chatAdapter *sharkfininfra.Adapter

if cfg.PylonURL != "" {
	pylonClient := pylonclient.New(cfg.PylonURL, cfg.ServiceToken)
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()

	// Discover Hive via Pylon.
	if hiveSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Hive); err == nil {
		identityProvider = hiveinfra.New(hiveSvc.BaseURL, cfg.ServiceToken)
	} else {
		log.Warn("hive not found in Pylon, identity checks disabled", "err", err)
	}

	// Discover Sharkfin via Pylon.
	if sharkfinSvc, err := pylonClient.ServiceByName(startupCtx, cfg.PylonServices.Sharkfin); err == nil {
		if a, err := sharkfininfra.New(startupCtx, sharkfinSvc.BaseURL, cfg.ServiceToken); err == nil {
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
			log.Warn("sharkfin dial failed, chat disabled", "err", err)
		}
	} else {
		log.Warn("sharkfin not found in Pylon, chat disabled", "err", err)
	}
}
```

- [ ] **Step 3: Verify** — `go build ./internal/daemon/...` exits 0.

- [ ] **Step 4: Verify** — `go build ./...` exits 0 (full build).

- [ ] **Step 5: Commit** — `feat(daemon): discover Hive via Pylon, remove HiveURL field`

---

## Chunk 3: Auth config doc update

### Task 4: Update `docs/2026-04-05-auth-config-fix.md`

**Files:** `docs/2026-04-05-auth-config-fix.md`

The doc has two problems to fix: it references `FLOW_HIVE_URL` (removed) and `FLOW_PASSPORT_TOKEN` (stale — the correct env var is `FLOW_SERVICE_TOKEN`).

- [ ] **Step 1: Replace the config example block**

Replace:
```
FLOW_PASSPORT_URL=http://passport:3000
FLOW_PASSPORT_TOKEN=wf-svc_flow_xxxxx
FLOW_HIVE_URL=http://hive:17100
FLOW_SHARKFIN_URL=http://sharkfin:16000
```

With:
```
FLOW_PASSPORT_URL=http://passport:3000
FLOW_SERVICE_TOKEN=wf-svc_flow_xxxxx
FLOW_PYLON_URL=http://pylon:17300
# Service names default to "hive" and "sharkfin" — override only if needed:
# FLOW_PYLON_SERVICES_HIVE=hive
# FLOW_PYLON_SERVICES_SHARKFIN=sharkfin
```

Also remove or update any prose that references `FLOW_HIVE_URL`, `FLOW_PASSPORT_TOKEN`, or per-service URL flags.

- [ ] **Step 2: Commit** — `docs(config): fix stale env vars and update to Pylon-based discovery`

---

## Chunk 4: Smoke test

### Task 5: End-to-end config smoke test

This plan does not add unit-testable logic (service discovery at startup is side-effectful). Verify correctness by build + config loading.

- [ ] **Step 1: Run full test suite** — `go test ./...` exits 0.

- [ ] **Step 2: Manual config sanity** — write a minimal `config.yaml` using the new nested format and confirm `viper.GetString("pylon.url")` and `viper.GetString("pylon.services.hive")` resolve correctly in a small `go run` or test helper if needed.

- [ ] **Step 3: Confirm env var mapping** — verify that setting `FLOW_PYLON_URL=http://test:1` and running `go test ./internal/config/...` (or a quick manual smoke) shows the correct value.

---

## Summary of changes

| File | Change |
|---|---|
| `internal/config/config.go` | Remove `hive-url` default; add `pylon.url`, `pylon.services.hive`, `pylon.services.sharkfin`; `NewReplacer` unchanged (`"-", "_"` only) |
| `cmd/daemon/daemon.go` | Remove `--hive-url` flag and `hiveURL` var; read `pylon.url`, `pylon.services.*` from Viper; pass `PylonServices` struct to `NewServer` |
| `internal/daemon/server.go` | Add `PylonServices` struct; remove `HiveURL` from `ServerConfig`; discover Hive via `pylonClient.ServiceByName` inside unified Pylon block |
| `docs/2026-04-05-auth-config-fix.md` | Replace stale `FLOW_PASSPORT_TOKEN` with `FLOW_SERVICE_TOKEN`; replace `FLOW_HIVE_URL` with `FLOW_PYLON_URL` + nested service names |
