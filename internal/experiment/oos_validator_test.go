package experiment

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestOOSValidator_NewOOSValidator(t *testing.T) {
	validator := NewOOSValidator(nil, "/path/to/replay")
	if validator == nil {
		t.Fatal("NewOOSValidator returned nil")
	}
	if validator.replayDataPath != "/path/to/replay" {
		t.Errorf("expected replayDataPath /path/to/replay, got %s", validator.replayDataPath)
	}
}

func TestOOSValidator_Validate_OOSWindow(t *testing.T) {
	validator := NewOOSValidator(nil, "/path/to/replay")
	primaryEnd := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	result, err := validator.Validate("/candidate/path", "/baseline/path", primaryEnd)
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Validate returned nil result")
	}

	expectedStart := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	if !result.OOSWindowStart.Equal(expectedStart) {
		t.Errorf("expected OOSWindowStart %v, got %v", expectedStart, result.OOSWindowStart)
	}

	// OOS end = primaryEnd + 1 (gap) + 5 (embargo) + 30 (OOS length) = 2026-04-20
	expectedEnd := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if !result.OOSWindowEnd.Equal(expectedEnd) {
		t.Errorf("expected OOSWindowEnd %v, got %v", expectedEnd, result.OOSWindowEnd)
	}
}

func TestOOSValidator_Validate_GracefulDegradation(t *testing.T) {
	validator := NewOOSValidator(nil, "/nonexistent/replay")
	primaryEnd := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	result, err := validator.Validate("/candidate/path", "/baseline/path", primaryEnd)
	if err != nil {
		t.Fatalf("Validate should not return error for bad path: %v", err)
	}
	if result == nil {
		t.Fatal("Validate returned nil result")
	}
	if result.Passed {
		t.Error("expected Passed=false when dataset unavailable")
	}
}

func TestOOSValidator_ValidateWithBrief(t *testing.T) {
	validator := NewOOSValidator(nil, "/path/to/replay")
	primaryEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	brief := domain.MutationBrief{
		TargetAgentID: "test_agent",
		TargetSkill:   "test_skill",
		MutationType:  "prompt_tightening",
	}

	result, err := validator.ValidateWithBrief("/candidate", "/baseline", brief, primaryEnd)
	if err != nil {
		t.Fatalf("ValidateWithBrief should not return error for bad path: %v", err)
	}
	if result == nil {
		t.Fatal("ValidateWithBrief returned nil result")
	}

	if result.Passed {
		t.Error("expected Passed=false when dataset unavailable")
	}
}

func TestOOSValidator_ValidateWithConstraints(t *testing.T) {
	validator := NewOOSValidator(nil, "/path/to/replay")
	primaryEnd := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)

	brief := domain.MutationBrief{
		TargetAgentID: "test_agent",
		TargetSkill:   "test_skill",
		MutationType:  "portfolio_constraint_revision",
	}

	result, err := validator.ValidateWithConstraints("/candidate_constraints", "/baseline_constraints", brief, primaryEnd)
	if err != nil {
		t.Fatalf("ValidateWithConstraints should not return error for bad path: %v", err)
	}
	if result == nil {
		t.Fatal("ValidateWithConstraints returned nil result")
	}

	if result.Passed {
		t.Error("expected Passed=false when dataset unavailable")
	}
}

func TestOOSAcceptanceThreshold(t *testing.T) {
	v := NewOOSValidator(nil, "")
	threshold := v.params.Experiment.ImprovementThreshold.Value
	if threshold <= 0 {
		t.Errorf("expected positive threshold, got %f", threshold)
	}
}

func TestOOSMinimumObservations(t *testing.T) {
	v := NewOOSValidator(nil, "")
	minObs := v.params.Experiment.MaturityLevel1Observations.Value
	if minObs <= 0 {
		t.Errorf("expected positive minimum observations, got %d", minObs)
	}
}

// TestOOSEmbargo_DefaultFiveDays verifies the default walk-forward embargo
// (5 days) is applied: OOS window starts primaryEnd + 1 + 5 = primaryEnd + 6.
func TestOOSEmbargo_DefaultFiveDays(t *testing.T) {
	v := NewOOSValidator(nil, "")
	v.WithParameters(config.DefaultParametersConfig())
	primaryEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	start, end := v.oosWindow(primaryEnd)

	wantStart := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC) // +6 days
	if !start.Equal(wantStart) {
		t.Errorf("default embargo: start = %v, want %v", start, wantStart)
	}
	// OOSWindowDays default is 30, so end = start + 30 days
	wantEnd := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Errorf("default embargo: end = %v, want %v", end, wantEnd)
	}
}

// TestOOSEmbargo_ZeroFallsBackToFive protects the no-leakage invariant: a
// zero embargo (which would mean OOS starts immediately after train end and
// risks leakage) must NOT be honored — fall back to the 5-day default.
func TestOOSEmbargo_ZeroFallsBackToFive(t *testing.T) {
	v := NewOOSValidator(nil, "")
	cfg := config.DefaultParametersConfig()
	cfg.Experiment.WalkForwardEmbargoDays.Value = 0
	v.WithParameters(cfg)

	primaryEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	start, _ := v.oosWindow(primaryEnd)

	wantStart := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC) // +6 days (5-day fallback)
	if !start.Equal(wantStart) {
		t.Errorf("zero embargo fallback: start = %v, want %v", start, wantStart)
	}
}

// TestOOSEmbargo_CustomTenDays verifies a positive custom value is honored.
func TestOOSEmbargo_CustomTenDays(t *testing.T) {
	v := NewOOSValidator(nil, "")
	cfg := config.DefaultParametersConfig()
	cfg.Experiment.WalkForwardEmbargoDays.Value = 10
	v.WithParameters(cfg)

	primaryEnd := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	start, _ := v.oosWindow(primaryEnd)

	wantStart := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC) // +11 days (1 + 10)
	if !start.Equal(wantStart) {
		t.Errorf("custom 10-day embargo: start = %v, want %v", start, wantStart)
	}
}

// TestOOSEmbargo_NegativeFallsBackToFive ensures a misconfigured negative
// embargo is treated like zero and does not produce an OOS window that
// overlaps or precedes the primary window.
func TestOOSEmbargo_NegativeFallsBackToFive(t *testing.T) {
	v := NewOOSValidator(nil, "")
	cfg := config.DefaultParametersConfig()
	cfg.Experiment.WalkForwardEmbargoDays.Value = -3
	v.WithParameters(cfg)

	primaryEnd := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	start, _ := v.oosWindow(primaryEnd)

	// Negative should fall back to 5, giving start = primaryEnd + 6
	wantStart := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("negative embargo fallback: start = %v, want %v", start, wantStart)
	}
	// Invariant: OOS must always start strictly after primary window end
	if !start.After(primaryEnd) {
		t.Errorf("OOS start %v must be after primary end %v", start, primaryEnd)
	}
}
