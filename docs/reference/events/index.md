# Atlas 事件目錄

> 最後更新：2026-06-25
> Schema Version: 多版本並存（v1 為多數事件，EventDriftDetected 為 v2，EventFactorWeightRegression 為 v1）

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
| 8.5 | `LLMAnnotatorCircuitOpen` | LLM Provider 路由改為 capability-based（PR #628/#629），原 circuit breaker 事件由 metrics `llm_annotator:requests_good:rate5m` + alert rule `llm_annotator_availability_fast_burn` 取代 | [monitoring/rules/llm_annotator_alerts.yml](../../../monitoring/rules/llm_annotator_alerts.yml) |
| 8.6 (LLM) | `LLMAnnotatorFallbackUsed` | 同上，fallback 路徑由 router logs 與 metrics 揭露 | llm_annotator_alerts.yml |
| 8.7 | `LLMAnnotatorQuotaExceeded` | 同上，quota 控管整合進 router 計費 | llm_annotator_recording.yml |

## Wave 9 YELLOW 觀測性擴展（已合併至 v0.0.0.8）

| # | 事件類型 | EventType 常數 | 說明文件 | Alert Rule | 狀態 |
|---|---------|---------------|---------|-----------|------|
| 9.1 | 個別監控通道健康 | `EventChannelIndividualHealth` | [channel-individual-health.md](channel-individual-health.md) | [wave9_channel_individual_health.yml](../../../monitoring/rules/wave9_channel_individual_health.yml) | ✅ 已合併（v0.0.0.8） |
| 9.2 | Regime 轉變穩定確認 | `EventRegimeChangeConfirmed` | [regime-change-confirmed.md](regime-change-confirmed.md) | rule removed 2026-07-25 (placeholder, never-emitted metric) | ✅ event 已合併（v0.0.0.8） |
| 9.3 | 因子權重回歸偵測 | `EventFactorWeightRegression` | [factor-weight-regression.md](factor-weight-regression.md) | rule removed 2026-07-25 (placeholder, never-emitted metric) | ✅ event 已合併（v0.0.0.8） |
| 9.4 | 投資組合部位漂移偵測（v2：target weights drift） | `EventDriftDetected` | [drift-detector.md](drift-detector.md) | rule removed 2026-07-25 (placeholder, never-emitted metric) | ✅ v1 event 已合併（v0.0.0.8）；v2 透過 PR feat/drift-detector-v2 追加 target weights drift |
| 9.5 | API Gateway ingestion lag spike | `EventIngestionLagSpike` | [ingestion-lag-spike.md](ingestion-lag-spike.md) | rule removed 2026-07-25 (placeholder, never-emitted metric) | ✅ event 已合併（v0.0.0.8） |

> **PD-W9-1**：5 個 YELLOW 事件預設 `severity: "info"`（IngestionLagSpike 除外，severity=warning），alert rule 預設 `enabled: false`，由 operator 決定是否啟用。
> **Forward-compat 驗證**：Wave 9 全程 0 修改 #611 9 個檔案（`git diff --stat` 為空）。

### Wave 9 生產接線

以下 4 個 Wave 9 事件現已具備生產環境呼叫者：

| 事件 | 生產呼叫者 | 檔案 |
|------|-----------|------|
| `portfolio.position.update` | live orchestrator 在市場快照時發布持有部位更新 | `internal/live/orchestrator.go` |
| `market.regime.confirmed` | `RegimeDebouncer` 在 regime 穩定 30 秒後發布 | `internal/monitoring/service/regime_debouncer.go` |
| `factor.weight.regression` | `FactorWeightRegressionDetector` 在 regime 確認後偵測 | `internal/monitoring/service/factor_weight_regression.go` |
| `ingestion.lag.spike` | `IngestionLagMonitor` 透過 `ChannelHealthIngestionLagProvider` 取得 p99 latency | `internal/monitoring/service/ingestion_lag_monitor.go` |

`portfolio.drift.detected` 由 `DriftDetector` v2 產出，本身不是由上游直接發布，而是 Wave 9 觀測器根據 `portfolio.position.update` 與 `market.regime.confirmed` 計算後觸發。

所有 Wave 9 偵測器的統一啟動與生命週期管理由 `internal/monitoring/wave9_runtime.go` 的 `Wave9Observability` 負責。

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

- **v1（多數事件）**：預設 schema 版本。`BusEvent.SchemaVersion` 欄位值為 `1`。
- **v2（EventDriftDetected）**：2026-06 v2 升級，新增 target weights drift 偵測與 5 個新 payload 欄位（`target_weights`、`actual_weights`、`max_drift`、`max_drift_symbol`、`current_regime`）。v1 欄位完整保留，append-only 演進。
- **v2+（未來）**：當事件 payload 結構發生重大變更時，遞增版本號。前端可透過 `data.schema_version` 判斷如何解析 payload。

## 新增事件流程

1. 在 `internal/eventbus/eventbus.go` 加入 `EventType` 常數
2. 定義 `XxxEventPayload` struct（snake_case JSON tags）
3. 實作 `PublishXxxEvent` 方法（設定 `SchemaVersion: 1`）
4. 在 `eventDescriptions` 加入中文描述
5. 在 `internal/monitoring/api/events/sse_handler.go` 加入 6-component buffer
6. 在 `ServeHTTP` 加入 replay loop
7. 加入測試（eventbus_test.go + sse_handler_test.go）
8. 建立 `docs/reference/events/<event-name>.md` 說明文件
9. 更新本 index.md
