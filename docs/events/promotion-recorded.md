# `experiment.promotion_recorded` — 晉升記錄事件

> **Wave**：7.5
> **穩定性**：stable
> **首次上線**：v0.0.0.6
> **EventType 常數**：`eventbus.EventPromotionRecorded`
> **字串值**：`"experiment.promotion_recorded"`
> **Severity**：`info`

---

## 用途

當實驗通過 judge 並被晉升為新 baseline 時，`AutoRollback.RecordPromotion` 會先快照晉升前的系統 Sharpe，再將該快照存入 `promotedSnapshot` map 供後續 `Rollback` 比較使用。本事件承載這筆快照紀錄，供 SSE 即時串流、JSONL 審計軌跡與 Prometheus 監控使用。

**低頻事件**：每次實驗晉升（baseline promotion）觸發一次。`AutoRollback` 為晉升週期的單一寫入端，producer 端天然 dedup。

> **Wave 範圍**：本事件屬 Wave 1–7 既有事件（與 Wave 8 新事件相對），於 Wave 7.5 隨 `AutoJudgePromoter` scheduler 整合一併加入 SSE 串流。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/scheduler/auto_rollback.go:65-79` | `AutoRollback.RecordPromotion()` — 寫入 `promotedSnapshot` map、`logging.Info`，再發布事件 |
| `AutoJudgePromoter` 整合 | 當 judge 接受實驗、晉升為新 baseline 時，由 scheduler 呼叫 `RecordPromotion` |

**Producer 注入點**（`internal/scheduler/auto_rollback.go:67-79`）：
```go
func (r *AutoRollback) RecordPromotion(experimentID string, prePromotionSharpe float64) {
    r.promotedSnapshot[experimentID] = prePromotionSharpe
    logging.Info("auto_rollback", "promotion_recorded",
        "experiment_id", experimentID,
        "pre_promotion_sharpe", prePromotionSharpe)
    if r.eventBus != nil {
        r.eventBus.PublishPromotionRecorded(eventbus.PromotionRecordedPayload{
            ExperimentID:       experimentID,
            PrePromotionSharpe: prePromotionSharpe,
            Timestamp:          time.Now(),
        })
    }
}
```

> **Nil-safe**：`r.eventBus != nil` 檢查確保 `AutoRollback` 在未注入 event bus（測試或純背景排程）時仍可安全呼叫。注入介面為 `WithEventBus(eb *eventbus.ChannelEventBus)`（`auto_rollback.go:60-63`）。
>
> **Fallback 描述**：`eventDescriptions` map 未收錄本事件，`EnrichEvent` 走 `describeEvent` fallback（`eventbus.go:385-391`），回傳 `string(EventType) = "experiment.promotion_recorded"` 與 `severity = "info"`。

---

## Payload Schema

### `PromotionRecordedPayload`（3 欄位）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `ExperimentID` | `string` | `experiment_id` | ✓ | 通過 judge 並晉升的實驗 ID（寫入 `promotedSnapshot` map 的 key） |
| `PrePromotionSharpe` | `float64` | `pre_promotion_sharpe` | ✓ | 晉升前的系統 Sharpe ratio，後續 `Rollback` 比較基準 |
| `Timestamp` | `time.Time` | `timestamp` | ✓ | 記錄時間（`time.Now()`） |

### 獨特欄位說明

- **`ExperimentID`**：唯一識別被晉升的實驗。同一個 `experimentID` 多次晉升時，`promotedSnapshot` 會被覆寫（後者勝出），但 SSE buffer 仍保留每次事件，可作為晉升歷程審計軌跡。
- **`PrePromotionSharpe`**：晉升「前」的 Sharpe 快照，作為日後 `Rollback` 判斷「晉升後表現是否惡化」的對照組。`Rollback` 比較 `currentSharpe` 與 `prePromotionSharpe`，若差距超過閾值則觸發自動回退。

### Schema 版本

**目前版本**：v1
所有 `BusEvent` 自 Wave 8 PD-1 起內建 `schema_version int` 欄位，預設 `1`。`PromotionRecordedPayload` 結構簡單（3 欄位），向後相容性高；後續若新增 `experiment_type` 或 `promotion_reason` 等欄位，須 bump 至 v2 並同步前端型別。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedPromotionRecordedEvent`（`internal/monitoring/api/events/sse_handler.go:46-51`） |
| Buffer 大小 | 50 筆（FIFO，line 52 `maxBufferedPromotionEvents`） |
| Catchup 順序 | status → narrative → **promotion** → health-alert → risk-gate → industry-calendar → backtest-completed → calibration-completed → trade-slippage → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆晉升記錄事件 |

**低頻特性**：晉升事件頻率遠低於其他 buffer（narrative 為高頻、trade-slippage 為 per-fill）。50 筆 FIFO 對晉升週期而言已能覆蓋數月歷史，重連時一般可取得完整晉升歷程。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('experiment.promotion_recorded', handler)` 訂閱 |

**渲染建議**：
- Baseline 演進頁新增「Promotion History」時間軸，依 `Timestamp` 倒序排列
- 每筆紀錄顯示 `ExperimentID` 與 `PrePromotionSharpe`（保留 4 位小數）
- 點擊 `ExperimentID` 跳轉至對應實驗的 detail 頁，比對晉升前後 Sharpe 變化
- 若同一 `ExperimentID` 出現多次晉升（覆寫情境），以前端去重並標註「覆寫 X 次」徽章

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：1 小時內晉升次數過多 → 警告（可能 baseline 不穩定）
- alert: PromotionFlood
  expr: increase(atlas_promotion_recorded_total[1h]) > 5
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "1 小時內連續晉升 {{ $value }} 次"
    description: "可能代表 judge 閾值過寬或 replay 資料不穩，建議人工檢視 baseline policy"

# 範例：晉升時 PrePromotionSharpe 過低 → 警告
- alert: LowPrePromotionSharpe
  expr: atlas_promotion_recorded_pre_sharpe < 0.5
  for: 0m
  labels:
    severity: info
  annotations:
    summary: "晉升前 Sharpe 過低（{{ $value }}）"
    description: "ExperimentID: {{ $labels.experiment_id }}，建議人工複審晉升決策"
```

> 註：監控指標 `atlas_promotion_recorded_*` 待後續 metric emitter 整合後開新 issue 設計（不在本文件 scope）。

---

## 已知限制

| 限制 | 影響 | 規劃 |
|------|------|------|
| `promotedSnapshot` map 覆寫語意 | 同一 `experimentID` 多次晉升只保留最新快照，無法回查歷史 | 後續可改為 slice append，保留全部晉升記錄 |
| 無 `experiment_type` 欄位 | 無法區分晉升類型（如 baseline / shadow / canary） | schema v2 規劃新增 |
| EnrichEvent 走 fallback 描述 | 事件流顯示 `experiment.promotion_recorded` 字串而非中文友善描述 | 待補入 `eventDescriptions` map |
| nil eventBus 靜默跳過 | 未注入 bus 時事件不發布，呼叫端無感 | 既有設計，測試與背景排程友善 |

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestAutoRollback_RecordPromotion_EmitsEvent` | `internal/scheduler/auto_rollback_test.go:203-221` | 注入 bus 後 `RecordPromotion` 觸發 `EventPromotionRecorded`，計數驗證 |
| `TestAutoRollback_PromotionDegradation` | `internal/scheduler/auto_rollback_test.go:74-?` | `promotedSnapshot` 寫入與 `Rollback` 比較流程（含預晉升 Sharpe 基準） |
| `TestAutoRollback_History` | `internal/scheduler/auto_rollback_test.go:170-?` | 多次 `RecordPromotion` 累積行為 |
| `TestBufferPromotionRecordedEvent_AppendsToBuffer` | `internal/monitoring/api/events/sse_handler_test.go:264-?` | SSE buffer 寫入、`GetBufferedPromotionEvents` 讀取、`PromotionRecordedPayload` roundtrip |
| `TestSSEHandler_ServeHTTP_DeliversPromotionRecorded` | `internal/monitoring/api/events/sse_handler_test.go:295-?` | HTTP 端點訂閱 `experiment.promotion_recorded` 並收到事件 body |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.6 | 2026-04-? | Wave 7.5 隨 `AutoJudgePromoter` scheduler 整合加入 `EventPromotionRecorded` 常數、`PromotionRecordedPayload`、`PublishPromotionRecorded`、`AutoRollback.RecordPromotion` 觸發點、SSE buffer、本文件 |

---

## 相關事件

- [`experiment.backtest_completed`](./backtest-completed.md) — 自動回測完成（晉升前的前置步驟，於 Rollback 比較時對照）
- [`experiment.calibration_completed`](./calibration-completed.md) — 參數校準完成（晉升週期的姊妹事件，由同一 `AutoRollback` 系統管理）
- [`narrative.detected`](./narrative-event.md) — 宏觀敘事事件（晉升判斷的環境上下文之一，影響 Sharpe 評估）
