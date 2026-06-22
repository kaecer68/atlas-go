# ChannelIndividualHealth 事件

> **事件常數**：`EventChannelIndividualHealth = "monitor.channel.health.individual"`
> **對應 EventType 字串值**：`monitor.channel.health.individual`
> **Severity**：info（PD-W9-1）
> **Schema Version**：1

## 觸發條件

當 `internal/monitoring/gateway_adapter.go:39` `ChannelErrors()` 回傳的 per-channel error map 中，**任一 channel 出現錯誤訊息**時觸發。

ChannelHealthSynthesizer 每 30 秒（`ChannelHealthPollInterval`）輪詢一次 gateway adapter，比對上次發出的 channel+error 組合：
- **新錯誤**：立即 emit
- **同樣錯誤已在 5 秒內（`ChannelHealthDedupWindow`）emit 過**：略過（dedup）
- **同樣錯誤已超過 5 秒**：再次 emit（標記持續中）

## Payload Schema

```json
{
  "channel_id": "twse_capital_flow",
  "error_message": "timeout after 5s",
  "first_seen_at": "2026-06-22T10:30:00Z",
  "detected_at": "2026-06-22T10:31:00Z"
}
```

| 欄位 | 型別 | 說明 |
|------|------|------|
| `channel_id` | string | 失敗的 channel ID（與 `internal/apigateway/gateway.go` 的 `channelIDs()` 對齊） |
| `error_message` | string | 該 channel 對應的錯誤訊息 |
| `first_seen_at` | time.Time (RFC3339) | 此 channel+error 組合首次被偵測的時間 |
| `detected_at` | time.Time (RFC3339) | 此次事件 emit 的時間 |

## 對應 Source API

- `internal/monitoring/gateway_adapter.go:32-50` `ChannelErrors()`：回傳 per-channel error map snapshot
- 對應 Layer 2 of 4-layer data-visibility safeguard（見 `internal/monitoring/AGENTS.md`）

## 對應 Alert Rule

- `monitoring/rules/wave9_channel_individual_health.yml`：當 `rate(channel_errors_total[5m]) > 0.1` 持續 5 分鐘觸發 warning
- 預設 `enabled: false`（PD-W9-1），由 operator 決定啟用時機

## 對應 Service

- `internal/monitoring/service/channel_health_synthesizer.go`：`ChannelHealthSynthesizer` 介面 + 實作
- 依賴注入：`NewChannelHealthSynthesizer(bus eventbus.EventBus, provider ChannelHealthProvider)`
- `ChannelHealthProvider` 介面定義在 `service/` package（**不 import `monitoring` package**，避免循環依賴）
- 由 `internal/monitoring/dashboard_api.go` 或同等 wiring 程式負責將 `*macroDataGatewayAdapter` 注入

## 測試

- `internal/monitoring/service/channel_health_synthesizer_test.go`：
  - `TestChannelHealthSynthesizer_EmitsEventPerChannel`：2 個 channel 錯誤 → 2 個事件
  - `TestChannelHealthSynthesizer_DedupWithinWindow`：同樣錯誤 5 秒內只 1 個事件
  - `TestChannelHealthSynthesizer_EmitsAgainAfterDedupWindow`：超過 5 秒後重新 emit
  - `TestChannelHealthSynthesizer_NilProviderNoEmit`：provider 為 nil 不 panic
  - `TestChannelHealthSynthesizer_StartStopLifecycle`：背景 poll 正常運作

## Forward-Compat 驗證

- ✅ 只讀 `ChannelErrors()` public API（gateway_adapter.go 不在 #611 list）
- ✅ monitoring/service 不 import internal/monitoring（避免 import cycle）
- ✅ `cmd/atlas/main.go`（#611 Wave 1 P0）僅需新增 1 行 DI 註冊（`NewChannelHealthSynthesizer(bus, adapter).Start(ctx)`）

## 啟用情境（建議）

- **Production 即時監控**：啟用 alert rule，當單一 channel 持續錯誤時通知 on-call
- **Quiet hours 偵測**：透過 `detected_at - first_seen_at` 計算錯誤持續時間
- **Provider 切換驗證**：當切換 data provider 後，新 channel 出現錯誤時立即收到通知
- **資料可見性強化**：搭配 `crossmarket.go` 的 `DataStatus="degraded"` 提供更細粒度的失敗定位