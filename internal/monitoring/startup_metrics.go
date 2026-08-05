package monitoring

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Startup & runtime 階段的 metrics 埋點 helper。
//
// 設計動機：atlas-go 的 Prometheus 框架 (cmd/atlas/api_routes.go 的 /metrics 端點)
// 雖已就位，但業務邏輯端從未 increment counter，導致 /metrics 端點 200 但 body 為空。
// 本檔案建立 db_init 與 channel_health 兩個關鍵 failure mode 的 metric emission，
// 讓 Prometheus alert rule 有真實 metric 可查（避免 PR #925 那種引用不存在 metric 的 dead code）。
//
// 命名慣例：
//   - 以 atlas_ 前綴避免與 prometheus default metric 衝突
//   - 以 _total 後綴標示 counter（Prometheus 慣例）；gauge 不加 _total
//   - label 名小寫 snake_case、值域受限以避免 cardinality 爆炸

// MetricDBInitFailures 統計 bootstrap 階段 db_init 失敗次數。
// 用途：偵測「atlas 啟動但連不上 DB」這種會讓整個服務 silently degraded 的故障。
// Label phase 保留未來擴展（如 "startup"、"migration"）。
const MetricDBInitFailures = "atlas_db_init_failures_total"

// MetricChannelHealthErrors 統計 channel health check 失敗次數（per channel label）。
// 用途：偵測單一資料源（如 us_yahoo）持續 error，支援分通道告警。
// Label channel 值域為已知通道名（如 "us_yahoo", "fugle"），由呼叫端傳入。
const MetricChannelHealthErrors = "atlas_channel_health_errors_total"

// MetricStage3TaskRuns 統計 Stage 3 排程任務執行次數（per task × result label）。
// Label task 值域固定為 5 個 stage3 task ID；result ∈ {success,failed}。
const MetricStage3TaskRuns = "atlas_stage3_task_runs_total"

// MetricStage3AlertsFired 統計 Stage 3 alert rule 觸發次數（per rule × severity label）。
// Label rule 值域固定為 6 個 stage3 rule ID；severity ∈ {critical,warning,info}。
const MetricStage3AlertsFired = "atlas_stage3_alerts_fired_total"

// MetricStage3LedgerRecords 暴露目前 ledger 內 record 數量（gauge，不是 counter）。
// 命名沒有 _total 後綴是因為 gauge 加 _total 違反 Prometheus 慣例。
// Label ledger 值域目前固定 1 個（"event_flow_prediction"）；未來新增其他 ledger 再擴。
// 由 cmd/atlas/stage3_tasks.go 的 OnTaskComplete callback 觸發更新,典型 cadence 是 daily。
const MetricStage3LedgerRecords = "atlas_stage3_ledger_records"

// MetricDataAggregatorFailures 統計 DataAggregator.AggregateIndustry 失敗次數
// (per industry × kind label)。用途：把 `auto_cycle_update` channel 持續 error
// 拆成可區分的根因 (quota / rate_limited / no_data / parse_error / transport / unknown)，
// 取代只看 `last_error` 字串的 single-string 監控。詳見 docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md。
// Label industry 值域為已知 L1 industry ID (semiconductor/electronics/leo_satellite/...)；
// kind ∈ {"quota","rate_limited","no_data","parse_error","transport","unknown"}，值域受限避免 cardinality 爆炸。
const MetricDataAggregatorFailures = "atlas_data_aggregator_failures_total"

// RecordDBInitFailure increment db_init failure counter。
// nil collector 安全（bootstrap 早期 collector 可能尚未建立）。
func RecordDBInitFailure(c *MetricsCollector) {
	if c == nil {
		return
	}
	c.RecordCounter(MetricDBInitFailures, 1, map[string]string{
		"phase": "startup",
	})
}

// RecordChannelHealthError increment 指定 channel 的 health error counter。
// 空 channel name 視為無效輸入，不寫入（避免建立 label="" 的孤立 entry）。
// nil collector 安全。
func RecordChannelHealthError(c *MetricsCollector, channel string) {
	if c == nil || channel == "" {
		return
	}
	c.RecordCounter(MetricChannelHealthErrors, 1, map[string]string{
		"channel": channel,
	})
}

// RecordStage3TaskRun nil collector 安全；空 taskID/result 視為無效輸入不寫入。
func RecordStage3TaskRun(c *MetricsCollector, taskID, result string) {
	if c == nil || taskID == "" || result == "" {
		return
	}
	c.RecordCounter(MetricStage3TaskRuns, 1, map[string]string{
		"task":   taskID,
		"result": result,
	})
}

// RecordStage3AlertFired nil collector 安全;severity 自動小寫對齊 Prometheus convention。
func RecordStage3AlertFired(c *MetricsCollector, ruleID string, severity AlertLevel) {
	if c == nil || ruleID == "" {
		return
	}
	c.RecordCounter(MetricStage3AlertsFired, 1, map[string]string{
		"rule":     ruleID,
		"severity": strings.ToLower(severity.String()),
	})
}

// RecordStage3LedgerRecords 寫入目前 ledger 大小到 gauge。nil collector + nil store 都安全。
// 透過覆寫語意(RecordGauge)取代累加語意(RecordCounter):連續呼叫會把值改成最新 Len(),
// 不會誤加成累積值。Prometheus scrape 拉到的值始終 ≤ Len() 上限(1000 records from FIFO cap)。
func RecordStage3LedgerRecords(c *MetricsCollector, store ledger.EventFlowPredictionStore) {
	if c == nil || store == nil {
		return
	}
	c.RecordGauge(MetricStage3LedgerRecords, float64(store.Len()), map[string]string{
		"ledger": "event_flow_prediction",
	})
}

// RecordDataAggregatorFailure increment DataAggregator.AggregateIndustry 失敗次數。
// 用途：把 `auto_cycle_update` channel 持續 error 的根因拆成可監控的 kind label，
// 取代只看 channel_health.json 上一筆 frozen last_error 的盲點。
// kind 值域固定為 {"quota","rate_limited","no_data","parse_error","transport","unknown"}：
//   - quota:        FinMind 回 402 或 DailyQuotaTracker 觸發 ErrQuotaExhausted
//   - rate_limited: FinMind 回 429 或 ErrRateLimited
//   - no_data:      FinMind 回 200 但 data array 為空（symbol 沒收錄或月營收尚未 publish）
//   - parse_error:  FinMind 回 200 但欄位 parse 失敗（schema 變更）
//   - transport:    HTTP timeout / DNS / connection refused（網路問題）
//   - unknown:      其他未分類 error
//
// 空 industry / kind 視為無效輸入不寫入；nil collector 安全。
func RecordDataAggregatorFailure(c *MetricsCollector, industry, kind string) {
	if c == nil || industry == "" || kind == "" {
		return
	}
	c.RecordCounter(MetricDataAggregatorFailures, 1, map[string]string{
		"industry": industry,
		"kind":     kind,
	})
}
