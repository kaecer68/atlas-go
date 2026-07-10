package startup

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/portprobe"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func occupyAddr(t *testing.T, addr string) func() {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:"+portStr)
	if err != nil {
		t.Fatalf("occupy %s: %v", addr, err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

func TestPreflight_AllFree(t *testing.T) {
	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: freeAddr(t)},
	}
	if err := Preflight(claims); err != nil {
		t.Fatalf("Preflight(all free) = %v, want nil", err)
	}
}

func TestPreflight_OneForeign(t *testing.T) {
	foreignAddr := freeAddr(t)
	cleanup := occupyAddr(t, foreignAddr)
	defer cleanup()

	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: foreignAddr},
	}
	err := Preflight(claims)
	if err == nil {
		t.Fatal("Preflight(foreign) = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fubon-proxy") {
		t.Errorf("error should mention component, got: %q", msg)
	}
	if !strings.Contains(msg, "kill") {
		t.Errorf("error should suggest kill, got: %q", msg)
	}
	if !strings.Contains(msg, foreignAddr) {
		t.Errorf("error should mention address, got: %q", msg)
	}
}

func TestPreflight_MixFreeAndForeign_ReturnsFirstForeign(t *testing.T) {
	foreignAddr := freeAddr(t)
	cleanup := occupyAddr(t, foreignAddr)
	defer cleanup()

	claims := []PortClaim{
		{Component: "atlas-http", Addr: freeAddr(t)},
		{Component: "fubon-proxy", Addr: foreignAddr},
		{Component: "extra", Addr: freeAddr(t)},
	}
	err := Preflight(claims)
	if err == nil {
		t.Fatal("Preflight(mix) = nil, want error")
	}
	if !strings.Contains(err.Error(), "fubon-proxy") {
		t.Errorf("expected first foreign error, got: %q", err.Error())
	}
}

func TestPreflight_ProbeError_Continues(t *testing.T) {
	claims := []PortClaim{
		{Component: "bad-addr", Addr: "not-an-address"},
		{Component: "atlas-http", Addr: freeAddr(t)},
	}
	if err := Preflight(claims); err != nil {
		t.Fatalf("Preflight(probe error + free) = %v, want nil", err)
	}
}

// AllowZombieKill + function-var injection seam (Oracle 4th round, F10/F10a)
// Probe/kill/zombie stubs defined below let the four behavioural permutations
// run without a real subprocess holding a TCP port (compare with the
// integration-style TestPreflight_OneForeign above, which needs net.Listen).

func withStubProbes(t *testing.T, probe probeStubFn, kill killStubFn, zombie zombieStubFn) {
	t.Helper()
	prevProbe, prevKill, prevZombie := probeFn, killFn, isFubonZombieFn
	probeFn = probe
	killFn = kill
	isFubonZombieFn = zombie
	t.Cleanup(func() {
		probeFn = prevProbe
		killFn = prevKill
		isFubonZombieFn = prevZombie
	})
}

type (
	probeStubFn  = func(addr string) (portprobe.State, portprobe.Occupant, error)
	killStubFn   = func(pid int) error
	zombieStubFn = func(occ portprobe.Occupant) bool
)

// F10: AllowZombieKill=true + recognised fubon-proxy zombie → KillOccupant
// called, re-probe returns StateFree → Preflight returns nil. Mirrors the
// SIGKILL/orphan SIGKILL scenario: previous atlas was SIGKILL'd, fubon-proxy
// subprocess was orphaned holding :18081; new atlas startup reclaims.
func TestPreflight_AllowZombieKill_ZombieCase(t *testing.T) {
	var killed []int
	var probeCalls int
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			probeCalls++
			if probeCalls == 1 {
				return portprobe.StateForeign, portprobe.Occupant{PID: 39326, Command: "python fubon-proxy"}, nil
			}
			return portprobe.StateFree, portprobe.Occupant{}, nil
		},
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		func(occ portprobe.Occupant) bool {
			return strings.Contains(strings.ToLower(occ.Command), "python")
		},
	)

	if err := Preflight([]PortClaim{
		{Component: "fubon-proxy", Addr: "127.0.0.1:18081", AllowZombieKill: true},
	}); err != nil {
		t.Fatalf("expected no error after auto-kill, got: %v", err)
	}
	if len(killed) != 1 || killed[0] != 39326 {
		t.Errorf("expected KillOccupant(39326) called once, got calls: %v", killed)
	}
	if probeCalls != 2 {
		t.Errorf("expected exactly 2 probes (initial + re-probe after kill), got %d", probeCalls)
	}
}

// F10: AllowZombieKill=true but occupant is NOT a recognised fubon-proxy
// zombie (e.g., com.docker.backend, a legitimate service the user explicitly
// started) → killFn MUST NOT be called; Preflight must surface the
// actionable error and identify the foreign process.
func TestPreflight_AllowZombieKill_NonZombieForeign(t *testing.T) {
	var killed []int
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			return portprobe.StateForeign, portprobe.Occupant{PID: 5866, Command: "com.docker.backend"}, nil
		},
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		func(occ portprobe.Occupant) bool {
			lo := strings.ToLower(occ.Command)
			return strings.Contains(lo, "python") || strings.Contains(lo, "uvicorn")
		},
	)

	err := Preflight([]PortClaim{
		{Component: "fubon-proxy", Addr: "127.0.0.1:18081", AllowZombieKill: true},
	})
	if err == nil {
		t.Fatal("expected error for non-zombie foreign holder")
	}
	msg := err.Error()
	if !strings.Contains(msg, "com.docker.backend") || !strings.Contains(msg, "5866") {
		t.Errorf("error should identify non-zombie occupant (pid=5866 cmd=com.docker.backend), got: %q", msg)
	}
	if len(killed) != 0 {
		t.Errorf("killFn must NOT be called for non-zombie occupant, got calls: %v", killed)
	}
}

// F10a: AllowZombieKill=false (atlas-http style) → any Foreign returns
// actionable error and killFn is NEVER called regardless of zombie match.
func TestPreflight_NoAllowZombieKill_ForeignError(t *testing.T) {
	var killed []int
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			return portprobe.StateForeign, portprobe.Occupant{PID: 4096, Command: "python-looking"}, nil
		},
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		func(occ portprobe.Occupant) bool { return true },
	)

	err := Preflight([]PortClaim{
		{Component: "atlas-http", Addr: "127.0.0.1:18080"}, // AllowZombieKill defaults to false
	})
	if err == nil {
		t.Fatal("expected error when AllowZombieKill=false on foreign")
	}
	if !strings.Contains(err.Error(), "4096") {
		t.Errorf("error should mention pid=4096, got: %v", err)
	}
	if len(killed) != 0 {
		t.Errorf("killFn MUST NOT be called when AllowZombieKill is false, got calls: %v", killed)
	}
}

// F10: AllowZombieKill=true but probe reports StateHealthy (e.g., a
// legitimate external fubon-proxy the user explicitly started via docker
// compose). Preflight returns nil without touching killFn — healthy
// foreign is a sign to skip the spawn (handled later by fubonproxy.Start).
func TestPreflight_HealthyExternallyManaged_NoError(t *testing.T) {
	var killed []int
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			return portprobe.StateHealthy, portprobe.Occupant{PID: 7000, Command: "uvicorn fubon_proxy.main:app"}, nil
		},
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		func(occ portprobe.Occupant) bool { return true },
	)

	if err := Preflight([]PortClaim{
		{Component: "fubon-proxy", Addr: "127.0.0.1:18081", AllowZombieKill: true},
	}); err != nil {
		t.Fatalf("healthy external fubon-proxy should pass Preflight, got: %v", err)
	}
	if len(killed) != 0 {
		t.Errorf("killFn must NOT be called when state is Healthy, got calls: %v", killed)
	}
}

func TestPreflight_ExclusiveHealthy_Errors(t *testing.T) {
	var killed []int
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			return portprobe.StateHealthy, portprobe.Occupant{PID: 71689, Command: "com.docker.backend"}, nil
		},
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		func(occ portprobe.Occupant) bool { return false },
	)

	err := Preflight([]PortClaim{
		{Component: "atlas-http", Addr: ":18080"},
	})
	if err == nil {
		t.Fatal("exclusive healthy occupant must fail Preflight")
	}
	msg := err.Error()
	if !strings.Contains(msg, "atlas-http") {
		t.Errorf("error should mention component, got: %q", msg)
	}
	if !strings.Contains(msg, "already served") {
		t.Errorf("error should mention healthy conflict, got: %q", msg)
	}
	if !strings.Contains(msg, "docker compose stop atlas") {
		t.Errorf("docker occupant should suggest compose recovery, got: %q", msg)
	}
	if !strings.Contains(msg, "71689") {
		t.Errorf("error should mention pid, got: %q", msg)
	}
	if len(killed) != 0 {
		t.Errorf("killFn must NOT be called for exclusive healthy, got: %v", killed)
	}
}

func TestPreflight_ExclusiveHealthy_NativeHint(t *testing.T) {
	withStubProbes(t,
		func(addr string) (portprobe.State, portprobe.Occupant, error) {
			return portprobe.StateHealthy, portprobe.Occupant{PID: 44141, Command: "atlas"}, nil
		},
		func(pid int) error { return nil },
		func(occ portprobe.Occupant) bool { return false },
	)

	err := Preflight([]PortClaim{
		{Component: "atlas-http", Addr: ":18080"},
	})
	if err == nil {
		t.Fatal("expected exclusive healthy error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "kill 44141") {
		t.Errorf("native occupant should suggest kill, got: %q", msg)
	}
	if strings.Contains(msg, "docker compose") {
		t.Errorf("native occupant must not suggest docker recovery, got: %q", msg)
	}
}
