# Predicted Trade Cycle — 設計文件（草案）

> **版本**：v0.1（2026-07-19）
> **狀態**：設計草案，等待 SA11 dark launch ≥20 sessions 後進入實作
> **前提條件**：真實策略命中率 >55%（SA11 驗證後確認）
> **關聯**：（內部審計，`.omo/audit/`）§P2、`internal/sim/engine.go`、`internal/domain/recommendation.go`

---

## 1. 動機

### 現有模型的限制

目前模擬交易只驗證「方向正確性」：
- 策略產出「買入 2330」→ 系統以當日收盤價模擬成交 → N 日後看漲跌
- 散戶看到的結果是：「這個推薦方向對了嗎？」（hit rate）

但散戶真正想知道的是：
- **這筆推薦預計何時兌現？**（持有幾天？）
- **這筆推薦能賺多少？**（完整 ROI，不是逐日 mark-to-market）
- **系統的預測是否精準？**（預測目標價 vs 實際到達價）

### Predicted Trade Cycle 的目標

為每一筆推薦附加**出場條件**（目標價、停損價、最大持有天數），讓模擬交易從「方向賭注」升級為「完整交易週期」。這樣：
1. 散戶能看到每筆推薦的完整盈虧故事
2. 系統能追蹤預測精準度（不只是方向，還有價位和時間）
3. 策略競爭排名從「hit rate」升級為「ROI + 勝率 + 平均持有天數」

---

## 2. 資料結構擴充

### 2.1 現有 Recommendation 結構（domain/recommendation.go）

```go
type Recommendation struct {
    Symbol    string
    Direction string  // "buy" | "sell" | "hold"
    Confidence float64
    AgentID   string
    Strategy  string
    Reason    string
}
```

### 2.2 擴充後的 TradeIntent（新建）

```go
// TradeIntent wraps a Recommendation with exit conditions, forming a
// complete predicted trade cycle. Produced by the strategy layer alongside
// the existing Recommendation.
type TradeIntent struct {
    Recommendation          // embed existing fields

    // Exit conditions — when absent, the strategy has no price/time prediction.
    TargetPrice  float64 `json:"target_price,omitempty"`  // take-profit price
    StopLoss     float64 `json:"stop_loss,omitempty"`     // stop-loss price
    MaxHoldDays  int     `json:"max_hold_days,omitempty"` // auto-close after N days

    // Statistical backing (populated by calibration, not hardcoded).
    // When TargetPrice==0 and MaxHoldDays==0, the trade cycle degrades to
    // the existing "direction-only" behavior.
    AvgHistReturn  float64 `json:"avg_hist_return,omitempty"`  // historical avg return for this pattern
    AvgHistDays    int     `json:"avg_hist_days,omitempty"`    // historical avg holding days
}
```

### 2.3 新增 TradeCycleOutcome（outcome 層）

```go
// TradeCycleOutcome records the full lifecycle of one predicted trade.
type TradeCycleOutcome struct {
    Intent      TradeIntent `json:"intent"`
    EntryDate   time.Time   `json:"entry_date"`
    EntryPrice  float64     `json:"entry_price"`
    ExitDate    time.Time   `json:"exit_date"`
    ExitPrice   float64     `json:"exit_price"`
    ExitReason  string      `json:"exit_reason"` // "target_hit" | "stop_loss" | "timeout" | "strategy_sell"
    ROI         float64     `json:"roi"`
    DaysHeld    int         `json:"days_held"`
    TargetMet   bool        `json:"target_met"`   // did price reach target during hold?
    StopHit     bool        `json:"stop_hit"`     // did price hit stop-loss during hold?
}
```

---

## 3. 執行引擎擴充（sim/engine.go）

### 3.1 現有流程

```
Engine.Run(regime, quotes, recs)
  → RunDay(state, day, regime, quotes, recs)
    → mark positions to market
    → execute sells (stop-loss, rotation, rebalance)
    → execute buys (with slippage + market impact)
    → return DayResult{Orders, Trades, PortfolioValue}
```

### 3.2 新增 RunTradeCycle

```go
// RunTradeCycle executes buy recommendations as full trade cycles with
// exit conditions. It runs a multi-day simulation where each BUY intent
// is tracked until its exit condition triggers or max_hold_days expires.
func (e *Engine) RunTradeCycle(
    state *domain.SimulationState,
    startDay time.Time,
    quotesProvider func(day time.Time) ([]domain.Quote, error), // N-day replay provider
    intents []TradeIntent,
) ([]TradeCycleOutcome, error)
```

### 3.3 執行邏輯（偽碼）

```
for day in [startDay .. startDay + max(intents[i].MaxHoldDays)]:
    quotes := quotesProvider(day)
    for each open intent i:
        price := quotes[i.Symbol].Last

        // Check exit conditions
        if price >= intent.TargetPrice:
            close_trade(i, price, "target_hit")
        else if price <= intent.StopLoss:
            close_trade(i, price, "stop_loss")
        else if day - entry_date >= intent.MaxHoldDays:
            close_trade(i, price, "timeout")

    // Execute new entries for today's intents
    for each new intent arriving on this day:
        buy_with_slippage(state, intent, quotes)

return all TradeCycleOutcomes
```

### 3.4 與現有 Engine 的關係

- `RunTradeCycle` 是 **新增** 的方法，不修改現有 `Run` / `RunDay` / `RunWithState`
- 現有 `RunDailySimulation` 仍用 `engine.Run()` 產出當日投組
- `RunTradeCycle` 作為獨立評估管道：每月/每週跑一次，回溯驗證過去 N 天的所有預測交易週期
- 結果寫入 `data/state/trade_cycle_outcomes.jsonl`，不進達爾文權重（等同 B05 的 synthetic 隔離規則）

---

## 4. 策略層擴充

### 4.1 如何產出目標價和持有天數

**不要求策略做「價位預測」（那幾乎不可能）。** 而是從歷史統計中推導：

```
方向正確時的平均漲幅 = 校準框架從 backtest 計算
    → 存入 parameters.json
    → 策略產出 Recommendation 時附帶這個統計值

例如：
  L2 策略 "foreign-3day-inflow" 在過去 200 次正確預測中：
  - 平均漲幅 3.2%，標準差 2.1%
  - 平均持有天數 7.3 天
  → TradeIntent{ TargetPrice: entryPrice * 1.032, StopLoss: entryPrice * 0.98, MaxHoldDays: 10 }
```

### 4.2 校準機制

類似 F03 的 `PredictorCalibrator`，新增一個 `TradeCycleCalibrator`：

1. 從 `prediction_backtest` SQLite 讀取歷史命中記錄
2. 計算每條策略「命中時的平均漲幅、平均天數、勝率」
3. 寫回 `parameters.json` 的策略級別參數
4. 持續追蹤命中率，退化自動降權

### 4.3 策略沒產出 TradeIntent 時的行為

退化為現有模式（只有方向，無出場條件）。這確保向後相容，且讓策略逐步升級而非一次到位。

---

## 5. 回測驗證

### 5.1 歷史回測

用 TWSE replay 資料回測過去 90 天的所有推薦：

```
for each past session (T-90 .. T-1):
    intents := session.recommendations → attach historical avg stats
    outcomes := engine.RunTradeCycle(state, T, replayProvider, intents)
    store outcomes
```

計算：
- 每條策略的 average ROI
- 每條策略的 target_met_rate（目標價達成率）
- 每條策略的 stop_loss_rate（停損觸發率）
- 整體 Sharpe（以 trade cycle 為單位，非逐日）

### 5.2 與現有 F06 的關係

F06 的策略競爭排名目前用 `hit_rate + Sharpe`（逐日 mark-to-market）。TradeCycle 上線後，排名可以升級為：

```
策略分數 = w1 * hit_rate + w2 * avg_ROI + w3 * target_met_rate + w4 * (1 - stop_loss_rate)
```

---

## 6. UI 呈現

### 6.1 現有策略競爭頁（evolution_panel）

新增區塊「交易週期績效」：
- 每條策略的 average ROI、平均持有天數、目標價達成率
- 最近 10 筆完整交易週期的清單（含盈虧）

### 6.2 個股快查頁

當使用者查詢個股時，顯示「最近模擬交易週期」：
- 系統何時推薦買入、何時出場、盈虧多少
- 目標價是否達成

---

## 7. 實作階段

| 階段 | 內容 | 預估 | 前提 |
|------|------|------|------|
| **Phase A** | `domain.TradeIntent` + `domain.TradeCycleOutcome` 資料結構 | 0.5 天 | 無 |
| **Phase B** | `TradeCycleCalibrator` — 從 backtest 算出 avg return/days | 2 天 | F02/F03 穩定運行 |
| **Phase C** | `engine.RunTradeCycle` — 多日週期模擬引擎 | 3 天 | Phase A+B |
| **Phase D** | orchestrator 接線 — 每週跑一次回測驗證 | 1 天 | Phase C |
| **Phase E** | UI — evolution_panel 交易週期區塊 | 1.5 天 | Phase D |
| **Phase F** | 策略層擴充 — 產出附帶 TradeIntent 的推薦 | 1 天 | Phase B |

**總預估**：9 天（Phase A~F）。**前提**：SA11 ≥20 sessions + 真實命中率 >55%。

---

## 8. 不做的事（明確邊界）

- ❌ 不要求策略做價位預測（只用歷史統計）
- ❌ 不把 TradeCycle 結果餵進達爾文權重（避免循環論證）
- ❌ 不對散戶顯示「預測目標價」（避免暗示精準度）
- ❌ 不修改現有 `engine.Run` 行為

---

## 9. 風險與未知

1. **avg_hist_return 的穩定性**：如果歷史樣本少（<30 次命中），統計值不可靠。需設定最低樣本門檻，未達標時只顯示方向。
2. **過度擬合**：用 backtest 算出的 avg_return 可能 overfit 特定市場環境。需用 out-of-sample 驗證（參考 `ValidateCalibration` 的 holdout 架構）。
3. **散戶誤解**：如果 UI 顯示「平均漲幅 3.2%」，散戶可能誤以為是「保證賺 3.2%」。UI 必須附註「此為歷史統計，不保證未來績效」。
