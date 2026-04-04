// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Flow/internal/domain"
)

// MCPDeps holds the dependencies injected into MCP tool handlers.
type MCPDeps struct {
	Store domain.Store
}

// NewMCPHandler creates the mcp-go StreamableHTTPServer and returns it as
// an http.Handler ready to be mounted on /mcp.
func NewMCPHandler(deps MCPDeps) http.Handler {
	mcpServer := server.NewMCPServer(
		"flow",
		"1.0",
		server.WithToolCapabilities(false),
	)

	registerTools(mcpServer, deps)

	httpServer := server.NewStreamableHTTPServer(mcpServer)

	return httpServer
}
