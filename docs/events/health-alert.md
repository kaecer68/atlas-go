# `monitor.health.alert` — 系統健康監控警報事件

> **Wave**：1–7（stable）
> **穩定性**：stable
> **Schema 版本**：v1
> **EventType 常數**：`eventbus.EventHealthAlert`
> **字串值**：`"monitor.health.alert"`
> **Severity**：`warning`（預設描述；實際事件 Severity 直接取自 `alert.Severity`，可能為 `CRITICAL` / `WARNING` / `INFO`）

---

## 用途

由 `SystemHealthMonitor` 每日例行健康檢查觸發，當系統層級指標（系統 Sharpe 趨勢、Agent 健康度、Darwinian 權重分布、Max Drawdown、Factor Decay、資料延遲等）突破閾值時發出警報。本事件供 AutoRollback、Telegram 通知、Email 告警、Dashboard 健康卡片等下游消費者使用。

**批次事件**：`RunDaily` 一次檢查可產出多筆 alert（見下表 Category），逐筆 publish，無天然 dedup。一日最多產生 `len(alerts)` 筆事件，通常 0–3 筆。

**多類別覆蓋**：涵蓋 `sharpe_trend`、`sharpe`、`agent_population`、`weight_distribution` 等多種 Category，每類別有獨立閾值與處置建議。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/scheduler/system_health.go:69-117` | `SystemHealthMonitor.RunDaily` — 執行三類檢查（Sharpe 趨勢、Agent 群體、權重分布），彙整 `alerts` slice |
| `internal/scheduler/system_health.go:88-106` | 當 `len(alerts) > 0` 時，逐一呼叫 `m.eventBus.PublishHealthAlert()` |
| 排程來源 | `BackgroundTaskManager` 註冊的每日例行任務（非 bare `time.Ticker`，遵守 Constitution） |
| 依賴組件 | `portfolio.DarwinianWeightManager`（取得 agent weights）、`portfolio.AgentHealthManager`（取得 muted agents）、`domain.MaturityTracker`（記錄 maturity） |

**Producer 注入點**（`internal/scheduler/system_health.go:94-106`）：
```go
if len(alerts) > 0 {
    logging.Warn("health_monitor", "alerts_generated",
        "count", len(alerts),
        "critical", countBySeverity(alerts, "CRITICAL"),
        "warning", countBySeverity(alerts, "WARNING"))
    // Publish alerts to event bus for downstream consumers.
    for _, alert := range alerts {
        if m.eventBus != nil {
            m.eventBus.PublishHealthAlert(eventbus.HealthAlertPayload{
                Severity:        alert.Severity,
                Category:        alert.Category,
                Message:         alert.Message,
                Value:           alert.Value,
                Threshold:       alert.Threshold,
                SuggestedAction: alert.SuggestedAction,
                Timestamp:       alert.Timestamp,
            })
        }
    }
} else {
    maturity := "unknown"
    if m.tracker != nil {
        maturity = string(m.tracker.Current())
    }
    logging.Info("health_monitor", "all_clear",
        "maturity", maturity)
}
```

**Alert 類別總覽**：

| Category | 觸發條件 | Severity | SuggestedAction |
|----------|---------|----------|-----------------|
| `sharpe_trend` | 10 日 rolling system Sharpe 下降 > 10% | `WARNING` | `auto_rollback` |
| `sharpe` | 平均 system Sharpe < -0.5 | `CRITICAL` | `halt_and_review` |
| `agent_population` | > 50% agents muted | `CRITICAL` | `halt_and_review` |
| `agent_population` | > 30% agents muted | `WARNING` | `auto_propose_mutations` |
| `weight_distribution` | > 50% agents 權重卡在最小值 (≤ 0.31) | `WARNING` | `auto_reset_and_propose` |

> **重要**：`SystemHealthMonitor.eventBus` 為 nil-safe，每次 publish 前檢查 `m.eventBus != nil`，因此在測試環境（未注入 bus）下呼叫 `RunDaily` 不會 panic。

---

## Payload Schema

### `HealthAlertPayload`（7 欄位）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `Severity` | `string` | `severity` | ✓ | 警報等級：`CRITICAL` / `WARNING` / `INFO`（內部枚舉，**與 BusEvent.Severity 不同**） |
| `Category` | `string` | `category` | ✓ | 警報類別：`sharpe_trend` / `sharpe` / `agent_population` / `weight_distribution` / `drawdown` / `factor_decay` / `data_latency` |
| `Message` | `string` | `message` | ✓ | 人類可讀訊息（如 `system sharpe declining: 1.234 → 1.012 over 10 days`） |
| `Value` | `float64` | `value` | ✓ | 當前觀察值（如當下 system Sharpe、muted 比例） |
| `Threshold` | `float64` | `threshold` | ✓ | 觸發閾值（如 `0.9 * first_sharpe`、`-0.5`、`0.5`） |
| `SuggestedAction` | `string` | `suggested_action` | ✓ | 建議處置：`auto_rollback` / `halt_and_review` / `auto_propose_mutations` / `auto_reset_and_propose` |
| `Timestamp` | `time.Time` | `timestamp` | ✓ | 檢查時間（`time.Now()` at `RunDaily`） |

### 獨特欄位說明

- **`Severity`**：內部枚舉使用 `CRITICAL` / `WARNING` / `INFO`（全大寫）；發佈至 BusEvent 時直接寫入 `alert.Severity`，BusEvent.Severity 對應欄位會反映該枚舉。`eventDescriptions` map 的預設描述為 `warning`，但實際每筆事件 Severity 依 alert 內容而定。
- **`Category`**：7 種可能值；其中 `sharpe_trend` 與 `sharpe` 是 Sharpe 類別的兩種變體，前者看 10 日趨勢，後者看當下絕對值。
- **`Value` / `Threshold`**：單位依 Category 而異。`sharpe_trend` / `sharpe` 為 Sharpe 比率；`agent_population` / `weight_distribution` 為比例（0.0–1.0）。
- **`SuggestedAction`**：直接供 AutoRollback 等下游消費者的處置依據。`halt_and_review` 為最強處置，需人工介入。

### Schema 版本

**目前版本**：v1（已內建 `SchemaVersion: 1`，見 `PublishHealthAlert` 於 `eventbus.go:846`）

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedHealthAlertEvent`（`internal/monitoring/api/events/sse_handler.go`） |
| Buffer 大小 | 50 筆（FIFO，`maxBufferedHealthAlertEvents = 50`） |
| Catchup 順序 | narrative → promotion → **health-alert** → risk-gate → backtest-completed → calibration-completed → trade-slippage → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆健康警報事件 |

**批次特性**：單次 `RunDaily` 可能連續 publish 多筆 alert，SSE buffer 一次性寫入多筆；buffer 上限 50 筆意味著當健康持續惡化時，較舊的警報可能被淘汰。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有健康面板 | `web/static/js/components/circuit-breaker.js`、`sim-health.js` | 顯示系統狀態，**非 SSE-driven**（polling-based） |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('monitor.health.alert', handler)` 訂閱 |

**渲染建議**（Wave 8.10 整合測試階段）：
- 健康監控頁新增「Health Alert Stream」區塊，依 `Severity` 區分色塊
- `Severity === "CRITICAL"` → 紅色 badge + 強制醒目動畫
- `Severity === "WARNING"` → 橘色 badge
- `Severity === "INFO"` → 藍色 badge
- 點擊展開完整 payload（Category / Message / Value / Threshold / SuggestedAction）
- 當 `SuggestedAction === "halt_and_review"` 時，觸發前端 disable 按鈕（禁止下單/切倉等操作）

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：CRITICAL 等級健康警報出現 → 緊急告警
- alert: HealthAlertCritical
  expr: increase(atlas_health_alert_total{severity="CRITICAL"}[5m]) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "系統健康警報 CRITICAL：{{ $labels.category }}"
    description: "Message: {{ $labels.message }}, SuggestedAction: {{ $labels.suggested_action }}"

# 範例：WARNING 警報 1 小時內累積超過 5 筆 → 警告
- alert: HealthAlertWarningSpike
  expr: increase(atlas_health_alert_total{severity="WARNING"}[1h]) > 5
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "1 小時內累積 {{ $value }} 筆 WARNING 健康警報"
    description: "系統健康持續惡化，建議人工檢查 SuggestedAction 處置建議"
```

> 註：監控指標 `atlas_health_alert_*` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 已知限制

| 限制 | 影響 | 規劃 |
|------|------|------|
| 每日僅一次檢查 | 健康突發惡化時偵測延遲最多 24 小時 | 待 Wave 8 引入高頻檢查（in-progress，未排程） |
| 6 種 Category 為硬編碼 | 新增健康指標需改 `SystemHealthMonitor` 加新檢查函式 | 待 refactor 為 pluggable check interface |
| 無自動重試 publish | eventBus 緩衝滿時最舊事件被丟棄，alert 可能未送達 | 監控 alert loss rate |
| `SuggestedAction` 為字串建議 | 下游處置邏輯需自行 parse，無 enum 強約束 | 待 Wave 8 將其升級為 enum |

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestSystemHealthMonitor_AllClear` | `internal/scheduler/system_health_test.go` | 無 alert 時 logging + 不 publish |
| `TestSystemHealthMonitor_SharpeTrendDeclining` | `internal/scheduler/system_health_test.go` | 10 日 Sharpe 下降 → WARNING alert |
| `TestSystemHealthMonitor_NegativeSharpeCritical` | `internal/scheduler/system_health_test.go` | 負 Sharpe → CRITICAL alert |
| `TestSystemHealthMonitor_AgentPopulationMuted30` | `internal/scheduler/system_health_test.go` | 30% muted → WARNING + auto_propose_mutations |
| `TestSystemHealthMonitor_AgentPopulationMuted50` | `internal/scheduler/system_health_test.go` | 50% muted → CRITICAL + halt_and_review |
| `TestSystemHealthMonitor_WeightDistributionStuck` | `internal/scheduler/system_health_test.go` | > 50% 權重卡最小 → WARNING |
| `TestSystemHealthMonitor_BurnInStillRuns` | `internal/scheduler/system_health_test.go` | Burn-in 階段仍執行健康檢查（不回退） |
| `TestSystemHealthMonitor_EventBusPublish` | `internal/scheduler/system_health_test.go` | 注入 mock eventBus，驗證 PublishHealthAlert 呼叫與 payload 欄位 |
| `TestBufferHealthAlertEvent_AppendsToBuffer` | `internal/monitoring/api/events/sse_handler_test.go` | Buffer 寫入與 FIFO 讀取 |
| `TestBufferHealthAlertEvent_CapsAt50` | `internal/monitoring/api/events/sse_handler_test.go` | 超過 50 筆時丟棄最舊（FIFO 上限） |
| `TestSSEHandler_ServeHTTP_DeliversBufferedHealthAlertOnConnect` | `internal/monitoring/api/events/sse_handler_test.go` | SSE catchup 階段 replay 健康警報 |
| `PublishHealthAlert` 路由測試 | `internal/eventbus/eventbus_test.go:480` | Publish 路由、payload 欄位傳遞、SchemaVersion=1 |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.6 | 2026-04 (Wave 7.5) | Wave 1–7 stable 既有 `EventHealthAlert` 常數、`HealthAlertPayload`、`PublishHealthAlert`、`SystemHealthMonitor`、SSE buffer（`BufferedHealthAlertEvent`、max 50）、本文件 |

---

## 相關事件

- [`experiment.promotion_recorded`](./promotion-recorded.md) — promotion-recorded 事件（既有，SSE catchup 順序在 health-alert 之前）
- [`risk.gate.allowed` / `risk.gate.rejected`](./risk-gate-allowed.md) — 風險閘門事件（既有，SSE catchup 順序在 health-alert 之後）
- [`experiment.backtest_completed`](./backtest-completed.md) — backtest-completed 事件（既有）
- [`experiment.calibration_completed`](./calibration-completed.md) — calibration-completed 事件（既有）
- [`trade.slippage`](./trade-slippage.md) — 滑價事件（Wave 8.6，SSE catchup 順序最末）
- `industry.calendar.event` — 產業行事曆事件（既有）
- `simulation.start` / `simulation.complete` — 模擬生命週期事件（既有）
- `narrative.detected` — 敘事偵測事件（既有，SSE catchup 順序首位）