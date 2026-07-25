# atlas-mcp Query Examples — 投資人常見查詢

> **給 hermes / openclaw agent 的「自然語言 → tool 對照表」**。投資人問什麼、怎麼呼叫、回什麼。
>
> 涵蓋 ~25 個高頻率查詢（從 112 tool 中篩選，投資人每天都會用到的）。其他 83+ tool 見 [`reference/tool-catalog.md`](../reference/tool-catalog.md)。
>
> tier 標記（public / registered / premium）待 [#1068](https://github.com/kaecer68/atlas-go/issues/1068) 商業化確認；dev key 模式已上線（PR #1069）。

## 快速導覽（按 use case 分組）

- [C1 個股研究](#c1-個股研究) — 5 例
- [C2 市場全景](#c2-市場全景) — 3 例
- [C3 個股深度](#c3-個股深度) — 3 例
- [C4 持倉健康](#c4-持倉健康) — 3 例
- [C5 策略排名](#c5-策略排名) — 2 例
- [C6 資金流向](#c6-資金流向) — 2 例
- [C7 每日晨報](#c7-每日晨報) — 2 例
- [C8 稅務規劃](#c8-稅務規劃) — 1 例
- [系統健康](#系統健康) — 1 例

---

## C1 個股研究

> use case 詳見 [`use-cases/01-stock-research.md`](use-cases/01-stock-research.md)

### "2330 現在多少？"

```js
stock_get_quote { symbol: "2330" }
```

**回傳**：`{ last: 680.0, change: 5.0, change_pct: 0.74, open: 675, high: 682, low: 673, volume: 12345, yesterday_close: 675 }`

**顯示範例**：`台積電(2330) 收盤 $680.0，漲 $5.0 (+0.74%)`

**注意**：volume 單位是「千股」（12345 = 12,345,000 股）

### "2330 本益比多少？"

```js
stock_get_fundamentals { symbol: "2330" }
```

**回傳**：`{ PE: 22.5, PB: 5.8, PS: 7.2, DividendYield: 1.8, Sector: "半導體" }`

**顯示範例**：`半導體 | PE 22.5, PB 5.8, 殖利率 1.8%`

**注意**：ETF（如 0050）不適用 PE — bot 應改敘述

### "今天外資在買 2330 嗎？"

```js
stock_get_chips { symbol: "2330", date: "2026-07-10" }  // date 省略 = 最新
```

**回傳**：`{ foreign_investor_net: 12_500_000_000, domestic_fund_net: 2_310_000_000, dealer_net: -880_000_000, date: "2026-07-10" }`

**顯示範例**：`外資買超 125 億 / 投信買超 23.1 億 / 自營商賣超 8.8 億`

### "2330 是不是超買？"

```js
stock_get_technical { symbol: "2330", days: 60 }
```

**回傳**：`{ sma20: 678.5, sma50: 672.3, rsi14: 58.4, last_close: 680, macd_signal: "bullish" }`

**顯示範例**：`RSI(14): 58.4（中性偏強）| 站穩月線（SMA20 > SMA50）| MACD 偏多`

**注意**：RSI > 70 超買，< 30 超賣

### "比較 2330 和 2317 估值"

```js
// 目前無 batch tool — 需 agent 自行組合
stock_get_fundamentals { symbol: "2330" }  // → PE 22.5
stock_get_fundamentals { symbol: "2317" }  // → PE 18.2
```

**顯示範例**：`2330 PE 22.5 | 2317 PE 18.2 → 2317 估值相對低`

---

## C2 市場全景

> use case 詳見 [`use-cases/02-market-overview.md`](use-cases/02-market-overview.md)

### "現在大盤怎樣？"

```js
regime_get_history { days: 30 }
```

**回傳**：`[{ date, regime: "RISK_ON" | "RISK_OFF" | "NEUTRAL" | "TRANSITIONAL", confidence }]`

**顯示範例**：`近 30 天 regime: RISK_ON (20 天) → NEUTRAL (8 天) → TRANSITIONAL (2 天) | 市場偏多`

### "美股昨晚？"

```js
crossmarket_get_us_indices
```

**回傳**：`{ "S&P 500": 5847.23, "NASDAQ": 18234.56, "Dow Jones": 42156.78, "SOX": 4892.11, "NVDA": 134.56, "AAPL": 198.45, "MSFT": 421.78, "TSM ADR": 178.92 }`

**顯示範例**：`S&P +0.32% | NASDAQ +0.45% | SOX +0.67% (半導體強) | NVDA -0.12%`

### "台股 stress index？"

```js
macro_get_stress_index_current
```

**回傳**：`{ value: 17.08, level: "low" | "medium" | "high", updated_at: "..." }`

**顯示範例**：`Stress Index: 17.08 (low) | 市場無明顯壓力`

---

## C3 個股深度

> use case 詳見 [`use-cases/03-stock-deep-dive.md`](use-cases/03-stock-deep-dive.md)

### "2330 今天為什麼漲？"

```js
event_get_calendar { symbol: "2330", days: 7 }
narrative_get_events { filter: { symbols: ["2330"], days: 7 } }
```

**回傳**：calendar 列出近期事件；narrative 給出敘事解釋

**顯示範例**：`近 7 天事件：(1) 7/8 法說會優於預期 (2) 7/9 外資連 3 日買超 (3) 7/10 同業上修展望`

### "最近有什麼產業消息？"

```js
narrative_get_events { filter: { sector: "半導體", days: 3 } }
```

**回傳**：`[{ event, narrative, source, confidence }]`

**顯示範例**：`半導體近 3 天：台積電法說會優於預期、聯電產能利用率回升、世界先進 Q3 展望上修`

### "法人最近動態？"

```js
stock_get_chips { symbol: "2330", date: "2026-07-10" }
```

（同 C1 「今天外資在買 2330 嗎？」，但 date 鎖定為今日）

---

## C4 持倉健康

> use case 詳見 [`use-cases/04-portfolio-health.md`](use-cases/04-portfolio-health.md)

### "我的策略還活著嗎？"

```js
strategy_list_active
strategy_get_summary { strategy_id: "<從 list 取>" }
```

**回傳**：list 給策略 ID + 名稱；summary 給勝率 / Sharpe / 最大回撤

**顯示範例**：`Momentum-v3 (運作 142 天) | 勝率 58% | Sharpe 1.42 | Max DD -8.3% | 健康`

### "portfolio 風險？"

```js
risk_get_metrics
```

**回傳**：`{ VaR_95: -2.3, volatility_30d: 12.4, sharpe: 0.87, beta: 0.92, max_drawdown: -8.1 }`

**顯示範例**：`VaR 95%: -2.3% (1 天內最壞虧損) | 波動率 12.4% | Sharpe 0.87 | Beta 0.92 (低於大盤)`

### "上個月歸因？"

```js
strategy_get_attribution { strategy_id: "<id>", period: "last_30d" }
```

**回傳**：`{ sector_contribution, factor_contribution, top_winners, top_losers }`

**顯示範例**：`半導體 +1.8% | 電子 +0.6% | 金融 -0.3% | 個股歸因：2330 +0.9% / 2317 -0.2%`

---

## C5 策略排名

> use case 詳見 [`use-cases/05-strategy-ranking.md`](use-cases/05-strategy-ranking.md)

### "哪個策略好？"

```js
strategy_ranker { sort_by: "sharpe", top_n: 5 }
```

**回傳**：`[{ strategy_id, name, sharpe, win_rate, active, tier }]`

**顯示範例**：
```
Top 5 策略 (依 Sharpe):
1. Momentum-v3    Sharpe 1.42  Win 58%  [premium]
2. Value-Deep     Sharpe 1.21  Win 54%  [premium]
3. MeanRev-7D     Sharpe 1.08  Win 61%  [registered]
4. EarningsMomo   Sharpe 0.94  Win 52%  [registered]
5. SectorRot-Q    Sharpe 0.87  Win 49%  [registered]
```

### "策略詳情？"

```js
strategy_get_summary { strategy_id: "Momentum-v3" }
```

（同 C4 「我的策略還活著嗎？」）

---

## C6 資金流向

> use case 詳見 [`use-cases/06-capital-flow.md`](use-cases/06-capital-flow.md)

### "今天主力在買？"

```js
capital_flow_daily { date: "2026-07-10" }  // date 省略 = 最新
```

**回傳**：`{ foreign_net, domestic_fund_net, dealer_net, retail_proxy, signals: [...] }`

**顯示範例**：
```
今日 7 大資金勢力：
  外資    +125 億   🟢
  投信    +23 億    🟢
  自營商  -9 億     🔴
  散戶    -88 億    🔴
共振強度: 0.72 (偏多)
```

### "外資動向？"

```js
capital_flow_summary { period: "last_5d" }
```

**回傳**：`{ foreign_net_5d: 245e9, trend: "accumulating" | "distributing" | "neutral", sector_breakdown: [...] }`

**顯示範例**：`近 5 日外資淨買 245 億 (accumulating) | 主要買超：半導體 +180 / 電子 +65`

---

## C7 每日晨報

> use case 詳見 [`use-cases/07-daily-briefing.md`](use-cases/07-daily-briefing.md)

### "今天要關注什麼？"

```js
narrative_get_bundle
```

**回傳**：`{ date, market_summary, top_events, alerts, watchlist }`

**顯示範例**：
```
📅 2026-07-10 晨報摘要
市場: 大盤 +0.3% (半導體領漲)
重點事件: 2330 法說會 / 美股 ADR 變化
警報: 1 未確認 (alert_xyz)
關注: 半導體 / 金融
```

### "有什麼警報？"

```js
alert_list_unacknowledged { severity: "high" }
```

**回傳**：`[{ alert_id, severity, message, triggered_at }]`

**顯示範例**：`🚨 1 高優先級未確認：risk_get_metrics 觸發閾值 (-8% drawdown)`

---

## C8 稅務規劃

> use case 詳見 [`use-cases/08-tax-planning.md`](use-cases/08-tax-planning.md)
> tier=premium（待 #1068 確認）

### "今年稅務快照？"

```js
report_get_tax_snapshot { year: 2026 }
```

**回傳**：`{ realized_gain, dividend_income, cost_basis, tax_estimate, breakdown_by_security }`

**顯示範例**：`2026 已實現損益：+285,400 | 配息：+18,200 | 估計稅額：+42,810 (稅率 15%)`

---

## 系統健康

### "atlas-mcp 健康？"

```js
system_get_health
```

**回傳**：`{ status: "ok" | "degraded" | "down", tools_count: 91, uptime, last_audit }`

**顯示範例**：`✓ atlas-mcp ok | 116 tools registered | uptime 4h 23m`

**注意**：任何任務的第一步 — 確認 backend / atlas-mcp / LLM router 都活著

---

## 給 hermes / openclaw agent 的提示

1. **先呼叫 `system_get_health`** — 確認所有系統都活著
2. **從 use case 找對應的 tool**（本文件）
3. **回應格式化要 human-readable**（本文件每個範例都有「顯示範例」）
4. **多 tool 組合**時（如「2330 為什麼漲」需 calendar + narrative），平行呼叫

不確定的話：
- 112 tool 完整 catalog 見 [`reference/tool-catalog.md`](../reference/tool-catalog.md)
- 自然語言 → use case 對應見 [`README.md`](README.md)
