package portprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultLsofTimeout = 5 * time.Second

var (
	lsofPath    = "lsof"
	lsofTimeout = defaultLsofTimeout
)

// State describes the occupancy state of a TCP address after probing.
type State int

const (
	// StateFree means the address is not currently bound.
	StateFree State = iota
	// StateHealthy means the address is bound and serves a /health endpoint.
	StateHealthy
	// StateForeign means the address is bound by a process that does not
	// appear to be the expected healthy service.
	StateForeign
)

// Occupant describes a process that is listening on a port.
type Occupant struct {
	PID     int
	Command string
}

// Probe checks whether addr is free, held by a healthy service, or held by a
// foreign process. It uses net.Listen (not net.Dial) so it has no side effects
// on a healthy listener. The addr should be a host:port string such as
// "127.0.0.1:18081".
func Probe(addr string) (State, Occupant, error) {
	port, host, err := parsePort(addr)
	if err != nil {
		return 0, Occupant{}, fmt.Errorf("probe %q: %w", addr, err)
	}

	// First check the exact address the caller intends to bind. A listener on
	// the same host:port (including a wildcard listener that covers this
	// address) makes the address unavailable, even if the occupant set
	// SO_REUSEADDR. This prevents false negatives on Linux/macOS where a
	// loopback listener and a wildcard listener could otherwise appear to
	// coexist.
	exactLn, exactErr := net.Listen("tcp", addr)
	if exactErr != nil {
		if !errors.Is(exactErr, syscall.EADDRINUSE) {
			return 0, Occupant{}, fmt.Errorf("probe %q: %w", addr, exactErr)
		}
		return classifyOccupied(port, host)
	}
	_ = exactLn.Close()

	// The exact address is free. Also verify that no wildcard listener occupies
	// the port on another address family (e.g. Docker Desktop port forwards on
	// 0.0.0.0 or [::]).
	portStr := strconv.Itoa(port)
	ln4, err4 := net.Listen("tcp", "0.0.0.0:"+portStr)
	if err4 == nil {
		_ = ln4.Close()
		ln6, err6 := net.Listen("tcp", "[::]:"+portStr)
		if err6 == nil {
			_ = ln6.Close()
			// net.Listen reports free, but on macOS a listener that set
			// SO_REUSEADDR can coexist with our transient bind, producing a
			// false negative. Cross-check with lsof before declaring the port
			// free; on systems without lsof we keep the net.Listen result.
			occ, lookupErr := lookupOccupantByPort(port)
			if lookupErr == nil && occ.PID > 0 {
				return classifyOccupied(port, host)
			}
			return StateFree, Occupant{}, nil
		}
		err4 = err6
	}
	if !errors.Is(err4, syscall.EADDRINUSE) {
		return 0, Occupant{}, fmt.Errorf("probe %q: %w", addr, err4)
	}
	return classifyOccupied(port, host)
}

func classifyOccupied(port int, host string) (State, Occupant, error) {
	healthURL := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + healthPath
	for attempt := range 5 {
		if isHealthy(healthURL) {
			occ, _ := lookupOccupantByPort(port)
			return StateHealthy, occ, nil
		}
		if attempt < 4 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	occ, _ := lookupOccupantByPort(port)
	return StateForeign, occ, nil
}

// LookupOccupant uses lsof to find the process listening on addr.
func LookupOccupant(addr string) (Occupant, error) {
	port, _, err := parsePort(addr)
	if err != nil {
		return Occupant{}, fmt.Errorf("lookup occupant %q: %w", addr, err)
	}
	return lookupOccupantByPort(port)
}

// IsFubonZombie reports whether occ looks like a leftover fubon-proxy process.
func IsFubonZombie(occ Occupant) bool {
	cmd := strings.ToLower(occ.Command)
	return strings.Contains(cmd, "python") || strings.Contains(cmd, "uvicorn")
}

// KillOccupant terminates pid with SIGTERM, waits one second, then sends
// SIGKILL if the process is still alive.
func KillOccupant(pid int) error {
	return killOccupantImpl(pid)
}

// WaitForPortFree polls until no TCP listener is bound on any local address
// for the given port. It returns true once the port is free, or false if the
// timeout expires. This is useful after KillOccupant because a process may
// have exited while its listening socket is still held by the kernel or by a
// zombie child that has not yet been reaped.
func WaitForPortFree(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	portStr := strconv.Itoa(port)
	for time.Now().Before(deadline) {
		free := true
		for _, addr := range []string{"127.0.0.1:" + portStr, "0.0.0.0:" + portStr, "[::]:" + portStr} {
			ln, err := net.Listen("tcp", addr)
			if err == nil {
				_ = ln.Close()
				continue
			}
			if errors.Is(err, syscall.EADDRINUSE) {
				free = false
				break
			}
		}
		if free {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

const healthPath = "/health"

func parsePort(addr string) (int, string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, "", fmt.Errorf("parse address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, "", fmt.Errorf("parse port %q: %w", portStr, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return port, host, nil
}

func isHealthy(url string) bool {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func lookupOccupantByPort(port int) (Occupant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lsofTimeout)
	defer cancel()
	out, err := exec.CommandContext(
		ctx, lsofPath,
		"-nP",
		fmt.Sprintf("-iTCP:%d", port),
		"-sTCP:LISTEN",
		"-FpcL",
	).Output()
	if err != nil {
		// exec.CommandContext cancels the process when ctx fires; the resulting
		// error is a *os/exec.ExitError / signal: killed whose Cause is empty,
		// so we re-survey ctx.Err() to distinguish deadline/cancellation from a
		// genuine lsof failure.
		if cerr := ctx.Err(); cerr != nil {
			return Occupant{}, fmt.Errorf("lsof timeout after %s on port %d: %w", lsofTimeout, port, cerr)
		}
		return Occupant{}, fmt.Errorf("lsof: %w", err)
	}
	var occ Occupant
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			if pid, perr := strconv.Atoi(line[1:]); perr == nil {
				occ.PID = pid
			}
		case 'c':
			occ.Command = line[1:]
		}
	}
	if occ.PID == 0 {
		return Occupant{}, fmt.Errorf("port %d held but lsof reported no PID", port)
	}
	return occ, nil
}

func killOccupantImpl(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sigterm %d: %w", pid, err)
	}
	time.Sleep(1 * time.Second)
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("sigkill %d: %w", pid, err)
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}
