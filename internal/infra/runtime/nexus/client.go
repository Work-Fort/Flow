// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Work-Fort/Flow/internal/domain"
)

// url builds an absolute URL by joining BaseURL and path. The path
// MUST start with "/". Trailing slash on BaseURL is tolerated.
func (d *Driver) url(path string) string {
	return strings.TrimRight(d.cfg.BaseURL, "/") + path
}

// do issues req with the configured client, attaching the bearer
// token when one is configured. Returns the response — caller is
// responsible for closing Body.
func (d *Driver) do(req *http.Request) (*http.Response, error) {
	if d.cfg.ServiceToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.ServiceToken)
	}
	return d.http.Do(req)
}

// statusErr maps a non-2xx response to a domain-aware error.
// The body is read (best-effort, capped) and inlined into the
// error message for diagnostic value.
func (d *Driver) statusErr(resp *http.Response) error {
	const maxBody = 4096
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	bodyStr := strings.TrimSpace(string(body))
	switch resp.StatusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("nexus rejected: %s: %w", bodyStr, domain.ErrValidation)
	case http.StatusNotFound:
		return fmt.Errorf("nexus not found: %s: %w", bodyStr, domain.ErrNotFound)
	case http.StatusConflict:
		return fmt.Errorf("nexus conflict: %s: %w", bodyStr, domain.ErrInvalidState)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("nexus unavailable: %s: %w", bodyStr, domain.ErrRuntimeUnavailable)
	default:
		return fmt.Errorf("nexus http %d: %s", resp.StatusCode, bodyStr)
	}
}

// getJSON performs GET path, decoding a JSON response into out.
// out may be nil to discard the body.
func (d *Driver) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url(path), nil)
	if err != nil {
		return err
	}
	resp, err := d.do(req)
	if err != nil {
		return fmt.Errorf("nexus get %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return d.statusErr(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// postJSON performs POST path with body, decoding the response into
// out. body may be nil for empty-body POSTs; out may be nil to
// discard.
func (d *Driver) postJSON(ctx context.Context, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("nexus post %s: marshal: %w", path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url(path), rdr)
	if err != nil {
		return err
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.do(req)
	if err != nil {
		return fmt.Errorf("nexus post %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return d.statusErr(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// delete performs DELETE path. 404 from Nexus is reported as a
// wrapped ErrNotFound — callers may translate to nil for idempotent
// teardown paths.
func (d *Driver) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, d.url(path), nil)
	if err != nil {
		return err
	}
	resp, err := d.do(req)
	if err != nil {
		return fmt.Errorf("nexus delete %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return d.statusErr(resp)
	}
	return nil
}
