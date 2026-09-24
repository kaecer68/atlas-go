package sectorallocation

import (
	"log/slog"
	"math"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// ---- Industry hit-rate consumption chain (issues #1942 / #1948) ----------
//
// #1942/#1948 exposed the canonical, cost-aware INDUSTRY-level hit-rate
// (stockpicker caliber: hit = forward_return - cost_rate > 0, Wilson CI,
// min_samples calibration gate). Until this change nothing in production
// consumed it: the sectorallocation weight decision and the capital-flow
// assessment both ignored it, so the aggregate was a report, not evidence.
//
// This file wires the evidence into the sectorallocation recommendation path,
// behind the config gate
//
//	configs/parameters.json -> sector_allocation.industry_hit_rate_consume_enabled
//
// which defaults to false. With the gate off every function here is a no-op
// and ComputeProjectedTarget runs exactly the pre-change code path (see
// industry_hitrate_byte_identity_test.go, which diffs the gate-off output
// against a snapshot generated from the pre-change revision).
//
// Reversibility: the chain is driven by one config flag plus one injected
// provider. No historical data is written or rewritten; flipping the gate back
// to false restores the pre-change behavior on the next config reload.
//
// Import direction: stockpicker -> ledger -> portfolio -> sectorallocation, so
// this package cannot import stockpicker. The producer adapts its report into
// the local mirror types below (internal/stocktools is the production
// adapter); tests inject a fake.

// IndustryHitRateSummary is the subset of the canonical industry hit-rate row
// (stockpicker.IndustryWinRateSummary) that the consumption chain reads.
type IndustryHitRateSummary struct {
	IndustryID   string
	Direction    string // "buy" (default) or "avoid" (inverted semantics)
	WilsonLower  float64
	WilsonUpper  float64
	WinRate      float64
	Observations int
	// CalibrationStatus is the canonical stockpicker calibration verdict for
	// this row ("eligible" / "calibrating" / "degraded"). Only "eligible" rows
	// carry enough samples to be consumed; see IndustryHitRateCalibrationEligible.
	CalibrationStatus string
}

// IndustryHitRateReport is the subset of stockpicker.IndustryWinRateReport the
// consumption chain reads.
type IndustryHitRateReport struct {
	Industries []IndustryHitRateSummary
}

// IndustryHitRateProvider is the read-side port the consumption chain needs.
// The production binding is stocktools.SectorAllocationHitRateProvider (the
// read-only canonical aggregate); tests inject a fake.
//
// LoadIndustryWinRate must return the rows for one
// (source, condition_id, rolling_window) tuple. An empty report (no rows) is a
// normal, non-error answer: the consumer fails closed on it.
type IndustryHitRateProvider interface {
	LoadIndustryWinRate(source, conditionID, rollingWindow string) (IndustryHitRateReport, error)
}

// IndustryHitRateCalibrationEligible mirrors stockpicker.CalibrationEligible:
// the canonical caliber found enough samples (min_samples) for the row to be
// referenceable. Rows in any other status are ignored by the consumption
// chain, which is what makes "insufficient data" fail closed instead of
// degenerating into a guess.
const IndustryHitRateCalibrationEligible = "eligible"

// Decision reasons. These strings are the deterministic, log/JSON-visible
// explanation of what the consumption chain did. They are part of the
// evidence contract (capitalflow.IndustryHitRateEvidence.Reason) and are
// asserted by tests, so treat them as API.
const (
	// IndustryHitRateReasonDisabled: gate off (default). No provider call.
	IndustryHitRateReasonDisabled = "disabled"
	// IndustryHitRateReasonNoProvider: gate on but nothing registered. Wiring
	// bug, not a data problem.
	IndustryHitRateReasonNoProvider = "no_provider"
	// IndustryHitRateReasonProviderError: the provider call failed.
	IndustryHitRateReasonProviderError = "provider_error"
	// IndustryHitRateReasonNoRows: the report has no industry rows at all.
	IndustryHitRateReasonNoRows = "no_rows"
	// IndustryHitRateReasonInsufficient: rows exist but none is calibration
	// eligible (all below min_samples). Fail closed to the pre-change
	// behavior: no tilt is derived from an uncalibrated row.
	IndustryHitRateReasonInsufficient = "insufficient_calibration"
	// IndustryHitRateReasonApplied: at least one eligible row produced a tilt.
	IndustryHitRateReasonApplied = "applied"
)

// industryHitRateTiltScale maps one unit of (WilsonLower - 0.5) to a weight
// delta: a 25 percentage-point Wilson edge (0.50 -> 0.75) is worth a 0.05
// weight tilt. industryHitRateTiltCap bounds a single row's contribution so a
// single driver can never approach the Projector's exposure floor/ceiling on
// its own (ProjectionConstraints: min 0.005, max 0.50, MaxIterations 10).
const (
	industryHitRateTiltScale = 0.2
	industryHitRateTiltCap   = 0.05
)

// IndustryHitRateTilt is one L1 row's signed, additive driver delta:
// positive tilts toward the industry, negative away from it.
type IndustryHitRateTilt struct {
	IndustryID    string
	Direction     string
	WilsonLower   float64
	Observations  int
	TiltMagnitude float64
}

// IndustryHitRateDecision is the outcome of one evaluation. Reason is always
// set; Applied tells whether Tilts may be consumed. Err is non-nil only when
// the provider itself failed (Reason == provider_error).
type IndustryHitRateDecision struct {
	Applied        bool
	Reason         string
	RowsTotal      int
	RowsCalibrated int
	Tilts          []IndustryHitRateTilt
	Err            error
}

// EvaluateIndustryHitRateConsume reads the gate, asks the provider for the
// canonical industry hit-rate and returns the tilts that are safe to consume.
//
// Fail-closed rules (spec §2 of docs/specs/industry-hitrate-consumption-spec.md):
//   - gate off            -> not applied, no provider call
//   - no provider         -> not applied (wiring bug, logged at WARN)
//   - provider error      -> not applied, error returned (logged at WARN)
//   - zero rows           -> not applied (reason no_rows)
//   - no eligible row     -> not applied (reason insufficient_calibration)
//   - otherwise           -> applied, one tilt per eligible L1 row
//
// A row is consumed only when its canonical CalibrationStatus is "eligible" AND
// its IndustryID is a canonical L1 key. Nothing is imputed, interpolated or
// defaulted: a missing row simply does not tilt its industry.
func EvaluateIndustryHitRateConsume(provider IndustryHitRateProvider, source, conditionID, rollingWindow string) IndustryHitRateDecision {
	if !config.GetIndustryHitRateConsumeEnabled() {
		return IndustryHitRateDecision{Reason: IndustryHitRateReasonDisabled}
	}
	if provider == nil {
		slog.Warn("sector_allocation.industry_hit_rate_consume.degraded",
			slog.String("reason", IndustryHitRateReasonNoProvider),
			slog.String("source", source))
		return IndustryHitRateDecision{Reason: IndustryHitRateReasonNoProvider}
	}

	report, err := provider.LoadIndustryWinRate(source, conditionID, rollingWindow)
	if err != nil {
		slog.Warn("sector_allocation.industry_hit_rate_consume.degraded",
			slog.String("reason", IndustryHitRateReasonProviderError),
			slog.String("source", source),
			slog.String("error", err.Error()))
		return IndustryHitRateDecision{Reason: IndustryHitRateReasonProviderError, Err: err}
	}

	decision := IndustryHitRateDecision{RowsTotal: len(report.Industries)}
	tilts := make([]IndustryHitRateTilt, 0, len(report.Industries))
	for _, row := range report.Industries {
		id := industry.SectorID(row.IndustryID)
		if !industry.IsL1(id) {
			continue // stale/non-canonical key: never reaches the Projector
		}
		if row.CalibrationStatus != IndustryHitRateCalibrationEligible {
			continue // uncalibrated row: fail closed, do not impute
		}
		decision.RowsCalibrated++
		tilts = append(tilts, IndustryHitRateTilt{
			IndustryID:    row.IndustryID,
			Direction:     row.Direction,
			WilsonLower:   row.WilsonLower,
			Observations:  row.Observations,
			TiltMagnitude: tiltMagnitude(row.WilsonLower, row.Direction),
		})
	}
	if len(tilts) == 0 {
		reason := IndustryHitRateReasonInsufficient
		if decision.RowsTotal == 0 {
			reason = IndustryHitRateReasonNoRows
		}
		// Expected while the outcome ledger is thin: DEBUG, not WARN. The
		// durable trace is the assessment evidence block's reason field.
		slog.Debug("sector_allocation.industry_hit_rate_consume.fail_closed",
			slog.String("reason", reason),
			slog.String("source", source),
			slog.Int("rows_total", decision.RowsTotal),
			slog.Int("rows_calibrated", 0))
		return IndustryHitRateDecision{Reason: reason, RowsTotal: decision.RowsTotal}
	}

	decision.Applied = true
	decision.Reason = IndustryHitRateReasonApplied
	decision.Tilts = tilts
	return decision
}

// tiltMagnitude converts a row into its signed driver delta. "avoid" conditions
// are inverted: a high hit-rate in an avoid-condition means the industry is
// weakening, so the tilt points away from it. The result is clamped to
// +/- industryHitRateTiltCap.
func tiltMagnitude(wilsonLower float64, direction string) float64 {
	magnitude := (wilsonLower - 0.5) * industryHitRateTiltScale
	if direction == "avoid" {
		magnitude = -magnitude
	}
	return math.Max(-industryHitRateTiltCap, math.Min(industryHitRateTiltCap, magnitude))
}

// BuildIndustryHitRateTilt is the tilt-only view of
// EvaluateIndustryHitRateConsume: nil tilts whenever the decision is not
// applied (gate off, fail-closed, provider failure), so a caller that ignores
// the error still cannot consume uncalibrated evidence.
func BuildIndustryHitRateTilt(provider IndustryHitRateProvider, source, conditionID, rollingWindow string) ([]IndustryHitRateTilt, error) {
	decision := EvaluateIndustryHitRateConsume(provider, source, conditionID, rollingWindow)
	if !decision.Applied {
		return nil, decision.Err
	}
	return decision.Tilts, nil
}

// TiltToDriverMap projects tilts onto the map[industry.SectorID]float64 shape
// DriverInputs uses. Non-L1 keys are dropped defensively (the Projector
// rejects them outright), and an empty result is nil so the Projector sees a
// nil driver map (zero contribution).
func TiltToDriverMap(tilts []IndustryHitRateTilt) map[industry.SectorID]float64 {
	if len(tilts) == 0 {
		return nil
	}
	out := make(map[industry.SectorID]float64, len(tilts))
	for _, t := range tilts {
		id := industry.SectorID(t.IndustryID)
		if !industry.IsL1(id) {
			continue
		}
		out[id] = t.TiltMagnitude
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ApplyIndustryHitRateToDrivers returns drivers with the hit-rate tilts added
// to the CapitalFlow driver map.
//
// Gate off / fail-closed / provider failure: the input is returned unchanged
// and untouched (no copy, no map write), which is the byte-identical guarantee
// the byte-identity test pins down.
//
// Applied: the CapitalFlow map is copied before writing, so the caller's map is
// never mutated. Deltas are summed into existing entries: the hit-rate evidence
// and the E07 capital-flow tilt are independent evidences on the same driver,
// and the Projector normalizes the total, so dropping one of them would
// silently disable the other.
func ApplyIndustryHitRateToDrivers(drivers DriverInputs, provider IndustryHitRateProvider, source, conditionID, rollingWindow string) (DriverInputs, error) {
	decision := EvaluateIndustryHitRateConsume(provider, source, conditionID, rollingWindow)
	if !decision.Applied {
		return drivers, decision.Err
	}
	tiltMap := TiltToDriverMap(decision.Tilts)
	if tiltMap == nil {
		return drivers, nil
	}
	merged := make(map[industry.SectorID]float64, len(drivers.CapitalFlow)+len(tiltMap))
	for id, v := range drivers.CapitalFlow {
		merged[id] = v
	}
	for id, v := range tiltMap {
		merged[id] += v
	}
	drivers.CapitalFlow = merged
	return drivers, nil
}
