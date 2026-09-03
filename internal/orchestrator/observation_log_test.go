package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ===========================================================================
// PR-3c — observation-mode decision log (plan §5.1 / k3 B1).
// ===========================================================================

func eligibleAssessment(inst, beh string) *capitalflow.CapitalFlowAssessment {
	return &capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-09-04",
		CalibrationStatus: capitalflow.CalibrationCalibrating,
		Institutional:     capitalflow.DirectionalAssessment{Available: true, Direction: inst},
		Behavioral:        capitalflow.DirectionalAssessment{Available: true, Direction: beh},
	}
}

// B1 fallback cast fix: only "risk_off" maps to RiskOff; mixed /
// sector_rotation / carry_trade_unwind (macro_assessment determinePrimaryFlow
// outputs) must map to Neutral — never enum-external strings.
func TestLegacyPrimaryFlowAction_FallbackCastFix(t *testing.T) {
	cases := []struct {
		flow string
		want sectorallocation.CapitalFlowAction
	}{
		{"risk_off", sectorallocation.CapitalFlowActionRiskOff},
		{"mixed", sectorallocation.CapitalFlowActionNeutral},
		{"sector_rotation", sectorallocation.CapitalFlowActionNeutral},
		{"carry_trade_unwind", sectorallocation.CapitalFlowActionNeutral},
		{"", sectorallocation.CapitalFlowActionNeutral},
	}
	for _, tc := range cases {
		if got := legacyPrimaryFlowAction(tc.flow); got != tc.want {
			t.Errorf("legacyPrimaryFlowAction(%q) = %q, want %q", tc.flow, got, tc.want)
		}
	}
	// The full function must agree for ineligible/absent assessments.
	plan := &portfolio.SectorRotationPlan{PrimaryFlow: "sector_rotation"}
	if got := capitalFlowActionFromPlan(plan); got != sectorallocation.CapitalFlowActionNeutral {
		t.Errorf("capitalFlowActionFromPlan(sector_rotation) = %q, want neutral", got)
	}
}

func TestObservationEntryFromPlan_LabelOnlyAndGateReason(t *testing.T) {
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow:           "sector_rotation",
		CapitalFlowAssessment: eligibleAssessment("bullish", "bullish"),
	}
	legacy := legacyPrimaryFlowAction(plan.PrimaryFlow)
	observed := deriveE07CapitalFlowAction(plan.CapitalFlowAssessment)
	entry := observationEntryFromPlan(plan, legacy, observed, "2026-09-04")

	if !entry.ActionIsLabelOnly {
		t.Errorf("action_is_label_only = false, want always true in Phase 3")
	}
	if entry.ObservedAction != "risk_on" {
		t.Errorf("observed_action = %q, want risk_on (E07 consensus)", entry.ObservedAction)
	}
	if entry.LegacyAction != "neutral" {
		t.Errorf("legacy_action = %q, want neutral (B1 cast fix)", entry.LegacyAction)
	}
	if !entry.WouldHaveMutated {
		t.Errorf("would_have_mutated = false, want true (label change risk_on vs neutral)")
	}
	if entry.Reason != "gate_closed_calibrating" {
		t.Errorf("reason = %q, want gate_closed_calibrating", entry.Reason)
	}
	if entry.MapperVersion != "" {
		t.Errorf("mapper_version = %q, want empty (no action→delta mapper yet)", entry.MapperVersion)
	}
	if entry.CrossMarketAvailable {
		t.Errorf("cross_market_available = true, want false (H-CF-02 unvalidated)")
	}
}

func TestObservationEntryFromPlan_NoAssessment(t *testing.T) {
	// sector_rotation → legacy neutral (B1 fix) and no assessment → observed
	// neutral: no label change, reason carries the no-data marker.
	entry := observationEntryFromPlan(&portfolio.SectorRotationPlan{PrimaryFlow: "sector_rotation"},
		legacyPrimaryFlowAction("sector_rotation"), sectorallocation.CapitalFlowActionNeutral, "2026-09-04")
	if entry.Reason != "no_assessment" {
		t.Errorf("reason = %q, want no_assessment", entry.Reason)
	}
	if entry.WouldHaveMutated {
		t.Errorf("would_have_mutated = true, want false (neutral vs neutral)")
	}
}

func TestJSONLObservationLogger_AppendAndReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "capital_flow_observation.jsonl")
	logger := NewJSONLObservationLogger(path)

	entries := []ObservationEntry{
		{AsOfTradingDate: "2026-09-01", LegacyAction: "neutral", ObservedAction: "risk_on",
			WouldHaveMutated: true, ActionIsLabelOnly: true, Reason: "gate_closed_calibrating"},
		{AsOfTradingDate: "2026-09-02", LegacyAction: "neutral", ObservedAction: "neutral",
			WouldHaveMutated: false, ActionIsLabelOnly: true, Reason: "gate_closed_calibrating"},
		{AsOfTradingDate: "2026-09-03", LegacyAction: "risk_off", ObservedAction: "risk_off",
			WouldHaveMutated: false, ActionIsLabelOnly: true, Reason: "gate_closed_calibrating"},
	}
	for _, e := range entries {
		logger.Observe(e)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	firstLine := string(raw)
	if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
		firstLine = firstLine[:i]
	}
	var parsed ObservationEntry
	if err := json.Unmarshal([]byte(firstLine), &parsed); err != nil {
		t.Fatalf("first line not valid JSON: %v", err)
	}

	rep, err := BuildObservationReport(path)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if rep.TotalEntries != 3 {
		t.Errorf("TotalEntries = %d, want 3", rep.TotalEntries)
	}
	if rep.LabelChanges != 1 {
		t.Errorf("LabelChanges = %d, want 1", rep.LabelChanges)
	}
	if !rep.AllLabelOnly {
		t.Errorf("AllLabelOnly = false, want true")
	}
	if rep.FirstDate != "2026-09-01" || rep.LastDate != "2026-09-03" {
		t.Errorf("window = %s..%s, want 2026-09-01..2026-09-03", rep.FirstDate, rep.LastDate)
	}
	if rep.ObservedCounts["risk_on"] != 1 || rep.ObservedCounts["neutral"] != 1 || rep.ObservedCounts["risk_off"] != 1 {
		t.Errorf("ObservedCounts = %v", rep.ObservedCounts)
	}
}

func TestStrategyEvolver_ObserverDoesNotBlockApply(t *testing.T) {
	// A panicking observer must not break the rotation path: observation is
	// best-effort by contract. (We only assert the wiring compiles and the
	// observer is called — failure softness is enforced inside the JSONL
	// logger by dropping errors with warnings.)
	var called int
	logger := observerFunc(func(e ObservationEntry) { called++ })
	e := NewStrategyEvolver()
	e.WithCapitalFlowObserver(logger)
	if e.cfObserver == nil {
		t.Fatal("cfObserver not wired")
	}
	e.cfObserver.Observe(ObservationEntry{})
	if called != 1 {
		t.Fatalf("observer called %d times, want 1", called)
	}
}

// observerFunc adapts a function to ObservationLogger (test helper).
type observerFunc func(ObservationEntry)

func (f observerFunc) Observe(e ObservationEntry) { f(e) }
