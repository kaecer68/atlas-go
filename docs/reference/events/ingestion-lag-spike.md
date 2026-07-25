# IngestionLagSpike 事件

> **事件常數**：`EventIngestionLagSpike = "apigateway.ingestion.lag.spike"`
> **對應 EventType 字串值**：`apigateway.ingestion.lag.spike`
> **Severity**：warning（注意：5 個 YELLOW 事件中唯一非 info 級別）
> **Schema Version**：1

## 觸發條件

IngestionLagMonitor 透過 constructor DI 注入的 `IngestionLagProvider` 讀取 ingestion p99 latency。

每 30 秒（`IngestionLagCheckInterval`）檢查：
1. 從 `provider.P99LatencySeconds()` 讀 p99 latency
2. 若 p99 < 5s（`IngestionLagP99Threshold`）→ 不 emit
3. 若 60 秒內（`IngestionLagDedupWindow`）已 emit 過 → 跳過（dedup）
4. 否則 → emit `EventIngestionLagSpike`

## Payload Schema

```json
{
  "p99_latency_seconds": 7.5,
  "threshold_seconds": 5.0,
  "detected_at": "2026-06-22T10:30:00Z"
}
```

| 欄位 | 型別 | 說明 |
|------|------|------|
| `p99_latency_seconds` | float64 | 觀察到的 p99 latency（秒） |
| `threshold_seconds` | float64 | 觸發閾值（固定 5.0） |
| `detected_at` | time.Time (RFC3339) | 事件 emit 時間 |

## 對應 Source

- `IngestionLagProvider` interface：`P99LatencySeconds() float64`
- 生產實作：`internal/monitoring/service/ingestion_lag_provider.go` 的 `ChannelHealthIngestionLagProvider`
  - 從 `ChannelHealthStore.ChannelLatencyMs(channelID)` 讀取每個 channel 的毫秒級延遲
  - 對所有 channel 的延遲排序後取 99th percentile
  - 無 latency 資料的 channel 會被忽略；全部無資料時回傳 0
- 底層 store：`internal/apigateway/health.go` 的 `UnifiedHealthStore` 維護 `ChannelLatencyMs`
- ✅ **PR #695 已完成生產資料來源**：不再需要額外在 `internal/apigateway/background.go` 新增 histogram
- 殘留 gaps：目前僅測量 fetch 完成後的 channel latency，fill-driven latency 仍由 background task 日誌間接觀察

## 對應 Alert Rule

- 對應 Prometheus rule 檔案已於 2026-07-25 移除：原 `monitoring/rules/disabled/wave9_ingestion_lag_spike.yml` 引用 `ingestion_latency_seconds_bucket` 但該 histogram metric 從未 emit；`EventIngestionLagSpike` 仍透過 eventbus 發布，未來若實作 metric emit 應連同 rule 重新建立。

## 對應 Service

- `internal/monitoring/service/ingestion_lag_monitor.go`：`IngestionLagMonitor` 介面 + 實作
- `internal/monitoring/service/ingestion_lag_provider.go`：`ChannelHealthIngestionLagProvider` 生產實作
- 依賴注入：`NewIngestionLagMonitor(bus eventbus.EventBus, provider IngestionLagProvider)`
- `IngestionLagProvider` interface 定義在 service/ 內（**不 import apigateway**，避免 import cycle）

## 測試

- `internal/monitoring/service/ingestion_lag_monitor_test.go`：
  - `TestIngestionLagMonitor_NoEmitBelowThreshold`：2s latency 不 emit
  - `TestIngestionLagMonitor_EmitsAboveThreshold`：7.5s emit + 驗證 severity=warning
  - `TestIngestionLagMonitor_DedupWithinWindow`：60 秒內只 emit 一次
  - `TestIngestionLagMonitor_EmitsAgainAfterDedupWindow`：60 秒後重新 emit
  - `TestIngestionLagMonitor_NilProviderNoEmit`：nil provider 不 panic
  - `TestIngestionLagMonitor_StartStopLifecycle`：背景 poll 正常運作
- `internal/monitoring/service/ingestion_lag_provider_test.go`：
  - 驗證 `ChannelHealthIngestionLagProvider.P99LatencySeconds()` 從 mock health store 正確計算 p99

## Forward-Compat 驗證

- ✅ monitoring/service 不 import internal/apigateway（避免 import cycle）
- ✅ 0 修改 #611 9 個檔案

## 啟用情境（建議）

- **Production 即時監控**：當 ingestion p99 > 5s 立即通知 on-call
- **Provider 健康監控**：可推斷哪個 data provider 變慢（搭配 ChannelIndividualHealth）
- **回測驗證**：Wave 9.6 整合測試時驗證 ingestion lag 與回測準確度關係