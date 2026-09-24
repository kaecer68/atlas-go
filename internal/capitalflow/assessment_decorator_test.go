package capitalflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

// TestRegisterAssessmentDecorator_NilNoop: a nil decorator is ignored instead
// of panicking at request time.
func TestRegisterAssessmentDecorator_NilNoop(t *testing.T) {
	defer capitalflow.ResetAssessmentDecorators()
	capitalflow.RegisterAssessmentDecorator(nil)
	capitalflow.ResetAssessmentDecorators()
}

// TestRegisterAssessmentDecorator_AppendsAndResets: the registry holds several
// decorators and ResetAssessmentDecorators clears them (the wiring owner's
// rollback path).
func TestRegisterAssessmentDecorator_AppendsAndResets(t *testing.T) {
	t.Cleanup(capitalflow.ResetAssessmentDecorators)
	capitalflow.RegisterAssessmentDecorator(func(*capitalflow.CapitalFlowAssessment) {})
	capitalflow.RegisterAssessmentDecorator(func(*capitalflow.CapitalFlowAssessment) {})
	capitalflow.ResetAssessmentDecorators()
}

// TestCapitalFlowAssessment_IndustryHitRateEvidence_OmittedWhenNil pins the
// assessment-side byte-identity guarantee: the new evidence key must not appear
// in the JSON while the consumption gate is off (evidence nil).
func TestCapitalFlowAssessment_IndustryHitRateEvidence_OmittedWhenNil(t *testing.T) {
	encoded, err := json.Marshal(capitalflow.CapitalFlowAssessment{})
	if err != nil {
		t.Fatalf("marshal assessment: %v", err)
	}
	if strings.Contains(string(encoded), "industry_hit_rate_evidence") {
		t.Fatalf("nil evidence must be omitted, got %s", encoded)
	}
}

// TestCapitalFlowAssessment_IndustryHitRateEvidence_SerializesWhenSet pins the
// wire contract consumed by /api/capital-flow/daily.
func TestCapitalFlowAssessment_IndustryHitRateEvidence_SerializesWhenSet(t *testing.T) {
	assessment := capitalflow.CapitalFlowAssessment{
		IndustryHitRateEvidence: &capitalflow.IndustryHitRateEvidence{
			Source:         "stockpicker-momentum-20d-positive",
			ConditionID:    "momentum-20d-positive",
			RollingWindow:  "120d",
			Applied:        false,
			Reason:         "insufficient_calibration",
			RowsTotal:      3,
			RowsCalibrated: 0,
		},
	}
	encoded, err := json.Marshal(assessment)
	if err != nil {
		t.Fatalf("marshal assessment: %v", err)
	}
	if !strings.Contains(string(encoded), `"industry_hit_rate_evidence"`) {
		t.Fatalf("evidence must be serialized when set, got %s", encoded)
	}
	for _, key := range []string{`"applied":false`, `"reason":"insufficient_calibration"`, `"rows_total":3`, `"rows_calibrated":0`} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("missing %s in %s", key, encoded)
		}
	}
}
