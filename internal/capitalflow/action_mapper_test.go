package capitalflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

func TestNoOpCapitalFlowActionMapper_VersionIsEmpty(t *testing.T) {
	var m capitalflow.CapitalFlowActionMapper = capitalflow.NoOpCapitalFlowActionMapper{}
	if m.MapperVersion() != "" {
		t.Fatalf("NoOp mapper version must be empty (disabled), got %q", m.MapperVersion())
	}
}

func TestNoOpCapitalFlowActionMapper_MapReturnsUnavailable(t *testing.T) {
	m := capitalflow.NoOpCapitalFlowActionMapper{}
	a := capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-07-17",
		CalibrationStatus: capitalflow.CalibrationEligible,
		PrimaryFlow:       "risk_off",
	}
	action, _, err := m.Map(context.Background(), time.Now(), a)
	if err != nil {
		t.Fatalf("NoOp.Map failed: %v", err)
	}
	if action != capitalflow.CapitalFlowActionUnavailable {
		t.Fatalf("NoOp.Map must return unavailable, got %q", action)
	}
}

func TestDefaultCapitalFlowActionMapper_EmptyVersionReturnsUnavailable(t *testing.T) {
	m := capitalflow.DefaultCapitalFlowActionMapper{Version: ""}
	a := capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-07-17",
		CalibrationStatus: capitalflow.CalibrationEligible,
		PrimaryFlow:       "risk_on",
	}
	action, _, err := m.Map(context.Background(), time.Now(), a)
	if err != nil {
		t.Fatalf("Default.Map failed: %v", err)
	}
	if action != capitalflow.CapitalFlowActionUnavailable {
		t.Fatalf("Default with empty version must return unavailable, got %q", action)
	}
}

func TestDefaultCapitalFlowActionMapper_CalibratingReturnsUnavailable(t *testing.T) {
	m := capitalflow.DefaultCapitalFlowActionMapper{Version: "v1.0.0-walkforward"}
	a := capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-07-17",
		CalibrationStatus: capitalflow.CalibrationCalibrating,
		PrimaryFlow:       "risk_on",
	}
	action, _, err := m.Map(context.Background(), time.Now(), a)
	if err != nil {
		t.Fatalf("Default.Map failed: %v", err)
	}
	if action != capitalflow.CapitalFlowActionUnavailable {
		t.Fatalf("Default with calibrating assessment must return unavailable (spec §6.2), got %q", action)
	}
}

func TestDefaultCapitalFlowActionMapper_EligibleReturnsNeutral(t *testing.T) {
	m := capitalflow.DefaultCapitalFlowActionMapper{Version: "v1.0.0-walkforward"}
	a := capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-07-17",
		CalibrationStatus: capitalflow.CalibrationEligible,
		PrimaryFlow:       "",
	}
	action, tilt, err := m.Map(context.Background(), time.Now(), a)
	if err != nil {
		t.Fatalf("Default.Map failed: %v", err)
	}
	if action != capitalflow.CapitalFlowActionNeutral {
		t.Fatalf("Default with eligible but no strong signal must return neutral, got %q", action)
	}
	if len(tilt) != 0 {
		t.Fatalf("Default must return empty tilt for neutral (no speculation), got %d keys", len(tilt))
	}
}

func TestCapitalFlowActionMapper_InterfaceCompliance(t *testing.T) {
	var _ capitalflow.CapitalFlowActionMapper = capitalflow.NoOpCapitalFlowActionMapper{}
	var _ capitalflow.CapitalFlowActionMapper = capitalflow.DefaultCapitalFlowActionMapper{Version: "v1.0.0"}
}
