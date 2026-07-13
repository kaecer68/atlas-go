# Wave 9 Observability — 5 偵測器協調器

> 類型：release handoff  
> 來源：`internal/monitoring/AGENTS.md`  
> 日期：2026-06-26（隨 PR #749 文件整理搬遷）

`internal/monitoring/wave9_runtime.go` 引入 `Wave9Observability`，負責在 live mode 下統一啟動、協調與關閉 5 個 Wave 9 觀測性偵測器。

## 協調的 5 個偵測器

| 偵測器 | 檔案 | 產出事件 | 職責 |
|--------|------|---------|------|
| `RegimeDebouncer` | `service/regime_debouncer.go` | `EventRegimeChangeConfirmed` | regime 穩定 30 秒後發布確認訊號 |
| `FactorWeightRegressionDetector` | `service/factor_weight_regression.go` | `EventFactorWeightRegression` | regime 變化後偵測 factor weight 位移 |
| `DriftDetector` (v2) | `service/drift_detector.go` + `drift_helpers.go` | `EventDriftDetected` | 集中度 / turnover / target weights drift |
| `ChannelHealthSynthesizer` | `service/channel_health_synthesizer.go` | `EventChannelIndividualHealth` | 將 per-channel 錯誤轉為事件 |
| `IngestionLagMonitor` | `service/ingestion_lag_monitor.go` | `EventIngestionLagSpike` | p99 ingestion latency > 5s 時預警 |

## `detectorFactory` 介面模式

為了在測試中注入 spy，`Wave9Observability` 透過內部 `detectorFactory` 介面抽象偵測器建構：

```go
type detectorFactory interface {
    newRegimeDebouncer(bus eventbus.EventBus) service.RegimeDebouncer
    newFactorWeightRegressionDetector(bus eventbus.EventBus, provider service.WeightProvider) service.FactorWeightRegressionDetector
    newDriftDetector(bus eventbus.EventBus, provider service.TargetWeightsProvider) service.DriftDetector
    newChannelHealthSynthesizer(bus eventbus.EventBus, provider service.ChannelHealthProvider) service.ChannelHealthSynthesizer
    newIngestionLagMonitor(bus eventbus.EventBus, provider service.IngestionLagProvider) service.IngestionLagMonitor
}
```

生產實作為 `defaultDetectorFactory`；`withDetectorFactory` 選項僅供測試使用。

## 啟動與關閉順序

**啟動順序**（`Start`）：
1. `RegimeDebouncer` 先啟動（其他偵測器可能依賴穩定 regime 訊號）。
2. `IngestionLagMonitor`、`ChannelHealthSynthesizer`、`FactorWeightRegressionDetector` 並行啟動。
3. `DriftDetector` 最後啟動（它需要 regime 與 position update 都已就位）。

**關閉順序**（`Stop` / `Close`）— **LIFO**：
1. `DriftDetector`
2. `FactorWeightRegressionDetector`
3. `ChannelHealthSynthesizer`
4. `IngestionLagMonitor`
5. `RegimeDebouncer`

任何一個啟動失敗會立即回傳錯誤。**v0.0.0.18+**：`errs` channel 改用 `errors.Join` 聚合所有平行啟動失敗（不再只回傳第一個）；失敗時 `defer` cleanup 會以 LIFO 順序 `Stop()` 已經成功啟動的 detector，並把 `w.started = false` 與所有 detector 欄位參照清空（讓 retry 拿到 fresh instances）；cleanup 過程中任何 `Stop()` 失敗同樣以 `errors.Join` 摺進最終回傳的 error，呼叫端可從 error chain 區分「partial-failure 但 cleanup 乾淨」與「partial-failure 且 leaked subscriptions」。

## 必要與選用 providers

| Provider | 必要性 | 注入選項 |
|----------|--------|---------|
| `ChannelHealthProvider` | 必要 | `WithChannelHealthProvider` |
| `IngestionLagProvider` | 必要 | `WithIngestionLagProvider` |
| `WeightProvider` | 選用 | `WithWeightProvider`；nil 時 factor regression detector no-op |
| `TargetWeightsProvider` | 選用 | `WithTargetWeightsProvider`；nil 時 drift detector 降級為 v1 |

生產環境的 provider 設定請參考 `docs/environment.md`。

## 與 Dashboard API 的關係

`Wave9Observability` 本身不暴露 HTTP endpoint；它產生的事件經由 `EventBus` 進入 `internal/monitoring/api/events/sse_handler.go`（緩衝管理）與 `internal/monitoring/api/events/sse_handler_subscriptions.go`（**v0.0.0.18+**：批次訂閱註冊 helper `apievents.RegisterDashboardBufferSubs(bus)`，跨模式共用），再推送至 dashboard 即時事件流。`cmd/atlas/main.go` 的 `run()` 與 `runLiveTrading()` 都會呼叫 `RegisterDashboardBufferSubs` 與 `risk.NewAuditSubscriber` 重新註冊在當下使用的 bus（`dashEventBus` 與 `eventBus` 是兩個獨立 bus 實例），確保 `runLiveTrading` 的 SSE catchup 不會是空的。運維時可在 log 中搜尋 `started component=wave9_observability` 與 `wave9_observability stopped` 確認生命週期。

## 向後相容

- `NewDriftDetector(bus)` 保留（無 target drift 能力）
- v1 6 個測試一字不改
- v1 payload 欄位全部保留
- `DriftEventSchemaVer` 從 1 bump 到 2（消費者可透過此欄位區分）
- `thresholds.target_drift` 一律存在（即使 nil provider），讓前端可顯示閾值
