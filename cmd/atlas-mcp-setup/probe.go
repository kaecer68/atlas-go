package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// ProbeResult aggregates the 4 health probes the wizard runs before
// suggesting the user commit to a config.
type ProbeResult struct {
	AtlasGoBackend ProbeCheck // atlas-go HTTP API on :18080
	AtlasMCPBinary ProbeCheck // local bin/atlas-mcp exists + executable
	AtlasMCPAdmin  ProbeCheck // atlas-mcp admin server on :9090
	WritableTarget ProbeCheck // target client config path is writeable
}

// ProbeCheck is one probe's outcome.
type ProbeCheck struct {
	OK     bool
	Detail string
	Err    error
}

// probeAll runs the 4 health probes. Best-effort: a single failure does
// not abort the rest.
func probeAll(cfg SetupConfig, target ClientInstall) ProbeResult {
	var r ProbeResult

	// 1. atlas-go backend port 18080
	state, _, err := portprobe.Probe("127.0.0.1:18080")
	r.AtlasGoBackend = ProbeCheck{
		OK:     state == portprobe.StateFree || state == portprobe.StateHealthy,
		Detail: fmt.Sprintf("atlas-go backend on 127.0.0.1:18080 (state=%s)", stateName(state)),
		Err:    err,
	}

	// 2. atlas-mcp binary exists + is executable
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = cfg.REPOROOT + "/bin/atlas-mcp"
	}
	if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() {
		executable := info.Mode()&0o111 != 0
		r.AtlasMCPBinary = ProbeCheck{
			OK:     executable,
			Detail: fmt.Sprintf("atlas-mcp binary: %s (executable=%v, size=%d bytes)", binaryPath, executable, info.Size()),
		}
		if !executable {
			r.AtlasMCPBinary.Detail += " — run: chmod +x " + binaryPath
		}
	} else {
		r.AtlasMCPBinary = ProbeCheck{
			OK:     false,
			Detail: fmt.Sprintf("atlas-mcp binary not found at %s — run: make build-mcp", binaryPath),
			Err:    err,
		}
	}

	// 3. atlas-mcp admin server on :9090
	state9090, _, err9090 := portprobe.Probe("127.0.0.1:9090")
	r.AtlasMCPAdmin = ProbeCheck{
		OK:     state9090 == portprobe.StateFree || state9090 == portprobe.StateHealthy,
		Detail: fmt.Sprintf("atlas-mcp admin on 127.0.0.1:9090 (state=%s)", stateName(state9090)),
		Err:    err9090,
	}

	// 4. target client config path writeable
	if target.ConfigPath == "" {
		r.WritableTarget = ProbeCheck{OK: true, Detail: "no target config path selected yet"}
	} else if target.Writeable {
		r.WritableTarget = ProbeCheck{
			OK:     true,
			Detail: fmt.Sprintf("target config writeable: %s", target.ConfigPath),
		}
	} else {
		r.WritableTarget = ProbeCheck{
			OK:     false,
			Detail: fmt.Sprintf("target config NOT writeable: %s — check directory permissions", target.ConfigPath),
		}
	}

	return r
}

// String formats ProbeResult for human display.
func (r ProbeResult) String() string {
	checks := []ProbeCheck{
		r.AtlasGoBackend, r.AtlasMCPBinary, r.AtlasMCPAdmin, r.WritableTarget,
	}
	var out strings.Builder
	for _, c := range checks {
		mark := "✗"
		if c.OK {
			mark = "✓"
		}
		fmt.Fprintf(&out, "  %s %s\n", mark, c.Detail)
	}
	return out.String()
}

// stateName renders a portprobe.State as a human string.
// portprobe.State is an unexported int type without a String() method, so
// we keep the mapping local to the setup tool.
func stateName(s portprobe.State) string {
	switch s {
	case portprobe.StateFree:
		return "free"
	case portprobe.StateHealthy:
		return "healthy"
	case portprobe.StateForeign:
		return "foreign"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}
