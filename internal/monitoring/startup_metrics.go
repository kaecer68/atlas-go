package monitoring

// Startup & runtime 階段的 metrics 埋點 helper。
//
// 設計動機：atlas-go 的 Prometheus 框架 (cmd/atlas/api_routes.go 的 /metrics 端點)
// 雖已就位，但業務邏輯端從未 increment counter，導致 /metrics 端點 200 但 body 為空。
// 本檔案建立 db_init 與 channel_health 兩個關鍵 failure mode 的 metric emission，
// 讓 Prometheus alert rule 有真實 metric 可查（避免 PR #925 那種引用不存在 metric 的 dead code）。
//
// 命名慣例：
//   - 以 atlas_ 前綴避免與 prometheus default metric 衝突
//   - 以 _total 後綴標示 counter（Prometheus 慣例）
//   - label 名小寫 snake_case、值域受限以避免 cardinality 爆炸

// MetricDBInitFailures 統計 bootstrap 階段 db_init 失敗次數。
// 用途：偵測「atlas 啟動但連不上 DB」這種會讓整個服務 silently degraded 的故障。
// Label phase 保留未來擴展（如 "startup"、"migration"）。
const MetricDBInitFailures = "atlas_db_init_failures_total"

// MetricChannelHealthErrors 統計 channel health check 失敗次數（per channel label）。
// 用途：偵測單一資料源（如 us_yahoo）持續 error，支援分通道告警。
// Label channel 值域為已知通道名（如 "us_yahoo", "fugle"），由呼叫端傳入。
const MetricChannelHealthErrors = "atlas_channel_health_errors_total"

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