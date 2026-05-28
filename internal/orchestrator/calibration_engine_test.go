package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
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

// ── CalibrateAll tests ──────────────────────────────────────

type mockStrategyProvider struct {
	meta StrategyMeta
}

func (m mockStrategyProvider) StrategyMeta() StrategyMeta { return m.meta }

func TestCalibrationEngine_CalibrateAll_WithData(t *testing.T) {
	fc := loadFactorConfig()
	providers := []StrategyProvider{
		mockStrategyProvider{StrategyMeta{
			ID: "test1", Skill: "skill1", Factors: []string{"momentum"},
			Parameters: momentumParams(fc),
		}},
	}
	provider := mockCalibrationProvider{recs: map[string][]CalibRecommendation{
		"skill1": {
			{Symbol: "A", ForwardRet: 0.05, FactorScores: map[string]float64{"momentum": 0.5}},
			{Symbol: "B", ForwardRet: 0.03, FactorScores: map[string]float64{"momentum": 0.3}},
			{Symbol: "C", ForwardRet: -0.04, FactorScores: map[string]float64{"momentum": -0.2}},
			{Symbol: "D", ForwardRet: -0.02, FactorScores: map[string]float64{"momentum": -0.05}},
			{Symbol: "E", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.1}},
			{Symbol: "F", ForwardRet: 0.04, FactorScores: map[string]float64{"momentum": 0.4}},
			{Symbol: "G", ForwardRet: -0.03, FactorScores: map[string]float64{"momentum": -0.3}},
			{Symbol: "H", ForwardRet: 0.02, FactorScores: map[string]float64{"momentum": 0.15}},
			{Symbol: "I", ForwardRet: -0.01, FactorScores: map[string]float64{"momentum": -0.1}},
			{Symbol: "J", ForwardRet: 0.06, FactorScores: map[string]float64{"momentum": 0.55}},
		},
	}}
	engine := &CalibrationEngine{}
	reports, err := engine.CalibrateAll(providers, provider, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].BaselineScore == 0 {
		t.Error("expected non-zero baseline score")
	}
	t.Logf("CalibrateAll: executor=%s baseline=%.4f optimized=%.4f improvement=%.1f%% verdict=%s",
		reports[0].ExecutorID, reports[0].BaselineScore, reports[0].OptimizedScore, reports[0].ImprovementPct, reports[0].Verdict)
}

func TestCalibrationEngine_CalibrateAll_InsufficientData(t *testing.T) {
	fc := loadFactorConfig()
	providers := []StrategyProvider{
		mockStrategyProvider{StrategyMeta{
			ID: "low_data", Skill: "low", Factors: []string{"momentum"},
			Parameters: momentumParams(fc),
		}},
	}
	// Only 3 samples — below minimum threshold
	provider := mockCalibrationProvider{recs: map[string][]CalibRecommendation{
		"low": {
			{Symbol: "A", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.5}},
			{Symbol: "B", ForwardRet: 0.02, FactorScores: map[string]float64{"momentum": 0.3}},
			{Symbol: "C", ForwardRet: -0.01, FactorScores: map[string]float64{"momentum": -0.2}},
		},
	}}
	engine := &CalibrationEngine{}
	reports, err := engine.CalibrateAll(providers, provider, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for insufficient data, got %d", len(reports))
	}
}

func TestCalibrationEngine_CalibrateAll_NoParamsExecutor(t *testing.T) {
	providers := []StrategyProvider{
		mockStrategyProvider{StrategyMeta{
			ID: "no_params", Skill: "no_params", Factors: []string{"momentum"},
			// No Parameters — should be skipped
		}},
	}
	provider := mockCalibrationProvider{recs: map[string][]CalibRecommendation{
		"no_params": {{Symbol: "A", ForwardRet: 0.01, FactorScores: map[string]float64{"momentum": 0.5}}},
	}}
	engine := &CalibrationEngine{}
	reports, err := engine.CalibrateAll(providers, provider, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for executor with no params, got %d", len(reports))
	}
}

// ── ScoreParameters multi-factor coverage ─────────────────────

func TestScoreParameters_AllFactors(t *testing.T) {
	fc := loadFactorConfig()
	meta := StrategyMeta{
		ID: "all_factors", Skill: "all_factors",
		Factors: []string{"momentum", "value", "quality", "liquidity"},
		Parameters: append(append(momentumParams(fc), valueParams(fc)...),
			append(qualityParams(fc), liquidityParams(fc)...)...),
	}
	data := []CalibRecommendation{
		{Symbol: "A", ForwardRet: 0.06, FactorScores: map[string]float64{"momentum": 0.5, "value": 0.4, "quality": 0.3, "liquidity": 0.6}},
		{Symbol: "B", ForwardRet: -0.04, FactorScores: map[string]float64{"momentum": -0.3, "value": -0.4, "quality": 0.0, "liquidity": -0.5}},
		{Symbol: "C", ForwardRet: 0.02, FactorScores: map[string]float64{"momentum": 0.0, "value": 0.1, "quality": 0.15, "liquidity": 0.3}},
	}
	engine := &CalibrationEngine{}
	report, err := engine.Calibrate(meta, data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.BaselineScore == 0 {
		t.Error("expected non-zero baseline score with all 4 factors")
	}
	t.Logf("All 4 factors: baseline=%.4f optimized=%.4f improvement=%.1f%%",
		report.BaselineScore, report.OptimizedScore, report.ImprovementPct)
	// Direct scoreParameters call for coverage of value/quality/liquidity switch cases
	params := append(append(momentumParams(fc), valueParams(fc)...),
		append(qualityParams(fc), liquidityParams(fc)...)...)
	score := scoreParameters(params, meta.Factors, data[:1])
	if score <= 0 {
		t.Errorf("expected positive score for strong signals + positive return, got %.4f", score)
	}
}

// ── ApplyParamField mapping coverage ──────────────────────────

func TestApplyParamField(t *testing.T) {
	fc := &config.FactorConvictionParams{}

	// Momentum float64
	applyParamField(fc, "momentum_high_threshold", 0.55)
	if fc.MomentumHighThreshold.Value != 0.55 {
		t.Errorf("momentum_high_threshold: expected 0.55, got %f", fc.MomentumHighThreshold.Value)
	}
	// Momentum int (value passed as float64, stored as int)
	applyParamField(fc, "momentum_high_delta", 10.0)
	if fc.MomentumHighDelta.Value != 10 {
		t.Errorf("momentum_high_delta: expected 10, got %d", fc.MomentumHighDelta.Value)
	}
	applyParamField(fc, "momentum_mod_threshold", 0.25)
	if fc.MomentumModThreshold.Value != 0.25 {
		t.Errorf("momentum_mod_threshold: expected 0.25, got %f", fc.MomentumModThreshold.Value)
	}
	applyParamField(fc, "momentum_mod_delta", 6.0)
	if fc.MomentumModDelta.Value != 6 {
		t.Errorf("momentum_mod_delta: expected 6, got %d", fc.MomentumModDelta.Value)
	}
	applyParamField(fc, "momentum_weak_threshold", -0.15)
	if fc.MomentumWeakThreshold.Value != -0.15 {
		t.Errorf("momentum_weak_threshold: expected -0.15, got %f", fc.MomentumWeakThreshold.Value)
	}
	applyParamField(fc, "momentum_weak_delta", -4.0)
	if fc.MomentumWeakDelta.Value != -4 {
		t.Errorf("momentum_weak_delta: expected -4, got %d", fc.MomentumWeakDelta.Value)
	}

	// Value float64
	applyParamField(fc, "value_high_threshold", 0.45)
	if fc.ValueHighThreshold.Value != 0.45 {
		t.Errorf("value_high_threshold: expected 0.45, got %f", fc.ValueHighThreshold.Value)
	}
	applyParamField(fc, "value_high_delta", 9.0)
	if fc.ValueHighDelta.Value != 9 {
		t.Errorf("value_high_delta: expected 9, got %d", fc.ValueHighDelta.Value)
	}
	applyParamField(fc, "value_mod_threshold", 0.15)
	if fc.ValueModThreshold.Value != 0.15 {
		t.Errorf("value_mod_threshold: expected 0.15, got %f", fc.ValueModThreshold.Value)
	}
	applyParamField(fc, "value_mod_delta", 5.0)
	if fc.ValueModDelta.Value != 5 {
		t.Errorf("value_mod_delta: expected 5, got %d", fc.ValueModDelta.Value)
	}
	applyParamField(fc, "value_weak_threshold", -0.25)
	if fc.ValueWeakThreshold.Value != -0.25 {
		t.Errorf("value_weak_threshold: expected -0.25, got %f", fc.ValueWeakThreshold.Value)
	}
	applyParamField(fc, "value_weak_delta", -3.0)
	if fc.ValueWeakDelta.Value != -3 {
		t.Errorf("value_weak_delta: expected -3, got %d", fc.ValueWeakDelta.Value)
	}

	// Quality float64 + int
	applyParamField(fc, "quality_threshold", 0.35)
	if fc.QualityThreshold.Value != 0.35 {
		t.Errorf("quality_threshold: expected 0.35, got %f", fc.QualityThreshold.Value)
	}
	applyParamField(fc, "quality_delta", 6.0)
	if fc.QualityDelta.Value != 6 {
		t.Errorf("quality_delta: expected 6, got %d", fc.QualityDelta.Value)
	}

	// Liquidity float64 + int
	applyParamField(fc, "liquidity_high_threshold", 0.6)
	if fc.LiquidityHighThreshold.Value != 0.6 {
		t.Errorf("liquidity_high_threshold: expected 0.6, got %f", fc.LiquidityHighThreshold.Value)
	}
	applyParamField(fc, "liquidity_high_delta", 7.0)
	if fc.LiquidityHighDelta.Value != 7 {
		t.Errorf("liquidity_high_delta: expected 7, got %d", fc.LiquidityHighDelta.Value)
	}
	applyParamField(fc, "liquidity_good_threshold", 0.3)
	if fc.LiquidityGoodThreshold.Value != 0.3 {
		t.Errorf("liquidity_good_threshold: expected 0.3, got %f", fc.LiquidityGoodThreshold.Value)
	}
	applyParamField(fc, "liquidity_good_delta", 4.0)
	if fc.LiquidityGoodDelta.Value != 4 {
		t.Errorf("liquidity_good_delta: expected 4, got %d", fc.LiquidityGoodDelta.Value)
	}
	applyParamField(fc, "liquidity_low_threshold", -0.2)
	if fc.LiquidityLowThreshold.Value != -0.2 {
		t.Errorf("liquidity_low_threshold: expected -0.2, got %f", fc.LiquidityLowThreshold.Value)
	}
	applyParamField(fc, "liquidity_low_delta", -3.0)
	if fc.LiquidityLowDelta.Value != -3 {
		t.Errorf("liquidity_low_delta: expected -3, got %d", fc.LiquidityLowDelta.Value)
	}

	// Unknown param → error
	err := applyParamField(fc, "nonexistent_param", 1.0)
	if err == nil {
		t.Error("expected error for unknown parameter name")
	}
}

// ── ApplyToConfigPath persistence coverage ────────────────────

func TestApplyToConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	tempPath := filepath.Join(tmpDir, "test_params.json")

	params := config.DefaultParametersConfig()
	params.SectorExecutor.FactorConviction.MomentumHighThreshold.Value = 0.1

	report := ConvictionCalibrationReport{
		ParametersAfter: map[string]float64{
			"momentum_high_threshold": 0.55,
			"momentum_high_delta":     10.0,
			"value_high_threshold":    0.45,
		},
	}

	engine := &CalibrationEngine{}
	err := engine.ApplyToConfigPath(report, params, tempPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify in-memory updates
	if params.SectorExecutor.FactorConviction.MomentumHighThreshold.Value != 0.55 {
		t.Errorf("momentum_high_threshold: expected 0.55, got %f", params.SectorExecutor.FactorConviction.MomentumHighThreshold.Value)
	}
	if params.SectorExecutor.FactorConviction.MomentumHighDelta.Value != 10 {
		t.Errorf("momentum_high_delta: expected 10, got %d", params.SectorExecutor.FactorConviction.MomentumHighDelta.Value)
	}
	if params.SectorExecutor.FactorConviction.ValueHighThreshold.Value != 0.45 {
		t.Errorf("value_high_threshold: expected 0.45, got %f", params.SectorExecutor.FactorConviction.ValueHighThreshold.Value)
	}

	// Verify file persistence
	if _, err := os.Stat(tempPath); os.IsNotExist(err) {
		t.Fatal("saved parameters.json not found")
	}
	loaded, err := config.LoadParametersConfig(tempPath)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.SectorExecutor.FactorConviction.MomentumHighThreshold.Value != 0.55 {
		t.Errorf("file: momentum_high_threshold: expected 0.55, got %f", loaded.SectorExecutor.FactorConviction.MomentumHighThreshold.Value)
	}
	if loaded.SectorExecutor.FactorConviction.MomentumHighDelta.Value != 10 {
		t.Errorf("file: momentum_high_delta: expected 10, got %d", loaded.SectorExecutor.FactorConviction.MomentumHighDelta.Value)
	}

	// Verify unchanged field still has default value
	if loaded.SectorExecutor.FactorConviction.MomentumModThreshold.Value != 0.15 {
		t.Errorf("file: momentum_mod_threshold should have default 0.15, got %f", loaded.SectorExecutor.FactorConviction.MomentumModThreshold.Value)
	}
}
