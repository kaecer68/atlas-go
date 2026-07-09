# Atlas Stock API Contract（前端單一權威來源）

> **文件角色**：定義 `/api/stock/*` 4 個 endpoint 的 HTTP contract（路徑、查詢、回應、錯誤、單位、Source-of-truth），供 client_web 與 atlas-mcp 共用。
> **狀態**：v1.1（2026-07-09 新增 Symbol normalization + Technical 7 欄位修正）
> **關聯**：[`.omo/plans/2026-07-09-stock-quote-frontend.md`](../../.omo/plans/2026-07-09-stock-quote-frontend.md) | [`docs/specs/stock-quote-page.md`](stock-quote-page.md)
> **Source-of-truth**：handler 源碼 `internal/stocktools/handler.go` + 各資料源 struct

---

## §0 前置約定（必須遵守）

### 0.1 認證
所有 `/api/stock/*` 端點受 JWT 認證保護（subscription 模組 gate）。前端必須先呼叫 `POST /api/auth/login` 取得 token，再以 `Authorization: Bearer <token>` header 呼叫 stock API。**未認證 → 401**（curl 實機驗證）。

### 0.2 Symbol 格式約定（v1.1 新增 normalization）

**API contract：所有 4 個 endpoint 接受純數字 symbol**（如 `?symbol=2330`）。

**理由**：Fugle、TWSE T86、ledger QuoteStore 均以純數字為 key；只有 `data/fundamentals.json` 內部使用 Yahoo-suffix（`2330.TW`）。為避免前端處理兩種格式，**後端在 `HandleFundamentals` 內部呼叫 `normalizeFundamentalsSymbol()` 自動補 `.TW`**，讓 API contract 保持純數字。

| 端點 | 內部處理 | 前端可送 |
|---|---|---|
| `/api/stock/quote` | 直接傳給 FugleClient（純數字） | `2330` |
| `/api/stock/fundamentals` | `normalizeFundamentalsSymbol()` 自動加 `.TW` | `2330` 或 `2330.TW` 都接受 |
| `/api/stock/chips` | 直接傳給 TWSE T86（純數字） | `2330` |
| `/api/stock/technical` | 直接傳給 ledger QuoteStore（純數字） | `2330` |

**邊緣情況**（`normalizeFundamentalsSymbol`）：
- 空字串 → 回傳空字串（會被前置 400 擋下）
- 已是 `XXXX.TW` / `.US` / `.HK` / `.JP` / `.CN` suffix → 原樣回傳
- 其他格式（如 `2330.SH`）→ 原樣回傳（不靜默腐蝕非台股 symbol）

### 0.3 共用錯誤格式
所有錯誤回傳 `{"error": "<message>"}`：

| HTTP status | 情境 |
|---|---|
| 400 | symbol 缺失 |
| 401 | JWT 缺失或過期 |
| 500/503 | Provider 未配置 / 資料未載入 / 上游 API 失敗 |

---

## §1 `GET /api/stock/quote`

**Handler**：`internal/stocktools/handler.go::HandleQuote`（line 45-61）
**資料源**：`marketdata.FugleClient.GetQuote()` → `domain.Quote` struct（`internal/domain/shared/shared.go:27`）
**條件性啟用**：無 `FUGLE_API_KEY` 環境變數時 `deps.FugleClient=nil` → 回 `503 quote provider not configured`

**Query**：`symbol=<digits>`（必填）

**Response 200**（10 個欄位）：
```json
{
  "symbol": "2330",
  "last": 680.0,
  "open": 670.0,
  "high": 685.0,
  "low": 668.0,
  "volume": 12345,
  "market": "TSE",
  "as_of": "2026-07-07T14:30:00+08:00",
  "is_tradable": true,
  "source": "fugle"
}
```

**前端衍生欄位**（不存於原始 response）：
- `change = last - open`（漲跌點數）
- `change_pct = (change / open) * 100`（漲跌幅 %）
- `volume_lots = volume / 1000`（成交量張數，台股慣用單位）

**單位**：
- `last/open/high/low`：台幣元
- `volume`：股（前端需轉張）
- `as_of`：ISO8601 with timezone

---

## §2 `GET /api/stock/fundamentals`

**Handler**：`internal/stocktools/handler.go::HandleFundamentals`（line 63-72，**v1.1 新增 normalizeFundamentalsSymbol 呼叫**）
**資料源**：`portfolio.FundamentalProvider.Get()` 從 `data/fundamentals.json`（本地 JSON，非即時）
**條件性啟用**：無 `data/fundamentals.json` 檔案或檔案為空 → `503 fundamentals data not loaded`

**Query**：`symbol=<digits>`（必填；v1.1 開始接受純數字，內部自動加 `.TW`）

**Response 200**（最多 5 個欄位，Code 完整支援但 Data 目前只填 3 個）：
```json
{
  "PE": 25.3,
  "PB": 6.1,
  "PS": 0.0,
  "DividendYield": 1.5,
  "Sector": "semiconductor"
}
```

**已知資料缺口**（v1.1）：
- `data/fundamentals.json` 實際只有 `PE/PB/DividendYield`（1070 個 symbols，5351 行）
- `PS=0` 與 `Sector=""` 是正常零值（前端需區分顯示「—」/「未分類」）
- `SectorMedianPE(sector)` 已實作（`fundamental_loader.go:72`）但 stock API 未暴露，留待後續 PR

**Sector enum**（`fundamental_loader.go:13-22`）：
`semiconductor / financials / electronics / shipping / energy / consumer / industrial / other`

**Symbolization 例外**：
- 雖然 `FundamentalProvider.Get(symbol)` 直接做 map lookup，但 v1.1 新增的 `normalizeFundamentalsSymbol()` 確保 API 接受純數字
- `fp.Get("2330")` → 內部轉 `fp.Get("2330.TW")` → 命中 data

---

## §3 `GET /api/stock/chips`

**Handler**：`internal/stocktools/handler.go::HandleChips`（line 75-95）
**資料源**：`marketdata.TWSECapitalFlowProvider.FetchSymbolFlow()` → `marketdata.SymbolFlow` struct（`twse_capital_flow_provider.go:60-68`）

**Query**：
- `symbol=<digits>`（必填）
- `date=<YYYYMMDD>`（選填；預設當日，內部 fallback 7 天內最近交易日）

**Response 200**（6 個欄位）：
```json
{
  "symbol": "2330",
  "name": "台積電",
  "foreign_investor_net": 500.0,
  "domestic_fund_net": 500.0,
  "dealer_net": 400.0,
  "date": "20260708"
}
```

**單位**（已自動轉好）：
- `*_net` 單位是**張**（`parseTWDVolume(row[N]) / 1e3`，前端**不需再除 1000**）
- 台股三大法人買賣超慣用單位即「張」

---

## §4 `GET /api/stock/technical`

**Handler**：`internal/stocktools/handler.go::HandleTechnical`（line 117-143）
**資料源**：`ledger.QuoteStore.LoadQuotes()` → `computeTechnical()`（line 145-160，**v1.1 修正：7 個欄位，非 4 個**）

**Query**：
- `symbol=<digits>`（必填）
- `days=<int>`（選填；預設 90，最大 365）

**Response 200**（**7 個欄位**，v1.1 修正）：
```json
{
  "symbol": "2330",
  "date": "2026-07-08",
  "close": 680.0,
  "volume": 12345,
  "sma20": 675.5,
  "sma50": 670.2,
  "rsi14": 58.3
}
```

**欄位說明**：
| 欄位 | 來源 | 單位 | 說明 |
|---|---|---|---|
| `symbol` | `latest.Symbol` | — | 與 query 相同 |
| `date` | `latest.Date.Format("2006-01-02")` | YYYY-MM-DD | 最後一個交易日 |
| `close` | `latest.Close` | 台幣元 | 最後收盤價 |
| `volume` | `latest.Volume` | 股 | 最後成交量 |
| `sma20` | `sma(closes, 20)` 末端 20 日均線 | 台幣元 | 簡單移動平均 |
| `sma50` | `sma(closes, 50)` 末端 50 日均線 | 台幣元 | 簡單移動平均 |
| `rsi14` | `rsi(closes, 14)` 末端 14 日 RSI | 0-100 | 相對強弱指標 |

**已知限制**（v1.1）：
- 只有 7 個欄位（單點 KPI），**不含歷史 bars**（無法繪製走勢圖）
- 前端若需 sparkline，必須等後續 PR 擴充 `bars []domain.DailyBar`
- RSI 公式在 `handler.go:190` 有 pre-existing bug（`100-(100/(1+rs))*100` 數學錯誤），不在 PR-A scope，留待後續 bugfix PR

**Error 503**：`insufficient historical quote data`（bars 數 < 2）

---

## §5 跨 API 約定（前端 client 設計約束）

### 5.1 並發呼叫
**4 個 API 互相獨立，必須 `Promise.all` 並發**，不可順序呼叫（避免 TTFB > 4 秒）。

### 5.2 部分失敗策略
使用 `Promise.allSettled` 取代 `Promise.all`：
- 任一 API 失敗不應讓整頁 crash
- 每個 section 獨立呈現 `loading / loaded / empty / error` 四狀態
- 全部 4 個都失敗才顯示全頁 error（信任 footer 仍顯示）

### 5.3 快取 TTL（前端 localStorage）
| 端點 | TTL | 理由 |
|---|---|---|
| `/quote` | 30s | 即時報價，過 30s 即過時 |
| `/fundamentals` | 1 天 | 本地 JSON，每日無顯著變化 |
| `/chips` | 1 天 | TWSE T86 每日收盤後更新 |
| `/technical` | 5 分鐘 | ledger 本地，計算結果短期穩定 |

### 5.4 Symbol 查詢一致性
前端不需區分 4 個端點的 symbol 格式 — 全部送純數字，後端會處理（§0.2）。

---

## §6 已知限制與後續 PR

| 限制 | 影響 | 後續 PR 建議 |
|---|---|---|
| Technical 只有 7 個欄位，無歷史 bars | 無法繪製 sparkline | `feat/technical-history-bars` |
| SectorMedianPE 未暴露 | 無法做同產業 PE 對照 | `feat/sector-median-pe-endpoint` |
| data/fundamentals.json 只有 3 個欄位 | PS/Sector 無資料可顯示 | `chore/fundamentals-data-ps-sector` |
| FugleClient 條件性啟用 | quote API 可能 503 | `feat/quote-fallback-twse-openapi` |
| RSI 公式 bug（pre-existing） | rsi14 數值錯誤 | `fix/rsi-formula` |

---

## §7 變更紀律

任何對 stock API 的修改必須同步更新本文件對應章節：

| 變更 | 必須同步 |
|---|---|
| handler.go 任何 handler 改動 | §1/§2/§3/§4 + §0.2 |
| 新增欄位於 domain.Quote / FundamentalData / SymbolFlow | §1/§2/§3/§4 Schema 表 |
| 新增或修改 normalize helper | §0.2 + 新增章節 |
| 新增 endpoint | 新增 §N |
| 變更單位或資料源 | §對應 + §6 Known Limits |

| 版本 | 日期 | 變更 |
|---|---|---|
| v1.0 | 2026-07-09 | 初版（4 API typed schema + JWT + Symbol 格式陷阱） |
| v1.1 | 2026-07-09 | 新增 §0.2 normalizeFundamentalsSymbol 規範 + §4 修正 Technical 7 欄位（實際源碼驗證） |