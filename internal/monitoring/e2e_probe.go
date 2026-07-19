// Package monitoring: end-to-end chain probe (H06).
//
// Daily synthetic probe that verifies the data → strategy → recommendation
// full chain is functional. Reports breakage per layer so operators can
// pinpoint which segment is broken — not just "something is wrong."
//
// Maturity: experimental

package monitoring

import (
	"context"
	"fmt"
	"log"
	"time"
)

// E2EProbeResult captures per-layer health of a single probe run.
type E2EProbeResult struct {
	RunAt           time.Time
	DataLayerOK     bool
	StrategyLayerOK bool
	RecoLayerOK     bool
	DataDetail      string
	StrategyDetail  string
	RecoDetail      string
}

// E2EProbeDeps are the external systems the probe needs to reach.
// Every field is optional — nil means "skip this layer check".
type E2EProbeDeps struct {
	// DataLayerCheck returns nil if macro/capital-flow data is fresh.
	DataLayerCheck func(ctx context.Context) error
	// StrategyLayerCheck returns nil if active strategies are producing signals.
	StrategyLayerCheck func(ctx context.Context) error
	// RecoLayerCheck returns nil if recommendations are being generated.
	RecoLayerCheck func(ctx context.Context) error
}

// RunE2EProbe executes a single end-to-end chain probe.
// Returns the per-layer result; errors are recorded inside the result,
// never returned as a Go error (probe failure should not crash the caller).
func RunE2EProbe(ctx context.Context, deps E2EProbeDeps) E2EProbeResult {
	r := E2EProbeResult{RunAt: time.Now().UTC()}

	if deps.DataLayerCheck != nil {
		if err := deps.DataLayerCheck(ctx); err != nil {
			r.DataDetail = err.Error()
		} else {
			r.DataLayerOK = true
			r.DataDetail = "ok"
		}
	} else {
		r.DataDetail = "skipped (no checker wired)"
	}

	if deps.StrategyLayerCheck != nil {
		if err := deps.StrategyLayerCheck(ctx); err != nil {
			r.StrategyDetail = err.Error()
		} else {
			r.StrategyLayerOK = true
			r.StrategyDetail = "ok"
		}
	} else {
		r.StrategyDetail = "skipped (no checker wired)"
	}

	if deps.RecoLayerCheck != nil {
		if err := deps.RecoLayerCheck(ctx); err != nil {
			r.RecoDetail = err.Error()
		} else {
			r.RecoLayerOK = true
			r.RecoDetail = "ok"
		}
	} else {
		r.RecoDetail = "skipped (no checker wired)"
	}

	return r
}

// E2EProbeSummary returns a one-line human-readable summary.
func (r E2EProbeResult) Summary() string {
	dataIcon := icon(r.DataLayerOK)
	stratIcon := icon(r.StrategyLayerOK)
	recoIcon := icon(r.RecoLayerOK)
	return fmt.Sprintf("[e2e-probe] data=%s strategy=%s reco=%s | data:%s strategy:%s reco:%s",
		dataIcon, stratIcon, recoIcon,
		r.DataDetail, r.StrategyDetail, r.RecoDetail)
}

// AllOK returns true when every wired layer is healthy.
func (r E2EProbeResult) AllOK() bool {
	return r.DataLayerOK && r.StrategyLayerOK && r.RecoLayerOK
}

func icon(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// E2EProbeTaskFunc returns a scheduler-compatible task function that
// runs the probe once and logs the result. Dependencies are wired at
// task-creation time (closure).
func E2EProbeTaskFunc(deps E2EProbeDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		result := RunE2EProbe(ctx, deps)
		msg := result.Summary()
		if result.AllOK() {
			log.Printf("[e2e-probe] %s", msg)
		} else {
			log.Printf("[e2e-probe] WARNING: %s", msg)
		}
		return nil // never fail the scheduler — probe result is informational
	}
}
