package sectorallocation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
)

// withHitRateConfig flips the config gate for the in-package tests and restores
// the previous parameters singleton on cleanup (same pattern as the external
// tests' withHitRateGate).
func withHitRateConfig(t *testing.T, enabled bool) {
	t.Helper()
	prevPath := config.GetParametersConfigPath()
	cfg := config.DefaultParametersConfig()
	cfg.SectorAllocation.IndustryHitRateConsumeEnabled.Value = enabled
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal test parameters: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test parameters: %v", err)
	}
	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})
	if got := config.GetIndustryHitRateConsumeEnabled(); got != enabled {
		t.Fatalf("gate did not load: got %v, want %v", got, enabled)
	}
}

// These are in-package tests: the decorator and the evidence summary are
// package-private on purpose (only init() may register the decorator, and only
// this package knows the production query tuple).

type stubHitRateProvider struct{ rows []IndustryHitRateSummary }

func (s stubHitRateProvider) LoadIndustryWinRate(_, _, _ string) (IndustryHitRateReport, error) {
	return IndustryHitRateReport{Industries: s.rows}, nil
}

// countingHitRateProvider is stubHitRateProvider plus a read counter, so the
// demoted-condition test can prove the chain refuses BEFORE the provider read.
type countingHitRateProvider struct {
	rows  []IndustryHitRateSummary
	calls int
}

func (p *countingHitRateProvider) LoadIndustryWinRate(_, _, _ string) (IndustryHitRateReport, error) {
	p.calls++
	return IndustryHitRateReport{Industries: p.rows}, nil
}

func eligibleStubRow(id string, wilson float64) IndustryHitRateSummary {
	return IndustryHitRateSummary{
		IndustryID:        id,
		Direction:         "buy",
		WilsonLower:       wilson,
		WilsonUpper:       wilson + 0.1,
		Observations:      120,
		CalibrationStatus: IndustryHitRateCalibrationEligible,
	}
}

// TestIndustryHitRateAssessmentDecorator_GateOff_LeavesEvidenceNil is the
// assessment-side half of the byte-identity guarantee: with the default config
// the evidence block stays absent (nil => `omitempty` drops the key).
func TestIndustryHitRateAssessmentDecorator_GateOff_LeavesEvidenceNil(t *testing.T) {
	withHitRateConfig(t, false)
	RegisterIndustryHitRateProvider(stubHitRateProvider{rows: []IndustryHitRateSummary{eligibleStubRow("semiconductor", 0.9)}})
	t.Cleanup(ResetIndustryHitRateProvider)

	assessment := capitalflow.CapitalFlowAssessment{}
	industryHitRateAssessmentDecorator(&assessment)

	if assessment.IndustryHitRateEvidence != nil {
		t.Fatalf("gate off must leave the evidence nil, got %+v", assessment.IndustryHitRateEvidence)
	}
}

// The next two tests drive the fail-closed reasons through a RETAINED
// condition instead of the decorator: since 2026-10-06 the decorator's
// production tuple (stockpicker-momentum-20d-positive) is demoted, so the
// decorator short-circuits at the demoted guard (see
// TestIndustryHitRateAssessmentDecorator_GateOn_DemotedCondition_Refuses) and
// can no longer reach the insufficient_calibration / no_provider branches. The
// branches themselves are unchanged and stay covered here through the same
// evaluate + evidence-summary functions the decorator calls.
const (
	retainedHitRateSource    = "stockpicker-price-volume-bottom-divergence"
	retainedHitRateCondition = "price-volume-bottom-divergence"
)

func TestIndustryHitRateEvidence_GateOn_FailClosed_ReportsReason(t *testing.T) {
	withHitRateConfig(t, true)
	row := eligibleStubRow("semiconductor", 0.9)
	row.CalibrationStatus = "calibrating"
	decision := EvaluateIndustryHitRateConsume(
		stubHitRateProvider{rows: []IndustryHitRateSummary{row}},
		retainedHitRateSource, retainedHitRateCondition, "120d")

	ev := buildIndustryHitRateEvidence(decision)
	if ev == nil {
		t.Fatal("gate on must always publish the evidence block (applied or fail-closed)")
	}
	if ev.Applied {
		t.Errorf("thin evidence must not be applied: %+v", ev)
	}
	if ev.Reason != IndustryHitRateReasonInsufficient {
		t.Errorf("reason = %q, want %q", ev.Reason, IndustryHitRateReasonInsufficient)
	}
	if ev.RowsTotal != 1 || ev.RowsCalibrated != 0 {
		t.Errorf("rows_total/rows_calibrated = %d/%d, want 1/0", ev.RowsTotal, ev.RowsCalibrated)
	}
	if ev.MaxTilt != 0 || ev.MinTilt != 0 || ev.MeanWilsonLower != 0 {
		t.Errorf("fail-closed evidence must not carry tilt stats: %+v", ev)
	}
}

func TestIndustryHitRateEvidence_GateOn_NoProvider_ReportsReason(t *testing.T) {
	withHitRateConfig(t, true)

	ev := buildIndustryHitRateEvidence(EvaluateIndustryHitRateConsume(
		nil, retainedHitRateSource, retainedHitRateCondition, "120d"))

	if ev == nil || ev.Reason != IndustryHitRateReasonNoProvider {
		t.Fatalf("evidence = %+v, want reason no_provider", ev)
	}
}

// TestIndustryHitRateAssessmentDecorator_GateOn_DemotedCondition_Refuses
// (2026-10-06) replaces the former "applied" decorator test: the production
// query tuple consumes stockpicker-momentum-20d-positive, which is demoted for
// negative net-cost expectancy (-0.987%, t=-21.3). With the gate on the
// decorator must now publish the explicit demoted_condition reason instead of
// tilting real sector weights from a measured loser. The tilt-summary half of
// the old coverage lives in
// TestBuildIndustryHitRateEvidence_SummarizesTilts (retained condition).
func TestIndustryHitRateAssessmentDecorator_GateOn_DemotedCondition_Refuses(t *testing.T) {
	withHitRateConfig(t, true)
	provider := &countingHitRateProvider{rows: []IndustryHitRateSummary{
		eligibleStubRow("semiconductor", 0.7),
		eligibleStubRow("financials", 0.3),
	}}
	RegisterIndustryHitRateProvider(provider)
	t.Cleanup(ResetIndustryHitRateProvider)

	assessment := capitalflow.CapitalFlowAssessment{CalibrationStatus: "calibrating"}
	industryHitRateAssessmentDecorator(&assessment)

	ev := assessment.IndustryHitRateEvidence
	if ev == nil {
		t.Fatal("gate on must always publish the evidence block (applied or fail-closed)")
	}
	if ev.Applied {
		t.Errorf("demoted condition must not be applied: %+v", ev)
	}
	if ev.Reason != IndustryHitRateReasonDemotedCondition {
		t.Errorf("reason = %q, want %q", ev.Reason, IndustryHitRateReasonDemotedCondition)
	}
	if provider.calls != 0 {
		t.Errorf("demoted condition must be refused before the provider read, calls = %d", provider.calls)
	}
	if ev.MaxTilt != 0 || ev.MinTilt != 0 || ev.MeanWilsonLower != 0 {
		t.Errorf("refused evidence must not carry tilt stats: %+v", ev)
	}
	if ev.Source != industryHitRateSource || ev.ConditionID != industryHitRateCondition || ev.RollingWindow != industryHitRateRollingWindow {
		t.Errorf("evidence query tuple = %s/%s/%s, want %s/%s/%s",
			ev.Source, ev.ConditionID, ev.RollingWindow,
			industryHitRateSource, industryHitRateCondition, industryHitRateRollingWindow)
	}
	// Advisory only: the decorator must not touch the calibration verdict.
	if assessment.CalibrationStatus != "calibrating" {
		t.Errorf("decorator changed CalibrationStatus to %q", assessment.CalibrationStatus)
	}
}

// TestBuildIndustryHitRateEvidence_SummarizesTilts keeps the tilt-summary
// coverage of the former applied-decorator test, driven directly with a
// RETAINED condition so the summary math stays exercised even though the
// production tuple is demoted.
func TestBuildIndustryHitRateEvidence_SummarizesTilts(t *testing.T) {
	withHitRateConfig(t, true)
	provider := stubHitRateProvider{rows: []IndustryHitRateSummary{
		eligibleStubRow("semiconductor", 0.7), // +0.04
		eligibleStubRow("financials", 0.3),    // -0.04
	}}
	decision := EvaluateIndustryHitRateConsume(provider, retainedHitRateSource, retainedHitRateCondition, "120d")
	ev := buildIndustryHitRateEvidence(decision)

	if ev == nil || !ev.Applied || ev.Reason != IndustryHitRateReasonApplied {
		t.Fatalf("evidence = %+v, want applied", ev)
	}
	if ev.RowsTotal != 2 || ev.RowsCalibrated != 2 {
		t.Errorf("rows_total/rows_calibrated = %d/%d, want 2/2", ev.RowsTotal, ev.RowsCalibrated)
	}
	if ev.MaxTilt != 0.04 || ev.MinTilt != -0.04 {
		t.Errorf("tilt bounds = %v/%v, want 0.04/-0.04", ev.MaxTilt, ev.MinTilt)
	}
	if ev.MeanWilsonLower != 0.5 {
		t.Errorf("mean WilsonLower = %v, want 0.5", ev.MeanWilsonLower)
	}
	// The evidence block always echoes the PRODUCTION tuple (that is
	// buildIndustryHitRateEvidence's contract: it reports what the chain was
	// configured to consume), even though the decision above was driven from a
	// retained condition to keep the tilt math exercised.
	if ev.Source != industryHitRateSource || ev.ConditionID != industryHitRateCondition || ev.RollingWindow != industryHitRateRollingWindow {
		t.Errorf("evidence query tuple = %s/%s/%s, want %s/%s/%s",
			ev.Source, ev.ConditionID, ev.RollingWindow,
			industryHitRateSource, industryHitRateCondition, industryHitRateRollingWindow)
	}
}
