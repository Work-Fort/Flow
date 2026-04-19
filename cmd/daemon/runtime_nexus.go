// SPDX-License-Identifier: GPL-2.0-only

//go:build !e2e

package daemon

import (
	"github.com/charmbracelet/log"

	flowDaemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/infra/runtime/nexus"
)

// injectStubRuntime is a no-op in production builds — the e2e build
// tag overrides this with the env-gated stub injector.
func injectStubRuntime(_ *flowDaemon.ServerConfig) {}

// injectNexusRuntime constructs the Nexus RuntimeDriver when a
// --nexus-url is configured. When the URL is empty, the runtime
// remains nil and /v1/runtime/_diag/* returns 503 (operator opt-in).
//
// Called AFTER injectStubRuntime so the e2e stub injector still
// wins in e2e builds. In production (this build), injectStubRuntime
// is the no-op above, so this is the only writer of cfg.Runtime.
func injectNexusRuntime(cfg *flowDaemon.ServerConfig, nexusURL, serviceToken string) {
	if cfg.Runtime != nil {
		return // already populated (defence in depth — should be unreachable in !e2e)
	}
	if nexusURL == "" {
		log.Info("nexus runtime driver disabled (no --nexus-url)")
		return
	}
	cfg.Runtime = nexus.New(nexus.Config{
		BaseURL:      nexusURL,
		ServiceToken: serviceToken,
	})
	log.Info("nexus runtime driver enabled", "url", nexusURL)
}
