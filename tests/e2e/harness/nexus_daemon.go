// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// NexusDaemon represents a spawned Nexus daemon subprocess for the
// Flow e2e suite. Lifecycle mirrors flow's own Daemon helper
// (Setpgid + *os.File stderr + WaitDelay + negative-pid SIGTERM).
type NexusDaemon struct {
	cmd        *exec.Cmd
	addr       string
	xdgDir     string
	stderrFile *os.File
}

// RequireNexusBinary returns the path to the Nexus binary, falling
// back to "nexus" on PATH. When neither resolves, calls t.Skip with
// an actionable message. Use this at the top of every Nexus-driven
// e2e test.
func RequireNexusBinary(t testing.TB) string {
	t.Helper()
	var binary string
	if p := os.Getenv("NEXUS_BINARY"); p != "" {
		if _, err := os.Stat(p); err == nil {
			binary = p
		} else {
			t.Skipf("NEXUS_BINARY=%s does not exist; build nexus or unset the env to fall back to PATH", p)
		}
	} else if p, err := exec.LookPath("nexus"); err == nil {
		binary = p
	} else {
		t.Skipf("nexus binary not found on PATH and NEXUS_BINARY unset; build nexus (cd ../../nexus/lead && mise run build) and set NEXUS_BINARY=/path/to/nexus")
	}
	requireNexusCloneDriveEndpoint(t, binary)
	return binary
}

// requireNexusCloneDriveEndpoint spins up a minimal Nexus daemon (no
// btrfs, no caps — just enough to wire the HTTP router) and probes
// POST /v1/drives/clone with an empty body. A 405 response means the
// route does not exist in this binary (pre-CloneDrive); t.Skip is
// called with an actionable message. Any other status (400/422 = bad
// input = route registered) is treated as present. The probe daemon
// uses /tmp for its XDG dirs since we only need the HTTP layer.
func requireNexusCloneDriveEndpoint(t testing.TB, binary string) {
	t.Helper()

	addr, err := freePort()
	if err != nil {
		t.Skipf("probe: free port: %v", err)
	}
	probeDir, err := os.MkdirTemp("", "nexus-probe-*")
	if err != nil {
		t.Skipf("probe: mkdir: %v", err)
	}
	defer os.RemoveAll(probeDir)

	cmd := exec.Command(binary, "daemon",
		"--listen", addr,
		"--namespace", "nexus-clone-probe",
		"--log-level", "disabled",
		"--quota-helper", "",
		"--network-enabled=false",
		"--dns-enabled=false",
	)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(probeDir, "config"),
		"XDG_STATE_HOME="+filepath.Join(probeDir, "state"),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("probe: start nexus: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			cmd.Wait() //nolint:errcheck
		}
	}()

	// Wait for /health. 30s for the same CI parallel-load reason as
	// flow daemon startup — see daemon.go:177.
	deadline := time.Now().Add(30 * time.Second)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get("http://" + addr + "/health"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	resp, err := client.Post("http://"+addr+"/v1/drives/clone", "application/json", strings.NewReader("{}"))
	if err != nil {
		// Daemon not responding — can't determine capability; let test proceed.
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Skipf(
			"Nexus binary at %s lacks POST /v1/drives/clone (got 405) — "+
				"rebuild from nexus/lead at master and set NEXUS_BINARY, "+
				"or run `cd ~/Work/WorkFort/nexus/lead && mise run install:local` "+
				"to update the installed nexus",
			binary,
		)
	}
}

// RequireBtrfsForNexus skips when the working directory is not on
// btrfs. The Nexus daemon's drive subsystem requires a btrfs root.
func RequireBtrfsForNexus(t testing.TB) {
	t.Helper()
	const btrfsSuperMagic = 0x9123683e
	var st syscall.Statfs_t
	if err := syscall.Statfs(".", &st); err != nil {
		t.Skipf("statfs: %v", err)
	}
	if st.Type != btrfsSuperMagic {
		t.Skip("working directory is not on btrfs; mount a btrfs filesystem or run from a btrfs subvolume")
	}
}

// randomNexusNamespace returns a unique containerd namespace for each
// spawned Nexus daemon so that concurrent test runs never share
// containerd objects. Mirrors Nexus's own e2e harness pattern.
func randomNexusNamespace() string {
	b := make([]byte, 4)
	rand.Read(b) //nolint:errcheck // rand.Read never fails on Linux
	return fmt.Sprintf("nexus-e2e-%x", b)
}

// StartNexusDaemon spawns a Nexus daemon configured to listen on a
// free port and use a temp XDG state dir. Returns the base URL and
// a stop closure that the caller MUST defer. Capabilities-dependent
// features (networking, DNS) are NOT enabled — this harness is
// scoped to drive operations only, which the Flow Nexus driver
// exercises in its v1 happy path. Adding network-enabled scenarios
// is a separate plan once the Flow driver itself needs them.
func StartNexusDaemon(t testing.TB) (baseURL string, stop func()) {
	t.Helper()
	binary := RequireNexusBinary(t)

	// Must be on btrfs (for BtrfsStorage) AND use an absolute path (so
	// containerd can bind-mount drives into the container without a relative
	// path that breaks when the shim's cwd differs from ours).
	// os.Getwd() gives an absolute path; the cwd is tests/e2e/ when mise
	// runs e2e:nexus, which is on the project btrfs subvolume.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	xdgDir, err := os.MkdirTemp(cwd, "flow-nexus-e2e-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}

	addr, err := freePort()
	if err != nil {
		os.RemoveAll(xdgDir)
		t.Fatalf("free port: %v", err)
	}

	stderrFile, err := os.CreateTemp("", "flow-nexus-stderr-*")
	if err != nil {
		os.RemoveAll(xdgDir)
		t.Fatalf("create stderr temp: %v", err)
	}

	// Nexus's daemon CLI takes --listen <host:port> (single arg), NOT
	// --bind/--port. Verified at nexus/lead/cmd/daemon.go:362 and used
	// by Nexus's own e2e harness at
	// nexus/lead/tests/e2e/harness/harness.go:150.
	//
	// We disable network/DNS and the quota helper:
	//   --quota-helper ""        — nexus-quota requires CAP_SYS_ADMIN;
	//                              btrfs subvol ops fail without it.
	//                              Mirrors the Nexus e2e harness's own
	//                              opt-out at nexus/lead/tests/e2e/
	//                              nexus_test.go:130.
	//   --network-enabled=false  — Flow driver v1 only exercises
	//                              drive create + VM
	//                              create+start+stop+delete; nothing
	//                              needs CNI/bridge config.
	//   --dns-enabled=false      — same reason; CoreDNS would expect
	//                              nexus-dns helper with CAP_NET_BIND_SERVICE.
	// Keeping these off makes the harness runnable from any directory
	// on a btrfs filesystem without sudo or extra capabilities.
	// Use a unique containerd namespace per spawn so concurrent test
	// invocations and the system nexus daemon never share containers.
	ns := randomNexusNamespace()
	args := []string{
		"daemon",
		"--listen", addr,
		"--namespace", ns,
		"--log-level", "debug",
		"--quota-helper", "",
		"--network-enabled=false",
		"--dns-enabled=false",
	}
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(xdgDir, "config"),
		"XDG_STATE_HOME="+filepath.Join(xdgDir, "state"),
	)
	cmd.Stdout = stderrFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		stderrFile.Close()
		os.Remove(stderrFile.Name())
		os.RemoveAll(xdgDir)
		t.Fatalf("start nexus: %v", err)
	}

	// Wait for /health to respond. 30s for the same CI parallel-load
	// reason as flow daemon startup — see daemon.go:177.
	deadline := time.Now().Add(30 * time.Second)
	healthURL := "http://" + addr + "/health"
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 200 * time.Millisecond}
		if resp, err := client.Get(healthURL); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	d := &NexusDaemon{cmd: cmd, addr: addr, xdgDir: xdgDir, stderrFile: stderrFile}
	stop = func() { d.stop(t) }
	t.Cleanup(stop)
	return "http://" + addr, stop
}

// stop sends SIGTERM to the process group, waits up to 5s, then
// SIGKILLs. Dumps captured stderr to t.Logf if the test failed.
func (d *NexusDaemon) stop(t testing.TB) {
	t.Helper()
	if d.cmd == nil || d.cmd.Process == nil {
		return
	}
	pgid := d.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- d.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
	}
	d.cmd = nil

	var stderrBytes []byte
	if d.stderrFile != nil {
		stderrBytes, _ = os.ReadFile(d.stderrFile.Name())
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
		d.stderrFile = nil
	}
	// Nexus writes structured logs to XDG_STATE_HOME/nexus/debug.log, not stderr.
	// Always include them when the test fails so the error context is visible.
	if t.Failed() {
		if logBytes, err := os.ReadFile(filepath.Join(d.xdgDir, "state", "nexus", "debug.log")); err == nil && len(logBytes) > 0 {
			t.Logf("nexus debug.log:\n%s", logBytes)
		}
	}
	if t.Failed() && len(stderrBytes) > 0 {
		t.Logf("nexus stderr:\n%s", stderrBytes)
	}
	if bytes.Contains(stderrBytes, []byte("DATA RACE")) {
		t.Fatal("data race in nexus daemon (see stderr above)")
	}
	os.RemoveAll(d.xdgDir)
}
