// Package portprobe provides stateless helpers for probing TCP ports and
// managing processes that occupy them. It is intentionally free of mutable
// package state and sync primitives; callers own any lifecycle coordination.
package portprobe

import (
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
// "127.0.0.1:8081".
func Probe(addr string) (State, Occupant, error) {
	port, host, err := parsePort(addr)
	if err != nil {
		return 0, Occupant{}, fmt.Errorf("probe %q: %w", addr, err)
	}

	portStr := strconv.Itoa(port)
	ln4, err4 := net.Listen("tcp", "0.0.0.0:"+portStr)
	if err4 == nil {
		_ = ln4.Close()
		ln6, err6 := net.Listen("tcp", "[::]:"+portStr)
		if err6 == nil {
			_ = ln6.Close()
			return StateFree, Occupant{}, nil
		}
		err4 = err6
	}
	if !errors.Is(err4, syscall.EADDRINUSE) {
		return 0, Occupant{}, fmt.Errorf("probe %q: %w", addr, err4)
	}

	healthURL := "http://" + net.JoinHostPort(host, portStr) + healthPath
	for attempt := 0; attempt < 5; attempt++ {
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
	out, err := exec.Command("lsof",
		"-nP",
		fmt.Sprintf("-iTCP:%d", port),
		"-sTCP:LISTEN",
		"-FpcL",
	).Output()
	if err != nil {
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
