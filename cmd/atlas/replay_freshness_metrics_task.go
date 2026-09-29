package main

// replay 資料新鮮度匯出任務（replay 告警的 production 半邊）。
//
// 為什麼存在（2026-09-27 實查，唯讀）
// ----------------------------------
// 生產 Prometheus 在這次接線之前**完全沒有** replay「資料內容」的訊號：
//
//   atlas_channel_health_status{channel="twse_replay_sync"}   ← 抓取層狀態
//   atlas_channel_health_status{channel="twse_replay"}        ← 抓取層狀態
//   atlas_channel_data_staleness_seconds{channel="twse_replay"} ← 取自 channel_health.json
//                                                                 的 last_fetch_at（最後
//                                                                 「抓取」距今，不是資料日期）
//
// 2026-08-24 ~ 2026-09-24 期間 CSV→JSONL 轉檔停擺一個月，上面這三條序列**全程正常**
// （`twse_replay_sync` 的每日 fetch 成功、`twse_replay` 的 hourly fetch 也成功），
// 因為它們量的是「有沒有抓到檔」，不是「檔案裡最新的一天是哪一天」。
// ⇒ 資料內容落後在那段時間是**零觀測**。
//
// 為什麼是背景任務而不是新 cron / textfile collector
// --------------------------------------------------
// 應用其實**已經在算**這個值：`internal/replay.GetLatestDate`（.csv 與 .jsonl 都支援，
// jsonl 走尾端 chunk 讀取不載入全檔）是 `cmd/atlas` 既有的 helper，`monitoring/service`
// 的 `checkReplayHealth` 與 `adapter_twse.HealthCheck` 也已用「最新資料日」的年齡
// （<3d ok / <14d warn / 其後 error）判定健康。缺的只是把它接上 `/metrics`。
// 本檔就是那條接線，走與 `channel_health_metrics_export` /
// `calibration_freshness_metrics_export` 完全相同的管線：不需要動 production
// （不新增 cron、不依賴 textfile collector、不新增監控設定樹）。
//
// 輸出（4 個 gauge；無 label，各 1 條序列）
// ----------------------------------------
//   atlas_replay_csv_latest_date_timestamp_seconds
//       replay CSV 內最新的**資料日**（正規化為該日 UTC 00:00 的 Unix 秒）
//   atlas_replay_jsonl_latest_date_timestamp_seconds
//       轉檔後 JSONL 內最新的資料日（同上；讀不到時**不輸出**該序列）
//   atlas_replay_jsonl_behind_trading_days
//       JSONL 落後 CSV 幾個**交易日**（#2145；穩態＝**1**，不是 0 —— 詳見常數註解）。
//       兩個日期都讀得到時才輸出（缺席 ≠ 0）。
//   atlas_replay_freshness_checked_timestamp_seconds
//       本任務最後一次執行的 Unix 秒（探針心跳）
//
// ⚠️ 兩個刻意設計，判讀規則前必讀
// -------------------------------
//  1. **值只保留日期**（UTC 00:00）。CSV 的日期欄是 `2026-09-24`、JSONL 是
//     `2026-09-24T00:00:00Z`；只留日期才能相減（CSV↔JSONL 是否同步），也才能用
//     PromQL 的 `day_of_week()` 判「最新資料日是不是週末」（幻影列的特徵）。
//  2. **讀不到就不輸出**，不寫 0。0 會被讀成「1970 年」＝看起來像「落後 56 年」，
//     那是本 repo 已定案的缺陷形狀（缺資料被當成值為 0；見 issue #1995）。
//     「JSONL 序列缺席」由規則的 `unless` 子句負責告警，不是靠哨兵值。
//     缺席時記一則結構化 WARN 當第二條可見路徑。

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/replay"
)

const (
	// MetricReplayCSVLatestDate is the newest data date present in the replay CSV.
	MetricReplayCSVLatestDate = "atlas_replay_csv_latest_date_timestamp_seconds"
	// MetricReplayJSONLLatestDate is the newest data date present in the
	// converted JSONL that FactorEngine consumes.
	MetricReplayJSONLLatestDate = "atlas_replay_jsonl_latest_date_timestamp_seconds"
	// MetricReplayFreshnessChecked is the probe heartbeat (last run of this task).
	MetricReplayFreshnessChecked = "atlas_replay_freshness_checked_timestamp_seconds"
	// MetricReplayJSONLBehindTradingDays is how many Taiwan TRADING days the
	// replay JSONL is behind the replay CSV by (#2145): the trading days in the
	// half-open interval (JSONL latest data date, CSV latest data date] — the
	// CSV endpoint IS included, because it is the session the NEXT conversion
	// will write.
	//
	// ⚠️ Steady state is therefore **1, not 0** ("CSV has today's session, JSONL
	// has the previous data day"). Do not "fix" a healthy 1 into 0: the metric
	// counts trading days AFTER the JSONL's newest date, and the rule's
	// threshold (>= 3) is calibrated against that convention.
	//
	// Emitted only when BOTH dates are readable: like the other replay gauges, a
	// missing input must not be reported as 0 (that would read as "caught up",
	// the absent-means-zero defect shape of issue #1995). Non-trading days
	// (weekends and Taiwan holidays) are skipped through the single authority
	// taiwanholidays.IsTradingDay, which is what makes a long holiday — CSV and
	// JSONL separated by several calendar days but no trading day — read as 1
	// instead of firing the old calendar-day rule.
	//
	// One series, no labels (there is one replay dataset), matching the rest of
	// this family.
	MetricReplayJSONLBehindTradingDays = "atlas_replay_jsonl_behind_trading_days"
)

// replayFreshnessMetricsInterval 是匯出週期。
//
// 5m 與 `channel_health_metrics_export` / `calibration_freshness_metrics_export` 相同
// （同一個 manager、同一種 gauge 品質）。為什麼不更短：這個量一天最多前進一次
// （replay CSV 的寫入排程是每日 15:30 UTC，見 docs/operations/docker-compose.crons.yml），
// 5m 已比訊號本身的變化快 288 倍。為什麼不更長：gauge 是 last-write-wins，
// 週期必須明顯小於規則的 `for:`（30m 以上）與 Prometheus 的 5m lookback（最新樣本
// 過期後查詢就看不到序列了）。
const replayFreshnessMetricsInterval = 5 * time.Minute

// registerReplayFreshnessMetricsTask wires the replay_freshness_metrics_export
// background task into the BackgroundTaskManager.
func registerReplayFreshnessMetricsTask(d backfillDeps) {
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "replay_freshness_metrics_export",
		ChannelID: "",
		Interval:  replayFreshnessMetricsInterval,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			exportReplayFreshnessMetrics(d.cfg.ReplayDataPath, d.collector, time.Now())
			// 永遠回 nil：檔案讀不到是**被觀測的狀態**（序列缺席 + 規則 + WARN log），
			// 不是任務錯誤。回 error 會讓 manager 的失敗處理再告警一次，同一件事
			// 會有兩個出口（本 repo 對重複 paging 有明確紀律，見
			// calibration_freshness_metrics_task.go 同段註解）。
			return nil
		},
	})
	log.Printf("[Gateway] registered replay_freshness_metrics_export background task (5m interval)")
}

// exportReplayFreshnessMetrics 讀 replay CSV 與其轉檔 JSONL 的最新資料日並輸出 gauge。
//
// replayCSVPath 為空時回退到 `constants.ReplayCSVPath`（與 `cmd/daily-replay-sync`
// 的 `-csv` 預設值同一個常數，避免兩邊各記一份路徑）。
func exportReplayFreshnessMetrics(replayCSVPath string, collector *monitoring.MetricsCollector, now time.Time) {
	if collector == nil {
		return
	}
	if replayCSVPath == "" {
		replayCSVPath = constants.ReplayCSVPath
	}
	collector.RecordGauge(MetricReplayFreshnessChecked, float64(now.Unix()), nil)

	csvDate, csvErr := replayLatestDate(replayCSVPath)
	if csvErr != nil {
		logging.Warn("replay_freshness", "csv_latest_date_unreadable",
			"path", replayCSVPath,
			"err", csvErr.Error(),
			"reason", "CSV 讀不到 ⇒ 本輪不輸出資料日序列（不寫 0：0 會被讀成 1970 年）",
		)
	} else {
		collector.RecordGauge(MetricReplayCSVLatestDate, float64(csvDate.Unix()), nil)
	}

	jsonlPath := replayJSONLPath(replayCSVPath)
	jsonlDate, jsonlErr := replayLatestDate(jsonlPath)
	if jsonlErr != nil {
		// 不輸出序列：缺席由規則的 `unless` 子句處理（見檔頭 §刻意設計 2）。
		logging.Warn("replay_freshness", "jsonl_latest_date_unreadable",
			"path", jsonlPath,
			"err", jsonlErr.Error(),
			"reason", "JSONL 讀不到（轉檔沒跑過或檔案被刪）⇒ 不輸出序列;規則以 unless 判「CSV 有、JSONL 沒有」",
		)
	} else {
		collector.RecordGauge(MetricReplayJSONLLatestDate, float64(jsonlDate.Unix()), nil)
	}

	// #2145：落後幾個**交易日**（日曆日比較會讓長連假誤報）。
	// 只在兩個日期都讀得到時輸出 —— 缺席由規則的 `unless` arm 處理，不得寫 0（#1995）。
	if csvErr == nil && jsonlErr == nil {
		collector.RecordGauge(
			MetricReplayJSONLBehindTradingDays,
			float64(marketdata.TradingDaysBetween(jsonlDate, csvDate)),
			nil,
		)
	}
}

// replayJSONLPath 由 CSV 路徑推導轉檔產物路徑。
//
// 這條規則與寫入端完全相同（`cmd/atlas/operations_tasks.go` 的 auto_backfill
// 把 CSV 轉成 `strings.TrimSuffix(path, ".csv") + ".jsonl"`）——刻意共用同一條推導，
// 不新增第二個可設定的路徑，否則「檢查 A、寫入 B」會是下一個 E10 級缺陷。
func replayJSONLPath(replayCSVPath string) string {
	return strings.TrimSuffix(replayCSVPath, ".csv") + ".jsonl"
}

// replayLatestDate 回傳 replay 檔內最新資料日，正規化為該日 UTC 00:00。
//
// 刻意丟掉時分秒：CSV 的日期欄只有日期（`2026-09-24`），JSONL 是
// `2026-09-24T00:00:00Z`；兩者要能相減，且 `day_of_week()` 要能反映「資料日」
// 的星期。以 UTC 00:00 表示日期時，`day_of_week()` 的 UTC 語意正好等於該日曆日的星期。
func replayLatestDate(path string) (time.Time, error) {
	raw, err := replay.GetLatestDate(path)
	if err != nil {
		return time.Time{}, err
	}
	if len(raw) > 10 {
		raw = raw[:10]
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse latest date %q: %w", raw, err)
	}
	return d, nil
}
