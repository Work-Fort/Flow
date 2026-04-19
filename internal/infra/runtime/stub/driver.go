// SPDX-License-Identifier: GPL-2.0-only

// Package stub provides a test-only RuntimeDriver implementation. It
// records every call so tests can assert on the sequence and returns
// deterministic VolumeRef / RuntimeHandle values. Used by the e2e
// harness's runtime diagnostic endpoint and by scheduler unit tests.
// Never used in production builds.
package stub

import (
	"context"
	"fmt"
	"sync"

	"github.com/Work-Fort/Flow/internal/domain"
)

// Driver is a test double for domain.RuntimeDriver. Safe for
// concurrent use.
type Driver struct {
	mu      sync.Mutex
	calls   []string
	started map[string]bool
	masters map[string]domain.VolumeRef
	nextID  int
}

// New returns an empty Driver.
func New() *Driver {
	return &Driver{
		started: make(map[string]bool),
		masters: make(map[string]domain.VolumeRef),
	}
}

// Calls returns a copy of the recorded call log.
func (d *Driver) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.calls))
	copy(out, d.calls)
	return out
}

// Reset clears the call log.
func (d *Driver) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = nil
	d.started = make(map[string]bool)
	d.masters = make(map[string]domain.VolumeRef)
	d.nextID = 0
}

func (d *Driver) record(s string) {
	d.calls = append(d.calls, s)
}

func (d *Driver) nextRef(kind string) string {
	d.nextID++
	return fmt.Sprintf("%s-%d", kind, d.nextID)
}

// StartAgentRuntime implements domain.RuntimeDriver.
func (d *Driver) StartAgentRuntime(_ context.Context, agentID string, _ domain.VolumeRef, _ domain.VolumeRef) (domain.RuntimeHandle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("StartAgentRuntime:" + agentID)
	h := domain.RuntimeHandle{Kind: "stub", ID: d.nextRef("rt")}
	d.started[h.ID] = true
	return h, nil
}

// StopAgentRuntime implements domain.RuntimeDriver.
func (d *Driver) StopAgentRuntime(_ context.Context, h domain.RuntimeHandle) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("StopAgentRuntime")
	d.started[h.ID] = false
	return nil
}

// IsRuntimeAlive implements domain.RuntimeDriver.
func (d *Driver) IsRuntimeAlive(_ context.Context, h domain.RuntimeHandle) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("IsRuntimeAlive")
	return d.started[h.ID], nil
}

// CloneWorkItemVolume implements domain.RuntimeDriver.
func (d *Driver) CloneWorkItemVolume(_ context.Context, master domain.VolumeRef, workItemID string) (domain.VolumeRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	project := "unknown"
	for p, m := range d.masters {
		if m == master {
			project = p
			break
		}
	}
	d.record("CloneWorkItemVolume:" + project + ":" + workItemID)
	return domain.VolumeRef{Kind: "stub", ID: d.nextRef("work")}, nil
}

// DeleteVolume implements domain.RuntimeDriver.
func (d *Driver) DeleteVolume(_ context.Context, _ domain.VolumeRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("DeleteVolume")
	return nil
}

// RefreshProjectMaster implements domain.RuntimeDriver.
func (d *Driver) RefreshProjectMaster(_ context.Context, projectID string, gitRef string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record("RefreshProjectMaster:" + projectID + ":" + gitRef)
	if _, ok := d.masters[projectID]; !ok {
		d.masters[projectID] = domain.VolumeRef{Kind: "stub", ID: d.nextRef("master")}
	}
	return nil
}

// GetProjectMasterRef implements domain.RuntimeDriver.
func (d *Driver) GetProjectMasterRef(projectID string) domain.VolumeRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.masters[projectID]
}
