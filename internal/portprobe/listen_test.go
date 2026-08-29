package portprobe

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestListen_FreeAddr(t *testing.T) {
	addr := freeLocalAddr(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen(%q) on free addr: unexpected error: %v", addr, err)
	}
	defer func() { _ = ln.Close() }()

	if ln.Addr().String() != addr {
		t.Errorf("Listen(%q) addr = %q, want %q", addr, ln.Addr().String(), addr)
	}
}

func TestListen_OccupiedHealthy(t *testing.T) {
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

	// Give /health a moment to become reachable so Probe classifies the
	// listener as StateHealthy rather than StateForeign.
	time.Sleep(200 * time.Millisecond)

	ln, err := Listen(addr)
	if err == nil {
		_ = ln.Close()
		t.Fatalf("Listen(%q) on occupied healthy addr: expected error, got nil", addr)
	}
	if ln != nil {
		t.Errorf("Listen(%q) returned non-nil listener on error path", addr)
	}
	if !strings.Contains(err.Error(), "already serving") {
		t.Errorf("Listen error = %q, want substring 'already serving'", err.Error())
	}
	if !strings.Contains(err.Error(), "portprobe") {
		t.Errorf("Listen error = %q, want substring 'portprobe'", err.Error())
	}
}

func TestDockerRecoverySuffix(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"com.docker.backend", true},
		{"docker-proxy", true},
		{"Docker Desktop", true},
		{"atlas", false},
		{"", false},
	}
	for _, tc := range cases {
		got := dockerRecoverySuffix(tc.cmd)
		if tc.want && got == "" {
			t.Errorf("dockerRecoverySuffix(%q) = empty, want docker hint", tc.cmd)
		}
		if !tc.want && got != "" {
			t.Errorf("dockerRecoverySuffix(%q) = %q, want empty", tc.cmd, got)
		}
		if tc.want && !strings.Contains(got, "docker compose stop atlas") {
			t.Errorf("dockerRecoverySuffix(%q) = %q, want compose stop hint", tc.cmd, got)
		}
	}
}

func TestListen_OccupiedForeign(t *testing.T) {
	addr := freeLocalAddr(t)
	// Handler that does NOT respond to /health, so Probe classifies the
	// listener as StateForeign.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	cleanup := occupyTCP(t, addr, handler)
	defer cleanup()

	ln, err := Listen(addr)
	if err == nil {
		_ = ln.Close()
		t.Fatalf("Listen(%q) on occupied foreign addr: expected error, got nil", addr)
	}
	if !strings.Contains(err.Error(), "foreign") {
		t.Errorf("Listen error = %q, want substring 'foreign'", err.Error())
	}
	if !strings.Contains(err.Error(), "Port 18080 Conflict Recovery") {
		t.Errorf("Listen error = %q, want runbook reference", err.Error())
	}
}

func TestListen_BadAddr(t *testing.T) {
	_, err := Listen("not-a-valid-address")
	if err == nil {
		t.Fatalf("Listen(\"not-a-valid-address\"): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "portprobe") {
		t.Errorf("Listen bad-addr error = %q, want substring 'portprobe'", err.Error())
	}
}

// TestListen_ConcurrentBindSameAddr documents the binding contract under
// concurrent Listen calls on the same free address. The contract is:
// at least one call must succeed, and on any failure the error must be
// the formatted portprobe diagnostic (not a bare net.Listen error).
//
// On Linux without SO_REUSEADDR, exactly one call succeeds and the other
// gets the EADDRINUSE diagnostic. On macOS, net.Listen enables SO_REUSEADDR
// by default, so both calls can succeed simultaneously — this is the
// platform behaviour called out in listen.go's Probe-before-bind comment.
func TestListen_ConcurrentBindSameAddr(t *testing.T) {
	addr := freeLocalAddr(t)

	type result struct {
		ln interface {
			Close() error
		}
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			ln, err := Listen(addr)
			var closer interface {
				Close() error
			}
			if ln != nil {
				closer = ln
			}
			results <- result{ln: closer, err: err}
		}()
	}

	var successes, failures int
	for range 2 {
		r := <-results
		if r.err == nil {
			successes++
			if r.ln != nil {
				_ = r.ln.Close()
			}
			continue
		}
		failures++
		if !strings.Contains(r.err.Error(), "portprobe") {
			t.Errorf("Concurrent Listen failure: got non-portprobe error %q; want formatted diagnostic", r.err.Error())
		}
	}

	if successes < 1 {
		t.Errorf("Concurrent Listen on free addr: got %d successes, %d failures; want at least 1 success",
			successes, failures)
	}
	if successes+failures != 2 {
		t.Errorf("Concurrent Listen: %d total results, want 2", successes+failures)
	}
}

// TestListen_FreeIPv6Addr verifies Listen works on a free [::1]:port. The
// existing freeLocalAddr helper is IPv4-only; this exercises the IPv6 path
// explicitly.
func TestListen_FreeIPv6Addr(t *testing.T) {
	tmp, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback not available: %v", err)
	}
	addr := tmp.Addr().String()
	_ = tmp.Close()

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen(%q) on free IPv6 addr: unexpected error: %v", addr, err)
	}
	defer func() { _ = ln.Close() }()
}

// TestListen_FreeAddr_LsofMissing verifies Listen still works when lsof is
// absent. lookupOccupantByPort returns an error, Probe degrades to net.Listen
// result (StateFree), and Listen binds normally.
func TestListen_FreeAddr_LsofMissing(t *testing.T) {
	t.Setenv("PATH", "")

	addr := freeLocalAddr(t)
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen(%q) without lsof on free addr: unexpected error: %v", addr, err)
	}
	defer func() { _ = ln.Close() }()
}
