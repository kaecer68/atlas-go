package portprobe

import (
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on random port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func occupyTCP(t *testing.T, addr string, handler http.Handler) func() {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port %q: %v", addr, err)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:"+portStr)
	if err != nil {
		t.Fatalf("listen on 0.0.0.0:%s: %v", portStr, err)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

func TestProbe_FreePort(t *testing.T) {
	addr := freeLocalAddr(t)
	// The listener is closed by t.Cleanup after this function returns, so the
	// port should be free when Probe binds it.

	state, occ, err := Probe(addr)
	if err != nil {
		t.Fatalf("Probe(%q): unexpected error: %v", addr, err)
	}
	if state != StateFree {
		t.Errorf("Probe(%q) state = %v, want StateFree", addr, state)
	}
	if occ.PID != 0 || occ.Command != "" {
		t.Errorf("Probe(%q) occupant = %+v, want empty", addr, occ)
	}
}

func TestProbe_HealthyOccupant(t *testing.T) {
	addr := freeLocalAddr(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	cleanup := occupyTCP(t, addr, handler)
	defer cleanup()

	state, occ, err := Probe(addr)
	if err != nil {
		t.Fatalf("Probe(%q): unexpected error: %v", addr, err)
	}
	if state != StateHealthy {
		t.Errorf("Probe(%q) state = %v, want StateHealthy", addr, state)
	}
	if occ.PID == 0 {
		t.Errorf("Probe(%q) occupant PID = 0, want non-zero", addr)
	}
}

func TestProbe_ForeignOccupant(t *testing.T) {
	addr := freeLocalAddr(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	cleanup := occupyTCP(t, addr, handler)
	defer cleanup()

	state, occ, err := Probe(addr)
	if err != nil {
		t.Fatalf("Probe(%q): unexpected error: %v", addr, err)
	}
	if state != StateForeign {
		t.Errorf("Probe(%q) state = %v, want StateForeign", addr, state)
	}
	if occ.PID == 0 {
		t.Errorf("Probe(%q) occupant PID = 0, want non-zero", addr)
	}
}

func TestProbe_ProbeErrorOnInvalidAddr(t *testing.T) {
	_, _, err := Probe("not-an-address")
	if err == nil {
		t.Fatal("Probe(not-an-address): expected error, got nil")
	}
}

func TestLookupOccupant_ResolvesListener(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skipf("lsof not available: %v", err)
	}

	addr := freeLocalAddr(t)
	cleanup := occupyTCP(t, addr, http.NotFoundHandler())
	defer cleanup()

	occ, err := LookupOccupant(addr)
	if err != nil {
		t.Fatalf("LookupOccupant(%q): %v", addr, err)
	}
	if occ.PID == 0 {
		t.Errorf("LookupOccupant(%q) PID = 0", addr)
	}
	if occ.Command == "" {
		t.Errorf("LookupOccupant(%q) Command = empty", addr)
	}
}

func TestIsFubonZombie(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"python", "python", true},
		{"python3", "python3", true},
		{"uvicorn", "uvicorn", true},
		{"/usr/bin/python3", "/usr/bin/python3", true},
		{"Python", "Python", true},
		{"python main.py", "python main.py", true},
		{"uvicorn main:app", "uvicorn main:app", true},
		{"java", "java", false},
		{"nginx", "nginx", false},
		{"node", "node", false},
		{"go", "go", false},
		{"sh", "sh", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occ := Occupant{PID: 12345, Command: tt.cmd}
			if got := IsFubonZombie(occ); got != tt.want {
				t.Errorf("IsFubonZombie(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestKillOccupant(t *testing.T) {
	// Use a shell that handles SIGTERM gracefully so the test exercises the
	// SIGTERM -> wait -> signal(0) path. On Windows this test is skipped because
	// the signal semantics differ.
	if runtime.GOOS == "windows" {
		t.Skip("skipping signal-based kill test on windows")
	}

	cmd := exec.Command("sh", "-c", "trap 'exit 0' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start shell: %v", err)
	}
	pid := cmd.Process.Pid

	if err := KillOccupant(pid); err != nil {
		t.Fatalf("KillOccupant(%d): %v", pid, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// reaped
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("process (pid %d) not reaped within 3s", pid)
	}
}

func TestKillOccupant_NonExistentPID(t *testing.T) {
	err := KillOccupant(999999999)
	if err == nil {
		t.Fatal("KillOccupant on non-existent PID should return error")
	}
	if !strings.Contains(err.Error(), "sigterm") && !strings.Contains(err.Error(), "find process") {
		t.Errorf("unexpected error message: %v", err)
	}
}
