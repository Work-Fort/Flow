// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Work-Fort/Flow/internal/domain"
)

// VolumeKind is the value of VolumeRef.Kind for refs emitted by this
// driver. RuntimeHandleKind is the equivalent for runtime handles.
const (
	VolumeKind        = "nexus-drive"
	RuntimeHandleKind = "nexus-vm"
)

// ErrUnsupportedKind is returned when a method receives a VolumeRef
// or RuntimeHandle whose Kind was not produced by this driver.
var ErrUnsupportedKind = errors.New("nexus driver: unsupported ref kind")

// Config carries driver construction parameters.
type Config struct {
	// BaseURL is the Nexus daemon's REST root, e.g. "http://nexus:9600".
	// Trailing slash is tolerated.
	BaseURL string
	// ServiceToken is the Passport API key the driver attaches as a
	// Bearer credential on every request. Empty disables auth (used
	// by e2e against an unauthed Nexus).
	ServiceToken string
	// HTTPClient is optional; nil yields a 30s-timeout default.
	HTTPClient *http.Client
	// VMImage is the OCI image used when StartAgentRuntime creates a
	// fresh VM. Empty defaults to "docker.io/library/alpine:latest"
	// — sufficient for the diagnostic happy path; production wiring
	// overrides per-deployment.
	VMImage string
}

// Driver implements domain.RuntimeDriver against a Nexus daemon.
type Driver struct {
	cfg  Config
	http *http.Client

	mu      sync.Mutex
	masters map[string]domain.VolumeRef // projectID -> master ref
}

// New constructs a Driver. Network I/O is deferred until a method is
// called.
func New(cfg Config) *Driver {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.VMImage == "" {
		cfg.VMImage = "docker.io/library/alpine:latest"
	}
	return &Driver{
		cfg:     cfg,
		http:    cfg.HTTPClient,
		masters: make(map[string]domain.VolumeRef),
	}
}

// --- domain.RuntimeDriver method stubs (filled in by subsequent tasks) ---

func (d *Driver) StartAgentRuntime(_ context.Context, _ string, _ domain.VolumeRef, _ domain.VolumeRef) (domain.RuntimeHandle, error) {
	return domain.RuntimeHandle{}, errors.New("nexus driver: StartAgentRuntime not yet implemented")
}

func (d *Driver) StopAgentRuntime(_ context.Context, _ domain.RuntimeHandle) error {
	return errors.New("nexus driver: StopAgentRuntime not yet implemented")
}

func (d *Driver) IsRuntimeAlive(_ context.Context, _ domain.RuntimeHandle) (bool, error) {
	return false, errors.New("nexus driver: IsRuntimeAlive not yet implemented")
}

// CloneWorkItemVolume issues POST /v1/drives/clone with a CSI-shaped
// body. Mount path is intentionally OMITTED from the request: per
// Nexus's clone endpoint contract, an unset mount_path inherits the
// source drive's mount_path (which the project master already has
// set). Flow does not need to assert a per-work-item mount path.
func (d *Driver) CloneWorkItemVolume(ctx context.Context, master domain.VolumeRef, workItemID string) (domain.VolumeRef, error) {
	if master.Kind != VolumeKind {
		return domain.VolumeRef{}, fmt.Errorf("master.Kind=%q: %w", master.Kind, ErrUnsupportedKind)
	}
	body := struct {
		SourceVolumeRef string `json:"source_volume_ref"`
		Name            string `json:"name"`
	}{
		SourceVolumeRef: master.ID,
		Name:            "work-item-" + workItemID,
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := d.postJSON(ctx, "/v1/drives/clone", body, &resp); err != nil {
		return domain.VolumeRef{}, err
	}
	return domain.VolumeRef{Kind: VolumeKind, ID: resp.ID}, nil
}

// DeleteVolume issues DELETE /v1/drives/{id}. Idempotent: a 404
// response is reported as success so cleanup paths can be
// unconditional. A zero-value VolumeRef (orchestrator aborted before
// clone) is also a no-op.
func (d *Driver) DeleteVolume(ctx context.Context, v domain.VolumeRef) error {
	if v.Kind == "" && v.ID == "" {
		return nil
	}
	if v.Kind != VolumeKind {
		return fmt.Errorf("v.Kind=%q: %w", v.Kind, ErrUnsupportedKind)
	}
	err := d.delete(ctx, "/v1/drives/"+v.ID)
	if err != nil && errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

func (d *Driver) RefreshProjectMaster(_ context.Context, _ string, _ string) error {
	return errors.New("nexus driver: RefreshProjectMaster not yet implemented")
}

func (d *Driver) GetProjectMasterRef(projectID string) domain.VolumeRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.masters[projectID]
}
