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
- **TWSE 與 TAIFEX JSON 解析**：一律使用 `charset_decoder.go::DecodeJSON`（禁止直接 `json.NewDecoder`/`json.Unmarshal` 外部 API body）。詳見 `docs/investigations/marketdata-twse-charset-decoder.md`。
- **Fallback 邏輯**：
    - `HybridProvider` 偵測到價格全為 0 (如 `Last=0`) 時視為無效數據，會觸發回退。
    - 總經指標若單一欄位缺失，`CompositeMacroProvider` 會略過該欄位而不影響整體合併。

---

## 陷阱提醒

- **TWSE OpenAPI 只提供批量接口**：`GetQuote` (單支) 實際上是抓取全市場數據後過濾，頻繁呼叫會極速消耗 Rate Limit。
- **Fugle 符號格式**：Fugle 盤中 API 符號通常為純數字 (如 `2330`)，不帶 `.TW`。
- **Yahoo Macro 符號映射**：美債 10 年期請使用 `^TNX`，匯率請確認 `USD/TWD` 的載入正確性。
- **Yahoo Provider 每日漲跌幅計算**：US 股票/指數 provider 必須使用 `range: "5d"` + `prev := closes[len(closes)-2]`（前一日收盤價），**禁止**使用 `range: "1y"` + `closes[0]`（會產出年增率而非日增率）。日漲跌幅超過 ±30% 應 reject（bounds cap）。詳見 PR #948。
- **ETF NAV 資料來源**：目前無任何 API channel 提供即時 ETF 淨值。詳細通道調查與數據流見 `docs/investigations/2026-05-29-etf-nav-data-source.md`。
- **providerBreaker 泛化熔斷器**：`circuit_breaker.go` 提供 `providerBreaker` + `newProviderBreaker(name, cfg)`。新增 provider 熔斷：(1) 構造 `providerBreaker`，(2) 註冊到 `HybridProvider.breakers` map，(3) 在 `GetQuotes` 對應位置呼叫 `shouldTry()` + `recordSuccess()` / `recordFailure()`。Fubon 與 Fugle 熔斷完全獨立。

---

## fubonproxy 連線位址

Fubon-proxy URL 由 `internal/fubonproxy` 統一提供（禁止自行構造 URL）；完整規則見 `internal/fubonproxy/AGENTS.md`。歷史 RCA 見 `docs/investigations/2026-06-fubonproxy-ipv4-uvloop.md`。

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
