package main

// quotes 新鮮度匯出任務（F54 phase 1，2026-10-03）。
//
// 背景：quotes 沒有排程結構性保障（2026-10-02 實損 2/117；週一早晨 universe
// build 是下個風險點）。本任務每 5 分鐘查一次 quotes.max(date)，交給
// internal/monitoring 的交易日感知 gauge 族，由既有 /metrics 端點輸出、
// monitoring/rules/quotes_freshness_alerts.yml 判定。管線與
// calibration_freshness_metrics_export 完全相同（同一個 BackgroundTaskManager、
// 同一種 gauge 品質），不新造任何監控系統、不動 production 設定。
//
// 為什麼是背景任務而不是 cron/textfile：「檢查本身有沒有在跑」必須可觀測
// （_checked_timestamp_seconds 心跳），外部 cron 的失敗在 atlas 監控裡看不見。
//
// 後端支援：只有關聯式後端實作 ledger.QuoteMaxDater（postgres/sqlite）。
// JSONL 後端（QuoteStore 介面的另一個實作）不支援 ⇒ 註冊時跳過並留 log，
// 不讓「後端不支援」被誤讀成「資料不新鮮」。

import (
	"context"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// quotesFreshnessMetricsInterval 是匯出週期。與 calibration_freshness 相同：
// 這個量一天最多變一次，5m 遠快於訊號變化；同時必須明顯小於規則的 for:
// （15m/1h）與 Prometheus 5m lookback，否則序列會週期性缺席。
const quotesFreshnessMetricsInterval = 5 * time.Minute

// registerQuotesFreshnessMetricsTask 把 quotes_freshness_metrics_export 註冊進
// BackgroundTaskManager（形狀與 registerCalibrationFreshnessMetricsTask 相同）。
func registerQuotesFreshnessMetricsTask(d backfillDeps) {
	if d.quoteStore == nil {
		log.Printf("[Gateway] quotes_freshness_metrics_export skipped: quote store unavailable")
		return
	}
	maxDater, ok := d.quoteStore.(ledger.QuoteMaxDater)
	if !ok {
		log.Printf("[Gateway] quotes_freshness_metrics_export skipped: quote store backend %T does not implement ledger.QuoteMaxDater", d.quoteStore)
		return
	}
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "quotes_freshness_metrics_export",
		ChannelID: "",
		Interval:  quotesFreshnessMetricsInterval,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			// 永遠回 nil：查詢失敗是**被觀測的狀態**（run_ok=0 + 專門規則），
			// 不是任務錯誤。回 error 會讓 manager 再告警一次，同一件事兩個出口
			// （本 repo 對重複 paging 有明確紀律）。
			exportQuotesFreshnessMetrics(ctx, maxDater, d.collector, time.Now())
			return nil
		},
	})
	log.Printf("[Gateway] registered quotes_freshness_metrics_export background task (5m interval)")
}

// exportQuotesFreshnessMetrics 是背景任務的單一進入點（抽出來讓測試可以直接
// 斷言觀察結果，不用跑排程器）。
func exportQuotesFreshnessMetrics(ctx context.Context, maxDater ledger.QuoteMaxDater, collector *monitoring.MetricsCollector, now time.Time) monitoring.QuotesFreshnessObservation {
	maxDate, err := maxDater.MaxQuoteDate(ctx)
	obs := monitoring.ObserveQuotesFreshness(collector, maxDate, err, now)
	switch {
	case !obs.RunOK:
		logging.Warn("quotes_freshness", "query_failed",
			"reason", "quotes.max(date) 查詢失敗 — 新鮮度未知（fail-closed：run_ok=0）",
		)
	case !obs.Fresh:
		logging.Warn("quotes_freshness", "stale",
			"reason", obs.Reason,
			"max_date", obs.MaxDate.Format("2006-01-02"),
			"expected_date", obs.ExpectedDate.Format("2006-01-02"),
			"age_seconds", int(obs.AgeSeconds),
		)
	}
	return obs
}
