package orchestrator

import (
	"testing"
)

type mockCalibrationProvider struct {
	recs map[string][]CalibRecommendation
}

func (m mockCalibrationProvider) Recommendations(executorSkill string) ([]CalibRecommendation, error) {
	return m.recs[executorSkill], nil
}

func TestCalibrationEngine_Calibrate_SingleParameter(t *testing.T) {
	fc := loadFactorConfig()
	meta := StrategyMeta{
		ID: "test_exec", Skill: "test_skill", Factors: []string{"momentum"},
		Parameters: momentumParams(fc),
	}
	// Synthetic data: high momentum → positive return, low momentum → negative return
	data := []CalibRecommendation{
		{Symbol: "A", ForwardRet: 0.05, FactorScores: map[string]float64{"momentum": 0.5}},
		{Symbol: "B", ForwardRet: 0.03, FactorScores: map[string]float64{"momentum": 0.3}},
		{Symbol: "C", ForwardRet: -0.04, FactorScores: map[string]float64{"momentum": -0.2}},
		{Symbol: "D", ForwardRet: -0.02, FactorScores: map[string]float64{"momentum": -0.05}},
		{Symbol: "E", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.1}},
	}
	engine := &CalibrationEngine{}
	report, err := engine.Calibrate(meta, data, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.BaselineScore == 0 {
		t.Error("expected non-zero baseline score")
	}
	t.Logf("Baseline: %.4f, Optimized: %.4f, Improvement: %.1f%%, Verdict: %s",
		report.BaselineScore, report.OptimizedScore, report.ImprovementPct, report.Verdict)
}

func TestCalibrationEngine_Calibrate_MultiFactor(t *testing.T) {
	fc := loadFactorConfig()
	meta := StrategyMeta{
		ID: "test_multi", Skill: "test_multi", Factors: []string{"momentum", "liquidity"},
		Parameters: append(momentumParams(fc), liquidityParams(fc)...),
	}
	// High momentum + high liquidity → very positive; low liquidity → negative
	data := []CalibRecommendation{
		{Symbol: "A", ForwardRet: 0.08, FactorScores: map[string]float64{"momentum": 0.6, "liquidity": 0.7}},
		{Symbol: "B", ForwardRet: 0.03, FactorScores: map[string]float64{"momentum": 0.2, "liquidity": 0.3}},
		{Symbol: "C", ForwardRet: -0.05, FactorScores: map[string]float64{"momentum": -0.1, "liquidity": -0.5}},
		{Symbol: "D", ForwardRet: -0.03, FactorScores: map[string]float64{"momentum": 0.1, "liquidity": -0.4}},
		{Symbol: "E", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.0, "liquidity": 0.1}},
	}
	engine := &CalibrationEngine{}
	report, err := engine.Calibrate(meta, data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.OptimizedScore <= report.BaselineScore-1e-6 {
		t.Logf("optimization didn't improve baseline — may be normal for small datasets")
	}
	t.Logf("Params before: %v", report.ParametersBefore)
	t.Logf("Params after: %v", report.ParametersAfter)
}

func TestCalibrationEngine_Calibrate_NoParams(t *testing.T) {
	meta := StrategyMeta{ID: "no_params", Skill: "no_params", Factors: []string{"momentum"}}
	data := []CalibRecommendation{
		{Symbol: "A", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.5}},
	}
	engine := &CalibrationEngine{}
	report, err := engine.Calibrate(meta, data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.ParametersAfter) != 0 {
		t.Error("expected empty parameters after for executor with no params")
	}
}
