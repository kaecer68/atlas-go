# Atlas 事件目錄

> 最後更新：2026-06-22
> Schema Version: 1（全部事件）

## Wave 8 事件（已完成）

| # | 事件類型 | EventType 常數 | 說明文件 | PR | 狀態 |
|---|---------|---------------|---------|-----|------|
| 8.0 | 風險閘門基礎設施（三向 routing：BLOCK/HALT → rejected、REDUCE/ALERT_ONLY → overridden、ALLOW → allowed） | `EventRiskGateRejected` / `EventRiskGateAllowed` / `EventRiskGateOverridden` | [risk-gate-allowed.md](risk-gate-allowed.md) / [risk-gate-overridden.md](risk-gate-overridden.md) | #619, #620, Wave 8.2 收尾 | ✅ 已合併 |
| 8.3 | 產業日曆事件 | `EventIndustryCalendar` | [industry-calendar.md](industry-calendar.md) | #621 | ✅ 已合併 |
| 8.8 | 自動回測完成 | `EventBacktestCompleted` | [backtest-completed.md](backtest-completed.md) | #622 | ✅ 已合併 |
| 8.9 | 參數校準完成 | `EventCalibrationCompleted` | [calibration-completed.md](calibration-completed.md) | #623 | ✅ 已合併 |
| 8.6 | 交易滑價 | `EventTradeSlippage` | [trade-slippage.md](trade-slippage.md) | #625 | ✅ 已合併 |

## Wave 8 LLM 事件（推遲至 Wave 8.11+，因 LLM 重構期間吸收）

| # | 事件類型 | 推遲原因 | 替代追蹤 |
|---|---------|---------|---------|
| 8.5 | `LLMAnnotatorCircuitOpen` | LLM Provider 路由改為 capability-based（PR #628/#629），原 circuit breaker 事件由 metrics `llm_annotator:requests_good:rate5m` + alert rule `llm_annotator_availability_fast_burn` 取代 | [monitoring/rules/llm_annotator_alerts.yml](../../monitoring/rules/llm_annotator_alerts.yml) |
| 8.6 (LLM) | `LLMAnnotatorFallbackUsed` | 同上，fallback 路徑由 router logs 與 metrics 揭露 | llm_annotator_alerts.yml |
| 8.7 | `LLMAnnotatorQuotaExceeded` | 同上，quota 控管整合進 router 計費 | llm_annotator_recording.yml |

## 既有事件（Wave 1-7）

| 事件類型 | EventType 常數 | Schema | 說明文件 |
|---------|---------------|--------|---------|
| 市場快照 | `EventMarketSnapshot` | v1 | – |
| 模擬開始 | `EventSimulationStart` | v1 | – |
| 模擬完成 | `EventSimulationComplete` | v1 | – |
| 體制變更 | `EventRegimeChange` | v1 | – |
| 部位更新 | `EventPositionUpdate` | v1 | – |
| 推薦生成 | `EventRecommendation` | v1 | – |
| Guard 結果 | `EventGuardOutcomes` | v1 | – |
| Darwinian 夾制 | `EventDarwinianClamping` | v1 | – |
| Agent 健康變更 | `EventAgentHealthChange` | v1 | – |
| Conviction 夾制 | `EventConvictionClamping` | v1 | – |
| 訂單事件 | `EventOrderFilled` | v1 | – |
| 風險事件（停損/停利） | `EventRiskStopLoss` / `EventRiskTakeProfit` | v1 | – |
| 實驗數據不足 | `EventExperimentInsufficientData` | v1 | – |
| 訂單錯誤 | `EventOrderError` | v1 | – |
| 宏觀敘事 | `EventNarrative` | v1 | [narrative-event.md](narrative-event.md) |
| 健康警報 | `EventHealthAlert` | v1 | [health-alert.md](health-alert.md) |
| 晉升記錄 | `EventPromotionRecorded` | v1 | [promotion-recorded.md](promotion-recorded.md) |

## Schema Version 說明

- **v1（當前）**：所有事件的預設 schema 版本。`BusEvent.SchemaVersion` 欄位值為 `1`。
- **v2+（未來）**：當事件 payload 結構發生 breaking change 時，遞增版本號。前端可透過 `data.schema_version` 判斷如何解析 payload。

## 新增事件流程

1. 在 `internal/eventbus/eventbus.go` 加入 `EventType` 常數
2. 定義 `XxxEventPayload` struct（snake_case JSON tags）
3. 實作 `PublishXxxEvent` 方法（設定 `SchemaVersion: 1`）
4. 在 `eventDescriptions` 加入中文描述
5. 在 `internal/monitoring/api/events/sse_handler.go` 加入 6-component buffer
6. 在 `ServeHTTP` 加入 replay loop
7. 加入測試（eventbus_test.go + sse_handler_test.go）
8. 建立 `docs/events/<event-name>.md` 說明文件
9. 更新本 INDEX.md
