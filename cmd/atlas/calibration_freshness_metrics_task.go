package main

// 校準產物新鮮度匯出任務（issue #1944 I31 的 production 半邊）。
//
// 背景：PR #1991 定了「CI 只驗結構、freshness 由 production 主機執行」的政策，
// 但把「接上生產監控」明示為未完成。本檔就是那個缺失的接線：用既有的
// BackgroundTaskManager 週期性評估 `configs/parameters.json`，把結果交給
// `internal/monitoring` 的 gauge 族（`atlas_calibration_freshness_*`），
// 由 `cmd/atlas/api_routes.go` 既有的 `/metrics` 端點輸出。
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

import (
	"context"
	"log"
	"path/filepath"
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
			exportCalibrationFreshnessMetrics(d.cfg.WorkDir, d.collector, time.Now())
			// 永遠回 nil：檢查失敗是**被觀測的狀態**（run_ok=0 + 專門的規則），
			// 不是任務錯誤。回 error 會讓 manager 的失敗處理再告警一次，
			// 同一件事會有兩個出口（本 repo 對重複 paging 有明確紀律）。
			return nil
		},
	})
	log.Printf("[Gateway] registered calibration_freshness_metrics_export background task (5m interval)")
}

// exportCalibrationFreshnessMetrics 評估校準產物並輸出 gauge 族。
//
// 正常（新鮮）時**完全靜默**：不寫 log，只更新序列 —— 值班看的是規則，
// 不是每 5 分鐘一則 INFO。不新鮮或無法評估時各記一則結構化 WARN
// （這是 scrape 之外的第二條可見路徑，例如 Loki 上的盤查）。
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
	case !obs.Fresh:
		// event 用 artifact_not_fresh 而不是 artifact_stale：這個狀態包含
		// 「超過契約」與「從未記錄校準時間」兩種，日誌不應該替值班的人選邊。
		logging.Warn("calibration_freshness", "artifact_not_fresh",
			"path", path,
			"code", string(obs.FreshnessCode),
			"age_seconds", obs.AgeSeconds,
			"have_age", obs.HaveAge,
			"contract_hours", int(monitoring.CalibrationFreshnessContract.Hours()),
			"last_calibrated", formatCalibrationTimestamp(obs.LastCalibrated),
			"findings", len(obs.Findings),
		)
	}
	return obs
}

// calibrationParametersPath 解析受監控的校準產物路徑。
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

// formatCalibrationTimestamp 讓「從未記錄校準時間」在日誌裡是明確的字串，
// 而不是容易誤讀的 1970 時間戳。
func formatCalibrationTimestamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}
