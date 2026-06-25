package builtin

import (
	"testing"

	"math"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// -----------------------------------------------------------------------------
// Gates 1-5: existing tests, kept verbatim.
// -----------------------------------------------------------------------------

func TestImproveSharpeLike_Pass(t *testing.T) {
	e := ImproveSharpeLike()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{BaselineValue: 1.0, CandidateValue: 1.1},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestImproveSharpeLike_Fail(t *testing.T) {
	e := ImproveSharpeLike()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{BaselineValue: 1.0, CandidateValue: 0.9},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestPreserveDownsideProtection_Pass(t *testing.T) {
	e := PreserveDownsideProtection()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.1, -0.05, 0.08, 0.03},
		CandidateReturns: []float64{0.08, -0.04, 0.06, 0.02},
	}
	r := e.Eval(input, acceptance.EvalParams{DrawdownProtectionRatio: 0.8})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestPreserveDownsideProtection_Fail(t *testing.T) {
	e := PreserveDownsideProtection()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.1, -0.05, 0.08, 0.03},
		CandidateReturns: []float64{0.05, -0.30, 0.02, 0.01},
	}
	r := e.Eval(input, acceptance.EvalParams{DrawdownProtectionRatio: 0.8})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestNoDrawdownSpike_Pass(t *testing.T) {
	e := NoDrawdownSpike()
	input := domain.PromptExperimentResult{
		OOSResult: &domain.OOSResult{Passed: true, Reason: "ok"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestNoDrawdownSpike_Fail(t *testing.T) {
	e := NoDrawdownSpike()
	input := domain.PromptExperimentResult{
		OOSResult: &domain.OOSResult{Passed: false, Reason: "drawdown spike"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestFactorWeightStability_Pass(t *testing.T) {
	e := FactorWeightStability()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 2,
		CandidateFactorCount: 10, CandidateFallbackCount: 3,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestFactorWeightStability_Fail(t *testing.T) {
	e := FactorWeightStability()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 1,
		CandidateFactorCount: 10, CandidateFallbackCount: 8,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestRetailSentimentFilter_Pass(t *testing.T) {
	e := RetailSentimentFilter()
	input := domain.PromptExperimentResult{
		Brief: domain.MutationBrief{RSITwScore: 0.3},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestRetailSentimentFilter_RejectExtreme(t *testing.T) {
	e := RetailSentimentFilter()
	input := domain.PromptExperimentResult{
		Brief: domain.MutationBrief{RSITwScore: 0.85},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for extreme RSI")
	}
}

// -----------------------------------------------------------------------------
// Gates 6-17: new tests (2 per gate: pass + fail).
// -----------------------------------------------------------------------------

// 6. no_material_drawdown_degradation

func TestNoMaterialDrawdownDegradation_Pass(t *testing.T) {
	e := NoMaterialDrawdownDegradation()
	input := domain.PromptExperimentResult{
		Brief:       domain.MutationBrief{MaturityLevel: "level_1_exploratory"},
		Experiment:  domain.ExperimentRecord{MutationType: "prompt_tighten"},
		JudgeChecks: []string{"check 1", "check 2"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass (2 checks >= 2 required), got: %s", r.Reason)
	}
}

func TestNoMaterialDrawdownDegradation_Fail(t *testing.T) {
	e := NoMaterialDrawdownDegradation()
	input := domain.PromptExperimentResult{
		Brief:       domain.MutationBrief{MaturityLevel: "level_2_window_validated"},
		Experiment:  domain.ExperimentRecord{MutationType: "prompt_tighten"},
		JudgeChecks: []string{"check 1"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail (1 check < 3 required for level_2)")
	}
}

// 7. no_constraint_bypass

func TestNoConstraintBypass_Pass(t *testing.T) {
	e := NoConstraintBypass()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{CandidateValue: 1.5},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestNoConstraintBypass_RejectNaN(t *testing.T) {
	e := NoConstraintBypass()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{CandidateValue: math.NaN()},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for NaN candidate value")
	}
}

// 8. maintain_sharpe_like

func TestMaintainSharpeLike_Pass(t *testing.T) {
	e := MaintainSharpeLike()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.02, 0.01, 0.015, -0.005, 0.03, 0.01, 0.025, 0.0, 0.02, -0.01},
		CandidateReturns: []float64{0.03, 0.02, 0.01, -0.01, 0.04, 0.02, 0.015, 0.01, 0.025, 0.0},
	}
	r := e.Eval(input, acceptance.EvalParams{SharpeStabilityThreshold: 0.01})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestMaintainSharpeLike_FailFlat(t *testing.T) {
	e := MaintainSharpeLike()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.001, 0.001, 0.001, 0.001, 0.001},
		CandidateReturns: []float64{0.001, 0.001, 0.001, 0.001, 0.001},
	}
	r := e.Eval(input, acceptance.EvalParams{SharpeStabilityThreshold: 1.0})
	if r.Passed {
		t.Error("expected fail for flat (near-zero volatility) returns")
	}
}

// 9. reduce_concentration_risk

func TestReduceConcentrationRisk_Pass(t *testing.T) {
	e := ReduceConcentrationRisk()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.01, -0.02, 0.015, -0.01, 0.02, 0.0, -0.01, 0.01, 0.02, -0.005, 0.015, 0.01, -0.02, 0.005, 0.01, -0.01, 0.02, 0.0, -0.005, 0.015, 0.01, -0.01, 0.005, 0.02, -0.01, 0.01, 0.015, -0.005, 0.02, 0.0, -0.01, 0.01},
		CandidateReturns: []float64{0.005, -0.01, 0.01, -0.005, 0.01, 0.0, -0.005, 0.005, 0.01, -0.005, 0.01, 0.005, -0.01, 0.005, 0.005, -0.005, 0.01, 0.0, -0.005, 0.01, 0.005, -0.005, 0.005, 0.01, -0.005, 0.005, 0.01, -0.005, 0.01, 0.0, -0.005, 0.005},
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.5})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestReduceConcentrationRisk_Fail(t *testing.T) {
	e := ReduceConcentrationRisk()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.005, -0.005, 0.005, -0.005, 0.005, 0.0, 0.0, 0.005, -0.005, 0.005, 0.0, -0.005, 0.005, 0.0, 0.0, 0.005, -0.005, 0.0, 0.005, 0.0, 0.005, -0.005, 0.0, 0.005, 0.0, -0.005, 0.005, 0.0, 0.0, 0.005, -0.005, 0.0},
		CandidateReturns: []float64{0.15, -0.20, 0.10, -0.15, 0.20, 0.25, -0.18, 0.12, -0.22, 0.08, -0.10, 0.14, 0.30, -0.25, 0.05, -0.30, 0.18, 0.22, -0.12, 0.28, -0.08, 0.16, -0.05, 0.20, 0.10, -0.15, 0.25, -0.20, 0.12, 0.18, -0.10, 0.22},
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.2})
	if r.Passed {
		t.Error("expected fail for high candidate volatility")
	}
}

// 10. factor_quality

func TestFactorQuality_Pass(t *testing.T) {
	e := FactorQuality()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 2,
		CandidateFactorCount: 10, CandidateFallbackCount: 3,
	}
	r := e.Eval(input, acceptance.EvalParams{MaxFallbackRatio: 0.4})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestFactorQuality_Fail(t *testing.T) {
	e := FactorQuality()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 2,
		CandidateFactorCount: 10, CandidateFallbackCount: 6,
	}
	r := e.Eval(input, acceptance.EvalParams{MaxFallbackRatio: 0.4})
	if r.Passed {
		t.Error("expected fail for high candidate fallback ratio")
	}
}

// 11. reduce_false_positive_rate

func TestReduceFalsePositiveRate_Pass(t *testing.T) {
	e := ReduceFalsePositiveRate()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.01, -0.01, 0.02, -0.005, 0.015},
		CandidateReturns: []float64{0.02, -0.005, 0.01, -0.01, 0.005},
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.5})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestReduceFalsePositiveRate_Fail(t *testing.T) {
	e := ReduceFalsePositiveRate()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.01, 0.02, -0.005, 0.015, 0.01},
		CandidateReturns: []float64{-0.02, -0.03, -0.01, 0.005, -0.025},
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.2})
	if r.Passed {
		t.Error("expected fail for high candidate negative return ratio")
	}
}

// 12. maintain_cro_authority

func TestMaintainCROAuthority_Pass(t *testing.T) {
	e := MaintainCROAuthority()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 110,
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.5})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestMaintainCROAuthority_Fail(t *testing.T) {
	e := MaintainCROAuthority()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 300,
	}
	r := e.Eval(input, acceptance.EvalParams{VolatilityToleranceRatio: 1.5})
	if r.Passed {
		t.Error("expected fail for 3x observation growth")
	}
}

// 13. reduce_sector_blindspots

func TestReduceSectorBlindspots_Pass(t *testing.T) {
	e := ReduceSectorBlindspots()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 80,
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass (80%% >= 50%% coverage), got: %s", r.Reason)
	}
}

func TestReduceSectorBlindspots_Fail(t *testing.T) {
	e := ReduceSectorBlindspots()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 30,
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for 30%% coverage below 50%% threshold")
	}
}

// 14. maintain_industry_coverage

func TestMaintainIndustryCoverage_Pass(t *testing.T) {
	e := MaintainIndustryCoverage()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 90,
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass (90%% >= 50%% coverage), got: %s", r.Reason)
	}
}

func TestMaintainIndustryCoverage_Fail(t *testing.T) {
	e := MaintainIndustryCoverage()
	input := domain.PromptExperimentResult{
		BaselineObservations: 100, CandidateObservations: 40,
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for 40%% coverage below 50%% threshold")
	}
}

// 15. reduce_style_drift

func TestReduceStyleDrift_Pass(t *testing.T) {
	e := ReduceStyleDrift()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 2,
		CandidateFactorCount: 10, CandidateFallbackCount: 3,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestReduceStyleDrift_Fail(t *testing.T) {
	e := ReduceStyleDrift()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 1,
		CandidateFactorCount: 10, CandidateFallbackCount: 8,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if r.Passed {
		t.Error("expected fail for high style drift")
	}
}

// 16. maintain_momentum_catch

func TestMaintainMomentumCatch_Pass(t *testing.T) {
	e := MaintainMomentumCatch()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.01, -0.01, 0.02, 0.01, 0.015},
		CandidateReturns: []float64{0.02, -0.01, 0.01, 0.02, 0.01},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestMaintainMomentumCatch_Fail(t *testing.T) {
	e := MaintainMomentumCatch()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.01, 0.02, 0.015, 0.01, 0.02},
		CandidateReturns: []float64{-0.02, -0.03, -0.01, -0.02, -0.01},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for candidate with zero momentum catch")
	}
}

// 17. respect_holding_period

func TestRespectHoldingPeriod_Pass(t *testing.T) {
	e := RespectHoldingPeriod()
	params := acceptance.EvalParams{
		PromptBytes: []byte("use max_holding_days: 30 for risk management"),
	}
	r := e.Eval(domain.PromptExperimentResult{}, params)
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestRespectHoldingPeriod_Fail(t *testing.T) {
	e := RespectHoldingPeriod()
	params := acceptance.EvalParams{
		PromptBytes: []byte("pick stocks with high momentum"),
	}
	r := e.Eval(domain.PromptExperimentResult{}, params)
	if r.Passed {
		t.Error("expected fail for prompt without holding period constraint")
	}
}
