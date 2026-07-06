# AGENTS.md — internal/marketdata

本目錄負責台股行情與總體經濟指標的資料獲取與抽象化。

---

## OVERVIEW

`marketdata` 套件定義了資料提供者的介面，並實作多種適配器以對接外部 API。

- **核心介面**：
    - `Provider` (`provider.go`)：個股行情介面，要求實作 `GetQuotes`。
    - `MacroDataProvider` (`macro_provider.go`)：總經指標介面，要求實作 `FetchSnapshot`。
    - `CorporateActionProvider` (`corporate_action_provider.go`)：法人事件介面，要求實作 `GetCorporateActions(ctx, symbol, start, end)`。
- **資料流**：
    `External API (Fugle/TWSE) → Client → Provider/Adapter → domain.Quote / MacroDataSnapshot`

### CorporateAction 整合層

`AggregatedCorporateActionProvider`（`aggregated_corporate_action_provider.go`）是 `CorporateActionProvider` 的首選實作：

- 優先級：TWSE（包含除權息參考價）為主，FinMind 為備援
- Dedup：同一 `(symbol, ex_date)` 事件以 TWSE 為準；若 TWSE 缺漏該筆欄位但 FinMind 有，會回填 `CashDividend` / `StockDividend`
- Partial failure：兩 provider 中任一失敗仍要回傳另一個 provider 的結果，**不要**因為一邊失敗就 return error；只有「兩邊都失敗」才回傳 error
- 輸出排序：`ExDate` 升冪

---

## PROVIDERS

| 提供者 | 職責 | 備註 |
|------|------|------|
| `FugleProvider` | 透過 富果 Fugle Realtime API 獲取盤中即時行情。 | 需 API Key，限制 60 筆/分。 |
| `TWSEOpenAPIProvider` | 使用 證交所 OpenAPI 獲取當日行情。 | 每日更新，無需 Key，有速率限制。 |
| `HybridProvider` | 優先使用 Fugle，失敗時自動回退至 TWSE。 | 系統預設建議路徑。 |
| `TWSECapitalFlowProvider` | 獲取三大法人買賣超數據。 | 爬取 T86 報表。 |
| `YahooMacroProvider` | 透過 Yahoo Finance 獲取美債、DXY、VIX 等指標。 | 使用 range=5d + closes[len-2] 計算 daily change（非 YoY） |
| `TSMADRProvider` | 台積電 ADR（TSM）每日漲跌幅。 | range=5d，±30% bounds cap |
| `NVDAProvider` / `AAPLProvider` / `MSFTProvider` | US 科技股每日漲跌幅（`us_tech_provider.go`）。 | 共用 `fetchUSTechSnapshot` helper |
| `SPXIndexProvider` / `NDXIndexProvider` / `DJIIndexProvider` | US 指數每日漲跌幅（`us_index_provider.go`）。 | 共用 `fetchUSIndexSnapshot` helper |
| `SOXIndexProvider` | 費城半導體指數每日漲跌幅。 | range=5d，±30% bounds cap |
| `CompositeMacroProvider` | 組合多個總經提供者的數據快照。 | 採 Last-write-wins 合併策略。 |
| `BDIProvider` | 透過 CNBC JSON API 獲取波羅的海乾散貨指數 (`.BADI`) | 5s rate limit，回退至前一快照值 |
| `TaiwanVolatilityProvider` | TAIEX (^TWII) 20 日歷史波動率。 | range=3mo (≥21 bars)，年化波動率 = σ(log_returns_20d) × √252。僅在 `cfg.YahooEnabled=true` 時註冊。寫入 `MacroDataSnapshot.HistoricalVolatility`。 |

---

## CONVENTIONS

- **Rate Limiting**：所有對外請求必須使用 `golang.org/x/time/rate` 進行客戶端限流。
- **Error Context**：API 失敗時一律包含 HTTP 狀態碼與端點資訊：`fmt.Errorf("api error: status %d", resp.StatusCode)`。
- **Timezone**：台股資料解析時一律對齊 `CST` (UTC+8)。
- **Mocking**：單元測試請優先使用 `MockProvider` 或 `miniredis` (若涉及快取)。
- **Fallback 邏輯**：
    - `HybridProvider` 偵測到價格全為 0 (如 `Last=0`) 時視為無效數據，會觸發回退。
    - 總經指標若單一欄位缺失，`CompositeMacroProvider` 會略過該欄位而不影響整體合併。

---

## 陷阱提醒

- **TWSE OpenAPI 只提供批量接口**：`GetQuote` (單支) 實際上是抓取全市場數據後過濾，頻繁呼叫會極速消耗 Rate Limit。
- **Fugle 符號格式**：Fugle 盤中 API 符號通常為純數字 (如 `2330`)，不帶 `.TW`。
- **Yahoo Macro 符號映射**：美債 10 年期請使用 `^TNX`，匯率請確認 `USD/TWD` 的載入正確性。
- **Yahoo Provider 每日漲跌幅計算**：US 股票/指數 provider 必須使用 `range: "5d"` + `prev := closes[len(closes)-2]`（前一日收盤價），**禁止**使用 `range: "1y"` + `closes[0]`（會產出年增率而非日增率）。日漲跌幅超過 ±30% 應 reject（bounds cap）。詳見 PR #948。
- **ETF NAV 資料來源**：目前無任何 API channel 提供即時 ETF 淨值。`TWSEETFNAVScraper` 使用分層策略：Tier 1 (TWSE scrape) 為 stub，Tier 2 (收盤價代理) 為唯一可用路徑。台股 ETF 追蹤誤差通常 <0.5%。詳細通道調查見 `docs/investigations/2026-05-29-etf-nav-data-source.md`；待 FinMind 付費註冊後的接入計劃（未實作、工作區限定）見 `.omo/plans/2026-05-29-etf-nav-finmind.md`。
- **providerBreaker 泛化熔斷器**：`circuit_breaker.go` 提供 `providerBreaker` + `newProviderBreaker(name, cfg)`。新增 provider 熔斷：(1) 構造 `providerBreaker`，(2) 註冊到 `HybridProvider.breakers` map，(3) 在 `GetQuotes` 對應位置呼叫 `shouldTry()` + `recordSuccess()` / `recordFailure()`。Fubon 與 Fugle 熔斷完全獨立。

## ETF NAV 數據流

啟動時：`replay.Dataset.QuotesForDate(latestDate, ...)` → `domain.Quote{Close}` → `ETFAnalyzer.UpdateNAVFromQuotes(quotes)` → `metadata[symbol].NAV = quote.Last`。
運行時：`ETFNAVProvider.FetchNAV(symbol) → QuoteFetcher.GetQuotes() → 市價收盤價` 或 `ETFAnalyzer.RefreshNAVFromFetcher(ctx, fetcher)`。

**已知限制**：replay data 目前僅包含 0050.TW，其餘 10 檔 ETF 需擴展 replay data 生成範圍後才能校準 NAV。FinMind `TaiwanStockETF` dataset 為長期方案，待付費註冊後實施。

**數據源優先級**：富邦證券 → TWSE OpenAPI → Fugle → TEJ → FinMind（遵循 `internal/apigateway/CONSTITUTION.md` 規範）。

## TWSE Calendar API deprecation（2026-06-30）

TWSE 已 **整段 deprecate** calendar 相關 endpoint：

| Endpoint | 狀態 |
|---------|------|
| `https://www.twse.com.tw/rwd/zh/exRight?...` | 302 → `/rwd/` |
| `https://www.twse.com.tw/rwd/zh/meeting?...` | 302 → `/page-not-found.html` |
| `https://www.twse.com.tw/exchangeReport/TWTBA?...` | 307 → `/page-not-found.html` |
| `https://www.twse.com.tw/exchangeReport/TWTB9U?...` | 307 → `/page-not-found.html` |
| `https://openapi.twse.com.tw/v1/exchangeReport/TWTBA` | 302 → `/openapi.twse.com.tw/404.html` |
| `https://openapi.twse.com.tw/v1/exchangeReport/TWTB9U` | 302 → `/openapi.twse.com.tw/404.html` |
| `https://www.twse.com.tw/rwd/zh/calendar/{exRight,meeting}?...` | 302 → `/page-not-found.html` |

所有 endpoint 都回 HTML body（不是 JSON），導致 `json.NewDecoder` / `DecodeJSON` 解碼失敗 → dashboard 顯示 `'æ'` mojibake-shaped error。

### 優雅降級處理（PR #842+）

`twse_calendar_provider.go::fetchExDividendMonth` 與 `fetchMeetingMonth` 在拿到 `Content-Type: text/html` 時：
- log `warn level` `endpoint_html_response_deprecated`（含 endpoint + date + content_type）
- 回 `(nil, nil)` 給下游（empty events, no error）
- 不傳播 JSON decode error，避免 dashboard 出現 hard failure

下游（`AggregatedCorporateActionProvider`、`industry.EventCalendar`、`monitoring/dashboard_api.go` 等）本來就處理空 events 場景，行為完全向後相容。

### 復原策略

若 TWSE 重新提供 calendar endpoint：
1. 移除 `isHTMLContentType` 偵測區塊
2. 確認 TWSE 確實回 JSON 而非 HTML
3. 重跑 `TestTWSECalendarProvider_Fetch*Month` 既有 tests（用真實 JSON fixture）

**不要** disable `twse_calendar_provider.go`（10+ downstream callers 依賴），也不要替換成其他 source（目前無替代 data source）。

## TWSE Charset 解碼

TWSE 部分 endpoint（monthly statistics、除權息日曆、股東會日曆、MI_INDEX 等）會以 **Big5** 或 **GB2312** 而非 UTF-8 編碼回應 JSON payload，違反 RFC 8259 §8.1 的 UTF-8 強制規範。若直接用 `json.NewDecoder` / `json.Unmarshal` 解析，中文欄位會出現 `'æ'` 風格的 mojibake 或直接 decode failure。TAIFEX（台灣期貨交易所）為同類風險，亦已 refactor。

### 統一入口：`charset_decoder.go::DecodeJSON`

**TWSE 與 TAIFEX provider 解析 HTTP JSON 回應時，一律**透過 `DecodeJSON(body io.Reader, contentType string, dst any) error` 解析。**禁止**在 TWSE / TAIFEX 檔案內直接呼叫 `json.NewDecoder` 或 `json.Unmarshal` 處理外部 API body。

```go
// ✅ 正確（charset-aware）
var apiResp twseCalendarResponse
if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &apiResp); err != nil {
    return nil, fmt.Errorf("decode response: %w", err)
}

// ❌ 錯誤（會 mojibake）
var apiResp twseCalendarResponse
if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil { ... }
```

`DecodeJSON` 行為：
- 解析 `Content-Type` 的 `charset=` 參數
- UTF-8 / ASCII / 缺省 → 直接 `json.NewDecoder` 解析（零開銷）
- 其他 charset（Big5、GB2312、Shift_JIS 等）→ `golang.org/x/text/encoding/htmlindex` 查表 + `transform.NewReader` 串流轉碼後解析
- 未知 charset → 回 error（含 charset 名稱 + 原始 Content-Type）

### 已 refactor 的 call site（2026-06-30）

| 檔案 | 函式 | 模式 |
|------|------|------|
| `twse_openapi.go` | `GetQuotes`, `GetDailyQuote` | bytes-read（PR #917, 2026-07-03） |
| `twse_calendar_provider.go` | `fetchExDividendMonth`, `fetchMeetingMonth` | streaming |
| `twse_sector_index_provider.go` | `fetchSingleDay` | streaming（line 246 cache file read 跳過） |
| `twse_capital_flow_provider.go` | `fetchDate` | bytes-read（`io.ReadAll` + `bytes.NewReader`） |
| `twse_margin_provider.go` | `fetchDateExpanded` | bytes-read |
| `twse_oddlot_provider.go` | `fetchDate` | bytes-read |
| `twse_etf_provider.go` | `fetchDate` | bytes-read |
| `taifex_provider.go` | `FetchPCR`, `FetchRetailFuturesOI`, `FetchFutures` | bytes-read |

**注意**：cache file 讀取（`twse_sector_index_provider.go:246` 的 `loadFromCache`）**不要**用 `DecodeJSON` — 那是我們自己寫入的 UTF-8 cache，繼續用 `json.Unmarshal`。

### 已知未 refactor 的外部 source（follow-up）

- **Fugle、FinMind、Fubon、TEJ、Yahoo、BDI、frankfurter 等**：目前仍用 raw `json.Unmarshal` / `json.NewDecoder` 解析 HTTP body。這些 provider 目前回 UTF-8 故無 bug，但若日後發現 mojibake 應比照 TWSE / TAIFEX 套用 `DecodeJSON`。

測試覆蓋：`charset_decoder_test.go` 含 11 個 helper 測試（含 Big5 round-trip、未知 charset error、mojibake 防護），既有 8 個 provider 的 mock test 全部沿用 UTF-8 payload 故向後相容。

## fubonproxy 連線位址

Fubon-proxy URL 由 `internal/fubonproxy/manager.go` 的 `ProxyBaseURL()` / `ProxyHostPort()` 統一提供 — **禁止其他 .go 檔案以 `fmt.Sprintf("http://...:%d", ...)` 自行構造**，`fubon_url_guard_test.go::TestFubon_URLDriftGuard` AST 禁制會擋下。

- **host**：`host.docker.internal`（macOS / Windows Docker Desktop 自動注入；Linux 容器需 `daemon.json` 設 `extra_hosts`）。**不是 `127.0.0.1`** — 從 container 端用 `127.0.0.1` 會 hit container 自身 loopback，而非 host Python proxy。
- **port**：`18081` 預設，由 `cmd/atlas -fubon-port` flag 動態覆寫（同步注入 `fubonproxy.NewManager()`，確保 client URL 與 supervisor health URL 同源 — PR 2 Oracle F12）。
- **環境變數**：不再支援 `FUBON_PROXY_URL` 覆寫（PR #572 移除）。

歷史 RCA（PR #495 uvicorn/uvloop `IPV6_V6ONLY` 問題）見 `docs/investigations/2026-06-fubonproxy-ipv4-uvloop.md`。

**PR #837 follow-up**：原本 3 個 source files（`fubon_client.go`、`hybrid_provider.go`、`register_adapters.go`）各自硬編碼 `host.docker.internal:18081`，其中 `hybrid_provider.go` 完全忽略 `-fubon-port` flag → port drift → channel recurring failure。重構後統一從 `fubonproxy` 取得，並以 `TestFubon_URLDriftGuard` AST 禁制防止復發。

---

## FubonClient 韌性機制 (PR #943)

### Health Client 隔離
- `healthClient`：2s timeout，專用於 `HealthCheck()`／健康探測
- `httpClient`：`FubonAPITimeoutSec` timeout，用於資料請求
- `SetHealthClient()`：測試注入

### 背景健康探測
- `GetSharedFubonClient()` 自動啟動 15s 間隔 goroutine
- 連續 3 次失敗 → `IsHealthy() = false` → `Fetch()` fast-fail
- `ResetSharedFubonClient()` 自動停止探測

### TCP Pre-flight Check (Layer 3)
- fubon-proxy 在 SDK init 前用 `socket.connect(neoapi.fbs.com.tw:443, timeout=5s)` 做 pre-flight
- 不可達時直接回 503，不呼叫 C extension（C extension 在 macOS 上無法被 Python timeout 中斷）
