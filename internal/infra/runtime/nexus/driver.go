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

// StartAgentRuntime creates a fresh Nexus VM, attaches the creds
// and work drives, and starts the VM. The returned RuntimeHandle
// carries the Nexus VM ID. On any failure after VM creation, the
// VM is deleted before returning the error so no orphan VM is left
// in Nexus's pool.
//
// v1: a fresh VM per call. Pool reuse (tag=pool=claude-cli) is a
// follow-up plan.
func (d *Driver) StartAgentRuntime(ctx context.Context, agentID string, creds, work domain.VolumeRef) (domain.RuntimeHandle, error) {
	if creds.Kind != VolumeKind {
		return domain.RuntimeHandle{}, fmt.Errorf("creds.Kind=%q: %w", creds.Kind, ErrUnsupportedKind)
	}
	if work.Kind != VolumeKind {
		return domain.RuntimeHandle{}, fmt.Errorf("work.Kind=%q: %w", work.Kind, ErrUnsupportedKind)
	}

	createBody := struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}{
		Name:  "agent-" + agentID,
		Image: d.cfg.VMImage,
	}
	var vm struct {
		ID string `json:"id"`
	}
	if err := d.postJSON(ctx, "/v1/vms", createBody, &vm); err != nil {
		return domain.RuntimeHandle{}, fmt.Errorf("create vm: %w", err)
	}

	// From here on, any failure must clean up the VM to avoid leaking
	// a partly-configured pool member. A delete failure on the
	// cleanup path is logged in the wrapped error but does NOT
	// override the original failure cause.
	cleanupOnErr := func(origErr error) error {
		if delErr := d.delete(context.Background(), "/v1/vms/"+vm.ID); delErr != nil &&
			!errors.Is(delErr, domain.ErrNotFound) {
			return fmt.Errorf("%w (cleanup of %s also failed: %v)", origErr, vm.ID, delErr)
		}
		return origErr
	}

	attachBody := struct {
		VMID string `json:"vm_id"`
	}{VMID: vm.ID}
	if err := d.postJSON(ctx, "/v1/drives/"+creds.ID+"/attach", attachBody, nil); err != nil {
		return domain.RuntimeHandle{}, cleanupOnErr(fmt.Errorf("attach creds: %w", err))
	}
	if err := d.postJSON(ctx, "/v1/drives/"+work.ID+"/attach", attachBody, nil); err != nil {
		return domain.RuntimeHandle{}, cleanupOnErr(fmt.Errorf("attach work: %w", err))
	}
	if err := d.postJSON(ctx, "/v1/vms/"+vm.ID+"/start", nil, nil); err != nil {
		return domain.RuntimeHandle{}, cleanupOnErr(fmt.Errorf("start vm: %w", err))
	}

	return domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: vm.ID}, nil
}

// StopAgentRuntime stops the VM and then deletes it. Both calls are
// best-effort against an already-stopped or already-deleted VM:
// 404 and 409 from the stop call do not abort the delete; 404 from
// the delete call is treated as success (idempotent).
func (d *Driver) StopAgentRuntime(ctx context.Context, h domain.RuntimeHandle) error {
	if h.Kind == "" && h.ID == "" {
		return nil
	}
	if h.Kind != RuntimeHandleKind {
		return fmt.Errorf("h.Kind=%q: %w", h.Kind, ErrUnsupportedKind)
	}
	if err := d.postJSON(ctx, "/v1/vms/"+h.ID+"/stop", nil, nil); err != nil {
		// 404 (gone) and 409 (already stopped) are non-fatal — proceed
		// to delete. Anything else fails fast.
		if !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrInvalidState) {
			return err
		}
	}
	if err := d.delete(ctx, "/v1/vms/"+h.ID); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

// IsRuntimeAlive reports true when the VM's state is "running". A
// 404 (VM gone) is reported as (false, nil) — alive=false is the
// correct answer for a missing runtime. The driver caps internal
// network timeouts at the smaller of ctx's deadline and 2s per the
// port contract; the cap is enforced via http.Client.Timeout in
// New() (default 30s) plus the caller's ctx deadline.
func (d *Driver) IsRuntimeAlive(ctx context.Context, h domain.RuntimeHandle) (bool, error) {
	if h.Kind != RuntimeHandleKind {
		return false, fmt.Errorf("h.Kind=%q: %w", h.Kind, ErrUnsupportedKind)
	}
	// Cap at 2s OR the caller's ctx deadline, whichever is sooner.
	// context.WithTimeout returns a context whose deadline is the
	// EARLIER of (now+2s, parent.Deadline()), so passing 2s here
	// naturally inherits a tighter caller budget when the parent
	// already has one — confirmed by
	// TestIsRuntimeAlive_RespectsContextDeadline (50ms parent
	// deadline wins over the 2s cap).
	subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var vm struct {
		State string `json:"state"`
	}
	if err := d.getJSON(subCtx, "/v1/vms/"+h.ID, &vm); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return vm.State == "running", nil
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
