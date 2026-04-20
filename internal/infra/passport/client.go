// SPDX-License-Identifier: GPL-2.0-only
package passport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal Passport HTTP client for API key lifecycle management.
// Uses better-auth's /v1/api-key/* endpoints (basePath="/v1" in Passport's auth.ts).
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New creates a new Client. baseURL is the Passport daemon URL.
// token is a Passport service token (used in Authorization: Bearer).
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// MintAPIKey calls POST /v1/api-key/create and returns the plaintext key and its ID.
// The key is returned exactly once by Passport; Flow is responsible for storing the hash.
func (c *Client) MintAPIKey(ctx context.Context, name string) (plaintext, keyID string, err error) {
	body := map[string]any{
		"name":   name,
		"prefix": "wf-svc",
	}
	var resp struct {
		Key struct {
			Key string `json:"key"`
			ID  string `json:"id"`
		} `json:"key"`
	}
	if err := c.post(ctx, "/v1/api-key/create", body, &resp); err != nil {
		return "", "", fmt.Errorf("passport mint api key: %w", err)
	}
	if resp.Key.Key == "" || resp.Key.ID == "" {
		return "", "", fmt.Errorf("passport returned empty key or id")
	}
	return resp.Key.Key, resp.Key.ID, nil
}

// RevokeAPIKey calls DELETE-equivalent on /v1/api-key/delete. Best-effort —
// callers should log failure but not abort their primary operation.
func (c *Client) RevokeAPIKey(ctx context.Context, keyID string) error {
	body := map[string]any{"keyId": keyID}
	if err := c.post(ctx, "/v1/api-key/delete", body, nil); err != nil {
		return fmt.Errorf("passport revoke api key %s: %w", keyID, err)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("passport %s: status %d body %s", path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
