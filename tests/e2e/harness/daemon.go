// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"bytes"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// daemonCfg captures the per-spawn configuration. Tests build it via
// DaemonOption helpers.
type daemonCfg struct {
	pylonAddr      string
	passportAddr   string
	webhookBaseURL string
	dbDSN          string
	stubRuntime    bool
}

type DaemonOption func(*daemonCfg)

func WithWebhookBaseURL(u string) DaemonOption {
	return func(c *daemonCfg) { c.webhookBaseURL = u }
}

func WithDB(dsn string) DaemonOption {
	return func(c *daemonCfg) { c.dbDSN = dsn }
}

func WithStubRuntime() DaemonOption {
	return func(c *daemonCfg) { c.stubRuntime = true }
}

// Daemon represents a spawned flow daemon subprocess.
type Daemon struct {
	cmd        *exec.Cmd
	addr       string
	xdgDir     string
	stderrFile *os.File // temp file backing stdout+stderr; read at Stop time
	signJWT    func(id, username, displayName, userType string) string
	stops      []func()
}

// StartDaemon spawns a flow daemon subprocess wired to in-process fakes.
// pylonAddr and passportAddr are host:port pairs returned by the stubs.
// signJWT is the closure returned by StartJWKSStub — re-used so tests can
// mint JWTs that validate against the JWKS the daemon fetched at startup.
func StartDaemon(
	t testing.TB,
	binary, pylonAddr, passportAddr string,
	signJWT func(id, username, displayName, userType string) string,
	opts ...DaemonOption,
) (*Daemon, error) {
	t.Helper()

	cfg := &daemonCfg{
		pylonAddr:    pylonAddr,
		passportAddr: passportAddr,
	}
	for _, o := range opts {
		o(cfg)
	}

	xdgDir, err := os.MkdirTemp("", "flow-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	addr, err := freePort()
	if err != nil {
		os.RemoveAll(xdgDir)
		return nil, err
	}
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	args := []string{
		"daemon",
		"--bind", host,
		"--port", strconv.Itoa(port),
		"--passport-url", "http://" + passportAddr,
		"--pylon-url", "http://" + pylonAddr,
		"--service-token", "harness-service-token",
	}
	if cfg.webhookBaseURL != "" {
		args = append(args, "--webhook-base-url", cfg.webhookBaseURL)
	}

	dsn := cfg.dbDSN
	if dsn == "" {
		dsn = os.Getenv("FLOW_DB")
	}
	if dsn != "" {
		args = append(args, "--db", dsn)
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			if err := resetPostgres(dsn); err != nil {
				os.RemoveAll(xdgDir)
				return nil, fmt.Errorf("reset postgres: %w", err)
			}
		}
	}

	stderrFile, err := os.CreateTemp("", "flow-e2e-stderr-*")
	if err != nil {
		os.RemoveAll(xdgDir)
		return nil, fmt.Errorf("create stderr temp file: %w", err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+xdgDir+"/config",
		"XDG_STATE_HOME="+xdgDir+"/state",
		// Renewer cadence overrides so the agent_pool e2e test observes
		// a renew within its 2-second poll window.
		"FLOW_LEASE_RENEWER_INTERVAL=100ms",
		"FLOW_LEASE_TTL=2s",
	)
	if cfg.stubRuntime {
		cmd.Env = append(cmd.Env, "FLOW_E2E_RUNTIME_STUB=1")
	}
	// *os.File (not io.Writer) so exec.Cmd does not create a copy
	// goroutine; Setpgid puts the daemon and any descendants in a
	// fresh process group; WaitDelay force-closes any inherited fds
	// after the daemon exits. See the orphan-process hardening
	// section of go-service-architecture.
	cmd.Stdout = stderrFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		stderrFile.Close()
		os.Remove(stderrFile.Name())
		os.RemoveAll(xdgDir)
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	d := &Daemon{
		cmd: cmd, addr: addr, xdgDir: xdgDir,
		stderrFile: stderrFile, signJWT: signJWT,
	}

	if err := waitReady(addr, 5*time.Second); err != nil {
		d.kill()
		return nil, err
	}
	return d, nil
}

func (d *Daemon) Addr() string    { return d.addr }
func (d *Daemon) BaseURL() string { return "http://" + d.addr }

// SignJWT mints a 1-hour token signed by the JWKS stub's key.
func (d *Daemon) SignJWT(id, username, displayName, userType string) string {
	return d.signJWT(id, username, displayName, userType)
}

// Stop sends SIGTERM to the daemon's process group, waits up to 5s,
// then SIGKILLs the group. Cleans tempdir. Fails the test if the
// daemon emitted a DATA RACE marker on stderr. On test failure, dumps
// the captured stderr to t.Logf so a daemon panic, fatal log line,
// or context-cancel chain explains the failure even after stderr
// scrolled off the live tty.
func (d *Daemon) Stop(t testing.TB) {
	t.Helper()
	if d.cmd.Process != nil {
		// pgid == pid because of Setpgid; signal the whole group so
		// any leaked descendants die with the daemon.
		pgid := d.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- d.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("daemon did not exit after SIGTERM, killing group")
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
	}

	var stderrBytes []byte
	if d.stderrFile != nil {
		// Read the captured output before unlinking.
		stderrBytes, _ = os.ReadFile(d.stderrFile.Name())
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
	}

	if t.Failed() && len(stderrBytes) > 0 {
		t.Logf("daemon stderr:\n%s", stderrBytes)
	}
	os.RemoveAll(d.xdgDir)
	if bytes.Contains(stderrBytes, []byte("DATA RACE")) {
		t.Fatal("data race detected in daemon (see stderr above)")
	}
}

func (d *Daemon) kill() {
	if d.cmd.Process != nil {
		pgid := d.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		d.cmd.Wait()
	}
	if d.stderrFile != nil {
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
	}
	os.RemoveAll(d.xdgDir)
}

// freePort returns 127.0.0.1:N for a currently-free N.
func freePort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr, nil
}

// waitReady polls /v1/health until it returns 200, 218 or 503 (any health
// reply means the listener is up), or until deadline.
func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := "http://" + addr + "/v1/health"
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready on %s within %s", addr, timeout)
}

// resetPostgres drops and recreates the public schema. Goose migrations
// re-run on next daemon startup. Mirrors sharkfin/lead/tests/e2e harness.
func resetPostgres(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP SCHEMA public CASCADE"); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	if _, err := db.Exec("CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}
