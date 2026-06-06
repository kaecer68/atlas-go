package config

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNarrativeCalibrator_ParamNames_Count(t *testing.T) {
	cal := &NarrativeCalibrator{}
	names := cal.ParamNames()
	if got := len(names); got != 9 {
		t.Errorf("ParamNames() returned %d names; want 9", got)
	}

	seen := make(map[string]bool)
	for _, n := range names {
		if _, dup := seen[n]; dup {
			t.Errorf("duplicate param name: %s", n)
		}
		seen[n] = true
	}
}

func TestNarrativeCalibrator_ParamNames_Registered(t *testing.T) {
	cal := &NarrativeCalibrator{}
	ie := NewInferenceEngine(DefaultParametersConfig())

	for _, name := range cal.ParamNames() {
		if _, ok := ie.GetParameter(name); !ok {
			t.Errorf("param %q not found in parameterTable — missing entry in param_table.go?", name)
		}
	}
}

func TestNarrativeEvaluator_Defaults(t *testing.T) {
	eval := NewNarrativeEvaluator()
	cfg := DefaultParametersConfig()
	score, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator returned error on defaults: %v", err)
	}
	if score <= 0 {
		t.Errorf("evaluator score on defaults = %f; want > 0", score)
	}
	if math.IsNaN(score) || math.IsInf(score, 0) {
		t.Errorf("evaluator score on defaults is non-finite: %f", score)
	}
}

func TestNarrativeEvaluator_Gradient(t *testing.T) {
	eval := NewNarrativeEvaluator()
	cfg := DefaultParametersConfig()

	baseline, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator baseline: %v", err)
	}

	cfg.Narrative.AIRevenueGrowthThreshold.Value = 10
	cfg.Narrative.CoWoSUtilizationThreshold.Value = 50
	cfg.Narrative.CapexGrowthThreshold.Value = 5
	cfg.Narrative.US10YChangeBpsThreshold.Value = 2
	cfg.Narrative.DXYChangePctThreshold.Value = 0.3
	cfg.Narrative.GeopoliticalGPRThreshold.Value = 50
	cfg.Narrative.OilChangePctThreshold.Value = 1
	cfg.Narrative.JPYChangePctThreshold.Value = 0.3
	cfg.Narrative.VIXLevelThreshold.Value = 10

	badScore, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator on bad values: %v", err)
	}
	if badScore >= baseline {
		t.Errorf("values far from ideals score %f >= baseline %f — expect degradation", badScore, baseline)
	}
}

func TestNarrativeCalibrator_EndToEnd(t *testing.T) {
	cal := &NarrativeCalibrator{}
	eval := NewNarrativeEvaluator()

	cfg := CalibrateConfig{
		InitialPoints:  4,
		Iterations:     6,
		MinImprovement: 0.03,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := CalibrateParameters(ctx, cal, eval, cfg)
	if err != nil {
		t.Fatalf("CalibrateParameters failed: %v", err)
	}

	if result.ParamCount != 9 {
		t.Errorf("result.ParamCount = %d; want 9", result.ParamCount)
	}
	if result.BaselineScore <= 0 {
		t.Errorf("BaselineScore = %f; want > 0", result.BaselineScore)
	}
	if result.OptimizedScore <= 0 {
		t.Errorf("OptimizedScore = %f; want > 0", result.OptimizedScore)
	}
	if result.Verdict == "" {
		t.Error("result.Verdict is empty")
	}
	if result.Summary == "" {
		t.Error("result.Summary is empty")
	}

	t.Logf("Baseline=%.4f Optimized=%.4f Verdict=%s", result.BaselineScore, result.OptimizedScore, result.Verdict)
	t.Logf("Summary: %s", result.Summary)
	for _, ch := range result.Changes {
		t.Logf("  %s: %.4f → %.4f (Δ%.1f%%, %s)", ch.ParamName, ch.Before, ch.After, ch.DeltaPct, ch.Confidence)
	}
}
