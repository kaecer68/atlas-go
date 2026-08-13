# atlas-mcp Stock Tools — 投資人查詢模板

> 給 MCP bot prompt engineer 與外部 agent（OpenClaw / Hermes / Claude Desktop）使用。
> 對應 MCP tools：`stock_get_quote` / `stock_get_fundamentals` / `stock_get_chips` / `stock_get_technical`。
> 全部 tool 在 `atlas-mcp server`（117 tools）內，**需要付費 tier 或 dev mode** 才能呼叫。

## 工具能力總覽

| Tool | 輸入 | 回傳關鍵欄位 | 投資人會問什麼 |
|------|------|-------------|---------------|
| `stock_get_quote` | `symbol` | `last`, `change`, `change_pct`, `open`, `high`, `low`, `volume`, `yesterday_close` | 「2330 現在多少錢？」 |
| `stock_get_fundamentals` | `symbol` | `PE`, `PB`, `PS`, `DividendYield`, `Sector` | 「2330 本益比多少？」 |
| `stock_get_chips` | `symbol`, `date?` | `foreign_investor_net`, `domestic_fund_net`, `dealer_net`, `date` | 「今天外資在買 2330 嗎？」 |
| `stock_get_technical` | `symbol`, `days?` | `close`, `sma20`, `sma50`, `rsi14`, `volume` | 「2330 RSI 多少？均線方向？」 |

## 投資人常見查詢範本

### 報價類

| 投資人問 | 呼叫方式 | 顯示建議格式 |
|----------|---------|------------|
| 「台積電現在多少？」 | `stock_get_quote {symbol: "2330"}` | `台積電(2330) 收盤 $680.0，漲 $5.0 (+0.74%)` |
| 「鴻海今天收盤多少？」 | `stock_get_quote {symbol: "2317"}` | `鴻海(2317) 收盤 $105.5，漲 $0.5 (+0.48%)` |
| 「今天台股收盤狀況」 | 批次查詢熱門股（無現成 batch tool）| 需 bot 自行組成 symbol 陣列 |

### 估值類

| 投資人問 | 呼叫方式 | 顯示建議格式 |
|----------|---------|------------|
| 「2330 本益比？」 | `stock_get_fundamentals {symbol: "2330"}` | `PE 22.5, PB 5.8, 殖利率 1.8%（半導體）` |
| 「0050 現在貴不貴？」 | `stock_get_fundamentals {symbol: "0050"}` | 註：ETF 估值不適用 PE，bot 應改敘述 |
| 「2330 是什麼產業？」 | `stock_get_fundamentals {symbol: "2330"}` | `Sector: 半導體` |

### 籌碼類

| 投資人問 | 呼叫方式 | 顯示建議格式 |
|----------|---------|------------|
| 「今天外資在買 2330 嗎？」 | `stock_get_chips {symbol: "2330"}` | `外資買超 15.42 億 / 投信買超 2.31 億 / 自營商賣超 0.88 億` |
| 「上週 2330 法人的動向？」 | 需逐日查詢（無現成 range tool） | 建議呼叫 `narrative_get_chains` 或 `capital_flow_daily` 取得週趨勢 |
| 「投信最近在買什麼？」 | 無現成 API | 需另開「投信買賣超排行」後端功能 |

### 技術類

| 投資人問 | 呼叫方式 | 顯示建議格式 |
|----------|---------|------------|
| 「2330 RSI 多少？」 | `stock_get_technical {symbol: "2330"}` | `RSI(14): 58.4（中性偏強）` |
| 「2330 月線方向？」 | `stock_get_technical {symbol: "2330", days: 30}` | `SMA20: 678.5, SMA50: 672.3, 收盤: 680 → 站穩月季線` |
| 「2330 是不是超買了？」 | `stock_get_technical {symbol: "2330"}` | `RSI: 72+ → 超買；RSI: 30- → 超賣` |

## Bot 提示詞建議

以下是給外部 agent 整合的 system prompt 片段範例：

```text
你具備以下台股個股查詢能力（透過 atlas-mcp 工具）：

1. stock_get_quote(symbol) - 即時報價（股價、漲跌、成交量）
2. stock_get_fundamentals(symbol) - 估值指標（PE/PB/PS/殖利率/產業）
3. stock_get_chips(symbol, date?) - 法人買賣超（外資/投信/自營商）
4. stock_get_technical(symbol, days?) - 技術指標（SMA20/SMA50/RSI14）

當投資人問到個股時：
- 問「多少錢/漲跌/成交量」→ 呼叫 stock_get_quote
- 問「本益比/估值/產業」→ 呼叫 stock_get_fundamentals
- 問「法人/外資/投信/主力」→ 呼叫 stock_get_chips
- 問「均線/RSI/超買超賣」→ 呼叫 stock_get_technical

回應時務必：
- 把數字配上單位（NT$ 億元、%）
- 把 RSI 轉為語意（<30 超賣, 30-70 中性, >70 超買）
- 把法人買賣超轉為語意（淨買 = 紅, 淨賣 = 綠, 台股紅漲綠跌慣例）
- 引用 `change_pct` 而非 `change` 來描述漲跌幅
```

## 已知限制

1. **無 batch API**：投資人問「台股 50 檔報價」時，bot 需自行組成多次呼叫。
2. **無 range 查詢**：`stock_get_chips` 只能查單日，跨日需逐日呼叫。
3. **無台股 ETF 估值**：`stock_get_fundamentals` 對 ETF（0050、00878）回傳的 PE 沒有意義，bot 應自行 fallback。
4. **依賴 Fugle + TWSE 資料源**：`FUGLE_API_KEY` 沒設時 `stock_get_quote` 會回 503。
5. **Symbol 必須是數字代碼**：不接受中文名稱（不會自動轉「台積電」→ 「2330」）。

## 測試與 E2E 驗證

- `cmd/atlas-mcp/server/tools_stock_test.go` — 單元測試（mock HTTP，驗 path / query param / 缺欄位）
- `cmd/atlas-mcp/server/tools_stock_e2e_test.go` — **E2E 測試**（mock 真實回傳 JSON 格式，驗證投資人會看到的欄位是否齊全）
- 跑測試：`go test ./cmd/atlas-mcp/server/ -run TestStockE2E -v`

## 變更紀律

新增欄位時：
1. 後端先改 `internal/stocktools/handler.go` 與對應 provider（`marketdata` / `portfolio`）。
2. 同步 `cmd/atlas-mcp/server/tools_stock.go` 的 tool `Description` 讓 bot 知道新欄位。
3. 更新本文件（投資人查詢模板）與 `tools_stock_e2e_test.go` 的 `wantFields` 清單。
4. 跑 `go test ./cmd/atlas-mcp/server/ -run TestStockE2E` 確認欄位未漏。
