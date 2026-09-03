// Package orchestr — observation-mode decision log for the capital-flow
// action (PR-3c, capital-flow model plan v1.1 §5.1; k3 review B1).
//
// Red lines:
//   - Record-only. The observed action NEVER changes ComputeProjectedTarget
//     inputs: the action is label-only (DriverProvenance["capital_flow"],
//     projector.go) — weights come from DriverInputs.CapitalFlow (delta map
//     from FactorInputProvider), not from capitalFlowActionFromPlan.
//   - A failed log append MUST NOT block the decision path (warn only).
//   - No mutation switch exists. A future action→delta mapper design PR
//     introduces capitalflow.mutation_enabled (default off).
package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ObservationEntry is one JSONL line of
// data/state/capital_flow_observation.jsonl (plan §5.1 schema).
//
// would_have_mutated semantics (B1 correction): the terminal decision is
// decision-inert, so this field only means "the E07 branch's returned LABEL
// differs from the legacy label" — not that any allocation would change.
// action_is_label_only is always true and documents that the action only
// reaches provenance.
type ObservationEntry struct {
	AsOfTradingDate string `json:"as_of_trading_date"`
	// Period is the seven-period classification when the caller can supply
	// it; empty when unknown at this layer (best-effort, never guessed).
	Period                      string `json:"period,omitempty"`
	AssessmentCalibrationStatus string `json:"assessment_calibration_status"`
	// LegacyAction is what the legacy fallback would have produced after
	// the B1 cast fix (risk_off → risk_off, everything else → neutral).
	LegacyAction string `json:"legacy_action"`
	// ObservedAction is the action the E07 branch WOULD return if the
	// calibration gate were open (label-only observation).
	ObservedAction string `json:"observed_action"`
	// WouldHaveMutated reports observed != legacy (label change only).
	WouldHaveMutated bool `json:"would_have_mutated"`
	// ActionIsLabelOnly is always true in Phase 3 (decision-inert).
	ActionIsLabelOnly           bool   `json:"action_is_label_only"`
	InstitutionalDirection      string `json:"institutional_direction,omitempty"`
	BehavioralDirection         string `json:"behavioral_direction,omitempty"`
	ForeignPositioningDirection string `json:"foreign_positioning_direction,omitempty"`
	CrossMarketAvailable        bool   `json:"cross_market_available"`
	Reason                      string `json:"reason"`
	// MapperVersion is empty: no action→delta mapper exists yet.
	MapperVersion string `json:"mapper_version"`
	RecordedAt    string `json:"recorded_at"`
}

// ObservationLogger records one observation entry. Implementations must be
// safe for concurrent use and must never propagate write failures to the
// decision path.
type ObservationLogger interface {
	Observe(entry ObservationEntry)
}

// JSONLObservationLogger appends entries as JSON lines. A shared file is
// append-only; a per-process mutex guards interleaved writes.
type JSONLObservationLogger struct {
	mu   sync.Mutex
	path string
}

// NewJSONLObservationLogger creates a logger appending to path (parent
// directories are created lazily on first write).
func NewJSONLObservationLogger(path string) *JSONLObservationLogger {
	return &JSONLObservationLogger{path: path}
}

// Observe appends one JSON line. Failures are logged as warnings and
// dropped — observation never blocks the decision path (plan §5.5).
func (l *JSONLObservationLogger) Observe(entry ObservationEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		logging.Warn("observation_log", "mkdir_failed", "path", l.path, "err", err.Error())
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logging.Warn("observation_log", "open_failed", "path", l.path, "err", err.Error())
		return
	}
	defer func() { _ = f.Close() }()

	if entry.RecordedAt == "" {
		entry.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		logging.Warn("observation_log", "marshal_failed", "err", err.Error())
		return
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		logging.Warn("observation_log", "append_failed", "path", l.path, "err", err.Error())
	}
}

// ObservationReport is the read-only 30-trading-day evaluation over a
// populated observation JSONL (plan §5.1): label-agreement statistics
// between observed (E07-if-eligible) and legacy actions. The next-stage
// mutation PR additionally requires the Phase 1 H-CF-05 next-day TAIEX
// hit-rate and the eventdriven prediction hit-rate (k3 B2) — those live in
// cmd/validate-capital-flow-hypotheses, not here.
type ObservationReport struct {
	TotalEntries   int            `json:"total_entries"`
	LabelChanges   int            `json:"label_changes"`
	ObservedCounts map[string]int `json:"observed_counts"`
	LegacyCounts   map[string]int `json:"legacy_counts"`
	// FirstDate / LastDate bound the observation window (empty when no data).
	FirstDate string `json:"first_date,omitempty"`
	LastDate  string `json:"last_date,omitempty"`
	// AllLabelOnly verifies every entry has action_is_label_only=true.
	AllLabelOnly bool `json:"all_label_only"`
}

// BuildObservationReport reads the JSONL at path and aggregates the
// label-agreement report. Read-only; returns an empty report with error
// when the file cannot be read.
func BuildObservationReport(path string) (ObservationReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return ObservationReport{}, fmt.Errorf("observation report: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var rep ObservationReport
	rep.ObservedCounts = map[string]int{}
	rep.LegacyCounts = map[string]int{}
	rep.AllLabelOnly = true
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e ObservationEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return rep, fmt.Errorf("observation report: parse %s: %w", path, err)
		}
		rep.TotalEntries++
		if !e.ActionIsLabelOnly {
			rep.AllLabelOnly = false
		}
		if e.WouldHaveMutated {
			rep.LabelChanges++
		}
		rep.ObservedCounts[e.ObservedAction]++
		rep.LegacyCounts[e.LegacyAction]++
		if rep.FirstDate == "" {
			rep.FirstDate = e.AsOfTradingDate
		}
		rep.LastDate = e.AsOfTradingDate
	}
	if err := sc.Err(); err != nil {
		return rep, fmt.Errorf("observation report: scan %s: %w", path, err)
	}
	return rep, nil
}

// observationEntryFromPlan builds the §5.1 entry from the plan and the two
// action derivations (legacy vs observed-if-eligible). Kept in one place so
// strategy_evolver.go stays thin.
func observationEntryFromPlan(plan *portfolio.SectorRotationPlan, legacy, observed sectorallocation.CapitalFlowAction, asOfDate string) ObservationEntry {
	entry := ObservationEntry{
		AsOfTradingDate:   asOfDate,
		LegacyAction:      string(legacy),
		ObservedAction:    string(observed),
		WouldHaveMutated:  observed != legacy,
		ActionIsLabelOnly: true,
		Reason:            "no_assessment",
	}
	if plan == nil || plan.CapitalFlowAssessment == nil {
		return entry
	}
	a := plan.CapitalFlowAssessment
	entry.AsOfTradingDate = firstNonEmpty(a.AsOfTradingDate, asOfDate)
	entry.AssessmentCalibrationStatus = a.CalibrationStatus
	entry.InstitutionalDirection = a.Institutional.Direction
	entry.BehavioralDirection = a.Behavioral.Direction
	entry.ForeignPositioningDirection = a.ForeignPosition.Direction
	entry.CrossMarketAvailable = a.CrossMarket.Available
	if a.EligibleForAutomation() {
		entry.Reason = "eligible"
	} else {
		entry.Reason = "gate_closed_" + a.CalibrationStatus
	}
	return entry
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
