// SPDX-License-Identifier: GPL-2.0-only
package daemon_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"

	daemon "github.com/Work-Fort/Flow/internal/daemon"
)

func TestRegisterTools_Count(t *testing.T) {
	s := server.NewMCPServer("test", "1.0")
	daemon.RegisterTools(s, daemon.MCPDeps{})
	got := len(s.ListTools())
	const want = 15
	if got != want {
		t.Errorf("tool count = %d, want %d (update TestRegisterTools_Count if you added a tool)", got, want)
	}
}
