// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

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

// nexusName converts an arbitrary string into a valid Nexus drive/VM name:
// only lowercase a-z, 0-9 and dashes; must start and end with a-z0-9;
// max 24 characters. When the sanitized form would exceed 24 chars, the
// first 16 sanitized chars are combined with a 7-char hex suffix derived
// from the full input so that two distinct long names never collide.
func nexusName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) <= 24 {
		return out
	}
	// Preserve uniqueness via a short hash suffix.
	h := sha256.Sum256([]byte(s))
	suffix := fmt.Sprintf("%x", h[:4]) // 8 hex chars
	prefix := strings.Trim(out[:15], "-")
	return prefix + "-" + suffix
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
		Name:  nexusName("agent-" + agentID),
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
		Name:            nexusName("work-item-" + workItemID),
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

// RefreshProjectMaster ensures a project master drive exists in
// Nexus and remembers it for later CloneWorkItemVolume calls.
//
// v1 implementation: idempotent first-time create only. The first
// call POSTs /v1/drives to create a fresh drive named
// "project-master-{projectID}" and records its Nexus drive ID in
// the in-process master map. Subsequent calls are no-ops — the
// gitRef argument is accepted but ignored.
//
// The k8s contract is "launch a one-shot Job that mounts the
// master PVC, runs git pull + warming script, then snapshots the
// PVC." The Nexus equivalent is "ephemeral VM with master drive
// attached, run warming script, snapshot the result." Both are
// non-trivial flows that earn their own plan; v1 of this driver
// covers the minimum needed for the diagnostic happy path so the
// rest of the orchestration can be exercised end-to-end.
//
// TODO(plan: warming) — actual git-pull + warming-script + master-
// drive-snapshot logic.
func (d *Driver) RefreshProjectMaster(ctx context.Context, projectID, _ string) error {
	d.mu.Lock()
	if _, ok := d.masters[projectID]; ok {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	body := struct {
		Name      string `json:"name"`
		Size      string `json:"size"`
		MountPath string `json:"mount_path"`
	}{
		Name: nexusName("project-master-" + projectID),
		// TODO(plan: warming) — expose a per-project size knob.
		// 1Gi is enough for the diag happy path and the typical
		// claude-cli runtime image; the warming flow needs the
		// real source repo size + a build-output budget, which
		// implies a per-project Config entry or a Nexus tag query.
		Size:      "1Gi",
		MountPath: "/work",
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := d.postJSON(ctx, "/v1/drives", body, &resp); err != nil {
		return fmt.Errorf("create master drive for %s: %w", projectID, err)
	}

	d.mu.Lock()
	d.masters[projectID] = domain.VolumeRef{Kind: VolumeKind, ID: resp.ID}
	d.mu.Unlock()
	return nil
}

func (d *Driver) GetProjectMasterRef(projectID string) domain.VolumeRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.masters[projectID]
}
