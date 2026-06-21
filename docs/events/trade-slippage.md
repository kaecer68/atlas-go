# `trade.slippage` — 滑價事件

> **Wave**：8.6
> **穩定性**：stable
> **首次上線**：v0.0.0.7
> **EventType 常數**：`eventbus.EventTradeSlippage`
> **字串值**：`"trade.slippage"`
> **Severity**：`info`

---

## 用途

每筆訂單成交時，計算期望價（`order.Price`）與實際成交價（`result.FillPrice`）之間的滑價，以 BPS 表示。本事件供執行品質監控（slippage drift）、訂單稽核（per-fill audit trail）與成本歸因（cost attribution）使用。

**高頻事件**：每筆 fill 皆發送一次，頻率等於成交率。依 Wave 8 PD-3 決策，producer 端天然 dedup（OrderManager.Run 每筆訂單只觸發一次 fill）。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/live/order_manager.go:180-204` | `OrderManager.Run` — `status == "filled"` 區塊 |
| `internal/live/broker.go` | `Broker.SubmitOrder` 回傳 `BrokerResult{FillPrice}` |
| 滑價計算 | `slippageBPS = abs(FillPrice - ExpectedPrice) / ExpectedPrice * 10000` |

**Producer 注入點**（`internal/live/order_manager.go:183-201`）：
```go
if status == "filled" {
    m.recordFillToRiskGate(order, result)
    if m.eventBus != nil {
        slippageBPS := 0.0
        if order.Price > 0 {
            slippageBPS = math.Abs(result.FillPrice-order.Price) / order.Price * 10000
        }
        slippageCost := math.Abs(result.FillPrice-order.Price) * float64(order.Quantity)
        m.eventBus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
            OrderID:       result.OrderID,
            Symbol:        order.Symbol,
            Side:          string(order.Side),
            Quantity:      order.Quantity,
            ExpectedPrice: order.Price,
            FillPrice:     result.FillPrice,
            SlippageBPS:   slippageBPS,
            SlippageCost:  slippageCost,
            BrokerMode:    m.Mode(),
            Timestamp:     time.Now(),
        })
    }
}
```

> **重要**：`OrderManager.eventBus` 型別為 `*live.ChannelEventBus`，但 `live.ChannelEventBus` 是 `eventbus.ChannelEventBus` 的 **type alias**（`internal/live/eventbus.go:25`），因此直接相容於 dash event bus。不需 bus 橋接。

---

## Payload Schema

### `TradeSlippageEventPayload`（10 欄位）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `OrderID` | `string` | `order_id` | ✓ | 券商回傳的訂單 ID |
| `Symbol` | `string` | `symbol` | ✓ | 個股代號（如 `2330`） |
| `Side` | `string` | `side` | ✓ | 買賣方向：`buy` / `sell` |
| `Quantity` | `int` | `quantity` | ✓ | 成交股數 |
| `ExpectedPrice` | `float64` | `expected_price` | ✓ | 策略期望價格（`order.Price`） |
| `FillPrice` | `float64` | `fill_price` | ✓ | 實際成交價格（`result.FillPrice`） |
| `SlippageBPS` | `float64` | `slippage_bps` | ✓ | 滑價（BPS），`abs(FillPrice - ExpectedPrice) / ExpectedPrice * 10000` |
| `SlippageCost` | `float64` | `slippage_cost` | ✓ | 滑價成本（TWD），`abs(FillPrice - ExpectedPrice) * Quantity` |
| `BrokerMode` | `string` | `broker_mode` | ✓ | 券商模式：`dry-run` / `paper` / `live` |
| `Timestamp` | `time.Time` | `timestamp` | ✓ | 成交時間 |

### 獨特欄位說明

- **`SlippageBPS`**：以 Basis Points 表示的滑價。例如 `5.0` = 0.05%。正值代表成交價不利（買入更貴/賣出更低），永遠非負。
- **`SlippageCost`**：以新台幣表示的滑價成本。`abs(FillPrice - ExpectedPrice) * Quantity`。例如 0.30 × 1,000 = 300 TWD。
- **`BrokerMode`**：區分 dry-run（永遠 match 期望價，BPS = 0）、paper（模擬成交）、live（券商真實成交）。

### Schema 版本

**目前版本**：v0（未版本化）
**規劃**：依 Wave 8 PD-1 決策，未來將加入 `schema_version int` 欄位（預設 `1`）。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedTradeSlippageEvent`（`internal/monitoring/api/events/sse_handler.go`） |
| Buffer 大小 | 50 筆（FIFO） |
| Catchup 順序 | narrative → promotion → health-alert → risk-gate → backtest-completed → calibration-completed → **trade-slippage** → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆滑價事件 |

**高頻考量**：滑價為 per-fill 事件，在活躍交易日可能產生大量事件。SSE buffer 為 50 筆 FIFO，客戶端重連時只取得最近 50 筆—歷史滑價可從 JSONL 回溯（PD-2 後）。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有訂單面板 | `web/static/js/components/risk-gate-panel.js` | 操作面板，**非 SSE-driven** |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('trade.slippage', handler)` 訂閱 |

**渲染建議**（Wave 8.10 整合測試階段）：
- 訂單監控頁新增「Slippage Monitor」區塊
- `SlippageBPS < 5` → 綠色 badge（正常範圍）
- `5 ≤ SlippageBPS < 15` → 橘色 badge（關注）
- `SlippageBPS ≥ 15` → 紅色 badge（異常滑價）
- 點擊展開完整 payload（OrderID / ExpectedPrice / FillPrice / SlippageCost / BrokerMode）

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：單一訂單滑價 > 20 BPS → 警告
- alert: TradeSlippageSpike
  expr: atlas_trade_slippage_bps > 20
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "{{ $labels.symbol }} 成交滑價過高（{{ $value }} BPS）"
    description: "OrderID: {{ $labels.order_id }}, BrokerMode: {{ $labels.broker_mode }}"

# 範例：5 分鐘內累積滑價成本 > 10,000 TWD → 警告
- alert: TradeSlippageCostBuildup
  expr: sum(increase(atlas_trade_slippage_cost_total[5m])) > 10000
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "5 分鐘內累積滑價成本 {{ $value }} TWD"
    description: "可能代表流動性不足或市場波動過大"
```

> 註：監控指標 `atlas_trade_slippage_*` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 已知限制

| 限制 | 影響 | 規劃 |
|------|------|------|
| dry-run 模式滑價永遠為 0 | 無實際執行品質資料，僅供結構驗證 | live/paper 模式會有真實填價 |
| 無歷史滑價聚合查詢 | 前端無法直接查詢過去 N 天的平均滑價 | 待 JSONL 啟用後可透過 ledger 回查 |
| 高頻事件不節流 | 活躍市場每秒可能數十筆事件，buffer 可能快速滾動 | PD-3 high-freq dedup（本事件已滿足 producer 端天然 dedup） |

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishTradeSlippage` | `internal/eventbus/eventbus_test.go` | Publish 路由、payload 欄位傳遞 |
| `TestSSEHandler_BufferTradeSlippageEvent` | `internal/monitoring/api/events/sse_handler_test.go` | Buffer 寫入、Get 讀取、EventType 對應 |
| `TestOrderManager_Run` | `internal/live/order_manager_test.go`（既有） | Run 流程含 fill 分支（間接覆蓋 PublishTradeSlippage） |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.7 | 2026-06-20 | Wave 8.6 加入 EventTradeSlippage 常數、TradeSlippageEventPayload、PublishTradeSlippage、SSE buffer、OrderManager fill hook、本文件 |

---

## 相關事件

- `order.filled` — 既有訂單成交事件（文件待撰寫）
- `order.rejected` — 訂單拒絕事件（文件待撰寫）
- `risk.stoploss.triggered` — 停損觸發（文件待撰寫）
