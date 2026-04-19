// SPDX-License-Identifier: GPL-2.0-only

//go:build e2e

package daemon

import (
	"os"

	flowDaemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/infra/runtime/stub"
)

// injectStubRuntime is compiled only in e2e builds. It reads
// FLOW_E2E_RUNTIME_STUB to allow per-test opt-in without a
// permanent production backdoor.
func injectStubRuntime(cfg *flowDaemon.ServerConfig) {
	if os.Getenv("FLOW_E2E_RUNTIME_STUB") == "1" {
		cfg.Runtime = stub.New()
	}
}

// injectNexusRuntime is a no-op in e2e builds — the stub injector
// above is the only writer of cfg.Runtime when the e2e tag is set.
// E2E tests that need a real Nexus driver build without the e2e
// tag (or use the dedicated nexus_driver_test.go scenarios that
// drive the production binary).
func injectNexusRuntime(_ *flowDaemon.ServerConfig, _, _ string) {}
