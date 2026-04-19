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
// Auth model (post-Passport-scheme-split). Passport's middleware now
// dispatches by Authorization scheme:
//   - "Bearer <jwt>"      → JWT validator only
//   - "ApiKey-v1 <key>"   → API-key validator only
//   - any other scheme    → 401 (no fallthrough)
//
// Pick the constructor that documents intent:
//   - NewClient(baseURL, jwt)        → Authorization: Bearer <jwt>
//   - NewClientAPIKey(baseURL, key)  → Authorization: ApiKey-v1 <key>
//   - NewClientNoAuth(baseURL)       → no Authorization header
//   - NewClientRawAuth(baseURL, raw) → Authorization: <raw>, exact bytes
//     (use for negative tests that need malformed/garbage headers)
type Client struct {
	baseURL    string
	authHeader string // empty means: send no Authorization header
	http       *http.Client
}

// NewClient returns a Client that sends `Authorization: Bearer <token>`.
// Use it only for JWTs — API keys must use NewClientAPIKey or the
// daemon's scheme dispatch will reject them with 401.
func NewClient(baseURL, jwt string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Bearer " + jwt,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientAPIKey returns a Client that sends `Authorization: ApiKey-v1 <key>`.
// Required for API-key auth after the Passport scheme-split — sending
// an API key under "Bearer" is rejected as a Cluster-3b regression.
func NewClientAPIKey(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "ApiKey-v1 " + apiKey,
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientNoAuth returns a Client that sends no auth headers.
func NewClientNoAuth(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientRawAuth returns a Client that sends the given string verbatim
// as the Authorization header value. Use only for negative tests
// (malformed scheme, missing space, garbage). Production-style auth
// MUST use NewClient or NewClientAPIKey.
func NewClientRawAuth(baseURL, rawAuth string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: rawAuth,
		http:       &http.Client{Timeout: 10 * time.Second},
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
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
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
