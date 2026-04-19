// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestDaemonStop_KillsProcessGroup(t *testing.T) {
	if os.Getenv("FLOW_BINARY") == "" {
		// NewEnv falls back to ../../build/flow when FLOW_BINARY is
		// unset; the leak test still needs a real binary to spawn.
		if _, err := os.Stat("../../build/flow"); err != nil {
			t.Skip("FLOW_BINARY not set and ../../build/flow missing; run via 'mise run e2e'")
		}
	}

	// NewEnv wires the JWKS stub, Pylon stub, fake Hive, fake Sharkfin
	// and spawns the daemon. We use it instead of calling StartDaemon
	// directly so the leak test exercises the same code path as every
	// other e2e test.
	env := NewEnv(t)

	pid := env.Daemon.cmd.Process.Pid

	// pgid must equal pid because StartDaemon sets Setpgid.
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Fatalf("daemon pgid = %d, want %d (Setpgid not set)", pgid, pid)
	}
	// Defence against the (vanishingly rare) case where the test
	// process itself happens to be in a group whose id equals the
	// daemon PID — that would let pgid == pid pass spuriously.
	if pgid == os.Getpid() {
		t.Fatalf("daemon pgid (%d) equals harness pid; daemon inherited harness group", pgid)
	}

	env.Cleanup(t) // tears down daemon + stubs in reverse order

	// After Cleanup, signalling the group with sig 0 must report no
	// such process — the canonical "is the group empty?" check.
	// Use errors.Is (not direct ==) because syscall.Errno implements
	// the errors.Is contract and errors.Is is the idiomatic choice.
	err = syscall.Kill(-pgid, 0)
	if !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill(-%d, 0) = %v, want ESRCH (group still has live members)", pgid, err)
	}
}
