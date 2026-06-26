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
- **ETF NAV 資料來源**：目前無任何 API channel 提供即時 ETF 淨值。`TWSEETFNAVScraper` 使用分層策略：Tier 1 (TWSE scrape) 為 stub，Tier 2 (收盤價代理) 為唯一可用路徑。台股 ETF 追蹤誤差通常 <0.5%。詳細通道調查見 `docs/investigations/2026-05-29-etf-nav-data-source.md`；待 FinMind 付費註冊後的接入計劃（未實作、工作區限定）見 `.omo/plans/2026-05-29-etf-nav-finmind.md`。
- **providerBreaker 泛化熔斷器（2026-06 重構）**：`internal/marketdata/circuit_breaker.go` 提供 `providerBreaker` struct + `newProviderBreaker(name, cfg)` 構造。新增 provider 熔斷只需：(1) 構造一個 `providerBreaker`，(2) 註冊到 `HybridProvider.breakers` map，(3) 在 `GetQuotes` 對應位置呼叫 `shouldTry()` + `recordSuccess()` / `recordFailure()`。Fubon 與 Fugle 熔斷完全獨立，不互相影響。

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

**已知限制**：replay data 目前僅包含 0050.TW，其餘 10 檔 ETF 需擴展 replay data 生成範圍後才能校準 NAV。FinMind `TaiwanStockETF` dataset 為長期方案，待付費註冊後實施。

**數據源優先級**：富邦證券 → TWSE OpenAPI → Fugle → TEJ → FinMind（遵循 `internal/apigateway/CONSTITUTION.md` 規範）。

## fubonproxy 連線位址

`FubonClient` 與 `HybridProvider` 預設使用 IPv4 `127.0.0.1:8081` 而非 `localhost:8081`；Python proxy 綁定固定 IPv4 `host="0.0.0.0"`。proxy 位址固定為 `127.0.0.1:8081`，不再支援環境變數覆寫（`FUBON_PROXY_URL` 已於 PR #572 移除）。若需 Docker/遠端部署支援，請參閱 `configs/parameters.json`。歷史 RCA（PR #495 uvicorn/uvloop `IPV6_V6ONLY` 問題）見 `docs/investigations/2026-06-fubonproxy-ipv4-uvloop.md`。
