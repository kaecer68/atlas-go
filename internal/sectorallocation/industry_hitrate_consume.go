// Package sectorallocation — industry hit-rate consumption (PR-β, 2026-09-24).
//
// This file implements the config-gated wiring of canonical industry-level
// hit-rate (#1942/#1948, stockpicker-stage) into the sectorallocation
// recommendation path. When configs.sector_allocation.industry_hit_rate_consume_enabled
// is true, hit-rate data drives a per-L1 tilt that is added to DriverInputs
// before ComputeProjectedTarget runs the projection.
//
// When the gate is false (default), every helper here is a no-op:
//   - BuildIndustryHitRateTilt returns an empty map
//   - ApplyIndustryHitRateToDrivers returns the input DriverInputs unchanged
//
// This guarantees byte-identical regression behavior vs dfc4e3a1 when the
// gate is off — the root agent verifies this as the backstop safety
// guarantee for PR-β.
//
// Production paths MUST stay byte-identical when the gate is off (spec §8.3,
// issue #1944 Batch 1); the gate is therefore opt-in by config and reversible.
//
// IMPORT-CYCLE NOTE: stockpicker → ledger → portfolio → sectorallocation,
// so sectorallocation cannot import stockpicker directly. The consumer
// interface below mirrors stockpicker.IndustryWinRateSummary as a minimal
// local type; the production wiring in internal/stocktools/industry_winrate.go
// adapts stockpicker output to this shape.
package sectorallocation

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// IndustryHitRateSummary is the minimal subset of
// stockpicker.IndustryWinRateSummary that BuildIndustryHitRateTilt needs.
// Defined locally to break the stockpicker → ledger → portfolio →
// sectorallocation import cycle (the producer copies into this shape).
type IndustryHitRateSummary struct {
	IndustryID   string
	Direction    string  // "buy" or "avoid"
	WilsonLower  float64 // Wilson 95% lower bound
	WilsonUpper  float64 // Wilson 95% upper bound
	WinRate      float64 // point estimate (扣成本命中率)
	Observations int
}

// IndustryHitRateReport is the minimal subset of stockpicker.IndustryWinRateReport
// BuildIndustryHitRateTilt needs. Same cycle-breaking rationale as above.
type IndustryHitRateReport struct {
	Industries []IndustryHitRateSummary
}

// IndustryHitRateProvider is the minimal interface BuildIndustryHitRateTilt
// needs. The production binding lives in internal/stocktools (where the
// stockpicker import is safe); tests inject a fake.
type IndustryHitRateProvider interface {
	// LoadIndustryWinRate returns the IndustryHitRateReport for one
	// (source, condition_id, rolling_window) tuple. The SectorResolver
	// is the canonical L1 mapping used to attribute symbols to industries.
	// Returns IndustryHitRateReport{} (no error) when the report has
	// no rows (the same coverage-passthrough behavior stockpicker itself
	// guarantees per spec §1.2).
	LoadIndustryWinRate(source, conditionID, rollingWindow string, resolve SectorResolver) (IndustryHitRateReport, error)
}

// SectorResolver mirrors stockpicker.SectorResolver so the local
// IndustryHitRateProvider does not need to import stockpicker. Production
// adapters pass through to the canonical L1 mapping.
type SectorResolver func(symbol string) (industryID string, ok bool)

// IndustryHitRateTilt holds the per-L1 tilt derived from a single industry
// hit-rate summary row. The TiltMagnitude is signed: positive values tilt
// toward the industry (more weight in target), negative values tilt away.
// The Evidence block carries enough metadata for downstream consumers to
// audit the source (industry_id, condition_id, wilson_lower/upper, etc.)
// without re-querying stockpicker.
type IndustryHitRateTilt struct {
	IndustryID    string  `json:"industry_id"`
	Direction     string  `json:"direction"`      // "buy" or "avoid" (mirrors report row)
	WilsonLower   float64 `json:"wilson_lower"`   // from the IndustryHitRateSummary
	WilsonUpper   float64 `json:"wilson_upper"`   // from the IndustryHitRateSummary
	WinRate       float64 `json:"win_rate"`       // point estimate
	Observations  int     `json:"observations"`   // sample count
	TiltMagnitude float64 `json:"tilt_magnitude"` // signed, additive in DriverInputs
}

// industryHitRateTiltScales defines the additive scale used when promoting
// a (WilsonLower - 0.5) delta into a per-L1 tilt. Calibrated by P0-3 (k3
// review R1): a 25 percentage-point spread (WilsonLower 0.5 → 0.75) maps to
// a ±0.05 weight tilt, well within the Projector's MaxIterations=10 clamp
// envelope. This is a one-way calibration knob; reverting requires flipping
// the gate to false (it is not loaded when the gate is off).
const (
	industryHitRateTiltScale      = 0.2 // 1.0 unit of (WilsonLower - 0.5) → 0.2 weight delta
	industryHitRateAvoidInversion = -1.0
)

// BuildIndustryHitRateTilt reads an IndustryHitRateReport and returns one
// IndustryHitRateTilt per L1 industry row. The tilt is the additive delta
// that should be added to DriverInputs.CapitalFlow to make the Projector
// prefer industries with high hit-rate and avoid those with low hit-rate.
//
// Behavior:
//   - When the gate is OFF (default), returns (nil, nil): no-op. Callers MUST
//     tolerate nil tilts (Projector treats nil maps as zero contribution).
//   - When the gate is ON but provider is nil, returns an error so the
//     caller can fall back to the calibrating assessment instead of
//     silently returning an empty map.
//   - When the gate is ON and provider returns a report with rows, each
//     row's WilsonLower is mapped to TiltMagnitude via
//     (WilsonLower - 0.5) * industryHitRateTiltScale. avoid-direction rows
//     invert (a high win_rate in an avoid condition = bad → tilt away).
//   - Rows whose WilsonLower is exactly 0 (degenerate) are skipped; the
//     mapping would be a noise amplifier.
//
// NOTE: This function is deliberately pure (no time, no I/O beyond provider
// call) so the unit test can run deterministically.
func BuildIndustryHitRateTilt(provider IndustryHitRateProvider, source, conditionID, rollingWindow string) ([]IndustryHitRateTilt, error) {
	if !config.GetIndustryHitRateConsumeEnabled() {
		return nil, nil // config-off: explicit no-op
	}
	if provider == nil {
		return nil, errIndustryHitRateProviderMissing
	}
	// Use the canonical L1 sector resolver (industry.L1Sectors()) so every
	// outcome attributed to a canonical L1 sector id lands in the right
	// tilt row. The resolver is the same one stocktools wires into the
	// production service; tests inject their own.
	resolve := func(symbol string) (industryID string, ok bool) {
		sid := industry.ClassifyBySymbol(symbol)
		if sid == "" || !industry.IsL1(sid) {
			return "", false
		}
		return string(sid), true
	}
	report, err := provider.LoadIndustryWinRate(source, conditionID, rollingWindow, resolve)
	if err != nil {
		return nil, err
	}
	out := make([]IndustryHitRateTilt, 0, len(report.Industries))
	for _, row := range report.Industries {
		if row.WilsonLower <= 0 {
			continue // degenerate row (zero evidence or boundary)
		}
		magnitude := (row.WilsonLower - 0.5) * industryHitRateTiltScale
		if row.Direction == "avoid" {
			magnitude *= industryHitRateAvoidInversion
		}
		out = append(out, IndustryHitRateTilt{
			IndustryID:    row.IndustryID,
			Direction:     row.Direction,
			WilsonLower:   row.WilsonLower,
			WilsonUpper:   row.WilsonUpper,
			WinRate:       row.WinRate,
			Observations:  row.Observations,
			TiltMagnitude: magnitude,
		})
	}
	return out, nil
}

// TiltToDriverMap converts a slice of tilts to the map[industry.SectorID]float64
// shape DriverInputs.CapitalFlow expects. L1 key validation is enforced so a
// stale or non-L1 id never reaches the Projector (which would otherwise fail
// the SA-INV-04 guard).
//
// When tilts is nil (config off), returns nil: the Projector treats nil maps
// as zero contribution, exactly the byte-identical pre-PR-β behavior.
func TiltToDriverMap(tilts []IndustryHitRateTilt) map[industry.SectorID]float64 {
	if len(tilts) == 0 {
		return nil // explicit no-op (config off OR report empty)
	}
	out := make(map[industry.SectorID]float64, len(tilts))
	for _, t := range tilts {
		id := industry.SectorID(t.IndustryID)
		if !industry.IsL1(id) {
			continue // defensive: never leak non-L1 keys to Projector
		}
		out[id] = t.TiltMagnitude
	}
	return out
}

// ApplyIndustryHitRateToDrivers adds the hit-rate-derived tilt into the
// CapitalFlow driver of drivers. When the gate is OFF (default), returns
// drivers unchanged — this is the byte-identical safety guarantee.
//
// When the gate is ON, the returned DriverInputs has the new tilts added
// to the existing CapitalFlow map (additive — does not overwrite
// already-set entries from the E07 capital-flow pipeline).
//
// Callers (typically the sectorallocation composition root or the
// ComputeProjectedTarget implementation) pass the result through the
// existing Projector.Project(...) call without further changes.
func ApplyIndustryHitRateToDrivers(drivers DriverInputs, provider IndustryHitRateProvider, source, conditionID, rollingWindow string) (DriverInputs, error) {
	if !config.GetIndustryHitRateConsumeEnabled() {
		return drivers, nil // config-off: explicit no-op (byte-identical)
	}
	tilts, err := BuildIndustryHitRateTilt(provider, source, conditionID, rollingWindow)
	if err != nil {
		return drivers, err
	}
	tiltMap := TiltToDriverMap(tilts)
	if tiltMap == nil {
		return drivers, nil
	}
	if drivers.CapitalFlow == nil {
		drivers.CapitalFlow = make(map[industry.SectorID]float64, len(tiltMap))
	}
	for id, v := range tiltMap {
		// additive: do not overwrite existing capital-flow tilts
		if _, exists := drivers.CapitalFlow[id]; !exists {
			drivers.CapitalFlow[id] = v
		}
	}
	return drivers, nil
}

// errIndustryHitRateProviderMissing is returned when the gate is ON but the
// caller passed a nil provider. It is a wired-but-misconfigured signal —
// the assessment surface must surface "industry_hit_rate_consume_enabled but
// no provider wired" rather than silently returning empty tilts.
var errIndustryHitRateProviderMissing = errConfig("industry_hit_rate_consume_enabled=true but no IndustryHitRateProvider wired")

// errConfig is a small sentinel type so the wiring code can use errors.Is
// without leaking an external dependency. The string form is what the
// assessment surface echoes.
type errConfig string

func (e errConfig) Error() string { return string(e) }
