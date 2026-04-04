// SPDX-License-Identifier: GPL-2.0-only
package mcpbridge

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewCmd returns the mcp-bridge cobra command.
func NewCmd() *cobra.Command {
	var agentID string
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "mcp-bridge",
		Short: "Stdio-to-HTTP MCP bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("host") {
				host = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			return run(agentID, host, port)
		},
	}

	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID (required)")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17200, "Daemon port")
	cmd.MarkFlagRequired("agent-id")

	return cmd
}

func run(agentID, host string, port int) error {
	mcpURL := fmt.Sprintf("http://%s:%d/mcp", host, port)

	client := &http.Client{Timeout: 60 * time.Second}
	var sessionID string

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		req, err := http.NewRequest("POST", mcpURL, bytes.NewReader(line))
		if err != nil {
			log.Error("create request", "err", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Agent-Id", agentID)
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Error("forward request", "err", err)
			continue
		}

		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			sessionID = sid
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Error("read response", "err", err)
			continue
		}

		if _, err := os.Stdout.Write(body); err != nil {
			return fmt.Errorf("stdout write: %w", err)
		}
		if _, err := os.Stdout.Write([]byte("\n")); err != nil {
			return fmt.Errorf("stdout write: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read: %w", err)
	}

	return nil
}
