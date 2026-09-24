package capitalflow

import "sync"

// AssessmentDecorator is a hook that runs after LatestAssessment builds the
// base CapitalFlowAssessment. It may mutate the assessment to attach
// additional metadata or side-effects. PR-β (2026-09-24, #1942/#1948) uses
// this surface for the sectorallocation-side industry hit-rate consumption
// chain: when configs.sector_allocation.industry_hit_rate_consume_enabled is
// true, a sectorallocation-registered decorator attaches
// IndustryHitRateEvidence to the assessment without otherwise altering it.
//
// Decorators MUST:
//   - Be safe for concurrent registration and call.
//   - Treat the assessment as a value copy (not a shared pointer) - read
//     fields, write derived fields; do not mutate CalibrationStatus (the
//     E07 calibration pipeline owns that flip per H-CF-02).
//   - Return quickly. The handler list is iterated synchronously inside
//     LatestAssessment; a slow decorator blocks /api/capital-flow/daily.
//
// When no decorator is registered, the assessment surface is byte-identical
// to pre-PR-β behavior (root verifies via regression on dfc4e3a1).
type AssessmentDecorator func(*CapitalFlowAssessment)

var (
	assessmentDecoratorMu sync.RWMutex
	assessmentDecorators  []AssessmentDecorator
)

// RegisterAssessmentDecorator appends a decorator that runs after the
// LatestAssessment base pipeline. Intended callers: composition roots in
// cmd/atlas that wire cross-module chains. Safe to call from init().
//
// Last-wins for the same decorator label is not provided on purpose: the
// only known caller is sectorallocation.init (one-shot registration); an
// accidental double-registration is more useful as a doubled observation
// (the decorator should be idempotent).
func RegisterAssessmentDecorator(fn AssessmentDecorator) {
	if fn == nil {
		return
	}
	assessmentDecoratorMu.Lock()
	defer assessmentDecoratorMu.Unlock()
	assessmentDecorators = append(assessmentDecorators, fn)
}

// ResetAssessmentDecorators clears the registry. Wiring owners call it on
// teardown/rollback (and tests use it between cases). Mirrors the pattern
// used in sectorallocation.ResetPolicyConsumers.
func ResetAssessmentDecorators() {
	assessmentDecoratorMu.Lock()
	defer assessmentDecoratorMu.Unlock()
	assessmentDecorators = nil
}

// applyAssessmentDecorators runs every registered decorator on a copy of
// assessment. Returns the (possibly mutated) copy. The base assessment is
// never mutated in place so concurrent readers see a stable value while
// the decorator runs.
//
// When no decorator is registered, returns assessment unchanged (true
// zero-cost path: no copy, no allocation).
func applyAssessmentDecorators(assessment CapitalFlowAssessment) CapitalFlowAssessment {
	assessmentDecoratorMu.RLock()
	deferred := make([]AssessmentDecorator, len(assessmentDecorators))
	copy(deferred, assessmentDecorators)
	assessmentDecoratorMu.RUnlock()
	if len(deferred) == 0 {
		return assessment // explicit no-op (config off path)
	}
	for _, fn := range deferred {
		fn(&assessment)
	}
	return assessment
}
