package capitalflow

import "testing"

// TestApplyAssessmentDecorators_NoDecoratorIsExactNoop proves the config-off
// path is a true no-op: with no decorator registered the assessment comes back
// unchanged (no copy is observable, no field is touched).
func TestApplyAssessmentDecorators_NoDecoratorIsExactNoop(t *testing.T) {
	ResetAssessmentDecorators()
	assessment := CapitalFlowAssessment{PrimaryFlow: "foreign", CalibrationStatus: "calibrating"}

	got := applyAssessmentDecorators(assessment)

	if got.PrimaryFlow != assessment.PrimaryFlow || got.CalibrationStatus != assessment.CalibrationStatus {
		t.Fatalf("no-op path changed the assessment: %+v -> %+v", assessment, got)
	}
	if got.IndustryHitRateEvidence != nil {
		t.Fatalf("no-op path added evidence: %+v", got.IndustryHitRateEvidence)
	}
}

// TestApplyAssessmentDecorators_RunsRegisteredDecorators covers the hook the
// industry hit-rate chain (sectorallocation) plugs into.
func TestApplyAssessmentDecorators_RunsRegisteredDecorators(t *testing.T) {
	t.Cleanup(ResetAssessmentDecorators)
	ResetAssessmentDecorators()
	RegisterAssessmentDecorator(func(a *CapitalFlowAssessment) {
		a.IndustryHitRateEvidence = &IndustryHitRateEvidence{Source: "first", Applied: true, Reason: "applied"}
	})
	RegisterAssessmentDecorator(func(a *CapitalFlowAssessment) {
		a.IndustryHitRateEvidence.RowsTotal = 7
	})

	got := applyAssessmentDecorators(CapitalFlowAssessment{})

	if got.IndustryHitRateEvidence == nil {
		t.Fatal("decorator did not run")
	}
	if got.IndustryHitRateEvidence.Source != "first" || got.IndustryHitRateEvidence.RowsTotal != 7 {
		t.Fatalf("decorators did not compose: %+v", got.IndustryHitRateEvidence)
	}
}

// TestApplyAssessmentDecorators_DoesNotMutateCallerValue documents that the
// decorator runs on a copy, so a concurrent reader of the base assessment is
// never exposed to a half-decorated value.
func TestApplyAssessmentDecorators_DoesNotMutateCallerValue(t *testing.T) {
	t.Cleanup(ResetAssessmentDecorators)
	ResetAssessmentDecorators()
	RegisterAssessmentDecorator(func(a *CapitalFlowAssessment) {
		a.CalibrationStatus = "decorated"
	})
	base := CapitalFlowAssessment{CalibrationStatus: "calibrating"}

	got := applyAssessmentDecorators(base)

	if got.CalibrationStatus != "decorated" {
		t.Fatalf("decorator did not run: %+v", got)
	}
	if base.CalibrationStatus != "calibrating" {
		t.Fatalf("base assessment was mutated: %+v", base)
	}
}
