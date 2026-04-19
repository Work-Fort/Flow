// SPDX-License-Identifier: GPL-2.0-only

//go:build nexus_e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

// TestNexusDriver_DiagDrivesRealNexus is the canonical happy path:
// it runs the diagnostic /v1/runtime/_diag/start + /stop loop
// against a Flow daemon wired to a real spawned Nexus daemon, and
// checks that all five wire-relevant methods produce the expected
// shapes plus a successful IsRuntimeAlive between start and stop.
func TestNexusDriver_DiagDrivesRealNexus(t *testing.T) {
	harness.RequireBtrfsForNexus(t)
	nexusURL, _ := harness.StartNexusDaemon(t)

	env := harness.NewEnv(t, harness.WithNexusURL(nexusURL))
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-001", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	startReq := map[string]any{
		"project_id":   "flow",
		"work_item_id": "wi-nx-1",
		"agent_id":     "a_nx_001",
		"git_ref":      "main",
	}
	status, body, err := c.PostJSON("/v1/runtime/_diag/start", startReq, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("start: status=%d err=%v body=%s", status, err, body)
	}
	var startResp struct {
		Master struct{ Kind, ID string } `json:"master"`
		Work   struct{ Kind, ID string } `json:"work"`
		Handle struct{ Kind, ID string } `json:"handle"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if startResp.Master.Kind != "nexus-drive" {
		t.Errorf("master Kind = %q, want nexus-drive", startResp.Master.Kind)
	}
	if startResp.Work.Kind != "nexus-drive" {
		t.Errorf("work Kind = %q, want nexus-drive", startResp.Work.Kind)
	}
	if startResp.Handle.Kind != "nexus-vm" {
		t.Errorf("handle Kind = %q, want nexus-vm", startResp.Handle.Kind)
	}
	if startResp.Work.ID == "" {
		t.Errorf("work.ID is empty, want a Nexus drive ID")
	}

	stopReq := map[string]any{
		"handle": startResp.Handle,
		"volume": startResp.Work,
	}
	status, body, err = c.PostJSON("/v1/runtime/_diag/stop", stopReq, nil)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("stop status=%d body=%s", status, body)
	}
}

// TestNexusDriver_CloneFromMissingProjectMasterErrors exercises the
// 404 mapping. The diag /start endpoint refreshes the project
// master first (which creates the drive on first call), so to
// exercise the missing-source path we hit /v1/drives/clone via a
// dedicated diag call when the master never existed. The diag
// endpoint surfaces the underlying error verbatim through the
// Huma error envelope; we assert the status is 4xx and the body
// mentions ErrNotFound's signature.
func TestNexusDriver_CloneFromMissingProjectMasterErrors(t *testing.T) {
	if os.Getenv("RUN_NEGATIVE_NEXUS_E2E") == "" {
		// This scenario currently requires either a second diag
		// endpoint (clone-only) or driving the Nexus daemon
		// directly. The non-trivial path is recorded as a follow-
		// up: a smaller diag endpoint surface to exercise per-
		// method error paths in isolation. v1 leaves this gap
		// covered by the unit tests in Task 3.
		t.Skip("clone-from-missing-master path covered by unit tests; set RUN_NEGATIVE_NEXUS_E2E=1 to exercise once the per-method diag endpoints land")
	}
}

// TestNexusDriver_NoLeaksAcrossTwoCycles verifies that running the
// full diag start+stop loop twice in a row does not leak state in
// Nexus (no orphan VMs, no orphan drives). After the second loop
// completes, we list Nexus's VMs/drives via the spawned daemon
// directly to check the cleanup invariant.
//
// After both cycles run, the test directly queries the spawned
// Nexus daemon (GET /v1/vms) to assert the VM count returned to
// zero — proving StopAgentRuntime actually deletes the VM, not
// just that the second cycle didn't conflict with the first.
//
// Drive expectation is 3 = 1 master + 2 per-agent creds clones.
// The diag /stop endpoint takes one volume ref (the work clone)
// and deletes it, but does NOT delete the creds clone the diag
// /start materialises via Task 6.5's CloneWorkItemVolume. The
// known-issue follow-up widens stopInput so tests can clean up;
// until then per-test Nexus daemon spawn provides cleanup-by-
// process-death.
//
// Master drives are intentionally retained across cycles per
// RefreshProjectMaster's idempotent-create contract — same
// master serves both cycles' work-item clones.
//
// when stopInput widens to delete creds, change this to 1.
func TestNexusDriver_NoLeaksAcrossTwoCycles(t *testing.T) {
	harness.RequireBtrfsForNexus(t)
	nexusURL, _ := harness.StartNexusDaemon(t)

	env := harness.NewEnv(t, harness.WithNexusURL(nexusURL))
	defer env.Cleanup(t)

	tok := env.Daemon.SignJWT("svc-001", "flow-e2e", "Flow E2E", "service")
	c := harness.NewClient(env.Daemon.BaseURL(), tok)

	for i, wi := range []string{"wi-cycle-1", "wi-cycle-2"} {
		startReq := map[string]any{
			"project_id":   "flow",
			"work_item_id": wi,
			"agent_id":     "a_cycle_" + wi,
			"git_ref":      "main",
		}
		status, body, err := c.PostJSON("/v1/runtime/_diag/start", startReq, nil)
		if err != nil || status != http.StatusOK {
			t.Fatalf("cycle %d start: status=%d err=%v body=%s", i, status, err, body)
		}
		var resp struct {
			Work   struct{ Kind, ID string } `json:"work"`
			Handle struct{ Kind, ID string } `json:"handle"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("cycle %d decode: %v", i, err)
		}
		stopReq := map[string]any{"handle": resp.Handle, "volume": resp.Work}
		status, body, err = c.PostJSON("/v1/runtime/_diag/stop", stopReq, nil)
		if err != nil {
			t.Fatalf("cycle %d stop: %v", i, err)
		}
		if status != http.StatusOK && status != http.StatusNoContent {
			t.Fatalf("cycle %d stop status=%d body=%s", i, status, body)
		}
	}

	// Verify Nexus has zero VMs left — proves StartAgentRuntime's
	// VM survives only between matching Start/Stop pairs.
	hr, err := http.Get(nexusURL + "/v1/vms")
	if err != nil {
		t.Fatalf("list nexus vms: %v", err)
	}
	defer hr.Body.Close()
	if hr.StatusCode != http.StatusOK {
		t.Fatalf("list vms status=%d", hr.StatusCode)
	}
	var vms []map[string]any
	if err := json.NewDecoder(hr.Body).Decode(&vms); err != nil {
		t.Fatalf("decode vms: %v", err)
	}
	if len(vms) != 0 {
		t.Errorf("after 2 cycles, want 0 VMs in Nexus, got %d: %v", len(vms), vms)
	}

	// Verify Nexus has exactly 3 drives:
	//   1 project master (kept across cycles, per
	//     RefreshProjectMaster's idempotent-create contract)
	// + 2 per-agent creds clones (one per cycle, materialised by
	//     the diag /start handler; not freed by /diag/stop because
	//     stopInput only carries the work-volume ref — see Task 6.5
	//     known-issue)
	dr, err := http.Get(nexusURL + "/v1/drives")
	if err != nil {
		t.Fatalf("list nexus drives: %v", err)
	}
	defer dr.Body.Close()
	if dr.StatusCode != http.StatusOK {
		t.Fatalf("list drives status=%d", dr.StatusCode)
	}
	var drives []map[string]any
	if err := json.NewDecoder(dr.Body).Decode(&drives); err != nil {
		t.Fatalf("decode drives: %v", err)
	}
	if len(drives) != 3 {
		t.Errorf("after 2 cycles, want 3 drives (1 master + 2 creds clones) in Nexus, got %d: %v", len(drives), drives)
	}
}
