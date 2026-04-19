// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"context"
	"errors"
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

func (d *Driver) CloneWorkItemVolume(_ context.Context, _ domain.VolumeRef, _ string) (domain.VolumeRef, error) {
	return domain.VolumeRef{}, errors.New("nexus driver: CloneWorkItemVolume not yet implemented")
}

func (d *Driver) DeleteVolume(_ context.Context, _ domain.VolumeRef) error {
	return errors.New("nexus driver: DeleteVolume not yet implemented")
}

func (d *Driver) RefreshProjectMaster(_ context.Context, _ string, _ string) error {
	return errors.New("nexus driver: RefreshProjectMaster not yet implemented")
}

func (d *Driver) GetProjectMasterRef(projectID string) domain.VolumeRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.masters[projectID]
}
