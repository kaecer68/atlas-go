# AGENTS.md — internal/marketdata

本目錄負責台股行情與總體經濟指標的資料獲取與抽象化。

---

## OVERVIEW

`marketdata` 套件定義了資料提供者的介面，並實作多種適配器以對接外部 API。

- **核心介面**：
    - `Provider` (`provider.go`)：個股行情介面，要求實作 `GetQuotes`。
    - `MacroDataProvider` (`macro_provider.go`)：總經指標介面，要求實作 `FetchSnapshot`。
    - `CorporateActionProvider` (`corporate_action_provider.go`)：法人事件介面（P1-2-α 引入），要求實作 `GetCorporateActions(ctx, symbol, start, end)`。
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
| `YahooMacroProvider` | 透過 Yahoo Finance 獲取美債、DXY、VIX 等指標。 | |
| `CompositeMacroProvider` | 組合多個總經提供者的數據快照。 | 採 Last-write-wins 合併策略。 |
| `BDIProvider` | 透過 CNBC JSON API 獲取波羅的海乾散貨指數 (`.BADI`) | 5s rate limit，回退至前一快照值 |

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
- **ETF NAV 資料來源**：目前無任何 API channel 提供即時 ETF 淨值。`TWSEETFNAVScraper` 使用分層策略：Tier 1 (TWSE scrape) 為 stub，Tier 2 (收盤價代理) 為唯一可用路徑。台股 ETF 追蹤誤差通常 <0.5%。

### ETF NAV 數據源調查 (2026-05-29)

五個優先級通道的 ETF NAV 可用性：

| 優先級 | 通道 | ETF NAV？ | 原因 |
|--------|------|----------|------|
| 1 | 富邦證券 | ❌ | fubon-neo SDK 僅提供 intraday.quote() OHLCV。proxy 4 個端點皆為即時報價，無 fund/NAV API。 |
| 2 | TWSE OpenAPI | ❌ | ETFReport/ETFNAV → 302 HTML。BFIBMS → redirect。getETFNetValue.jsp → HTML。無免費 REST API。 |
| 3 | Fugle | ❌ | fugle_client.go 僅提供即時報價 + meta。無 NAV。 |
| 4 | TEJ | ❌ | tej_provider.go 僅實作 TRAIL/TAPRCD (股價) 和 TWN/AFINA (財報)。無 ETF NAV dataset。 |
| 5 | FinMind | ⚠️ 待付費 | TaiwanStockETF dataset 存在於 FinMind catalog 中，但未實作。需付費 token (每 7 天換一次)。 |

### FinMind 迭代計劃

當 FinMind 付費註冊完成後，按以下步驟接入真實 ETF NAV：

1. `finmind_client.go` — 新增 `GetETFNAV(ctx, symbol, date)`，呼叫 `TaiwanStockETF` dataset
2. `etf_nav_scraper.go` — 實作 `attemptFinMindFetch()`，新增 `SourceFinMind` enum
3. `FetchNAV()` — 更新優先級：FinMind → close-price proxy
4. 無需修改 `ETFNAVFetcher` 介面或 `ETFAnalyzer`——scraper 內部升級即可

## ETF NAV 數據流

```
啟動時 (system.go):
  replay.Dataset.QuotesForDate(latestDate, ["0050","0056",...])
    → domain.Quote{Close}
      → ETFAnalyzer.UpdateNAVFromQuotes(quotes)
        → metadata[symbol].NAV = quote.Last

運行時:
  ETFNAVProvider.FetchNAV(symbol) → QuoteFetcher.GetQuotes() → 市價收盤價
  或
  ETFAnalyzer.RefreshNAVFromFetcher(ctx, fetcher)
```

**已知限制**：replay data 目前僅包含 0050.TW，其餘 10 檔 ETF 需擴展 replay data 生成範圍後才能校準 NAV。FinMind `TaiwanStockETF` dataset 為長期方案，待付費註冊後實施（見上方迭代計劃）。

**數據源優先級**：富邦證券 → TWSE OpenAPI → Fugle → TEJ → FinMind（遵循 `internal/apigateway/CONSTITUTION.md` 規範）。
