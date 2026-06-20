# `experiment.backtest_completed` — 自動回測完成事件

> **Wave**：8.8
> **穩定性**：stable
> **首次上線**：v0.0.0.7
> **EventType 常數**：`eventbus.EventBacktestCompleted`
> **字串值**：`"experiment.backtest_completed"`
> **Severity**：`info`

---

## 用途

當每日自動回測（`autobacktest_daily` 排程）成功完成、記錄 JSONL 快照、並 best-effort 同步至 live store 後，發布本事件供 SSE 即時串流、JSONL 審計軌跡與 Prometheus 監控使用。

本事件修正 Wave 7 之 false advertising：先前 `main.go:2229` 呼叫 `autobacktest.NewRunnerWithEventBus(cfg, dashboard.GetEventBus())` 但 `autobacktest.Runner` 未儲存 eventBus 也未呼叫 Publish。Wave 8.8 補上實際 Publish 呼叫。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/autobacktest/runner.go:66-87` | `RunAndStore()` 成功完成 `syncToLiveStore()` 後呼叫 `r.eventBus.PublishBacktestCompleted(...)` |
| 排程入口 | `autobacktest_daily` 背景任務（`cmd/atlas/main.go:2236-2243`），1h 間隔，週末跳過 |
| 觸發時機 | 每日 13:30 台北時間 ±1h，僅交易日（next 1 trading day from now） |
| 預估頻率 | 約 5 筆/週（5 個交易日） |

**Producer 注入點**（`autobacktest.NewRunnerWithEventBus`）：
```go
type Runner struct {
    btRunner *backtest.Runner
    cfg      config.Config
    eventBus *eventbus.ChannelEventBus  // Wave 8.8 新增
}

func NewRunnerWithEventBus(cfg config.Config, eventBus *eventbus.ChannelEventBus) *Runner {
    store := ledger.NewStore(cfg.LedgerDir)
    btRunner := backtest.NewRunner(cfg, store)
    if eventBus != nil {
        btRunner.WithEventBus(eventBus)
    }
    return &Runner{
        btRunner: btRunner,
        cfg:      cfg,
        eventBus: eventBus,  // Wave 8.8 新增
    }
}
```

**Publish 觸發點**（`RunAndStore` 成功路徑）：
```go
r.syncToLiveStore()

if r.eventBus != nil {
    r.eventBus.PublishBacktestCompleted(eventbus.BacktestCompletedEventPayload{
        WindowID:              summary.WindowID,
        StartDate:             summary.StartDate,
        EndDate:               summary.EndDate,
        SessionCount:          summary.SessionCount,
        OutcomeCount:          summary.OutcomeCount,
        WorstAgentID:          summary.WorstAgentID,
        WorstAgentSkill:       summary.WorstAgentSkill,
        WorstAgentLayer:       string(summary.WorstAgentLayer),
        WorstAgentWindowCount: summary.WorstAgentWindowCount,
        WorstAgentSharpeLike:  summary.WorstAgentSharpeLike,
        GeneratedAt:           summary.GeneratedAt,
        TargetDate:            targetDate,
        SyncSucceeded:         true,
    })
}

return nil
```

**注意**：
- `r.eventBus != nil` 守門確保 `NewRunner`（無 eventBus 版本）不會 panic
- `SyncSucceeded` 目前硬編為 `true`（`syncToLiveStore` 為 best-effort，失敗僅 Warn 不 return error；待後續重構再回傳 error）
- 若 `RunAndStore` 提早 return（snapshot 已存在 / Run error / GenerateReport error / recordSnapshot error），**不會發布本事件**

---

## Payload Schema

### `BacktestCompletedEventPayload`（13 欄位）

| 欄位 | 型別 | JSON tag | 來源 | 說明 |
|------|------|---------|------|------|
| `WindowID` | `string` | `window_id` | `summary.WindowID` | 回測 window 唯一識別 |
| `StartDate` | `time.Time` | `start_date` | `summary.StartDate` | window 起始日 |
| `EndDate` | `time.Time` | `end_date` | `summary.EndDate` | window 結束日（通常 = targetDate） |
| `SessionCount` | `int` | `session_count` | `summary.SessionCount` | window 內交易日數 |
| `OutcomeCount` | `int` | `outcome_count` | `summary.OutcomeCount` | 已記錄的推薦 outcome 數 |
| `WorstAgentID` | `string` | `worst_agent_id` | `summary.WorstAgentID` | window 內表現最差 agent ID |
| `WorstAgentSkill` | `string` | `worst_agent_skill` | `summary.WorstAgentSkill` | 最差 agent 的 skill |
| `WorstAgentLayer` | `string` | `worst_agent_layer` | `string(summary.WorstAgentLayer)` | 最差 agent 的層級（L1/L2/L3） |
| `WorstAgentWindowCount` | `int` | `worst_agent_window_count` | `summary.WorstAgentWindowCount` | 最差 agent 的 window 數 |
| `WorstAgentSharpeLike` | `float64` | `worst_agent_sharpe_like` | `summary.WorstAgentSharpeLike` | 最差 agent 的 sharpe-like 指標 |
| `GeneratedAt` | `time.Time` | `generated_at` | `summary.GeneratedAt` | summary 產生時間 |
| `TargetDate` | `time.Time` | `target_date` | 本地變數 `targetDate` | 本次回測的目標交易日 |
| `SyncSucceeded` | `bool` | `sync_succeeded` | 硬編 `true` | live store 同步是否成功（見上） |

### Schema 版本

**目前版本**：v0（未版本化）
**規劃**：依 Wave 8 PD-1 決策，未來將加入 `schema_version int` 欄位（預設 `1`）。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedBacktestCompletedEvent`（`internal/monitoring/api/events/sse_handler.go`） |
| Buffer 大小 | 50 筆（FIFO） |
| Catchup 順序 | narrative → promotion → health-alert → risk-gate → backtest-completed → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆 backtest-completed 事件 |

**注意**：backtest-completed buffer 獨立於其他 buffer，無法跨類型 replay。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有組件 | 待 Wave 8.10 新增 | 建議在 backtest 頁面新增「Recent Backtests」section |

**渲染建議**（Wave 8.10 整合測試階段）：
- 列表顯示最近 10 筆回測（WindowID / StartDate / EndDate / SessionCount / WorstAgentID / WorstAgentSharpeLike）
- 最差 agent sharpe < 0 → 紅色 badge
- 最差 agent sharpe 0~0.5 → 橘色 badge
- 點擊展開完整 payload（包含所有 13 欄位）

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：連續 2 個交易日未觸發 backtest_completed → 警告
- alert: BacktestScheduleMissing
  expr: |
    (time() - atlas_backtest_completed_last_success_timestamp) > 172800  # 48h
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "自動回測連續 2 個交易日未完成"
    description: "可能 autobacktest_daily 排程失敗或 RunAndStore 連續 error"

# 範例：最差 agent sharpe < -0.5 → 警告（策略表現嚴重下滑）
- alert: BacktestWorstAgentSevere
  expr: |
    atlas_backtest_completed_worst_agent_sharpe < -0.5
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "回測最差 agent sharpe 嚴重下滑"
    description: "WindowID: {{ $labels.window_id }}, sharpe: {{ $value }}"
```

> 註：監控指標 `atlas_backtest_completed_*` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishBacktestCompleted` | `internal/eventbus/eventbus_test.go`（本 PR 新增） | 訂閱 → 發布 → 驗證 payload 完整性 |
| `TestSSEHandler_BufferBacktestCompletedEvent` | `internal/monitoring/api/events/sse_handler_test.go`（本 PR 新增） | 緩衝 → SSE replay → 驗證 body 內容 |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.7 | 2026-06-21 | Wave 8.8 加入 EventBacktestCompleted 常數、BacktestCompletedEventPayload、PublishBacktestCompleted |
| v0.0.0.7 | 2026-06-21 | Wave 8.8 加入 autobacktest.Runner eventBus 欄位 + RunAndStore Publish 呼叫（修正 false advertising） |
| v0.0.0.7 | 2026-06-21 | Wave 8.8 加入 SSE buffer + replay + 本文件 |

---

## 相關事件

- [`experiment.promotion_recorded`](./promotion-recorded.md) — 實驗 promotion 記錄（既有）
- [`experiment.insufficient_data`](./experiment-insufficient-data.md) — 實驗資料不足（既有）
- [`monitor.sharpe.degradation`](./sharpe-degradation.md) — Sharpe 退化警告（既有）
- [`monitor.drawdown.breach`](./drawdown-breach.md) — 回撤突破閾值（既有）
