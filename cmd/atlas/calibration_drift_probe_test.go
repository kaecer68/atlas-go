package main

// 生產 overlay 的 drift probe（**opt-in 診斷，不是 CI 斷言**）。
//
// 為什麼存在：`TestExportCalibrationMetrics_OverlayFreshAndRealDriftVisible` 用固定 fixture
// 驗「#2007 的樣本會被算成 drift」;這一條則是把**生產真的那份 overlay** 讀回來，
// 套在出貨基準上跑一次完整的三個觀察,讓「部署後會看到什麼」在部署前就能回答
// （runbook §4 的驗收命令之一）。
//
// 使用方式（唯讀:只讀檔,不寫任何生產路徑）:
//
//	ssh kmacmini 'cat /Users/kaecer/workspace/atlas/data/state/parameters.calibrated.json' > /tmp/prod-overlay.json
//	PROD_OVERLAY=/tmp/prod-overlay.json go test ./cmd/atlas/ -run TestProbeProductionOverlayDrift -v
//
// 沒有設 `PROD_OVERLAY` 時 **skip**（CI 不依賴生產檔案,也不會有「紅在沒有生產資料」的假紅燈）。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestProbeProductionOverlayDrift(t *testing.T) {
	prodOverlay := os.Getenv("PROD_OVERLAY")
	if prodOverlay == "" {
		t.Skip("PROD_OVERLAY 未設定（opt-in 診斷;見本檔檔頭的使用方式）")
	}
	shipped, err := os.ReadFile(filepath.Join("..", "..", "configs", "parameters.json"))
	if err != nil {
		t.Skipf("出貨基準不存在（非完整 checkout）: %v", err)
	}

	// 把「出貨基準 + 生產 overlay」組進一個暫存 workDir:生產路徑一律不碰。
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "configs", "parameters.json"), shipped, 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(prodOverlay)
	if err != nil {
		t.Fatalf("讀取 PROD_OVERLAY=%s: %v", prodOverlay, err)
	}
	dst := config.CalibrationOverlayPath(workDir)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	// 讓解析走 workDir 慣例路徑（不是製程當下的權威路徑）。
	prevPath, prevOverlay := config.GetParametersConfigPath(), config.GetCalibratedOverlayPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.SetCalibratedOverlayPath(prevOverlay)
	})
	config.SetParametersConfigPath("")
	config.SetCalibratedOverlayPath("")

	entries := -1
	if ov, err := config.LoadCalibrationOverlay(dst); err == nil && ov != nil {
		entries = len(ov.Entries)
	}

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationMetrics(workDir, collector, time.Now().UTC())
	t.Logf("baseline: run_ok=%v fresh=%v code=%q age=%.0fs", obs.Baseline.RunOK, obs.Baseline.Fresh, obs.Baseline.FreshnessCode, obs.Baseline.AgeSeconds)
	t.Logf("overlay : run_ok=%v fresh=%v code=%q age=%.0fs bytes=%d entries=%d", obs.Overlay.RunOK, obs.Overlay.Fresh, obs.Overlay.FreshnessCode, obs.Overlay.AgeSeconds, len(raw), entries)
	t.Logf("drift   : run_ok=%v code=%q keys=%d %v", obs.Drift.RunOK, obs.Drift.UnverifiableCode, len(obs.Drift.DriftedKeys), obs.Drift.DriftedKeys)
	t.Logf("drift   : out_of_window=%v not_applicable=%v ssot_moved=%v", obs.Drift.OutOfWindowKeys, obs.Drift.UnknownKeys, obs.Drift.InvalidatedKeys)
	t.Logf("/metrics:\n%s", scrapeMetrics(t, collector))
}
