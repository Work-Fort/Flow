// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"os"
	"testing"
)

// EnvOption configures harness.NewEnv.
type EnvOption func(*envCfg)

type envCfg struct {
	backend     string // "sqlite" (default) or "postgres"
	stubRuntime bool
	nexusURL    string
}

// WithBackend selects the daemon's storage backend.
func WithBackend(b string) EnvOption {
	return func(c *envCfg) { c.backend = b }
}

// WithStubRuntimeEnv injects stub.Driver as the daemon's RuntimeDriver.
func WithStubRuntimeEnv() EnvOption {
	return func(c *envCfg) { c.stubRuntime = true }
}

// WithNexusURL wires the Flow daemon's --nexus-url flag so the
// production NexusDriver activates and binds /v1/runtime/_diag/*.
// Use with StartNexusDaemon to bring up a real Nexus first.
func WithNexusURL(url string) EnvOption {
	return func(c *envCfg) { c.nexusURL = url }
}

var defaultBackend = "sqlite"

// SetDefaultBackend overrides the backend used when NewEnv is called
// without WithBackend. Wired from main_test.go's -backend flag.
func SetDefaultBackend(b string) { defaultBackend = b }

// Env is the all-in-one harness construction. Each test's setup looks like:
//
//	env := harness.NewEnv(t)
//	defer env.Cleanup(t)
//	tok := env.Daemon.SignJWT(...)
//	c := harness.NewClient(env.Daemon.BaseURL(), tok)
//	... assertions ...
//
// Env exposes the underlying JWKS stub so tests can read its
// APIKeyVerifyCount() to assert the apikey path was actually traversed.
type Env struct {
	Daemon       *Daemon
	JWKS         *JWKSStub
	Hive         *FakeHive
	Sharkfin     *FakeSharkfin
	stopPylon    func()
	stopHive     func()
	stopSharkfin func()
}

// NewEnv stands up the JWKS stub, Pylon stub, fake Hive, fake Sharkfin,
// then spawns the daemon pointed at all of them. Calls t.Fatal on failure.
//
// The flow binary is read from FLOW_BINARY (set by `mise run e2e`). When
// FLOW_BINARY is unset, falls back to "../../build/flow" so tests can be
// run directly from inside tests/e2e during TDD.
//
// Tests using NewEnv are intentionally NOT t.Parallel — each spawn costs
// ~200ms (subprocess fork + readiness poll) and there is no shared state
// to keep multiple daemons consistent against. If suite latency becomes a
// problem, batch related assertions into a single test rather than
// parallelizing.
func NewEnv(t testing.TB, opts ...EnvOption) *Env {
	t.Helper()

	cfg := &envCfg{backend: defaultBackend}
	for _, o := range opts {
		o(cfg)
	}

	binary := os.Getenv("FLOW_BINARY")
	if binary == "" {
		binary = "../../build/flow"
	}

	jwks := StartJWKSStub()

	hive := NewFakeHive()
	hiveBase, stopHive := hive.Start()

	sharkfin := NewFakeSharkfin()
	sharkfinBase, stopSharkfin := sharkfin.Start()

	pylonServices := []PylonService{
		{Name: "hive", BaseURL: hiveBase, Label: "Hive", Route: "/hive"},
		{Name: "sharkfin", BaseURL: sharkfinBase, Label: "Sharkfin", Route: "/sharkfin"},
	}
	pylonAddr, stopPylon := StartPylonStub(pylonServices)

	var daemonOpts []DaemonOption
	if cfg.stubRuntime {
		daemonOpts = append(daemonOpts, WithStubRuntime())
	}
	if cfg.nexusURL != "" {
		daemonOpts = append(daemonOpts, withNexusDaemonURL(cfg.nexusURL))
	}
	if cfg.backend == "postgres" {
		dsn := os.Getenv("FLOW_E2E_PG_DSN")
		if dsn == "" {
			dsn = "postgres://postgres@127.0.0.1/flow_test?sslmode=disable"
		}
		daemonOpts = append(daemonOpts, WithDB(dsn))
	}

	d, err := StartDaemon(t, binary, pylonAddr, jwks.Addr, jwks.SignJWT, daemonOpts...)
	if err != nil {
		stopSharkfin()
		stopHive()
		stopPylon()
		jwks.Stop()
		t.Fatalf("start daemon: %v", err)
	}

	return &Env{
		Daemon: d, JWKS: jwks, Hive: hive, Sharkfin: sharkfin,
		stopPylon:    stopPylon,
		stopHive:     stopHive,
		stopSharkfin: stopSharkfin,
	}
}

// Cleanup stops everything in reverse order. Idempotent.
func (e *Env) Cleanup(t testing.TB) {
	t.Helper()
	if e.Daemon != nil {
		e.Daemon.Stop(t)
		e.Daemon = nil
	}
	if e.stopSharkfin != nil {
		e.stopSharkfin()
		e.stopSharkfin = nil
	}
	if e.stopHive != nil {
		e.stopHive()
		e.stopHive = nil
	}
	if e.stopPylon != nil {
		e.stopPylon()
		e.stopPylon = nil
	}
	if e.JWKS != nil {
		e.JWKS.Stop()
		e.JWKS = nil
	}
}
