# stocktools Coverage Notice 改動方案 — 2026-08-06

> **狀態**: 方案 (盤查完成 2026-08-06, 實作進行中)
> **Trigger**: 2026-08-06 弘塑 3131、閎康 3587（萬潤實為 6187，使用者輸入口誤）三家設備股 chips/fundamentals 缺口實查
> **根因報告**: `docs/investigations/2026-08-06-equipment-stocks-chips-gaps.md`、`docs/investigations/2026-08-06-equipment-stocks-chips-gaps-research.md`
> **範圍邊界**: **不擴大**資料源（不上櫃、不接 TPEX）。**只**在 stocktools 4 個 endpoint 加入統一的 coverage 告知，使前端/MCP/CLI caller 能正確識別「資料源未涵蓋此 symbol」與「資料源失敗」這兩種差異。

---

## 1. 盤查結果 (2026-08-06)

### 1.1 現有 4 endpoint 真實行為

| Endpoint | 既有結果 | 在涵蓋範圍外 (舉例 3131 上櫃) 真實表現 |
|----------|----------|-----------------------------------------|
| `GET /api/stock/quote` | Fugle 5s → TWSE 5s fallback → 200/404/503 | 200 OK（**Fugle 涵蓋上櫃**） |
| `GET /api/stock/fundamentals` | 從 `data/fundamentals.json` 拿 → 200 | 200 OK 但 5 個欄位全 0 + Sector 補不出（**snapshot 不含上櫃**） |
| `GET /api/stock/chips` | TWSE T86 5s+rate limit 7 天回溯 → 200/503 | **503 `context.Canceled`**（T86 不含上櫃、7 天回溯吃滿 handler 15s context） |
| `GET /api/stock/technical` | QuoteStore / Fugle on-demand → 200 | 200 OK 或空 bars（若 QuoteStore 無此 symbol） |

**沒有任何一條回「out-of-scope」**——使用者或 caller 無法區分「資料真的不存在」「資料源不涵蓋」「API 暫時失敗」。

### 1.2 已存在可複用介面

| 元件 | 提供 |
|------|------|
| `portfolio.FundamentalProvider.HasData()/Get(symbol)` | snapshot 內某個 `.TW` symbol 是否有資料（**直接作為 coverage 唯一依據**：snapshot = TWSE 上市 1070 隻，與「上櫃不涵蓋」前提完美對齊） |
| `industry.ClassifyBySymbol(symbol)` | 既有 sector 補綴；不在範圍也繼續嘗試 |
| `internal/monitoring/api/shared.Handler` | handler 簽名：`func(r *http.Request) (status int, data any)` |
| `internal/apigateway` RouteRegistry | stocktools 4 endpoint 都已是 `authFree`（公開路徑） |
| `cmd/atlas-mcp/descgen/` | auto-desc 由 `go generate` 重新生成；**手編 auto-desc.gen.go 被覆蓋** |

### 1.3 不重複造輪

- **`internal/industry/symbol_coverage.go`** 是「industry representative stocks 對 FinMind 收錄清單」覆蓋狀態，與本任務語義不同；**不**誤用、不混名。
- `internal/stocktools/data/state/` 唯有一個 `fugle_daily_quota.json`，與本任務無關。

---

## 2. 改動計畫 (Design)

### 2.1 Backend

#### 2.1.1 新增 `internal/stocktools/coverage.go`

```go
package stocktools

// CoverageEntry 描述單一 symbol 在 stocktools 4 個 endpoint 的涵蓋範圍。
// 設計：依賴 portfolio.FundamentalProvider 為唯一判斷來源。
//   - snapshot 內（1070 隻純 .TW）→ covered=true, listing="TWSE"
//   - snapshot 外 → covered=false, listing="UNKNOWN", reason="fundamentals_snapshot_not_covered"
// quote 端點因 Fugle 涵蓋全市場，仍回 covered=true 但用 listing_note 標註實際市場
type CoverageEntry struct {
    Symbol       string `json:"symbol"`
    Covered      bool   `json:"covered"`              // snapshot 是否收錄
    Listing      string `json:"listing"`              // "TWSE" 或 "UNKNOWN" 或 "TPEX_QUOTE_ONLY"
    QuoteCovered bool   `json:"quote_covered"`        // Fugle 端點仍能用
}

// LookupCoverage(symbol string, fp *portfolio.FundamentalProvider) CoverageEntry
```

#### 2.1.2 `internal/stocktools/handler.go` 4 handler guard

每個 handler 開頭加：
```go
if h.deps.Fundamentals != nil && h.deps.Fundamentals.HasData() {
    cov := LookupCoverage(symbol, h.deps.Fundamentals)
    if !cov.Covered {
        return http.StatusOK, map[string]any{
            "symbol":         symbol,
            "coverage_note":  "NOT_COVERED",
            "listing":        cov.Listing,
            "reason":         "本系統 chips/fundamentals 涵蓋 TWSE 上市普通股；此 symbol 不在資料範圍",
        }
    }
}
```

**重要**：guard 只在 `Fundamentals != nil && HasData()` 時觸發——保證既有用例（含 `Deps{}` 0 deps 注入）不被破壞。

**Status 決策**：200 OK + structured `coverage_note`，**不是 503**。理由：
- caller 是 LLM agent / 前端 / CLI：對它們 503 會被誤判為 server failure
- 200 + `coverage_note` 是結構化資料；前端可據此顯示徽章、CLI 可正常解析
- 後端既有「API 真實失敗」路徑保持原狀（503 / 錯誤訊息）

#### 2.1.3 新增 `internal/stocktools/coverage_endpoint.go`

```go
mux.Handle("GET /api/stock/coverage", shared.Get(h.HandleCoverage))
```

`HandleCoverage` 直接呼叫 LookupCoverage 回 JSON：`{symbol, covered, listing, quote_covered, reason}`。

### 2.2 MCP 告知

`cmd/atlas-mcp/server/tools_stock.go` 4 個 tool description 補一句「Coverage: TWSE-listed common stocks (1070 names; -ETFs/-delisted/-OTC)；不涵蓋時回 `coverage_note: "NOT_COVERED"`」。

**嚴格**: `cmd/atlas-mcp/auto-desc.gen.go` / `auto-desc.gen.json` 是 generated，**不手編**。改完後跑 `go generate ./cmd/atlas-mcp/...` 重生。`tools_conformance_test.go` invariant 1（每 tool 都有非空 description）必須仍然過。

### 2.3 Frontend

#### 2.3.1 入口預攔 `shared_web/static/js/components/stock-quote-search.js`

`doSearch(clean)` 在 `onSearch(clean)` 之前先 fetch `/api/stock/coverage?symbol=clean`：
- `covered=true` → 走原 `onSearch`
- `covered=false` → 改寫呼叫端的 `state.results`，注入 `coverage_note`，render 一個全頁未涵蓋卡片 + 提示用戶「chips/fundamentals 不適用此 symbol」

#### 2.3.2 render 端 — `stock-quote-header.js` / `-chips.js` / `-fundamentals.js` / `-technical.js`

- 偵測到 `result.coverage_note === 'NOT_COVERED'` → 顯示一個 `sq-coverage-notice` 徽章（**不視為 error**），區塊仍渲染 quote（Fugle 仍能用）
- 對 chips / fundamentals：原本「error→紅框」**移除 generic 錯誤訊息**，只在真的 API failure 才顯示紅框；out-of-scope 走中性徽章

#### 2.3.3 `services/stock-api-client.js` 的 `fetchStockBundle`

不必新增函式；`fetchCached` 透明處理已夠。**前端重點是 render 端**如何詮釋 `coverage_note`。

### 2.4 Spec

`docs/specs/stock-api-contract-spec.md` 新增 §1.5「Coverage Scope」：
- 4 個 stock endpoint 涵蓋 TWSE 上市普通股
- 不涵蓋時回 200 + `coverage_note: "NOT_COVERED"`
- 提供 `/api/stock/coverage?symbol=X` 供前端預攔
- 既有 200/404/503/錯誤路徑語意不變

---

## 3. 不變的 invariants（不可破壞）

| 項目 | 現狀 | 不破壞承諾 |
|------|------|------------|
| `internal/stocktools/handler_test.go` 11 個既有測試 | 全部 PASS | 一個都不能 break |
| chip handler `503 context.Canceled` 路徑 | handler 內部既有 `fetchLatestSymbolFlow` 7 天回溯 + 15s context | **不在 4 handler 內部修改** chips/quote/technical/fundamentals 既有邏輯；只在開頭加 guard |
| `Deps{}` 零注入測試 | 仍應回 503/400 原樣 | guard 條件 `Fundamentals != nil && HasData()` 必須包含此 false branch |
| `cmd/atlas-mcp/tools_conformance_test.go` | 期望 tool 數量 [105, 120]、description 不為空 | 修改 description 後必須仍 ≥ 60 個 + non-empty |
| `data/fundamentals.json` 結構 | map[string]FundamentalData | 不修改；只讀 |
| stocktools `authFree` 路徑 | `/api/stock/*` 在 isPublicPath 白名單 | 新增 `/api/stock/coverage` 也必須維持 authFree |

---

## 4. out-of-scope

- ❌ 不新增 TPEX chips provider（不上櫃承諾）
- ❌ 不擴大 `data/fundamentals.json` snapshot
- ❌ 不改既有 stocks agent universe
- ❌ 不動 `internal/industry/symbol_coverage.go`（語義不同）
- ❌ 不改既有 4 handler 內部流程，只在入口加 guard
- ❌ 不動 stocktools 4 endpoint 的路由 path
- ❌ 不影響 dashboard 與 macro 路徑

---

## 5. 變更邊界（file list）

**Backend** (5):
- `internal/stocktools/coverage.go` (新)
- `internal/stocktools/coverage_endpoint.go` (新) 或與 `coverage.go` 合一
- `internal/stocktools/handler.go` (改：4 handler 加 guard + RegisterRoutes 加 coverage route)
- `internal/stocktools/coverage_test.go` (新)
- `internal/stocktools/handler_test.go` (改：新增 NOT_COVERED 測試、舊測試零修改)

**MCP** (2):
- `cmd/atlas-mcp/server/tools_stock.go` (改：description 補字)
- `cmd/atlas-mcp/auto-desc.gen.json` (auto-generated by `go generate`)

**Frontend** (4):
- `shared_web/static/js/components/stock-quote-search.js` (改：預攔)
- `shared_web/static/js/components/stock-quote-header.js` (改：徽章支援)
- `shared_web/static/js/components/stock-quote-chips.js` (改：error/empty 區分)
- `shared_web/static/js/components/stock-quote-fundamentals.js` (改：error/empty 區分)

**Spec / Tracking** (2):
- `docs/specs/stock-api-contract-spec.md` (改：§1.5)
- `docs/manifests/2026-08-06-stock-coverage-notice.md` (本檔)

**Total: 12 檔**（含 3 新檔）

---

## 6. 測試計畫

| 測試 | 驗證 |
|------|------|
| `TestLookupCoverage_TWSE_TWSESnapshot` | 3131 → covered=false, listing=UNKNOWN, reason="fundamentals_snapshot_not_covered" |
| `TestLookupCoverage_TWSE_SnapshotHit` | 6641 → covered=true, listing=TWSE |
| `TestLookupCoverage_NilProviderNoCrash` | 零注入 → 回 covered=true (no guard activated) |
| `TestHandleQuote_NotCoveredReturns200WithNote` | 既有 handler_test 之外：symbol 不在 snapshot → 200 + coverage_note |
| `TestHandleChips_NotCoveredBypassesLongLookup` | 同上但不進 15s context 等待 |
| `TestHandleQuote_NoFundamentalsLoad_StillWorks` | 既有 TestHandleQuote / TestHandleQuote_TWSEFallback... / TestHandleQuoteMissingSymbol / TestHandleQuoteNoProvidersReturns503 全部仍 PASS |
| `TestHandleFundamentals` + `TestHandleFundamentalsRawSymbolNormalized` | snapshot `.TW` 仍命中 |
| `TestHandleChips` | 既有 200 路徑仍 200、body 仍含 `"symbol":"2330"` |
| `TestHandleTechnical` | 既有 QuoteStore 200 路徑仍 PASS |
| `tools_conformance_test` | 跑 `go test ./cmd/atlas-mcp/server` 仍 PASS |

---

## 7. 風險

| 風險 | 緩解 |
|------|------|
| `Fundamentals` 為 nil 時 guard 跳過 → 既有 stocktools 沒 snapshot 的測試仍走原本路徑 | guard 條件已 include nil check |
| handler_test 既有 11 個測試 case 用 `Deps{FugleClient: ...}` 而非含 Fundamentals → guard 跳過 | 同上 |
| coverage endpoint 名稱衝突 | grep 全 repo：`/api/stock/coverage` 0 match（驗證） |
| frontend render 變更可能 break 既有 chips/fundamentals 既有 error 顯示 | 嚴格區分 `state==='error'` 與 `result.coverage_note==='NOT_COVERED'`，僅後者走新路徑 |
| MCP description 改動後跑 go generate 失敗 | 嚴格只加字串、不改結構 |
| share pre-existing 「ALLBUT0999」 chips 邏輯保留 | 完全不動 twse_capital_flow_provider.go |

---

## 8. 驗證計畫

- `make check-binaries`：commit 前確認 binary buildinfo 對齊 HEAD（補救：純 `go build`、含 docker 例外交回 kaecer）
- `make ci-gate`：gofmt + go build + go vet + generate drift
- 手動打 5 個 endpoint × 3 隻 symbol（3131/3587/6641）= 15 個斷言
- 不跑 `make ci-full`（PR 前才跑）

---

## 9. 完成標準 (DOD)

- [ ] 12 檔變更（9 改 + 3 新）落地
- [ ] `internal/stocktools/handler_test.go` 11 既有測試全綠
- [ ] 4 handler 對 3131/3587/6641 的回應符合 §2.1.2 設計
- [ ] `/api/stock/coverage?symbol=3131` 回 `covered=false, listing=UNKNOWN, quote_covered=true`
- [ ] `/api/stock/coverage?symbol=6641` 回 `covered=true, listing=TWSE, quote_covered=true`
- [ ] 前端 input 3131 後顯示徽章而非紅框
- [ ] `tools_conformance_test.go` 不破壞
- [ ] `make ci-gate` exit 0
- [ ] `git commit` 在 `fix/20260806-stock-coverage-notice`，**不 push、不 PR**（容器 rebuild 由 kaecer 在主 worktree 執行）
- [ ] stop at checkpoint，告知 kaecer rebuild

---

## 10. 引用

- 根因：`docs/investigations/2026-08-06-equipment-stocks-chips-gaps.md`
- 範圍+調研：`docs/investigations/2026-08-06-equipment-stocks-chips-gaps-research.md`
- 既有產業 coverage（不可混淆）：`internal/industry/symbol_coverage.go`
- stocktools handler：`internal/stocktools/handler.go`
- MCP descgen：`cmd/atlas-mcp/descgen/main.go`
- 前端入口：`shared_web/static/js/pages/stock-quote.js` + `shared_web/static/js/components/stock-quote-search.js`
