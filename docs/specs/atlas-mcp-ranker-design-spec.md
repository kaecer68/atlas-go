# atlas-mcp 個股級工具、資金流向與策略排名設計文件

> **對應需求**：atlas-mcp 新增個股級工具（quote、fundamentals、chips、technical）；補齊 `capital_flow_daily` / `capital_flow_summary` / `strategy_ranker`；統一 tool 數文件權威來源。
> **分支**：`feat/atlas-mcp-stock-capitalflow-ranker-tools`
> **日期**：2026-07-07

---

## 1. 目標與範圍

### 1.1 目標

- 在 `atlas-mcp` 暴露 7 個新 MCP tool：
  - 個股級：`stock_get_quote`、`stock_get_fundamentals`、`stock_get_chips`、`stock_get_technical`
  - 資金流向：`capital_flow_daily`、`capital_flow_summary`
  - 策略排名：`strategy_ranker`
- 新增對應後端 HTTP API，讓 MCP server 維持「透過 `/api/*` 呼叫 atlas-go」的既有模式。
- 統一各文件中的 tool 數量與權威來源。

### 1.2 非目標

- 不破壞既有 MCP tool 的註冊、稽核、rate-limit 流程。
- 不新增獨立數據抓取通道；底層全部複用既有 `marketdata` / `portfolio` / `ledger` 能力。
- 不處理 SSE/streamable-HTTP transport 的剩餘 P1 工作。

---

## 2. 架構總覽

```
AI Agent (MCP client)
        │
        ▼
cmd/atlas-mcp/server
   tools_stock.go            tools_capitalflow.go      tools_strategy_ranker.go
        │                            │                         │
        └──────────────┬─────────────┴─────────────┬───────────┘
                       ▼                           ▼
              /api/stock/*                 /api/capital-flow/*
              /api/strategy-ranker/*
                       │
        ┌──────────────┼──────────────┬──────────────┐
        ▼              ▼              ▼              ▼
  marketdata    portfolio      marketdata      ledger
  .FugleClient  .Fundamental   .TWSECapital    .QuoteStore
                               FlowProvider
```

---

## 3. 新增後端 API

### 3.1 個股工具：`internal/stocktools`

新套件 `internal/stocktools`，提供 `Handler` 與 `RegisterRoutes`。

| Route | Method | 說明 |
|-------|--------|------|
| `/api/stock/quote` | GET | `symbol` 必填；回傳即時行情 |
| `/api/stock/fundamentals` | GET | `symbol` 必填；回傳基本面數據 |
| `/api/stock/chips` | GET | `symbol` 必填，`date` 可選（YYYY-MM-DD，預設最近交易日）；回傳三大法人/融資融券籌碼 |
| `/api/stock/technical` | GET | `symbol` 必填，`days` 可選（預設 90，max 365）；回傳 SMA/RSI |

#### 資料來源

- **quote**：`marketdata.GetSharedFugleClient(cfg.FugleAPIKey)` 的 `GetQuote(symbol)`。
- **fundamentals**：`portfolio.NewFundamentalProvider()` 載入 `data/fundamentals.json`（若存在）。
- **chips**：擴充 `marketdata.TWSECapitalFlowProvider`，新增依證券代號與日期從 TWSE T86 抓取單一標的三大法人買賣超；融資融券資料由 `marketdata.TWSEMarginBalanceProvider` 提供（目前為全市場彙總，未來可擴充單一標的）。
- **technical**：從 `ledger.QuoteStore` 讀取歷史日 K，計算 SMA20、SMA50、RSI14。

### 3.2 策略排名：`internal/strategy_ranker/handler.go`

| Route | Method | 說明 |
|-------|--------|------|
| `/api/strategy-ranker/rank` | GET | 回傳目前 active strategies 的排名與 tier（free / registered / premium）|

#### 資料來源

- 呼叫 `internal/monitoring/api/strategies` 的 `Registry` 取得 active strategy frames。
- 將 frame 中的 `HitRate`、`TotalTests` 等指標轉換為 `strategy_ranker.StrategyReport`，再透過 `strategy_ranker.RankAndTier` 產出 `RankedReport`。
- 未來若策略回測日報酬序列可取得，再換成完整績效指標（Sharpe、MaxDrawdown 等）。

---

## 4. MCP Tool 映射

| MCP Tool | HTTP API | Input | Output |
|----------|----------|-------|--------|
| `stock_get_quote` | `/api/stock/quote?symbol={symbol}` | `symbol` | `domain.Quote` 欄位 |
| `stock_get_fundamentals` | `/api/stock/fundamentals?symbol={symbol}` | `symbol` | PE、PB、PS、DividendYield、Sector |
| `stock_get_chips` | `/api/stock/chips?symbol={symbol}&date={date}` | `symbol`, `date?` | 外資/投信/自營商買賣超、融資融券餘額 |
| `stock_get_technical` | `/api/stock/technical?symbol={symbol}&days={days}` | `symbol`, `days?` | close、volume、sma20、sma50、rsi14 |
| `capital_flow_daily` | `/api/capital-flow/daily` | — | `capitalflow.DailyReport` |
| `capital_flow_summary` | `/api/capital-flow/summary` | — | `capitalflow.SummaryReport` |
| `strategy_ranker` | `/api/strategy-ranker/rank` | — | `[]strategy_ranker.RankedReport` |

---

## 5. Tool 數統一方案

目前文件數字不一致：

- `docs/reference/tool-catalog.md`：108
- `cmd/atlas-mcp/README.md`：79
- `cmd/atlas-mcp/server/AGENTS.md`：75 / 79
- `docs/specs/agent-mcp-server-spec.md`：約 70

**決議**：

1. `docs/reference/tool-catalog.md` 為**唯一權威 catalog**，所有新增/刪除 tool 必須同步更新該文件。
2. 其他文件不再重複寫死數字，改以「詳見 `docs/reference/tool-catalog.md`」或「以 `mcp/tools/list` / `system_get_health` 回傳為準」取代。
3. `cmd/atlas-mcp/server/AGENTS.md` 的 assertion 範圍（目前 77–79）更新為 84–86，以反映新增 7 個 tool。

---

## 6. 修改檔案清單

- `internal/stocktools/handler.go`（新增）
- `internal/stocktools/handler_test.go`（新增）
- `internal/marketdata/twse_capital_flow_provider.go`（新增單一標的查詢方法）
- `internal/strategy_ranker/handler.go`（新增）
- `internal/strategy_ranker/handler_test.go`（新增）
- `cmd/atlas/main.go`（註冊新路由、更新 `isPublicPath`）
- `cmd/atlas-mcp/server/tools_stock.go`（新增）
- `cmd/atlas-mcp/server/tools_capitalflow.go`（新增）
- `cmd/atlas-mcp/server/tools_strategy_ranker.go`（新增）
- `cmd/atlas-mcp/server/tools.go`（加入 `registerStockTools`、`registerCapitalFlowTools`、`registerStrategyRankerTools`）
- `cmd/atlas-mcp/server/server.go`（更新 `RegisteredToolCount` assertion 範圍）
- `cmd/atlas-mcp/server/prompts.go`（將 prompt 中的佔位 tool 名稱替換為正式名稱）
- `docs/reference/tool-catalog.md`（新增 catalog 區塊、更新總數）
- `cmd/atlas-mcp/README.md`（移除重複數字，指向 reference/tool-catalog.md）
- `cmd/atlas-mcp/server/AGENTS.md`（更新 assertion 範圍與說明）
- `docs/specs/agent-mcp-server-spec.md`（調整總數描述，指向 reference/tool-catalog.md）

---

## 7. 測試策略

- 單元測試：`internal/stocktools/handler_test.go`、`internal/strategy_ranker/handler_test.go` 使用 mock HTTP client / mock registry。
- MCP 層測試：`cmd/atlas-mcp/server/tools_stock_test.go`、`tools_capitalflow_test.go`、`tools_strategy_ranker_test.go` 透過 `httptest` 模擬後端。
- 整合測試：啟動 `cmd/atlas-mcp` 並驗證 `tools/list` 回傳包含 7 個新 tool。
- CI：`go test ./cmd/atlas-mcp/... ./internal/stocktools/... ./internal/strategy_ranker/...`、`go vet`、`gofmt`。

---

## 8. 風險與注意事項

- **資料可用性**：`technical` 依賴 `ledger.QuoteStore` 是否有該標的歷史日 K；若無資料則回傳 503 並說明缺失。
- **API key**：`stock_get_quote` 需要 `FUGLE_API_KEY`；未配置時回傳 503。
- **憲法合規**：所有外部 API 呼叫沿用既有 `marketdata` provider 與 rate limiter，不新增直接 `os.Getenv` 抓取。
- **Public path**：新增 `/api/stock` 與 `/api/strategy-ranker` 前綴至 `isPublicPath`（與 `/api/macro`、`/api/strategies` 同等級）。
- **Tool 數 drift**：新增 tool 後務必同步更新 `server.go` assertion 與 `docs/reference/tool-catalog.md`。
