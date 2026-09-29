package config

// #2133：「**沒有改善 ⇒ 不得寫入任何參數**」的機制性保證。
//
// 修法前的實測（2026-09-29）：一個**常數** evaluator ＋ 一個可解析的參數
// （darwinian_weight_min）也會被寫入 ——
//
//	constant evaluator must produce no changes, got 1:
//	  [{ParamName:darwinian_weight_min Before:0.3 After:0.20571428571428568 DeltaPct:-31.428571418095245}]
//
// 原因：`improvement` 只被拿去決定 verdict，寫入迴圈無條件執行；每參數的 gate
// 只有 `|deltaPct| < MinImprovement*100`，而並列候選的任意值很容易超過 2%。
// 本檔的兩條測試把「無改善 ⇒ 不寫」與「真有改善 ⇒ 寫、且在 bounds 內」釘住。

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

// flatEvalCalibrator 只宣告一個**真實存在於 parameterTable** 的名字，
// 確保「沒有變更」不是因為名字解析不到（那正是 #2123 的坑）。
type flatEvalCalibrator struct {
	names  []string
	bounds map[string][2]float64
}

func (c flatEvalCalibrator) ParamNames() []string               { return c.names }
func (c flatEvalCalibrator) ParamBounds() map[string][2]float64 { return c.bounds }

// itIsolateCalibrationConfig 把全域參數 singleton 指到一個 temp SSOT（避免污染其他測試），
// 並在測試結束還原。
func itIsolateCalibrationConfig(t *testing.T, minVal, maxVal float64) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := map[string]any{
		"version": "test",
		"darwinian": map[string]any{
			"weight_min": map[string]any{"value": minVal},
			"weight_max": map[string]any{"value": maxVal},
		},
	}
	writeTestSSOTDocument(t, path, body)

	prevPath := GetParametersConfigPath()
	prevOverlay := GetCalibratedOverlayPath()
	SetParametersConfigPath(path)
	SetCalibratedOverlayPath(filepath.Join(dir, "overlay.json"))
	ResetParametersConfig()
	t.Cleanup(func() {
		SetParametersConfigPath(prevPath)
		SetCalibratedOverlayPath(prevOverlay)
		ResetParametersConfig()
	})
}

// TestCalibrateParameters_FlatSurfaceWritesNothing 是 #2133 的核心驗收：
// 分數曲面平（常數 evaluator）⇒ 不得寫入任何參數，且 verdict 必須是「沒有改善」那一類。
func TestCalibrateParameters_FlatSurfaceWritesNothing(t *testing.T) {
	itIsolateCalibrationConfig(t, 0.3, 0.9)

	before := GetParametersConfig().Darwinian.WeightMin.Value
	cal := flatEvalCalibrator{names: []string{"darwinian_weight_min"}}
	eval := func(_ *ParametersConfig) (float64, error) { return 0.62, nil }

	res, err := CalibrateParameters(context.Background(), cal, eval, DefaultCalibrateConfig())
	if err != nil {
		t.Fatalf("CalibrateParameters: %v", err)
	}
	if len(res.Changes) != 0 {
		t.Fatalf("a flat score surface must not write any parameter, got %d change(s): %+v", len(res.Changes), res.Changes)
	}
	if res.Verdict != "unchanged" {
		t.Errorf("verdict = %q, want unchanged for a flat surface", res.Verdict)
	}
	if got := GetParametersConfig().Darwinian.WeightMin.Value; got != before {
		t.Fatalf("parameter mutated without improvement: %v -> %v", before, got)
	}
	if res.ParamCount == 0 {
		t.Error("ParamCount must still report the attempted parameter set")
	}
}

// TestCalibrateParameters_ImprovementAppliesWithinBounds 是反向護欄（避免修過頭）：
// 真有改善 ⇒ 必須寫入，且寫入值落在 BoundedCalibrator 給的 bounds 內。
func TestCalibrateParameters_ImprovementAppliesWithinBounds(t *testing.T) {
	const low, high = 0.2, 0.8
	itIsolateCalibrationConfig(t, 0.3, 0.9)

	cal := flatEvalCalibrator{
		names:  []string{"darwinian_weight_min"},
		bounds: map[string][2]float64{"darwinian_weight_min": {low, high}},
	}
	// 曲線在 0.5 有極大值 ⇒ optimizer 必須找到改善（相對 baseline 0.3 是 +N%）
	const target = 0.5
	eval := func(cfg *ParametersConfig) (float64, error) {
		v := cfg.Darwinian.WeightMin.Value
		return 1.0 - math.Abs(v-target), nil
	}

	res, err := CalibrateParameters(context.Background(), cal, eval, DefaultCalibrateConfig())
	if err != nil {
		t.Fatalf("CalibrateParameters: %v", err)
	}
	if len(res.Changes) == 0 {
		t.Fatalf("an improving evaluator must be able to write a change (result: %+v)", res)
	}
	for _, ch := range res.Changes {
		if ch.ParamName != "darwinian_weight_min" {
			t.Fatalf("unexpected parameter in changes: %s", ch.ParamName)
		}
		if ch.After < low || ch.After > high {
			t.Fatalf("applied value %v is outside the declared bounds [%v, %v] — bounds must clamp the search", ch.After, low, high)
		}
		if math.Abs(ch.After-target) > 0.2 {
			t.Errorf("applied value %v is far from the optimum %v (optimizer should find it inside the bounds)", ch.After, target)
		}
	}
	if res.Verdict != "calibrated" {
		t.Errorf("verdict = %q, want calibrated when a change was applied", res.Verdict)
	}
	if got := GetParametersConfig().Darwinian.WeightMin.Value; got != res.Changes[0].After {
		t.Errorf("config value = %v, want the applied value %v", got, res.Changes[0].After)
	}
}
