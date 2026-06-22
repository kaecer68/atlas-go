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
- 預期實作：在 `internal/apigateway/background.go` 加 `ingestion_latency_seconds` Prometheus histogram
  - 由 `BackgroundTaskManager.RecordIngestionLatency(d time.Duration)` 記錄每個 ingestion 操作的耗時
  - 由 `*BackgroundTaskManager` 實作 `IngestionLagProvider` interface，回傳 `histogram_quantile(0.99, rate(...))`
- ⚠️ **本 PR 範圍**：只提供 service 框架（monitoring/service/）與 IngestionLagProvider interface
- ⚠️ **待 follow-up PR**：`internal/apigateway/background.go` 加 histogram + 實作 IngestionLagProvider（**不在 #611 list**，可直接修改）
- ⚠️ **目前行為**：未接 provider 時 `nil` 不 emit，service 仍可啟動但永遠不觸發

## 對應 Alert Rule

- `monitoring/rules/wave9_ingestion_lag_spike.yml`：當 `histogram_quantile(0.99, rate(ingestion_latency_seconds_bucket[5m])) > 5` 持續 5 分鐘觸發 warning
- 預設 `enabled: false`（PD-W9-1）

## 對應 Service

- `internal/monitoring/service/ingestion_lag_monitor.go`：`IngestionLagMonitor` 介面 + 實作
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

## Forward-Compat 驗證

- ✅ monitoring/service 不 import internal/apigateway（避免 import cycle）
- ✅ 0 修改 #611 9 個檔案
- ✅ `internal/apigateway/background.go` 不在 #611 list，可獨立 PR 加 histogram（不影響本 PR）

## 待辦 Follow-up

1. **`internal/apigateway/background.go` 加 histogram**（獨立 PR）：
   ```go
   ingestionLatencySeconds prometheus.Histogram

   func (m *BackgroundTaskManager) RecordIngestionLatency(d time.Duration) {
       m.ingestionLatencySeconds.Observe(d.Seconds())
   }
   ```
2. **`*BackgroundTaskManager` 實作 `IngestionLagProvider`**：
   ```go
   func (m *BackgroundTaskManager) P99LatencySeconds() float64 {
       // 從 histogram exporter 讀 p99
   }
   ```
3. **wiring**：在 `cmd/atlas/main.go` 將 `*BackgroundTaskManager` 注入 `NewIngestionLagMonitor`

## 啟用情境（建議）

- **Production 即時監控**：當 ingestion p99 > 5s 立即通知 on-call
- **Provider 健康監控**：可推斷哪個 data provider 變慢（搭配 ChannelIndividualHealth）
- **回測驗證**：Wave 9.6 整合測試時驗證 ingestion lag 與回測準確度關係