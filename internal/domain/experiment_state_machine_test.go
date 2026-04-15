package domain

import (
	"strings"
	"testing"
)

func TestTransitionExperimentStatus_ValidTransitions(t *testing.T) {
	record := &ExperimentRecord{Status: ExperimentPlanned}

	if err := TransitionExperimentStatus(record, ExperimentRunning); err != nil {
		t.Fatalf("planned -> running should be valid: %v", err)
	}
	if err := TransitionExperimentStatus(record, ExperimentAccepted); err != nil {
		t.Fatalf("running -> accepted should be valid: %v", err)
	}
}

func TestTransitionExperimentStatus_InvalidTransition(t *testing.T) {
	record := &ExperimentRecord{Status: ExperimentAccepted}

	err := TransitionExperimentStatus(record, ExperimentRunning)
	if err == nil {
		t.Fatalf("accepted -> running should be invalid")
	}
	if !strings.Contains(err.Error(), "invalid experiment status transition") {
		t.Fatalf("expected transition error message, got %v", err)
	}
}

func TestCanTransitionExperimentStatus_InitialState(t *testing.T) {
	if !CanTransitionExperimentStatus("", ExperimentPlanned) {
		t.Fatalf("empty -> planned should be valid")
	}
	if !CanTransitionExperimentStatus("", ExperimentRunning) {
		t.Fatalf("empty -> running should be valid")
	}
	if CanTransitionExperimentStatus("", ExperimentAccepted) {
		t.Fatalf("empty -> accepted should be invalid")
	}
}

func TestTransitionExperimentStatus_ExpiredTransitions(t *testing.T) {
	planned := &ExperimentRecord{Status: ExperimentPlanned}
	if err := TransitionExperimentStatus(planned, ExperimentExpired); err != nil {
		t.Fatalf("planned -> expired should be valid: %v", err)
	}

	running := &ExperimentRecord{Status: ExperimentRunning}
	if err := TransitionExperimentStatus(running, ExperimentExpired); err != nil {
		t.Fatalf("running -> expired should be valid: %v", err)
	}

	accepted := &ExperimentRecord{Status: ExperimentAccepted}
	if err := TransitionExperimentStatus(accepted, ExperimentExpired); err == nil {
		t.Fatalf("accepted -> expired should be invalid")
	}
}
