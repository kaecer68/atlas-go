# `industry.calendar.event` — 台股產業日曆事件

> **Wave**：8.3
> **穩定性**：stable
> **首次上線**：v0.0.0.7
> **EventType 常數**：`eventbus.EventIndustryCalendar`
> **字串值**：`"industry.calendar.event"`
> **Severity**：`info`

---

## 用途

當 `industry.EventCalendar` 完成 `RefreshEvents` 或 `UpdateFromProvider` 後，每筆當前 active 的台股市場日曆事件（除權息旺季、MSCI 季度調整、財報季、春節、選舉行情等）會以 EventBus 訊息形式發布，供 SSE 串流與監控儀表板即時反應日曆驅動的市場狀態變化。

本事件為 PD-2「JSONL 審計軌跡策略」的驗證事件 — 產業日曆事件頻率穩定（24h 排程觸發 × 每次數筆），適合做為 JSONL 寫入效能基準的輸入源。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/industry/event_calendar.go:578` | `EventCalendar.RefreshEvents(now time.Time)` 重建年度事件清單 |
| `internal/industry/event_calendar.go:1087` | `EventCalendar.UpdateFromProvider(ctx, provider)` 從 TWSE / FinMind provider 拉取即時事件並合併 |
| `internal/monitoring/service/industry.go:22` | `IndustryService.EventCalendar` 公開欄位持有 EventCalendar 實例 |
| `internal/monitoring/dashboard_api.go:411-423` | `newWiredEventCalendar()` 啟動時建立 + 初始 refresh + provider update |
| `cmd/atlas/main.go:1168-1181` | 24h 排程背景任務 `auto_calendar_refresh`：呼叫 RefreshEvents + UpdateFromProvider，**Wave 8.3 新增** publish loop |

**Producer 注入點**（Wave 8.3 新增）：
```go
calendarProvider := marketdata.NewTWSECalendarProvider()
_ = taskMgr.Register(&apigateway.ScheduledTask{
    Name:     "auto_calendar_refresh",
    Interval: 24 * time.Hour,
    Enabled:  true,
    Task: func(ctx context.Context) error {
        bgCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
        defer cancel()
        svc.EventCalendar.UpdateFromProvider(bgCtx, calendarProvider)
        svc.EventCalendar.RefreshEvents(time.Now())
        // Wave 8.3: 發布所有當前 active 事件
        for _, evt := range svc.EventCalendar.DetectActiveEvents(time.Now()) {
            dashEventBus.PublishIndustryCalendarEvent(eventbus.IndustryCalendarEventPayload{...})
        }
        return nil
    },
})
```

---

## Payload Schema

### `IndustryCalendarEventPayload`

| 欄位 | 型別 | JSON tag | 必填 | 來源 | 說明 |
|------|------|---------|------|------|------|
| `EventID` | `string` | `event_id` | ✓ | `evt.ID` | 唯一識別（如 `ex_dividend_2026_06`、`taiwan50_rebalance_2026_03`） |
| `Name` | `string` | `name` | ✓ | `evt.Name` | 中文名稱（如 `除權息旺季`、`MSCI季度調整`） |
| `NameEN` | `string` | `name_en,omitempty` | ✗ | `evt.NameEN` | 英文名稱（部分事件為空） |
| `EventType` | `string` | `event_type` | ✓ | `evt.EventType` | 事件類型（14 種：`spring_festival` / `ex_dividend` / `shareholder_meeting` / `window_dressing` / `election` / `msci_rebalance` / `financial_report` / `investor_conference` / `monthly_revenue` / `long_holiday` / `dividend_payout` / `taiwan50_rebalance` / `futures_settlement` / `position_building`） |
| `Description` | `string` | `description` | ✓ | `evt.Description` | 人類可讀說明（含月份、季度、年度等動態資訊） |
| `Direction` | `string` | `direction` | ✓ | `evt.Direction` | 方向：`bullish` / `bearish` / `mixed` / `neutral` |
| `BaseWeight` | `float64` | `base_weight` | ✓ | `evt.BaseWeight` | 基礎權重（0.0 ~ 1.0） |
| `Active` | `bool` | `active` | ✓ | `evt.Active` | 當前是否 active（DetectActiveEvents 設為 true） |
| `StartDate` | `time.Time` | `start_date` | ✓ | `evt.StartDate` | 事件開始日（含時區） |
| `EndDate` | `time.Time` | `end_date` | ✓ | `evt.EndDate` | 事件結束日 |
| `PeakDate` | `time.Time` | `peak_date` | ✓ | `evt.PeakDate` | 影響力峰值日 |
| `DecayDays` | `int` | `decay_days` | ✓ | `evt.DecayDays` | 自峰值的衰減天數 |
| `AffectedIndustries` | `[]string` | `affected_industries` | ✓ | `evt.AffectedIndustries` | 受影響產業 ID 列表（如 `financials` / `semiconductor` / `electronics` / `consumer` / `ai_supply_chain` 等） |
| `SentimentAdjustment` | `float64` | `sentiment_adjustment` | ✓ | `evt.SentimentAdjustment` | 情緒乘數（基於 Direction × BaseWeight × Decay，限制在 ±0.05） |
| `DataSource` | `string` | `data_source` | ✓ | `string(evt.DataSource)` | 資料來源：`default_rules` / `twse_provider` / `finmind_provider` / `mops_provider` |
| `EvidenceQuality` | `string` | `evidence_quality` | ✓ | `string(evt.EvidenceQuality)` | 證據等級：`backtested` / `estimated` / `unverified` / `realtime` |
| `GeneratedAt` | `time.Time` | `generated_at` | ✓ | `evt.GeneratedAt` | 事件生成時間（用於 freshness check） |

### Schema 版本

**目前版本**：v0（未版本化）
**規劃**：依 Wave 8 PD-1 決策，未來將加入 `schema_version int` 欄位（預設 `1`）。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedIndustryCalendarEvent`（`internal/monitoring/api/events/sse_handler.go:84-90`） |
| Buffer 大小 | 50 筆（FIFO） |
| Catchup 順序 | narrative → promotion → health-alert → risk-gate → **industry-calendar** → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆日曆事件 |

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有組件 | `web/static/js/pages/industry.js` | 透過 `/api/dashboard/industry-seasonality-calendar` 取得年度行事曆（非 SSE-driven） |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('industry.calendar.event', handler)` 訂閱 |

**渲染建議**（Wave 8.10 整合測試階段）：
- Industry Calendar 頁面新增「Active Events (Live)」section，列出當前 active 的日曆事件
- 每筆事件顯示：名稱、Direction 徽章（color-coded）、SentimentAdjustment 數值、DataSource 標籤
- 點擊事件展開詳細：PeakDate 倒數計時、AffectedIndustries 列表、回測證據等級

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：連續 3 天 window_dressing 期間 trigger → 注意
- alert: IndustryCalendarWindowDressing
  expr: |
    increase(atlas_industry_calendar_total{event_type="window_dressing"}[3d]) > 0
  for: 1h
  labels:
    severity: info
  annotations:
    summary: "季底作帳行情開始"
    description: "Window dressing 期間啟動，注意半導體、金融、電子族群波動"

# 範例：realtime 證據事件 ≥ 5 筆/小時 → 確認 provider 正常運作
- alert: IndustryCalendarRealtimeFlow
  expr: |
    rate(atlas_industry_calendar_total{evidence_quality="realtime"}[1h]) >= 5
  for: 0m
  labels:
    severity: info
  annotations:
    summary: "Provider 正常推送即時事件"
```

> 註：監控指標 `atlas_industry_calendar_total` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishIndustryCalendarEvent` | `internal/eventbus/eventbus_test.go`（本 PR 新增） | 基本 publish/subscribe、payload 欄位傳遞 |
| `TestSSEHandler_BufferIndustryCalendarEvent` | `internal/monitoring/api/events/sse_handler_test.go`（本 PR 新增） | SSE buffer 寫入與讀取 |

---

## JSONL 審計軌跡（PD-2 規劃）

本 PR **不**啟用 JSONL 寫入。Wave 8 PD-2 規劃 `AnnotationStore.WriteEvent(EventType, payload)` 介面擴充，預定在 Wave 8.6（`LLMAnnotatorFallbackUsed`，高頻事件）落地後驗證效能。產業日曆事件可作為穩態基準的對照組。

預期 JSONL 負載（PD-2 估算）：
- 24h 排程 × 每次約 4-8 筆 active 事件 × 365 天 = 約 1,460-2,920 行/年
- 平均 4-8 行/天，遠低於 PD-2 預估的 900 行/秒高頻峰
- 適合做 PD-2 的 baseline 對照（非高頻驗證源）

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.7 | 2026-06-20 | Wave 8.3 加入 EventIndustryCalendar 常數、IndustryCalendarEventPayload、PublishIndustryCalendarEvent、SSE buffer、producer bridge |

---

## 相關事件

- [`monitor.risk_gate.rejected`](./risk-gate-rejected.md) — 風險閘道拒絕（Wave 8.0-8.1）
- [`monitor.risk_gate.overridden`](./risk-gate-allowed.md) — 風險閘道覆寫（Wave 8.2）
- `industry.*` 系列（Wave 9 規劃）— 觀測性擴展事件

## 相關模組

- `internal/industry/event_calendar.go` — EventCalendar 引擎（1148 行）
- `internal/industry/cycle_status_card.go` — 五層複合週期狀態卡（含 event 層）
- `internal/marketdata/calendar_provider.go` — CalendarEventProvider 介面（TWSE / FinMind）
- `internal/monitoring/service/industry.go` — IndustryService 持有 EventCalendar