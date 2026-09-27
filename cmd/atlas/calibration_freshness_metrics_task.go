package main

// 校準產物新鮮度與 drift 匯出任務（issue #1944 I31 的 production 半邊；#2007 的兩個
// bounded 子項）。
//
// 背景：PR #1991 定了「CI 只驗結構、freshness 由 production 主機執行」的政策，
// 但把「接上生產監控」明示為未完成。本檔就是那個缺失的接線：用既有的
// BackgroundTaskManager 週期性評估校準產物，把結果交給 `internal/monitoring` 的
// gauge 族，由 `cmd/atlas/api_routes.go` 既有的 `/metrics` 端點輸出。
//
// 為什麼是背景任務而不是 CLI/部署腳本：
//  * 這條路徑不需要動 production（不得新增 cron、不得依賴 textfile collector），
//    與 `channel_health_metrics_export`、`atlas_universe_*` 走完全相同的管線。
//  * 「檢查本身有沒有在跑」也變成可觀測的（`..._checked_timestamp_seconds`）——
//    外部 cron 的失敗在 atlas 的監控裡是看不到的。
//
// 與 `cmd/calibration-validate` 的關係：**同一套判定**。兩者都呼叫
// `config.ValidateCalibration`，差別只在輸出（前者人類/CI 可讀 JSON 與 exit code，
// 後者是 Prometheus 序列）。這是刻意的：不重寫第二套 staleness 語意。
//
// 三個觀察（#2007 之後）
// ---------------------
// 原本只評估 `configs/parameters.json`（image 內基準）。#2013 之後容器內校準改寫
// bind mount 上的 overlay（`data/state/parameters.calibrated.json`），基準在生產上
// 根本不會被寫 ⇒ 那個檢查量的是**錯的標的**，`CalibrationArtifactStale` 變成永久
// 誤報（2026-09-27 實測：age ≈ 83 天 = image 建置日，而 overlay 是當天寫的）。
// 因此本任務每一輪做三件事，全部在同一個時戳下：
//
//  1. `exportCalibrationFreshnessMetrics`——image 內基準（artifact=`parameters`）。
//     **語意不變**（仍然回答「基準多久沒被換過」），但狀態列**不再有告警規則**：
//     它在生產的「不新鮮」是預期狀態（基準只在重新部署時變）。log 也配合調整：
//     「不新鮮」不再每 5 分鐘 WARN（那正是永久誤報的另一半），只有
//     「無法評估」（run_ok=0，製程真的讀不到自己的參數檔）才留 WARN。
//  2. `exportCalibrationOverlayFreshness`——權威的 effective 產物
//     （artifact=`parameters_overlay`）。這是**告警規則現在的標的**。
//  3. `exportCalibrationDrift`——image 基準 vs overlay 疊加後生效值的差異
//     （`atlas_calibration_drift_*`）：把 #2007「repo 0.03 / effective 0.0108 而沒有
//     任何痕跡」變成可查詢的事實。drift 本身是 intended（業主定案），所以
//     只有**無法解釋**的偏離才告警（見 internal/monitoring/calibration_effective.go）。

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// calibrationFreshnessMetricsInterval 是匯出週期。
//
// 5m 與 `channel_health_metrics_export` 相同（同一個 manager、同一種 gauge 品質）。
// 為什麼不更短：這個量一天最多變一次（校準任務的 cadence 是 24h），5m 已經比訊號
// 本身的變化快 288 倍；為什麼不更長：gauge 是 last-write-wins，週期必須明顯小於
// 規則的 `for:`（都在 30m 以上）與 Prometheus 的 5m lookback，否則序列會週期性缺席。
const calibrationFreshnessMetricsInterval = 5 * time.Minute

// registerCalibrationFreshnessMetricsTask 把 calibration_freshness_metrics_export
// 註冊進 BackgroundTaskManager（形狀與 registerChannelHealthMetricsTask 相同）。
func registerCalibrationFreshnessMetricsTask(d backfillDeps) {
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "calibration_freshness_metrics_export",
		ChannelID: "",
		Interval:  calibrationFreshnessMetricsInterval,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			// 一個時戳、三個觀察：三個 gauge 族在同一輪對齊（同一個 now），
			// 值班把它們放在同一張圖上比較時不會有亞秒級的錯位。
			exportCalibrationMetrics(d.cfg.WorkDir, d.collector, time.Now())
			// 永遠回 nil：檢查失敗是**被觀測的狀態**（run_ok=0 + 專門的規則），
			// 不是任務錯誤。回 error 會讓 manager 的失敗處理再告警一次，
			// 同一件事會有兩個出口（本 repo 對重複 paging 有明確紀律）。
			return nil
		},
	})
	log.Printf("[Gateway] registered calibration_freshness_metrics_export background task (5m interval)")
}

// calibrationMetricsObservation 把一輪的三個觀察一起回傳（測試斷言用；
// production 不讀它——輸出的真相在 `/metrics`）。
type calibrationMetricsObservation struct {
	Baseline monitoring.CalibrationFreshnessObservation
	Overlay  monitoring.CalibrationFreshnessObservation
	Drift    monitoring.CalibrationDriftObservation
}

// exportCalibrationMetrics 是背景任務的單一進入點：同一個 now，三個觀察。
func exportCalibrationMetrics(workDir string, collector *monitoring.MetricsCollector, now time.Time) calibrationMetricsObservation {
	return calibrationMetricsObservation{
		Baseline: exportCalibrationFreshnessMetrics(workDir, collector, now),
		Overlay:  exportCalibrationOverlayFreshness(workDir, collector, now),
		Drift:    exportCalibrationDrift(workDir, collector, now),
	}
}

// exportCalibrationOverlayFreshness 評估**權威產物**（overlay）的新鮮度。
//
// 為什麼「不新鮮」在這裡仍然每輪 WARN（與下面基準那一條刻意不同）：overlay 不新鮮
// 是**告警狀態**（`CalibrationArtifactStale` 的標的），日誌與告警同調；而基準不新鮮
// 在 #2013 之後是預期狀態，WARN 只會變成每日 288 行的噪音。
func exportCalibrationOverlayFreshness(workDir string, collector *monitoring.MetricsCollector, now time.Time) monitoring.CalibrationFreshnessObservation {
	path := calibrationOverlayPath(workDir)
	obs := monitoring.ObserveCalibrationOverlayFreshness(collector, path, now)
	switch {
	case !obs.RunOK:
		logging.Warn("calibration_freshness", "overlay_unverifiable",
			"path", path,
			"code", string(obs.UnverifiableCode),
			"reason", "effective 產物（overlay）無法評估 — freshness 未知（fail-closed：run_ok=0）",
		)
	case !obs.Fresh:
		logging.Warn("calibration_freshness", "overlay_not_fresh",
			"path", path,
			"code", string(obs.FreshnessCode),
			"age_seconds", obs.AgeSeconds,
			"have_age", obs.HaveAge,
			"contract_hours", int(monitoring.CalibrationFreshnessContract.Hours()),
			"last_calibrated", formatCalibrationTimestamp(obs.LastCalibrated),
		)
	}
	return obs
}

// exportCalibrationDrift 輸出 image 基準 vs effective 的差異。
//
// 只有「無法比較」與「無法解釋的偏離」留 WARN——單純的 drift（drift_keys > 0）是
// intended 的正常狀態，每一輪為它寫一則 WARN 等於把雜訊當監控。
func exportCalibrationDrift(workDir string, collector *monitoring.MetricsCollector, now time.Time) monitoring.CalibrationDriftObservation {
	baseline := calibrationParametersPath(workDir)
	overlay := calibrationOverlayPath(workDir)
	obs := monitoring.ObserveCalibrationDrift(collector, baseline, overlay, now)
	switch {
	case !obs.RunOK:
		logging.Warn("calibration_drift", "drift_unmeasurable",
			"baseline_path", baseline,
			"overlay_path", overlay,
			"code", string(obs.UnverifiableCode),
			"reason", "無法比較基準與生效值 — drift 未知（fail-closed：run_ok=0，數值序列凍結）",
		)
	case len(obs.OutOfWindowKeys) > 0 || len(obs.UnknownKeys) > 0:
		// 這兩個條件對應 `CalibrationEffectiveDriftUnexplained`：比值超出單步窗，
		// 或 overlay 宣告了套不上的 entry（＝校準產物被靜默忽略）。
		logging.Warn("calibration_drift", "drift_unexplained",
			"baseline_path", baseline,
			"overlay_path", overlay,
			"drifted_keys", len(obs.DriftedKeys),
			"out_of_window_keys", strings.Join(obs.OutOfWindowKeys, ","),
			"not_applicable_keys", strings.Join(obs.UnknownKeys, ","),
			"ssot_moved_keys", len(obs.InvalidatedKeys),
		)
	}
	return obs
}

// exportCalibrationFreshnessMetrics 評估 **image 內基準**並輸出 gauge 族
// （artifact=`parameters`）。
//
// 正常（新鮮）時**完全靜默**：不寫 log，只更新序列 —— 值班看的是規則，
// 不是每 5 分鐘一則 INFO。「無法評估」時記一則結構化 WARN（scrape 之外的第二條
// 可見路徑，例如 Loki 上的盤查）：製程讀不到自己的參數檔是真的壞掉。
//
// 「不新鮮」同樣靜默，這是 #2007 之後的刻意改變：基準在 #2013 之後只有重新部署
// 才會變（容器內校準走 overlay），所以它的「超過 48h」是**預期狀態**而不是事件；
// 每 5 分鐘一則 WARN（288 行/日）會被值班正確地忽略。狀態仍以 gauge 呈現
// （可見、可查、可畫圖），但不再有規則、不再有日誌——「權威產物有沒有被刷新」
// 由 `exportCalibrationOverlayFreshness` 回答。
func exportCalibrationFreshnessMetrics(workDir string, collector *monitoring.MetricsCollector, now time.Time) monitoring.CalibrationFreshnessObservation {
	path := calibrationParametersPath(workDir)
	obs := monitoring.ObserveCalibrationFreshness(collector, path, now)
	switch {
	case !obs.RunOK:
		logging.Warn("calibration_freshness", "artifact_unverifiable",
			"path", path,
			"code", string(obs.UnverifiableCode),
			"findings", len(obs.Findings),
			"reason", "freshness unknown — 檢查無法評估產物（fail-closed：run_ok=0）",
		)
	}
	// 「基準不新鮮」刻意無日誌（見函式註解）：狀態仍在
	// `atlas_calibration_freshness_ok{artifact="parameters"}` 上，可查可畫圖。
	return obs
}

// calibrationParametersPath 解析 **image 內基準**（`configs/parameters.json`）的路徑。
//
// 這一條仍然是基準，不再是告警標的（見檔頭「三個觀察」）：它回答
// 「repo/映像的那份基準多久沒被換過」，也是 drift 比較的左邊。
//
// 權威來源是應用自己在用的那一個（`config.GetParametersConfigPath()`，即
// `internal/config/parameters.go` 的 `parametersPath`；生產的寫入者用的也是它，
// 見 FU-20260926-07）；只有在它還沒被設定時才回退到 workDir 的慣例路徑
// （與 `cmd/atlas/calibration_tasks.go` 的 `auto_threshold_calibrate` 相同）。
// 兩條路徑都指向同一個檔案，但優先用權威來源可以避免「檢查 A、寫入 B」。
func calibrationParametersPath(workDir string) string {
	if p := config.GetParametersConfigPath(); p != "" {
		return p
	}
	return filepath.Join(workDir, "configs", "parameters.json")
}

// calibrationOverlayPath 解析**權威的 effective 產物**（校準 overlay）的路徑。
//
// 優先序刻意與 `calibrationParametersPath` 同形：
//  1. 製程實際註冊的那一個（`config.GetCalibratedOverlayPath()`）。`cmd/atlas/main.go`
//     啟動時就註冊 `config.CalibrationOverlayPath(cfg.WorkDir)`，所以這是 runtime
//     真正在讀寫的檔案——監控必須看它，而不是「慣例上應該在那裡」的另一個路徑。
//  2. 沒註冊時回退到 workDir 的慣例路徑（`config.CalibrationOverlayPath`，
//     即 `<workDir>/data/state/parameters.calibrated.json`）。這讓「忘了註冊」這件事
//     仍然是**可觀測**的：fallback 路徑不存在 ⇒ `OVERLAY_ABSENT` +
//     `CalibrationArtifactStale`（warning），而不是靜默地什麼都不看。
func calibrationOverlayPath(workDir string) string {
	if p := config.GetCalibratedOverlayPath(); p != "" {
		return p
	}
	return config.CalibrationOverlayPath(workDir)
}

// formatCalibrationTimestamp 讓「從未記錄校準時間」在日誌裡是明確的字串，
// 而不是容易誤讀的 1970 時間戳。
func formatCalibrationTimestamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}
