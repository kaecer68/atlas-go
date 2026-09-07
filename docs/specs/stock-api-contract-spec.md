# Atlas Stock API Contract（前端單一權威來源）

> **文件角色**：定義 `/api/stock/*` 5 個 endpoint 的 HTTP contract（路徑、查詢、回應、錯誤、單位、Source-of-truth），供 client_web 與 atlas-mcp 共用。
> **狀態**：v1.4（2026-08-06 新增 §1.5 Coverage Scope、§1.6 Coverage Endpoint、§1.7 變更助記）
> **Source-of-truth**：handler 源碼 `internal/stocktools/handler.go` + 各資料源 struct

---

## §0 前置約定（必須遵守）

### 0.1 認證

所有 `/api/stock/*` 端點目前在 `cmd/atlas/main.go isPublicPath` 中註冊為 **public path**，瀏覽器與 MCP client 呼叫時**不需要 API key / JWT**（與 `/api/dashboard/*`、`/api/macro/*` 同級）。
若未來加入 subscription gate，會另開 spec 更新本章節。

### 0.2 Symbol 格式約定

**API contract：所有 5 個 endpoint 接受純數字 symbol**（如 `?symbol=2330`）。

| 端點 | 內部處理 | 前端可送 |
| --- | --- | --- |
| `/api/stock/quote` | 直接傳給 FugleClient / TWSE OpenAPI（純數字） | `2330` |
| `/api/stock/fundamentals` | `normalizeFundamentalsSymbol()` 自動補 `.TW` | `2330` 或 `2330.TW` 都接受 |
| `/api/stock/chips` | 直接傳給 TWSE T86（純數字） | `2330` |

### 1.5 Coverage Scope（2026-08-06 v1.4 新增）

stocktools 4+1 個 endpoint 的涵蓋範圍如下：

| 端點 | 資料源 | Scope 範圍 |
| --- | --- | --- |
| `/api/stock/quote` | Fugle（必要時 fallback TWSE OpenAPI） | Fugle 涵蓋較廣（含上櫃、興櫃）；TWSE fallback 涵蓋上市 |
| `/api/stock/fundamentals` | `data/fundamentals.json` snapshot | 純 `.TW` 後綴，~1070 隻 TWSE 上市普通股 |
| `/api/stock/chips` | TWSE T86（`selectType=ALLBUT0999`，排除 99xx） | ~1231 隻上市普通股（不含 ETF/權證） |
| `/api/stock/technical` | ledger QuoteStore 或 Fugle on-demand | QuoteStore 涵蓋 + Fugle 補抓 |
| `/api/stock/volume_divergence` | 同 `/technical`（QuoteStore 或 Fugle on-demand，需 ≥20 根 bars） | QuoteStore 涵蓋 + Fugle 補抓 |

**不涵蓋之 symbol**（如上櫃、興櫃、ETF）仍可被呼叫，但 handler 會在每個 endpoint response 內附加結構化欄位：

| 欄位 | 型別 | 說明 |
| --- | --- | --- |
| `coverage_note` | string | 固定值 `"NOT_COVERED"`，作為 out-of-scope 的結構化標記 |
| `covered` | bool | `false` 表示 stocktools 取不到 chips/fundamentals/technical |
| `listing` | string | stable tag：`TWSE`（在範圍內）或 `UNKNOWN`（不在範圍內） |
| `quote_covered` | bool | `true` 表示 Fugle 仍能用，前端可決定是否 render quote header |
| `reason` | string | 人可讀中文說明（如「本系統 chips/fundamentals 涵蓋台灣上市普通股」） |

不涵蓋時 handler 仍回 HTTP 200 + 上述結構化欄位（**不是** 503 也不是全 0 數據）。理由：

1. 對 LLM agent / MCP caller / 前端而言，503 易被誤判為 server failure。
2. 200 + `coverage_note` 是結構化資料，前端可據此顯示徽章、CLI 可正常解析。
3. 既有「API 真實失敗」路徑（provider 未配置、上游 API 失敗等）保持 503 / 4xx 不變，不被 coverage guard 覆蓋。

### 1.6 Coverage Endpoint（前端預攔）

`GET /api/stock/coverage?symbol=X` 是專為前端預攔設計的獨立 endpoint：

- HTTP 200 always，即使 symbol 不存在或為空字串（空字串回 400）。
- Response body：`{symbol, covered, listing, quote_covered, reason}`。
- 與 4 個 stocktools endpoint 共用同一個 LookupCoverage 函式（`internal/stocktools/coverage.go`）— 單一 source of truth。

### 1.7 變更助記（v1.3 → v1.4 不相容點）

| 變更 | 影響 |
| --- | --- |
| 新增 `coverage_note` field | 既有 caller 應忽略未知 field，相容；新 caller 可用於分支 |
| 新增 `/api/stock/coverage` endpoint | 純增量，不影響既有 endpoint |
| 4 個 stocktools endpoint 對 out-of-scope symbol 改回 200 + `coverage_note`（舊：503 或全 0） | **不相容**：依賴「全 0 = 找不到」的 caller 須改用 `coverage_note` 判斷 |
| MCP tool description 補 coverage 提示 | 純文本變更，不影響協議 |
| Portfolio `FundamentalProvider` 新增 `HasSymbol(canonical) bool` method | 純增量，不影響既有 `Get` 等 |

關聯文件：`.omo/manifests/2026-08-06-stock-coverage-notice.md`（harness 私有，不入 repo）


| `/api/stock/sector-median-pe` | 以 `sector` 查詢 fundamentals JSON | `sector=semiconductor` |

**邊緣情況**（`normalizeFundamentalsSymbol`）：

- 空字串 → 回傳空字串（會被前置 400 擋下）
- 已是 `XXXX.TW` / `.US` / `.HK` / `.JP` / `.CN` suffix → 原樣回傳
- 其他格式（如 `2330.SH`）→ 原樣回傳（不靜默腐蝕非台股 symbol）

### 0.3 共用錯誤格式

所有錯誤回傳 `{"error": "<message>"}`：

| HTTP status | 情境 |
| --- | --- |
| 400 | 必要參數缺失或格式錯誤（symbol / sector / date） |
| 401/403 | 若未來啟用認證時觸發；目前 public path 不會返回 |
| 404 | quote 找不到 symbol（TWSE fallback 路徑） |
| 500/503 | Provider 未配置 / 資料未載入 / 上游 API 失敗 |

### 0.4 缺失資料語義（前端顯示準則）

| 表示方式 | 意義 | 前端處理 |
| --- | --- | --- |
| `null` | 該欄位未提供或無法計算 | 顯示「—」 |
| `0` | 數值型欄位的合法零值，或資料確實為零 / 未就緒 | **不可視為有效數值**；應顯示「—」並檢查 `data_status` / `error` |
| 欄位 omitted | 該指標不存在於本次快照（如 `MacroDataSnapshot` 中未成功的 channel） | 視為無資料 |
| HTTP 200 + 全零物件 | 資料檔存在但找不到該 symbol / sector（例如 fundamentals 無此股） | 顯示「—」或「未分類」 |
| HTTP 503 | 後端資料源未就緒 | 顯示 API 錯誤狀態，禁止以 `0` 渲染 |

---

## §1 `GET /api/stock/quote`

**Handler**：`internal/stocktools/handler.go::HandleQuote`  
**資料源**：`marketdata.FugleClient.GetQuote()` → `domain.Quote` struct（`internal/domain/shared/shared.go:27`），Fugle 失敗時 fallback `TWSEOpenAPIProvider.GetQuotes()`  
**條件性啟用**：無 `FUGLE_API_KEY` 且無 TWSE quote provider 時 → `503 quote provider not configured`

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

| 欄位 | 型別 | 單位 / 語義 |
| --- | --- | --- |
| `symbol` | string | 與 query 相同，純數字 |
| `last` | float64 | 最新成交價，台幣元 |
| `open` | float64 | 開盤價，台幣元 |
| `high` | float64 | 最高價，台幣元 |
| `low` | float64 | 最低價，台幣元 |
| `volume` | int64 | 成交量（股）；前端若需「張數」請除以 1000 |
| `market` | string | 市場別，如 `TSE` |
| `as_of` | RFC3339 | 報價時間（含時區） |
| `is_tradable` | bool | 是否可交易 |
| `source` | string | 資料來源，如 `fugle`、`twse_openapi` |

**缺失資料**：任一價格欄位若上游未提供可能為 `0`，前端應顯示「—」。無法取得任何報價時回傳 503/404，不會以 `last:0` 隱藏錯誤。

---

## §2 `GET /api/stock/fundamentals`

**Handler**：`internal/stocktools/handler.go::HandleFundamentals`  
**資料源**：`portfolio.FundamentalProvider.Get()` 從 `data/fundamentals.json`（本地 JSON，非即時）  
**條件性啟用**：無 `data/fundamentals.json` 檔案或檔案為空 → `503 fundamentals data not loaded`

**Query**：`symbol=<digits>`（必填；接受純數字，內部自動加 `.TW`）

**Response 200**（最多 5 個欄位）：

```json
{
  "PE": 25.3,
  "PB": 6.1,
  "PS": 0.0,
  "DividendYield": 1.5,
  "Sector": "semiconductor"
}
```

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `PE` | float64 | 本益比 |
| `PB` | float64 | 股價淨值比 |
| `PS` | float64 | 股價營收比 |
| `DividendYield` | float64 | 殖利率（%） |
| `Sector` | string | 產業分類 |

**缺失資料**：

- 找不到該 symbol 時回傳 200 + 全零物件 + `Sector: ""`；前端應顯示「—」/「未分類」。
- `data/fundamentals.json` 實際可能只有 `PE/PB/DividendYield`；`PS=0` 與 `Sector=""` 是正常零值，非錯誤。

**Sector enum**：`semiconductor / financials / electronics / shipping / energy / consumer / industrial / other`

---

## §3 `GET /api/stock/chips`

**Handler**：`internal/stocktools/handler.go::HandleChips`  
**資料源**：`marketdata.TWSECapitalFlowProvider.FetchSymbolFlow()` → `marketdata.SymbolFlow` struct

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

| 欄位 | 型別 | 單位 / 語義 |
| --- | --- | --- |
| `symbol` | string | 與 query 相同 |
| `name` | string | 股票名稱 |
| `foreign_investor_net` | float64 | 外資及陸資買賣超（張） |
| `domestic_fund_net` | float64 | 投信買賣超（張） |
| `dealer_net` | float64 | 自營商買賣超（張） |
| `date` | string | 資料日期 `YYYYMMDD` |

**單位**：`*_net` 已為「張」（`parseTWDVolume(...) / 1e3`），前端**不需再除 1000**。正值=買超，負值=賣超，`0`=持平。

**缺失資料**：7 天內找不到該 symbol 任何資料 → `503`（訊息為 provider error / context canceled）。不會回傳全零 `SymbolFlow`。

---

## §4 `GET /api/stock/technical`

**Handler**：`internal/stocktools/handler.go::HandleTechnical`  
**資料源**：`ledger.QuoteStore.LoadQuotes()` → `computeTechnical()`

**Query**：

- `symbol=<digits>`（必填）
- `days=<int>`（選填；預設 90，最大 365）

**Response 200**（7 個欄位）：

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

| 欄位 | 型別 | 單位 / 語義 |
| --- | --- | --- |
| `symbol` | string | 與 query 相同 |
| `date` | string | 最後一個交易日 `YYYY-MM-DD` |
| `close` | float64 | 最後收盤價，台幣元 |
| `volume` | int64 | 最後成交量，股 |
| `sma20` | float64 | 20 日簡單移動平均，台幣元 |
| `sma50` | float64 | 50 日簡單移動平均，台幣元 |
| `rsi14` | float64 | 14 日 RSI，0–100 |

**缺失資料**：

- 歷史 bars < 2 → `503 insufficient historical quote data`。
- 單一指標樣本不足時該欄位值為 `0`（例如資料長度介於 2–19 根時 `sma20=0`），前端應顯示「—」。


---

## §4b `GET /api/stock/volume_divergence`

**Handler**：`internal/stocktools/handler.go::HandleVolumeDivergence`（2026-09-07 新增）
**資料源**：`ledger.QuoteStore.LoadQuotes()` → `domain.DetectVolumeDivergence()`（純函數，PIT-safe；Fugle on-demand fallback 同 §4）

**Query**：

- `symbol=<digits>`（必填）
- `window=<int>`（選填；預設 30 交易日，最大 120）

**Response 200**：

```json
{
  "symbol": "2330.TW",
  "latest_date": "2026-09-04",
  "window_days": 30,
  "bars_used": 30,
  "close": 130.0,
  "window_high": 130.0,
  "window_low": 100.0,
  "close_below_high_pct": 0.0,
  "close_above_low_pct": 30.0,
  "vol_ma5": 12000.0,
  "vol_ma20": 21000.0,
  "volume_declining": true,
  "top_divergence": true,
  "bottom_divergence": false,
  "interpretation": "頂背離：股價接近近30日新高，但成交量遞減（5日均量低於20日均量），上漲動能可能衰竭，持有者宜提高警覺。",
  "trading_day": false
}
```

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `symbol` | string | QuoteStore 正規化後代碼（`.TW` 後綴） |
| `latest_date` | string | 最後一根 bar 日期 `YYYY-MM-DD` |
| `window_days` | int | 實際使用的回看視窗（交易日） |
| `bars_used` | int | 實際分析的 bar 數（≤ window_days） |
| `close` | float64 | 最新收盤價 |
| `window_high` / `window_low` | float64 | 視窗內收盤價最高 / 最低 |
| `close_below_high_pct` | float64 | 收盤距視窗高點的幅度（%） |
| `close_above_low_pct` | float64 | 收盤距視窗低點的幅度（%） |
| `vol_ma5` / `vol_ma20` | float64 | 5 日 / 20 日平均成交量（股） |
| `volume_declining` | bool | `vol_ma5 < vol_ma20`（背離共同前提） |
| `top_divergence` | bool | 頂背離：收盤在視窗高點 1% 以內且量能遞減 |
| `bottom_divergence` | bool | 底背離：收盤在視窗低點 1% 以內且量能遞減 |
| `interpretation` | string | zh-TW 判讀摘要 |
| `trading_day` | bool（omitempty） | 僅在非交易日出現且為 `false`（同 §4 Phase C 標記） |

**缺失資料**：

- 歷史 bars < 20（`domain.DivergenceVolLongWindow`）→ `503 insufficient historical quote data`（明確訊息含實際 bar 數）。
- 價格面板退化（視窗高低點相同，例如長期停牌）→ `503 insufficient or degenerate price/volume panel`。
- 零成交量面板 → 200 但 `volume_declining=false`（無法判讀量縮）。

---

## §4c `GET /api/stock/condition_winrate`

**Handler**：`internal/stocktools/handler.go::HandleConditionWinRate`（2026-09-07, issue #1865）
**資料源**：stockpicker SQLite ledger `stock_signal_outcomes`（read-only，即時聚合，不重算回測）

**Query**：

- `condition_id=<id>`（必填；foreign-3d-net-buy / momentum-20d-positive / price-volume-top-divergence / price-volume-bottom-divergence）
- `rolling_window=<label>`（選填；預設 120d）

**Response 200**：`found` + 條件級聚合（跨股票）：`condition_id`、`source`、`direction`（buy/avoid — avoid=反向語義，低勝率=訊號有效）、`observations`、`symbols`、`hits`、`win_rate`、`wilson_lower/upper`、`calibration_status`、`avg_forward_return`、`data_start/end`。無資料 → 200 + `found:false` + `message`。

---
## §5 `GET /api/stock/sector-median-pe`

**Handler**：`internal/stocktools/handler.go::HandleSectorMedianPE`  
**資料源**：`portfolio.FundamentalProvider.SectorMedianPE()` 從 `data/fundamentals.json`

**Query**：`sector=<string>`（必填，見 §2 Sector enum）

**Response 200**（2 個欄位）：

```json
{
  "sector": "semiconductor",
  "median_pe": 22.4
}
```

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `sector` | string | 與 query 相同 |
| `median_pe` | float64 | 該產業所有有效 PE 的中位數；無資料時為 `0` |

**缺失資料**：該產業無有效 PE 時回傳 200 + `median_pe: 0`，前端應顯示「—」。

---

## §6 跨 API 約定（前端 client 設計約束）

### 6.1 並發呼叫

5 個 API 互相獨立，建議 `Promise.allSettled` 並發呼叫；任一失敗不應讓整頁 crash。

### 6.2 部分失敗策略

- 每個 section 獨立呈現 `loading / loaded / empty / error` 四狀態。
- 全部失敗才顯示全頁 error。

### 6.3 快取 TTL（前端 localStorage）

| 端點 | TTL | 理由 |
| --- | --- | --- |
| `/quote` | 30s | 即時報價 |
| `/fundamentals` | 1 天 | 本地 JSON，日變化小 |
| `/chips` | 1 天 | TWSE T86 每日收盤後更新 |
| `/technical` | 5 分鐘 | ledger 本地計算結果短期穩定 |
| `/volume_divergence` | 5 分鐘 | 與 technical 同資料源（日線 bars） |
| `/sector-median-pe` | 1 天 | 與 fundamentals 同資料源 |

### 6.4 Symbol 查詢一致性

前端全部送純數字；`sector-median-pe` 使用 §2 的 sector enum。

---

## §7 已知限制與後續 PR

| 限制 | 影響 | 後續 PR 建議 |
| --- | --- | --- |
| `/technical` 只有單點 KPI，無歷史 bars | 無法繪製 sparkline | `feat/technical-history-bars` |
| `data/fundamentals.json` 可能缺少 `PS/Sector` | PS/Sector 無資料可顯示 | `chore/fundamentals-data-ps-sector` |
| FugleClient 條件性啟用 | quote API 可能 503 | `feat/quote-fallback-twse-openapi` |
| RSI 公式為簡化版 | rsi14 數值僅供參考 | `fix/rsi-formula` |

---

## §8 變更紀律

| 變更 | 必須同步 |
| --- | --- |
| handler.go 任何 handler 改動 | §1–§5 + §0.2 |
| 新增欄位於 `domain.Quote` / `FundamentalData` / `SymbolFlow` | 對應 Schema 表 |
| 新增或修改 normalize helper | §0.2 |
| 新增 endpoint | 新增 §N |
| 變更單位或資料源 | §對應 + §7 |

| 版本 | 日期 | 變更 |
| --- | --- | --- |
| v1.0 | 2026-07-09 | 初版（4 API typed schema + Symbol 格式陷阱） |
| v1.1 | 2026-07-09 | 新增 §0.2 normalizeFundamentalsSymbol 規範 |
| v1.2 | 2026-07-09 | 恢復 wireframe 關聯連結 |
| v1.3 | 2026-07-12 | P2-3：新增 §5 sector-median-pe、§0.4 缺失資料語義、§0.1 認證說明修正 |
