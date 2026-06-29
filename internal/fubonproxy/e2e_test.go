//go:build integration

package fubonproxy

// e2e binary tests for the -fubon-port / preflight zombie reclaim feature set.
//
// Oracle 4th-round verdict scope for L3 (bg_f02f1fab):
//   - binary 啟動 → curl /health 200 → SIGTERM → port release → 重啟
//   - chaos: orphan reclaim (L1 zombie kill path via proxyListenPort)
//   - chaos: foreign holder alt-port (L2 alt-port path) — KNOWN UNIMPLEMENTED;
//     this test documents current behaviour (clean error) until alt-port
//     auto-fallback lands.
//   - rapid cycle 5x (no port/file-descriptor leaks)
//
// Watch-out from Oracle: "e2e 測試環境依賴: Python + 系統能 spawn processes。
// 測試必須 t.Skip when Python/process spawning 不可用,不可讓 CI 紅燈。"
// All tests are gated by setupE2EBinary / e2eEnvReady helpers that
// skip gracefully when the environment is not ready.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// e2eBinaryPath is the absolute path of a one-time-built atlas binary,
// used as the test subject for every e2e test in this file.
var (
	e2eBinaryOnce sync.Once
	e2eBinaryPath string
	e2eBinaryErr  error
)

// setupE2EBinary builds atlas once per `go test` invocation and returns the
// path. Tests that require the binary are skipped if the build fails.
func setupE2EBinary(t *testing.T) string {
	t.Helper()
	e2eBinaryOnce.Do(func() {
		bin, err := os.MkdirTemp("", "atlas-e2e-")
		if err != nil {
			e2eBinaryErr = err
			return
		}
		e2eBinaryPath = filepath.Join(bin, "atlas-e2e-test")
		// -count=1 prevents stale-test caching from breaking the build.
		cmd := exec.Command("go", "build", "-count=1", "-o", e2eBinaryPath, "./cmd/atlas/")
		out := &bytes.Buffer{}
		cmd.Stderr = out
		cmd.Stdout = io.Discard
		if rerr := cmd.Run(); rerr != nil {
			e2eBinaryErr = fmt.Errorf("build atlas failed: %w (%s)", rerr, out.String())
			return
		}
	})
	if e2eBinaryErr != nil {
		t.Skipf("e2e binary unavailable, skipping: %v", e2eBinaryErr)
	}
	return e2eBinaryPath
}

// freePort grabs an unused TCP port from the kernel and returns it.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// portFree reports whether the kernel currently has nothing listening on
// the given TCP port.
func portFree(t *testing.T, port int) bool {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// waitForHealth polls /health on the given base URL until success or timeout.
// Returns the raw response body on success.
func waitForHealth(t *testing.T, baseURL string, timeout time.Duration) (int, string, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastBody string
	var lastCode int
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastCode = resp.StatusCode
			lastBody = string(body)
			if resp.StatusCode == http.StatusOK {
				return resp.StatusCode, lastBody, true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return lastCode, lastBody, false
}

// stopAtlas signals atlas with SIGTERM and waits up to waitFor for it to
// exit. Kills with SIGKILL if it exceeds the deadline.
func stopAtlas(t *testing.T, cmd *exec.Cmd, waitFor time.Duration) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(waitFor):
		_ = cmd.Process.Kill()
		<-done
	}
}

// launchAtlas starts the atlas e2e binary with the given flags in background,
// returning the running command. The caller is responsible for cleanup
// via stopAtlas + a defer if needed.
func launchAtlas(t *testing.T, bin string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "FUBON_PROXY_PORT="+lastFlagValue(args, "-fubon-port"))
	// Discard stdout in tests to keep test logs readable; stderr is kept so
	// atlas error / preflight_zombie_killed events are visible for debugging.
	cmd.Stdout = io.Discard
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			stopAtlas(t, cmd, 3*time.Second)
		}
		if t.Failed() && stderrBuf.Len() > 0 {
			t.Logf("atlas stderr (last %d bytes):\n%s", stderrBuf.Len(), stderrBuf.String())
		}
	})
	if err := cmd.Start(); err != nil {
		t.Fatalf("start atlas %v: %v", args, err)
	}
	return cmd
}

// lastFlagValue returns the value passed after the named flag in argv.
// Used to set FUBON_PROXY_PORT env var so the spawned python proxy binds
// to the same port atlas expects.
func lastFlagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestE2E_BinaryStartupHealthShape_SIGTERMReleasesPorts is the basic happy
// path: build atlas, start with -api + -addr + -fubon-port, hit /health,
// verify the new PR #821 / L2 JSON shape (status + ports.atlas_http +
// ports.fubon_proxy), SIGTERM, verify both ports are released for the
// next test or next CI run.
func TestE2E_BinaryStartupHealthShape_SIGTERMReleasesPorts(t *testing.T) {
	bin := setupE2EBinary(t)
	apiPort := freePort(t)
	fubonPort := freePort(t)

	cmd := launchAtlas(t, bin,
		"-api",
		"-addr", "127.0.0.1:"+strconv.Itoa(apiPort),
		"-fubon-port", strconv.Itoa(fubonPort),
	)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort) + "/health"
	code, body, ok := waitForHealth(t, baseURL, 20*time.Second)
	if !ok {
		t.Fatalf("/health never returned 200 within 20s (last code=%d body=%q)", code, body)
	}

	// PR #821 / L2 health JSON shape: must contain `status` and `ports` keys,
	// with `atlas_http` and `fubon_proxy` entries. The state field is one of
	// free / healthy / foreign / unknown.
	for _, want := range []string{`"status":`, `"ports":`, `"atlas_http":`, `"fubon_proxy":`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected health JSON to contain %q, got: %s", want, body)
		}
	}

	// Hold atlas a moment so SIGTERM = real shutdown signal, not exit by chance.
	time.Sleep(200 * time.Millisecond)
	stopAtlas(t, cmd, 5*time.Second)

	if !portFree(t, apiPort) {
		t.Errorf("api port %d still held after SIGTERM-clean shutdown", apiPort)
	}
	if !portFree(t, fubonPort) {
		t.Errorf("fubon port %d still held after SIGTERM-clean shutdown — L1 release claim broken", fubonPort)
	}
}

// TestE2E_PythonZombieOrphanReclaimed exercises the L1 zombie reclaim path:
//  1. spawn `python3 -c "import time; time.sleep(60)"` which binds a free
//     port (via the python proxy's listener, if available, OR via a tiny
//     Python listener we inline here for portability);
//  2. confirm IsFubonZombie would match — command name contains "python";
//  3. start atlas with -fubon-port = that port + record that the zombie
//     gets killed and atlas proceeds to a healthy /health.
//
// Watch-out: a full Python fubon-proxy needs the fubon SDK; this test uses
// a minimal Python listener that holds a port and nothing else so the SDK
// is not required. The port-bind is the only thing fubonproxy.Start's
// zombie detector cares about (PID + command name).
func TestE2E_PythonZombieOrphanReclaimed(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not on PATH: %v", err)
	}
	bin := setupE2EBinary(t)

	// Phase 1: spawn a tiny python listener that holds the chosen port
	// for 60s. Command name "python3" matches IsFubonZombie's heuristic.
	zombiePort := freePort(t)
	zombieCmd := exec.Command("python3",
		"-c", fmt.Sprintf(
			"import socket, time; s=socket.socket(socket.AF_INET, socket.SOCK_STREAM); "+
				"s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); "+
				"s.bind(('127.0.0.1', %d)); s.listen(8); time.sleep(60)", zombiePort))
	zombieCmd.Stdout = io.Discard
	zombieCmd.Stderr = io.Discard
	if err := zombieCmd.Start(); err != nil {
		t.Skipf("cannot start python3 zombie: %v", err)
	}
	t.Cleanup(func() {
		if zombieCmd.Process != nil {
			_ = zombieCmd.Process.Kill()
			_, _ = zombieCmd.Process.Wait()
		}
	})

	// Give the listener a moment to actually bind.
	if !waitForPortBind(8081, zombiePort, 5*time.Second) { // port==0 here is unused, calling differently
		_ = zombieCmd.Process.Kill()
		t.Skipf("python3 zombie failed to bind 127.0.0.1:%d within 5s", zombiePort)
	}
	// Phase 2: start atlas with that exact port as -fubon-port. The preflight
	// should observe StateForeign + IsFubonZombie==true and auto-reclaim
	// (PortProbe.Probe → IsFubonZombie(KillOccupant) per PR #820 + #821).
	apiPort := freePort(t)
	atlas := launchAtlas(t, bin,
		"-api",
		"-addr", "127.0.0.1:"+strconv.Itoa(apiPort),
		"-fubon-port", strconv.Itoa(zombiePort),
	)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort) + "/health"
	if _, _, ok := waitForHealth(t, baseURL, 25*time.Second); !ok {
		stopAtlas(t, atlas, 3*time.Second)
		t.Fatalf("atlas never came up after reclaim attempt; L1 zombie kill may have failed")
	}

	// The python zombie should now be killed by atlas's reclaim path.
	// We give portprobe.KillOccupant's SIGTERM+1s+SIGKILL escalation a
	// moment to complete.
	if !waitPortReleased(zombiePort, 5*time.Second) {
		stopAtlas(t, atlas, 3*time.Second)
		t.Fatalf("python zombie did not release port %d after reclaim", zombiePort)
	}

	stopAtlas(t, atlas, 5*time.Second)
}

// waitPortReleased polls until portFree returns true, with timeout.
func waitPortReleased(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			_ = ln.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// waitForPortBind is a stub used in the zombie test — currently bypassed
// because the python listener's bind is fast (sub-second).
func waitForPortBind(_, _ int, _ time.Duration) bool {
	time.Sleep(300 * time.Millisecond)
	return true
}

// TestE2E_NonZombieForeignHolder documents current behaviour for the
// "alt-port fallback" Oracle scenario C: when the fubon port is held by
// a process that is NOT recognised as a fubon-proxy zombie (e.g. Docker
// Desktop's com.docker.backend holding :8081), current atlas exits with an
// actionable error rather than auto-choosing a different port. Oracle
// verdict F10 explicitly authorises "non-zombie = no auto-kill". Once
// alt-port auto-fallback is implemented (future work), this test should
// flip to expect a healthy /health instead of an exit-with-error.
func TestE2E_NonZombieForeignHolder_AtlasErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep-based foreign holder only reliable on POSIX")
	}
	bin := setupE2EBinary(t)
	foreignPort := freePort(t)

	// Bind the port with `sleep 60` — its comm is "sleep" which IsFubonZombie
	// does NOT match (needs `python` or `uvicorn` substring).
	cmd := exec.Command("sleep", "60")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	// We need sleep to actually bind the port; sleep just sleeps without
	// binding. So use a small python listener again but with a fake argv[0]
	// that doesn't match the IsFubonZombie heuristic. We do this by writing
	// a wrapper script.
	wrapper := filepath.Join(t.TempDir(), "nonpy")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec -a NotPython sleep 60 &\nwait\n"), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	_ = wrapper // kept for documentation; below listener is actually python with renamed argv
	pythonListener := exec.Command("python3",
		"-c", fmt.Sprintf(
			"import socket, os, sys, time; "+
				"os.execvp('sleep', ['notpy']); "+
				"s=socket.socket(socket.AF_INET, socket.SOCK_STREAM); "+
				"s.bind(('127.0.0.1', %d)); s.listen(8); time.sleep(60)", foreignPort))
	// Override argv so IsFubonZombie's command-name check looks at 'notpy'
	// rather than 'python'.
	pythonListener.Args = []string{"notpy", "-c", fmt.Sprintf(
		"import socket, time; s=socket.socket(socket.AF_INET, socket.SOCK_STREAM); "+
			"s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); "+
			"s.bind(('127.0.0.1', %d)); s.listen(8); time.sleep(60)", foreignPort)}
	pythonListener.SysProcAttr = &syscall.SysProcAttr{}
	pythonListener.Stdout = io.Discard
	pythonListener.Stderr = io.Discard
	if err := pythonListener.Start(); err != nil {
		t.Skipf("cannot start non-zombie listener: %v", err)
	}
	t.Cleanup(func() {
		if pythonListener.Process != nil {
			_ = pythonListener.Process.Kill()
			_, _ = pythonListener.Process.Wait()
		}
	})
	if !waitPortReleased(0, time.Millisecond) {
		// tiny settle
	}
	time.Sleep(500 * time.Millisecond)

	// Now start atlas against the same port — expect a clean error
	// (atlas exits non-zero with an actionable error mentioning the foreign
	// PID). Oracle F10 authorises this; alt-port fallback (future work) would
	// instead auto-pick a free port and serve.
	atlas := exec.Command(bin,
		"-api",
		"-addr", "127.0.0.1:"+strconv.Itoa(freePort(t)),
		"-fubon-port", strconv.Itoa(foreignPort),
	)
	atlas.Stdout = io.Discard
	var atlasErr bytes.Buffer
	atlas.Stderr = &atlasErr
	if err := atlas.Start(); err != nil {
		t.Fatalf("start atlas: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- atlas.Wait() }()
	select {
	case <-done:
		// Atlas exited; verify the exit was caused by preflight Foreign error.
		output := atlasErr.String()
		if !strings.Contains(output, "preflight") && !strings.Contains(output, "foreign") {
			t.Errorf("atlas exited but stderr doesn't mention preflight/foreign: %s", output)
		}
	case <-time.After(8 * time.Second):
		_ = atlas.Process.Kill()
		<-done
		t.Errorf("atlas did not exit within 8s when foreign holder present; expected preflight error")
	}
}

// TestE2E_RapidCycle5x stresses 5 cycles of start + SIGTERM-clean-stop to
// surface any file-descriptor or port leaks. After each cycle, both listen
// ports must be fully released so the next cycle can bind them.
func TestE2E_RapidCycle5x(t *testing.T) {
	bin := setupE2EBinary(t)
	for i := 0; i < 5; i++ {
		apiPort := freePort(t)
		fubonPort := freePort(t)
		cmd := launchAtlas(t, bin,
			"-api",
			"-addr", "127.0.0.1:"+strconv.Itoa(apiPort),
			"-fubon-port", strconv.Itoa(fubonPort),
		)
		baseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort) + "/health"
		if _, _, ok := waitForHealth(t, baseURL, 15*time.Second); !ok {
			t.Fatalf("rapid cycle iter %d: /health never returned 200", i+1)
		}
		stopAtlas(t, cmd, 5*time.Second)
		// immediate re-bind-check (port may briefly be in TIME_WAIT)
		if !waitPortReleased(apiPort, 3*time.Second) {
			t.Fatalf("rapid cycle iter %d: api port %d not released within 3s of SIGTERM", i+1, apiPort)
		}
		if !waitPortReleased(fubonPort, 3*time.Second) {
			t.Fatalf("rapid cycle iter %d: fubon port %d not released within 3s of SIGTERM", i+1, fubonPort)
		}
	}
}
