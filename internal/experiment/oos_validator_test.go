package experiment

import (
	"testing"
	"time"

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

	expectedStart := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	if !result.OOSWindowStart.Equal(expectedStart) {
		t.Errorf("expected OOSWindowStart %v, got %v", expectedStart, result.OOSWindowStart)
	}

	expectedEnd := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
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
	threshold := oosAcceptanceThreshold()
	if threshold <= 0 {
		t.Errorf("expected positive threshold, got %f", threshold)
	}
}

func TestOOSMinimumObservations(t *testing.T) {
	minObs := oosMinimumObservations()
	if minObs <= 0 {
		t.Errorf("expected positive minimum observations, got %d", minObs)
	}
}
