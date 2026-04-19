// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPClient is a hand-rolled MCP-over-streaming-HTTP wire client.
// It speaks JSON-RPC 2.0, sends Authorization: ApiKey-v1 <key>,
// follows Mcp-Session-Id across calls, and copes with the three
// response shapes Flow's NewStreamableHTTPServer produces:
// application/json, text/event-stream, and 202 Accepted.
//
// It deliberately does NOT import mark3labs/mcp-go. Drift between
// this client and the real MCP server's wire format must surface as
// test failure — see feedback_e2e_harness_independence.md.
type MCPClient struct {
	mcpURL string
	apiKey string
	http   *http.Client
	mu     sync.Mutex
	sess   string
	nextID atomic.Int64
}

// NewMCPClient constructs a client targeting the given /mcp URL and
// using the given Passport API key for Authorization.
func NewMCPClient(mcpURL, apiKey string) *MCPClient {
	return &MCPClient{
		mcpURL: mcpURL,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// SessionID returns the most recent Mcp-Session-Id the server set.
func (c *MCPClient) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sess
}

// SetSessionID seeds the session header. Useful for tests that want
// to verify the header round-trips.
func (c *MCPClient) SetSessionID(s string) {
	c.mu.Lock()
	c.sess = s
	c.mu.Unlock()
}

// Call invokes the named MCP tool with the given arguments and
// returns the first text-content block of the result. If the result
// is marked isError, returns a non-nil error whose message contains
// the error text.
func (c *MCPClient) Call(tool string, args map[string]any) (string, error) {
	id := c.nextID.Add(1)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("mcp marshal: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.mcpURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("mcp request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "ApiKey-v1 "+c.apiKey)
	if s := c.SessionID(); s != "" {
		httpReq.Header.Set("Mcp-Session-Id", s)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", tool, err)
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sess = sid
		c.mu.Unlock()
	}

	if resp.StatusCode == http.StatusAccepted {
		// notification accepted, no body
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mcp call %s: HTTP %d: %s", tool, resp.StatusCode, raw)
	}

	msgs, err := readMCPResponse(resp)
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", tool, err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("mcp call %s: empty response", tool)
	}

	// The last message is the actual response; intermediate messages
	// are notifications.
	last := msgs[len(msgs)-1]
	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(last, &rpc); err != nil {
		return "", fmt.Errorf("mcp call %s: parse response: %w (body=%s)", tool, err, last)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("mcp call %s: rpc error %d: %s", tool, rpc.Error.Code, rpc.Error.Message)
	}
	var text string
	if len(rpc.Result.Content) > 0 {
		text = rpc.Result.Content[0].Text
	}
	if rpc.Result.IsError {
		return "", errors.New(text)
	}
	return text, nil
}

// readMCPResponse handles SSE and JSON content types. Returns each
// data: line (SSE) or the entire body (JSON) as a separate message.
func readMCPResponse(resp *http.Response) ([][]byte, error) {
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		var msgs [][]byte
		sc := bufio.NewScanner(resp.Body)
		// Default buffer is 64 KiB; bump it for larger MCP responses.
		buf := make([]byte, 0, 256*1024)
		sc.Buffer(buf, 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data: ") {
				msgs = append(msgs, []byte(strings.TrimPrefix(line, "data: ")))
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read SSE: %w", err)
		}
		return msgs, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return [][]byte{body}, nil
}
