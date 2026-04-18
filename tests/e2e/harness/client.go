// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a raw http.Client wrapper for the test side of the harness.
// It explicitly does NOT depend on any WorkFort SDK — every contract
// is wire-only. If you reach for sharkfinclient or hiveclient here,
// stop and add the request inline instead.
//
// Auth model: Passport's middleware reads `Authorization: Bearer <token>`
// only — there is no separate API-key header. The validator chain
// internally tries JWT first and falls back to API-key validation
// against the same Bearer token. So both JWTs and API keys ride the
// same wire; tests pick the constructor that documents intent.
type Client struct {
	baseURL string
	token   string // empty means: send no Authorization header
	http    *http.Client
}

// NewClient returns a Client that sends `Authorization: Bearer <token>`.
// Use it for both JWTs and API keys — Passport's validator chain
// distinguishes them server-side.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientNoAuth returns a Client that sends no auth headers.
func NewClientNoAuth(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) authedRequest(method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// Do executes the request and returns (statusCode, responseBody).
// out, if non-nil and the response is 2xx, is JSON-decoded into.
// On non-2xx the response body is returned raw.
func (c *Client) Do(method, path string, body, out any) (int, []byte, error) {
	req, err := c.authedRequest(method, path, body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, respBody, fmt.Errorf("decode response: %w (body: %s)", err, respBody)
		}
	}
	return resp.StatusCode, respBody, nil
}

// GetJSON / PostJSON / PatchJSON / DeleteJSON are thin sugar.
func (c *Client) GetJSON(path string, out any) (int, []byte, error) {
	return c.Do(http.MethodGet, path, nil, out)
}
func (c *Client) PostJSON(path string, body, out any) (int, []byte, error) {
	return c.Do(http.MethodPost, path, body, out)
}
func (c *Client) PatchJSON(path string, body, out any) (int, []byte, error) {
	return c.Do(http.MethodPatch, path, body, out)
}
func (c *Client) DeleteJSON(path string) (int, []byte, error) {
	return c.Do(http.MethodDelete, path, nil, nil)
}
