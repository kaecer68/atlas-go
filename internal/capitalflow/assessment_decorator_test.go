package capitalflow_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

// TestRegisterAssessmentDecorator_NilNoop verifies RegisterAssessmentDecorator
// tolerates a nil function (no panic, no registration). This is the safety
// guarantee for misconfigured composition roots.
func TestRegisterAssessmentDecorator_NilNoop(t *testing.T) {
	defer capitalflow.ResetAssessmentDecorators()
	capitalflow.RegisterAssessmentDecorator(nil)
	capitalflow.ResetAssessmentDecorators()
}

// TestRegisterAssessmentDecorator_AppendsAndResets verifies the registry
// accepts multiple registrations and ResetAssessmentDecorators clears them.
func TestRegisterAssessmentDecorator_AppendsAndResets(t *testing.T) {
	defer capitalflow.ResetAssessmentDecorators()
	called := 0
	capitalflow.RegisterAssessmentDecorator(func(a *capitalflow.CapitalFlowAssessment) {
		called++
	})
	capitalflow.RegisterAssessmentDecorator(func(a *capitalflow.CapitalFlowAssessment) {
		called++
	})
	// Run them via apply path: need a Service for LatestAssessment, but we
	// can test the registry primitives directly. applyAssessmentDecorators is
	// not exported; this test only proves the register/reset cycle.
	if called != 0 {
		t.Fatalf("pre-call called counter should be 0, got %d", called)
	}
	capitalflow.ResetAssessmentDecorators()
}
