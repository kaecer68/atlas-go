# Manifest K — `stock_get_monthly_revenue` 端點擴充

> kaecer 2026-08-07 拍板的 monthly_revenue 端點實作方案(advisor 4 個關鍵選項已審計通過)
> 對位 hermes 派工 v4.0(2026-08-06 18:25 派工 prompt)+ 重新盤查後的修正方案

## Manifest 6 欄

| 欄 | 內容 |
|---|---|
| **範圍** | `stock_get_monthly_revenue` MCP 端點 + `GET /api/stock/monthly_revenue` HTTP 端點 + 通用化 TSMCRevenueProvider + 前端 stock-quote 頁面月營收 section |
| **不改的邊界** | (1) stocktools coverage guard 不擴大(PR #1477 「上櫃不納入」決策保留);(2) `MacroDataSnapshot.TSMCRevenue` 結構形狀不變(`silicon_cycle.go:408` 仍讀 `ChangePct`,向下相容);(3) 既有 4 個 stock 端點行為不動;(4) TSMCRevenueProvider 既有 `FetchSnapshot(ctx)` 行為不動(default 仍 "2330");(5) 不開新 SK 頁、不動 atlas-wiki |
| **相依** | (1) `FinMindClient.GetMonthRevenue(symbol, year, month)` — 通用 API 已存在,涵蓋上市+上櫃+興櫃(`finmind_client.go:258`);(2) `GlobalQuotaRegistry()` + `DailyQuotaTracker` 統一管理 FinMind 配額(避免 14400/day 撞牆);(3) `format-metric.js` 既有色彩 token(YoY/MoM 正綠負紅);(4) `stock-quote.js` 既有 4-section page-shell 結構 |
| **回滾** | PR-1:`git revert <sha>` 即可。`FetchSnapshotForSymbol` 與 `FetchSnapshot` 是**新增 method**,不動既有 method 行為。`HandleMonthlyRevenue` 與既有 4 個 handler 是**新增 route**,刪除 route 不影響既有 4 個。PR-2:`git revert <sha>` 即可,前端改動獨立 |
| **測試** | PR-1 後端:15 unit test(8 TSMCRevenueProvider + 6 stocktools + 1 MCP wrapper)+ `tools_transport_sse_test` 範圍 114-116 → 114-117。PR-2 前端:1 client unit test + 5 個 stock 瀏覽器手動驗收(2330/3131/3587/6640/2408) |
| **預期 PR 數** | **2 個**:PR-1 後端(branch `feat/20260806-monthly-revenue-backend`)+ PR-2 前端(branch `feat/20260806-monthly-revenue-frontend`)。**PR-3 scheduler 月頻 task 留給後續 session**,需等 PR-1 部署後 observation 1 個月才能評估撞牆風險 |

## 修法細節

### 修法 1:`internal/marketdata/tsmc_revenue_provider.go` — 通用化(hermes 紅線 #3)

**改既有檔,非新增 sibling**。新增 method:

```go
// FetchSnapshotForSymbol is the per-symbol variant of FetchSnapshot.
// The base FetchSnapshot is preserved (default symbol = "2330", used by
// the macro channel) and continues to feed silicon_cycle.go:408.
//
// Stocktools handlers use this method directly via Deps.Revenue.
func (p *TSMCRevenueProvider) FetchSnapshotForSymbol(ctx context.Context, symbol string) (MacroDataSnapshot, error) {
    // 同 FetchSnapshot 但 symbol 為參數
}
```

**保留** `FetchSnapshot(ctx)` 既有行為(default symbol = "2330")。

### 修法 2:`internal/stocktools/handler.go` — 新增 endpoint

- `Deps` 加 `Revenue *marketdata.TSMCRevenueProvider` 欄位
- `RegisterRoutes` 加 `GET /api/stock/monthly_revenue`
- 新增 `HandleMonthlyRevenue`:
  - 走「獨立 scope 例外」設計(**不** `LookupCoverage`)
  - `Deps.Revenue` nil → 503
  - symbol empty → 400
  - `GlobalQuotaRegistry().Get("finmind").Remaining() < 3` → 503(主動 fail-soft)
  - year/month malformed → 400
  - 15s context timeout
  - 預設 year/month = 上個月(`now.AddDate(0, -1, 0)`)

### 修法 3:`cmd/atlas-mcp/server/tools_stock.go` — MCP wrapper

- `registerStockTools` 加 `stock_get_monthly_revenue` 註冊
- 新增 `stockMonthlyRevenueInput`(symbol 必要 + year/month optional)
- 新增 `handleStockGetMonthlyRevenue` wrapper

### 修法 4:前端 PR-2(對位 kaecer 網頁 UX 驗收)

- `shared_web/static/js/services/stock-api-client.js`:加 `fetchStockMonthlyRevenue(symbol, year?, month?)` + TTL 7 天
- `shared_web/static/js/pages/stock-quote.js`:在 4 個 section 後加「月營收」section
- UX:
  - YoY > 30% 顯示「結構性訂單 signal ✅」綠色 badge(對位 hermes §4.3 #4 + SK-31 §4 #2)
  - YoY/MoM 正值綠色、負值紅色(對位 `format-metric.js`)
  - 與既有 4 個 section 排版一致
  - loading 與其他 4 個端點平行

## 驗收標準

### 程式碼驗收(commit 後)
- `go test ./internal/marketdata/ ./internal/stocktools/ ./cmd/atlas-mcp/...` 全部綠
- `gofmt -l .` 清
- `go vet ./...` 通過
- `make ci-gate` 通過

### MCP 端點驗收(PR-1 部署後)
- `mcp__atlas_mcp__stock_get_monthly_revenue(symbol="3131")` → 200 + 弘塑 2026-07 月營收 + YoY%
- `mcp__atlas_mcp__stock_get_monthly_revenue(symbol="3587")` → 200 + 閎康 2026-07 月營收 + YoY%(hermes 寫「萬潤」是錯的,實際 3587 = 閎康)
- `mcp__atlas_mcp__stock_get_monthly_revenue(symbol="6640")` → 200 + 均華 2026-07 月營收 + YoY%(hermes 寫「6641」是錯的,實際 6640)
- `mcp__atlas_mcp__macro_get_snapshot_latest` 仍回 2330 數據(向下相容,`silicon_cycle` 不破)

### 網頁 UX/UI 驗收(PR-2 部署後,kaecer 明確要求)
- 瀏覽器打開 `https://atlas-go/quote/2330` 看到「月營收」section
- 瀏覽器打開 `https://atlas-go/quote/3131` 看到(對位 hermes §4.3 #1)
- 瀏覽器打開 `https://atlas-go/quote/6640` 看到(對位 hermes §4.3 #3 修正代號)
- YoY > 30% 顯示「結構性訂單 signal ✅」badge
- YoY/MoM 色彩 token 正確
- Chrome DevTools Network 看 `GET /api/stock/monthly_revenue?symbol=3131` 200 OK

## 對位錨

- **mission**:散戶 AI 實戰金融工程(找信息差、悶聲賺錢)
- **hermes 派工 v4.0**:`~/workspace/atlas-notes/05-decisions/atlas-monthly-revenue-endpoint-prompt-2026-08-06.md`
- **SK-31 §3.5 + §6**:`atlas-wiki/skills/SK-31-ai-investment-cycle-2026.md` Layer 4 個股層 + §6 #1 缺口
- **PR #1477**:`docs/manifests/2026-08-06-stock-coverage-notice.md` — TWSE coverage scope 設計邊界
- **PR #1478**:`docs/investigations/2026-08-06-equipment-stocks-chips-gaps.md §8` — TPEX scope affirmation
- **8/6 quota collision**:`docs/investigations/2026-08-06-finmind-quota-collision.md` — process 重啟多 task 同步觸發撞牆
- **T3-A275.1**:governance log §3.5 6 條驗證缺口判定依據
- **Production mtime**:SOUL.md (8/3) / AGENTS.md (8/2) / 憲法 (7/20) 全守

## 預估工作量

- PR-1 後端:**0.4 人天**
- PR-2 前端:**0.3-0.5 人天**
- PR-3 scheduler:**0.5-1 人天**(留給後續 session)
