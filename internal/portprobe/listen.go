package portprobe

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
)

// Listen returns a TCP listener on addr, performing a brief diagnostic if the
// address is already in use. It is the port-binding counterpart to Probe and
// is designed to be called by HTTP server bootstraps that want a useful error
// message instead of a bare "address already in use" failure.
//
// On success the returned listener is bound and ready for srv.Serve.
//
// On EADDRINUSE, Listen probes the address to identify the occupant and
// returns a wrapped error describing the state, PID, and command so the
// operator can act via the runbook (see
// docs/operations_playbook.md → "Port 18080 Conflict Recovery").
//
// Self-PID guard: the current process is never reported as a killable
// occupant; the diagnostic message always includes the running pid so the
// operator can distinguish "self still draining" from "another atlas
// instance".
//
// Listen does NOT automatically kill any process. Auto-kill is deliberately
// left out because the recovery decision (kill stale fubon-proxy vs. wait
// for another atlas instance vs. ask the operator) is a policy call that
// belongs to the runbook, not the bind path.
//
// Probe-before-bind ordering: Probe runs first because on macOS a
// wildcard listener (e.g. 0.0.0.0:18080) and a loopback listener (e.g.
// 127.0.0.1:18080) can coexist due to SO_REUSEADDR semantics, producing a
// false-positive net.Listen success. Probe cross-checks both wildcard
// families and lsof, which catches the false-positive before we hand a
// duplicate listener back to the caller.
func Listen(addr string) (net.Listener, error) {
	// Step 1: probe. If Probe reports the port as occupied, surface the
	// diagnostic without attempting a net.Listen (which would falsely
	// succeed on macOS in the wildcard-vs-loopback cross-binding case).
	if state, occ, probeErr := Probe(addr); probeErr == nil {
		if state != StateFree {
			return nil, formatOccupantDiagnostic(addr, state, occ)
		}
	}

	// Step 2: bind for real. Probe said Free; trust net.Listen unless it
	// disagrees.
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("portprobe: listen %q: %w", addr, err)
	}

	// Race: the port became occupied between Probe and net.Listen. Re-probe
	// for a fresh diagnostic.
	state, occ, probeErr := Probe(addr)
	if probeErr != nil {
		return nil, fmt.Errorf("portprobe: %s in use (re-probe failed: %v; original error: %w)",
			addr, probeErr, err)
	}
	if state == StateFree {
		return nil, fmt.Errorf("portprobe: %s in use (re-probe reported free; transient): %w",
			addr, err)
	}
	return nil, formatOccupantDiagnostic(addr, state, occ)
}

func formatOccupantDiagnostic(addr string, state State, occ Occupant) error {
	self := os.Getpid()
	switch state {
	case StateHealthy:
		return fmt.Errorf("portprobe: %s already serving by pid %d (%s) (self=%d); refusing to start: another healthy atlas instance may be running%s",
			addr, occ.PID, occ.Command, self, dockerRecoverySuffix(occ.Command))
	case StateForeign:
		return fmt.Errorf("portprobe: %s occupied by foreign pid %d (%s) (self=%d); see docs/operations_playbook.md → \"Port 18080 Conflict Recovery\" before killing",
			addr, occ.PID, occ.Command, self)
	default:
		return fmt.Errorf("portprobe: %s in use by pid %d (%s) (self=%d)",
			addr, occ.PID, occ.Command, self)
	}
}

func dockerRecoverySuffix(command string) string {
	cmd := strings.ToLower(command)
	if strings.Contains(cmd, "docker") || strings.Contains(cmd, "com.docker") {
		return "; stop Docker atlas (`docker compose stop atlas`) or use a different -addr (docs/operations_playbook.md → \"Port 18080 Conflict Recovery\")"
	}
	return ""
}
