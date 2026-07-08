# AGENTS.md — internal/eventdriven

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 5 日事件驅動資金流預測 (ETF 換股 / MSCI 調整 / 月營收 / 季底作帳 / 國定假日)

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Predictor` | `predictor.go` | 主服務：`NewPredictor` / `SetCapitalFlow` / `Predict` |
| `Event` | `types.go` | 事件類型：ETF rebalance / MSCI adjustment / revenue / window dressing / holiday |
| `ExpectedFlow` | (返回) | 5 日預測：每日的方向 + 強度 + 信賴度 |
| `CalendarEvent` | (event calendar source) | 來源：TWSE 行事曆 + 規則偵測 |

## 事件類型

```
ETF_REBALANCE       — ETF 換股，影響追蹤指數成分股 (e.g. 0050、0056)
MSCI_ADJUSTMENT     — MSCI 季度調整，被納入/剔除
REVENUE_ANNOUNCEMENT — 月營收公告（影響次月動能）
WINDOW_DRESSING     — 季底作帳/結帳行情
HOLIDAY             — 國定假日效應（前一/後一日特徵）
```

## 資料流

```
GET /api/events/calendar               → 14 日 forward
       ↓
Event Calendar Service
       ↓
GET /api/events/prediction             → 5 日 flow prediction
       ↓
Predictor.Predict
  ├─ calendar.GetUpcoming(5)
  ├─ events → expectedFlow(events, capitalFlow)
  └─ return PredictionReport {Date, Predictions []FlowPrediction, Drivers, Summary}
```

## 與 P0-1 的關係

Sprint 2 T9 將使 `recommender::HandleRecommendations::EventsToday` 欄位接入 `Predictor.Predict(now).Predictions[0]`。

預期介面（已對齊 `internal/recommender/deps.go` 真實簽名）：
```go
type EventPredictor interface {
    PredictToday() (eventdriven.FlowPrediction, error)
    NextNDays(n int) ([]eventdriven.FlowPrediction, error)
}
```

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **假日效應 lag** | 假日前一日 / 後一日的特殊流動模式需要 historical window ≥ 3 年才穩定，目前可能未達。 |
| **MSCI 公告後 pre-positioning** | 公告當日 (e.g. 2/12) 才開始反映，但 smart money 在前一週就 position；可考慮加上 pre-window。 |
| **月營收解盲差** | 不同產業（電子/傳產/金融）營收截止日不同，需用 calendar 區分產業別。 |

## 與其他模組整合

- `internal/capitalflow/` — `SetCapitalFlow` 用作訊號權重之一
- `cmd/atlas-mcp/server/tools_events.go` — MCP 包裝
- `cmd/atlas/main.go:577` — `eventdriven.RegisterRoutes(mux, eventCalendar)`

## 測試

- `predictor_test.go` 測試事件 → flow 預測映射
- Confidence 範圍測試 (0.5, 1.0]（sigmoid of net weight × (drivers+1),見 predictor.go:131）
- Calendar edge cases: 跨年/閏年/連假
