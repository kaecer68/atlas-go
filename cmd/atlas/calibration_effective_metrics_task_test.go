package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// #2007 的兩個 bounded 子項在**任務層**的端到端驗證：
//   (1) 告警標的必須是權威產物（overlay）：基準不新鮮（預期）時 overlay 仍必須是 1；
//   (2) image-vs-effective drift 必須以**真實數值**出現在 /metrics 上
//       （repo 0.03 vs effective 0.0108）。
//
// 為什麼用 repo 的出貨參數檔當基準而不是小型 fixture：drift 比較的左邊是
// 「image 內基準」＝**出貨檔本身**，用合成 fixture 就驗不到「真實資料」這一格。
// 這條測試讀 321KB 的 `configs/parameters.json`（解析 + defaults merge），成本可接受。

// installRealBaseline 把出貨的 parameters.json 複製成 workDir 的慣例基準檔，
// 並把製程的權威路徑指向它（與 production 的形狀一致）。
func installRealBaseline(t *testing.T, workDir string) string {
	t.Helper()
	shipped := filepath.Join("..", "..", "configs", "parameters.json")
	data, err := os.ReadFile(shipped)
	if err != nil {
		t.Skipf("出貨參數檔不存在（非完整 checkout）: %v", err)
	}
	dir := filepath.Join(workDir, "configs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeOverlay 在慣例路徑（<workDir>/data/state/parameters.calibrated.json）寫一份
// overlay，entries 由呼叫端給。
func writeOverlay(t *testing.T, workDir string, updatedAt time.Time, entries map[string]any) string {
	t.Helper()
	path := config.CalibrationOverlayPath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"version":    "2",
		"updated_at": updatedAt.UTC().Format(time.RFC3339Nano),
		"source":     "test",
		"entries":    entries,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// useBaselineOnly 讓解析走 workDir 慣例路徑（不是製程權威路徑），並清掉註冊的 overlay，
// 使 fallback 解析被測到。兩者的還原都在 t.Cleanup。
func useBaselineOnly(t *testing.T) {
	t.Helper()
	prevPath, prevOverlay := config.GetParametersConfigPath(), config.GetCalibratedOverlayPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.SetCalibratedOverlayPath(prevOverlay)
	})
	config.SetParametersConfigPath("")
	config.SetCalibratedOverlayPath("")
}

// 供應鏈案例（#2007 的真實形狀）：image 內基準停在建置日（不新鮮，**預期**），
// overlay 是剛剛寫的（新鮮）⇒ overlay 的 _ok 必須是 1（告警要 resolve），
// 而 drift 必須看得見那個真實的偏離（risk/max_daily_loss_pct 0.03 → 0.0108）。
func TestExportCalibrationMetrics_OverlayFreshAndRealDriftVisible(t *testing.T) {
	workDir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	installRealBaseline(t, workDir)
	useBaselineOnly(t)

	overlayPath := writeOverlay(t, workDir, now.Add(-35*time.Minute), map[string]any{
		"risk_max_daily_loss_pct": map[string]any{
			// #2007 的樣本：repo 0.03 vs effective 0.0108。
			"value":         0.0108,
			"before":        0.03,
			"ssot":          map[string]any{"present": true, "value": 0.03},
			"calibrated_at": now.Add(-35 * time.Minute).UTC().Format(time.RFC3339Nano),
			"method":        "risk_gate_calibrate",
		},
	})

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationMetrics(workDir, collector, now)

	// 基準這一條：不新鮮是預期（出貨檔的 updated_at 屬於建置期），仍是可評估。
	if !obs.Baseline.RunOK {
		t.Fatalf("出貨基準必須可評估，got code=%q", obs.Baseline.UnverifiableCode)
	}
	if obs.Baseline.Fresh {
		t.Error("出貨基準不該是「新鮮」的（updated_at 屬建置期）—— 若這裡翻真，請確認 repo 的檔案被改過")
	}
	// overlay 這一條：這是告警現在的標的，必須是「可評估 + 新鮮」。
	if !obs.Overlay.RunOK || !obs.Overlay.Fresh {
		t.Fatalf("overlay run_ok=%v fresh=%v（code=%q），預期皆為 true ⇒ 告警必須 resolve",
			obs.Overlay.RunOK, obs.Overlay.Fresh, obs.Overlay.FreshnessCode)
	}
	if !obs.Overlay.HaveAge {
		t.Error("overlay 有 updated_at ⇒ age 必須輸出")
	}
	// drift 這一條：真實偏離要看得見，而且它是**可解釋**的（比值在單步窗內）。
	if !obs.Drift.RunOK {
		t.Fatalf("drift 必須可比較，got code=%q", obs.Drift.UnverifiableCode)
	}
	if got := obs.Drift.DriftedKeys; len(got) != 1 || got[0] != "risk_max_daily_loss_pct" {
		t.Fatalf("DriftedKeys = %v, want [risk_max_daily_loss_pct]", got)
	}
	if len(obs.Drift.OutOfWindowKeys) != 0 {
		t.Errorf("OutOfWindowKeys = %v, want none（0.36 在 [1/3, 3] 內）", obs.Drift.OutOfWindowKeys)
	}
	if len(obs.Drift.UnknownKeys) != 0 {
		t.Errorf("UnknownKeys = %v, want none", obs.Drift.UnknownKeys)
	}

	body := scrapeMetrics(t, collector)
	for _, want := range []string{
		// 舊序列語意不變（基準）；新序列是權威產物。
		`atlas_calibration_freshness_ok{artifact="parameters"} 0.000000`,
		`atlas_calibration_freshness_ok{artifact="parameters_overlay"} 1.000000`,
		`atlas_calibration_freshness_run_ok{artifact="parameters_overlay"} 1.000000`,
		`atlas_calibration_drift_run_ok{artifact="parameters_overlay"} 1.000000`,
		`atlas_calibration_drift_keys{artifact="parameters_overlay"} 1.000000`,
		`atlas_calibration_drift_out_of_window_keys{artifact="parameters_overlay"} 0.000000`,
		`atlas_calibration_drift_dropped_keys{artifact="parameters_overlay",reason="not_applicable"} 0.000000`,
		`atlas_calibration_drift_dropped_keys{artifact="parameters_overlay",reason="ssot_moved"} 0.000000`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q\n--- 全文 ---\n%s", want, body)
		}
	}
	if !strings.Contains(body, fmt.Sprintf("atlas_calibration_freshness_age_seconds{artifact=\"parameters_overlay\"} %d.000000", 35*60)) {
		t.Errorf("/metrics 缺少 overlay 的 age（2100 秒）\n%s", body)
	}
	_ = overlayPath
}

// overlay 不存在 ⇒ 可評估但不新鮮（OVERLAY_ABSENT，warning），且 drift 必須是
// 「0 但可比較」（沒有 overlay ＝ effective 就是基準，這是**事實**不是未知）。
func TestExportCalibrationMetrics_OverlayAbsentIsEvaluableNotUnknown(t *testing.T) {
	workDir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	installRealBaseline(t, workDir)
	useBaselineOnly(t)

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationMetrics(workDir, collector, now)

	if !obs.Overlay.RunOK {
		t.Fatal("overlay 不存在是**確定**狀態，run_ok 必須是 1（否則會誤用 error 級告警）")
	}
	if obs.Overlay.Fresh {
		t.Error("overlay 不存在不得算新鮮")
	}
	if obs.Overlay.FreshnessCode != monitoring.CalibrationOverlayAbsent {
		t.Errorf("freshness code = %q, want %q", obs.Overlay.FreshnessCode, monitoring.CalibrationOverlayAbsent)
	}
	if !obs.Drift.RunOK || len(obs.Drift.DriftedKeys) != 0 {
		t.Errorf("沒有 overlay ⇒ 沒有 drift，但必須是可比較的 0（run_ok=%v keys=%v）", obs.Drift.RunOK, obs.Drift.DriftedKeys)
	}
	body := scrapeMetrics(t, collector)
	for _, want := range []string{
		`atlas_calibration_freshness_run_ok{artifact="parameters_overlay"} 1.000000`,
		`atlas_calibration_freshness_ok{artifact="parameters_overlay"} 0.000000`,
		`atlas_calibration_drift_keys{artifact="parameters_overlay"} 0.000000`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q\n%s", want, body)
		}
	}
}

// fail-closed：基準讀不到 ⇒ drift 必須是「量不到」（run_ok=0）而且**不得**輸出
// keys=0（把未知寫成 0 就是本 repo 反覆在修的 false-green）。
func TestExportCalibrationMetrics_UnmeasurableDriftIsNotZero(t *testing.T) {
	workDir := t.TempDir() // 刻意不放基準檔
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	useBaselineOnly(t)
	writeOverlay(t, workDir, now.Add(-time.Minute), map[string]any{
		"risk_max_daily_loss_pct": map[string]any{"value": 0.0108},
	})

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationMetrics(workDir, collector, now)
	if obs.Drift.RunOK {
		t.Fatal("基準不存在 ⇒ drift 不可比較（run_ok 必須是 0）")
	}
	if obs.Drift.UnverifiableCode != monitoring.CalibrationDriftBaselineUnreadable {
		t.Errorf("code = %q, want %q", obs.Drift.UnverifiableCode, monitoring.CalibrationDriftBaselineUnreadable)
	}
	body := scrapeMetrics(t, collector)
	if want := `atlas_calibration_drift_run_ok{artifact="parameters_overlay"} 0.000000`; !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
	if strings.Contains(body, `atlas_calibration_drift_keys{`) {
		t.Errorf("不可比較時**不得**輸出 drift_keys（0 會被讀成「沒有偏離」）\n%s", body)
	}
}

// overlay 解析的優先序：製程註冊的那一個優先於 workDir 慣例路徑。
func TestCalibrationOverlayPath_PrefersRegisteredPath(t *testing.T) {
	prev := config.GetCalibratedOverlayPath()
	t.Cleanup(func() { config.SetCalibratedOverlayPath(prev) })

	workDir := t.TempDir()
	config.SetCalibratedOverlayPath("")
	if got, want := calibrationOverlayPath(workDir), config.CalibrationOverlayPath(workDir); got != want {
		t.Errorf("未註冊時 got %q, want %q（慣例路徑）", got, want)
	}

	registered := filepath.Join(t.TempDir(), "registered.calibrated.json")
	config.SetCalibratedOverlayPath(registered)
	if got := calibrationOverlayPath(workDir); got != registered {
		t.Errorf("已註冊時 got %q, want %q（必須看 runtime 真正讀寫的那個檔）", got, registered)
	}
}
