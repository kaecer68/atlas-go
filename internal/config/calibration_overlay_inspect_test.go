package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 觀測模式（InspectCalibratedOverlayLayer）的契約。
//
// 為什麼需要這一組測試：觀測模式與 runtime 載入**共用同一個實作**是刻意的
// （兩份判定邏輯會漂移），但兩者的**副作用必須相反**：
//   - runtime：把 overlay 收斂（丟掉不能套用的 entry、記錄首次看到的 baseline）；
//   - observer：一個 byte 都不能改（監控任務每 5 分鐘跑一次，讀-改-寫會與校準
//     寫入者的 merge 競爭，可能吃掉剛剛校準好的 entry）。
// 這兩個性質若沒有測試釘住，任何一次「順手重構」都可能讓監控任務開始寫檔。

// overlayInspectFixture 建一個同時包含兩種 entry 種類與三種失敗種類的 overlay，
// 讓 runtime 與觀測模式的判定可以直接互比。
func overlayInspectFixture(t *testing.T) (overlayPath string) {
	t.Helper()
	overlayPath = filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "inspect_fixture", map[string]CalibrationOverlayEntry{
		// 命名的 tunable：套用且真的偏離 SSOT（drift）。
		"risk_max_position_size": {
			Value: 0.13, Before: 0.15, SSOT: baselineOfFloat(0.15),
			CalibratedAt: time.Now(), Method: "bayesian_optimization",
		},
		// #2007 的真實樣本：repo 0.03 vs effective 0.0108。
		"risk_max_daily_loss_pct": {
			Value: 0.0108, Before: 0.03, SSOT: baselineOfFloat(0.03),
			CalibratedAt: time.Now(), Method: "bayesian_optimization",
		},
		// 點分路徑 entry：套用且偏離。
		"industry_cycle_thresholds": {
			Path:  "industry.cycle_thresholds.value",
			Value: map[string]any{"consumer": map[string]any{"expansion_revenue_pct": 0.05}},
			// SSOT=nil 是「尚未 reconcile」：loader 會在第一輪把 baseline 記下來，
			// 這一格因此同時覆蓋 entry.SSOT==nil 的分支。
			CalibratedAt: time.Now(), Method: "calibrate_seasonal",
		},
		// 未知的參數名（parameter table 沒有）⇒ 不能套用。
		"risk_nonexistent_param": {
			Value: 0.5, CalibratedAt: time.Now(),
		},
		// 未知的路徑（SSOT 沒有這個 section）⇒ 不能套用。
		"nope_section": {
			Path: "nope.section.value", Value: 1.0, CalibratedAt: time.Now(),
		},
		// baseline 已移動（SSOT 現在是 0.15，entry 說它疊在 0.99 上）⇒ 失效。
		"risk_max_drawdown_pct": {
			Value: 0.2, SSOT: baselineOfFloat(0.99), CalibratedAt: time.Now(),
		},
	})
	return overlayPath
}

func sameKeys(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	seen := map[string]bool{}
	for _, k := range got {
		seen[k] = true
	}
	for _, k := range want {
		if !seen[k] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// TestInspectCalibratedOverlayLayer_MatchesRuntimeDecisions 釘住「兩條路徑的判定
// 完全一致」：同一份 overlay 在 runtime 與觀測模式下的 Applied / Unknown /
// Invalidated / DriftedKeys 必須逐項相同。
func TestInspectCalibratedOverlayLayer_MatchesRuntimeDecisions(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := overlayInspectFixture(t)

	src, err := LoadParametersSource(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersSource: %v", err)
	}
	before := readFile(t, overlayPath)

	// ---- 觀測模式：先跑，檔案必須完全不動 ----
	_, inspector := InspectCalibratedOverlayLayer(src.Config, src.Raw, overlayPath)
	if inspector.Reconciled {
		t.Error("觀測模式不得 reconcile（Reconciled=true 代表它寫了檔）")
	}
	if after := readFile(t, overlayPath); after != before {
		t.Error("觀測模式改變了 overlay 檔案內容（監控任務必須唯讀）")
	}

	// ---- runtime：判定必須相同，但副作用相反 ----
	effective, runtime := applyOverlay(t, ssotPath, overlayPath)
	sameKeys(t, "Applied", runtime.AppliedKeys(), inspector.AppliedKeys())
	sameKeys(t, "DriftedKeys", runtime.DriftedKeys(), inspector.DriftedKeys())
	sameKeys(t, "Unknown", runtime.Unknown, inspector.Unknown)
	sameKeys(t, "Invalidated", runtime.Invalidated, inspector.Invalidated)
	sameKeys(t, "Unknown", []string{"risk_nonexistent_param", "nope_section"}, inspector.Unknown)
	sameKeys(t, "Invalidated", []string{"risk_max_drawdown_pct"}, inspector.Invalidated)

	// runtime 這條路會把不能套用的 entry 從檔案移除（收斂）。
	if !runtime.Reconciled {
		t.Error("runtime 模式必須 reconcile（移除不能套用的 entry）")
	}
	if after := readFile(t, overlayPath); after == before {
		t.Error("runtime 模式應該重寫 overlay（掉掉不能套用的 entry）")
	}

	// 觀測模式的 effective 值仍然是真的疊加結果（否則它不是「effective」）。
	if got := effective.Risk.MaxPositionSize.Value; got != 0.13 {
		t.Errorf("effective risk_max_position_size = %v, want 0.13", got)
	}
	if got := effective.Risk.MaxDailyLossPct.Value; got != 0.0108 {
		t.Errorf("effective risk_max_daily_loss_pct = %v, want 0.0108", got)
	}
}

// TestInspectCalibratedOverlayLayer_DriftedKeysExcludeBaselineRestated 釘住 drift
// 的定義：一個「只是把 baseline 值重述一次」的 entry 會被套用，但**不是** drift。
// 少了這條，drift 數會隨歷史成長而不是隨不一致成長。
func TestInspectCalibratedOverlayLayer_DriftedKeysExcludeBaselineRestated(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "restated", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {
			Value: 0.15, SSOT: baselineOfFloat(0.15), CalibratedAt: time.Now(),
		},
	})

	src, err := LoadParametersSource(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersSource: %v", err)
	}
	_, report := InspectCalibratedOverlayLayer(src.Config, src.Raw, overlayPath)
	sameKeys(t, "Applied", report.AppliedKeys(), []string{"risk_max_position_size"})
	if got := report.DriftedKeys(); len(got) != 0 {
		t.Errorf("DriftedKeys = %v, want none（值等於 baseline ⇒ 套用但無 drift）", got)
	}
}

// TestCalibrationOverlayDriftForRealProductionSample 用 #2007 記載的真實數值對
// （repo 0.03 vs effective 0.0108）驗證 drift 讀數：必須被算成 1 個偏離的 key，
// 且比值落在校準迴圈的單步窗內（⇒ 不該被當成異常升級）。
func TestCalibrationOverlayDriftForRealProductionSample(t *testing.T) {
	const (
		ssotValue      = 0.03
		effectiveValue = 0.0108
	)
	ssotPath := writeTestSSOT(t, 0.15, ssotValue)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_daily_loss_pct": {
			Value: effectiveValue, Before: ssotValue, SSOT: baselineOfFloat(ssotValue),
			CalibratedAt: time.Now(), Method: "bayesian_optimization",
		},
	})

	src, err := LoadParametersSource(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersSource: %v", err)
	}
	_, report := InspectCalibratedOverlayLayer(src.Config, src.Raw, overlayPath)
	sameKeys(t, "DriftedKeys", report.DriftedKeys(), []string{"risk_max_daily_loss_pct"})
	if len(report.Applied) != 1 {
		t.Fatalf("Applied = %v, want 1 entry", report.AppliedKeys())
	}
	ratio := report.Applied[0].Ratio
	if ratio < 0.35 || ratio > 0.37 {
		t.Errorf("ratio = %v, want ≈0.36 (effective/SSOT)", ratio)
	}
	if RatioOutsideSingleStepWindow(ratio) {
		t.Errorf("ratio %v 不該被判定為超出單步窗（0.36 ∈ [1/3, 3]）", ratio)
	}
}

func TestRatioOutsideSingleStepWindow(t *testing.T) {
	for _, tc := range []struct {
		ratio float64
		want  bool
	}{
		{ratio: 0, want: false}, // 不可比（非數值 / SSOT=0）不是異常
		{ratio: 1, want: false}, // 沒有位移
		{ratio: CalibrationOverlaySingleStepWindowMin, want: false}, // 邊界含
		{ratio: CalibrationOverlaySingleStepWindowMax, want: false}, // 邊界含
		{ratio: 0.1, want: true},                                    // 一步掉 90% ⇒ 不是單一輪校準能解釋的
		{ratio: 4, want: true},                                      // 一步漲 4 倍
		{ratio: -0.5, want: true},                                   // 符號翻轉
	} {
		if got := RatioOutsideSingleStepWindow(tc.ratio); got != tc.want {
			t.Errorf("RatioOutsideSingleStepWindow(%v) = %v, want %v", tc.ratio, got, tc.want)
		}
	}
}

// TestInspectCalibratedOverlayLayer_UnreadableOverlayIsReported 釘住 fail-closed
// 的來源：讀不動/解不開的 overlay 必須讓 report.Err 非 nil（監控據此把
// drift run_ok 設為 0，而不是把「量不到」當成「沒有 drift」）。
func TestInspectCalibratedOverlayLayer_UnreadableOverlayIsReported(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	src, err := LoadParametersSource(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersSource: %v", err)
	}

	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, report := InspectCalibratedOverlayLayer(src.Config, src.Raw, broken); report.Err == nil {
		t.Error("解不開的 overlay 必須回報 Err（fail-closed）")
	}

	// 檔案不存在不是錯誤：沒有 overlay ⇒ effective == baseline（零 drift 是**事實**）。
	missing := filepath.Join(t.TempDir(), "missing.json")
	_, report := InspectCalibratedOverlayLayer(src.Config, src.Raw, missing)
	if report.Err != nil {
		t.Errorf("不存在的 overlay 不該回報 Err，got %v", report.Err)
	}
	if len(report.DriftedKeys()) != 0 {
		t.Error("沒有 overlay 時 drift 必須是 0")
	}
}
