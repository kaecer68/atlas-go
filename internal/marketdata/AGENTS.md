# AGENTS.md — internal/marketdata

台股行情與總經指標的資料獲取與抽象化。

## OVERVIEW

- `Provider`：個股行情介面（`provider.go`）。
- `MacroDataProvider`：總經指標介面（`macro_provider.go`）。
- `CorporateActionProvider`：法人事件介面（`corporate_action_provider.go`）。
- `AggregatedCorporateActionProvider`：TWSE 優先、FinMind 備援；單一失敗仍回傳另一方結果；輸出按 `ExDate` 升冪。

## PROVIDERS

| 提供者 | 職責 |
|------|------|
| `FugleProvider` | 盤中即時行情 |
| `TWSEOpenAPIProvider` | 當日行情 |
| `HybridProvider` | Fugle 優先、TWSE 回退 |
| `TWSECapitalFlowProvider` | 三大法人買賣超 |
| `YahooMacroProvider` | 美債、DXY、VIX |
| `TSMADRProvider` / `NVDAProvider` / `AAPLProvider` / `MSFTProvider` | US 科技股/ADR |
| `SPXIndexProvider` / `NDXIndexProvider` / `DJIIndexProvider` / `SOXIndexProvider` | US 指數 |
| `CompositeMacroProvider` | 多總經來源合併 |
| `BDIProvider` | 波羅的海乾散貨指數 |
| `TaiwanVolatilityProvider` | TAIEX 20 日歷史波動率 |

> 詳細通道規則見 `internal/apigateway/CONSTITUTION.md`。

## CONVENTIONS

- 對外請求使用 `golang.org/x/time/rate` 客戶端限流。
- 錯誤包含 HTTP 狀態碼：`fmt.Errorf("api error: status %d", resp.StatusCode)`。
- 台股資料時區對齊 `CST` (UTC+8)。
- `HybridProvider` 價格全為 0 時視為無效，觸發回退。
- `CompositeMacroProvider` 單一欄位缺失時略過該欄位。

## 陷阱提醒

- **TWSE OpenAPI 只提供批量接口**：`GetQuote` 實際抓取全市場後過濾，頻繁呼叫極速消耗 rate limit。
- **Fugle 符號格式**：盤中 API 符號為純數字（如 `2330`），不帶 `.TW`。
- **Yahoo 日漲跌幅**：必須 `range:"5d"` + `closes[len-2]`；**禁止** `range:"1y"` + `closes[0]`；超 ±30% reject。
- **TWSE/TAIFEX JSON**：一律用 `charset_decoder.go::DecodeJSON(body, contentType, dst)` 處理 charset；**禁止**直接 `json.NewDecoder`/`json.Unmarshal`。
- **fubon-proxy URL**：由 `internal/fubonproxy.ProxyBaseURL()` / `ProxyHostPort()` 統一提供；host 用 `fubon-proxy` 或 `127.0.0.1`，**不用** `localhost`。
- **新增 provider 熔斷**：構造 `providerBreaker` → 註冊到 `HybridProvider.breakers` → `GetQuotes` 內呼叫 `shouldTry()` / `recordSuccess()` / `recordFailure()`。

## FubonClient 韌性機制

- `healthClient`（2s timeout）專用健康探測；`httpClient`（`FubonAPITimeoutSec`）專用資料請求。
- 15s 背景探測；連續 3 次失敗 → `IsHealthy()=false` → `Fetch()` fast-fail。
- **P2-17 request-failure breaker**：`providerBreaker` 計數**實際 request 失敗**（transport / 非 2xx / decode），
  3 連敗 → breaker open + `IsHealthy()=false`（不再等 15s 探測才發現 proxy 掛）；成功（request 或 probe）即復位。
  Open 期間 `GetQuote`/`GetQuotes`/`CheckMarketStatus` fast-fail（不碰網路）。不影響 hybrid fallback 鏈。
- SDK init 前 TCP pre-flight check（`neoapi.fbs.com.tw:443`），不可達時回 503。

## Response schema fingerprints（P2-15）

- 共用元件：`schema_fingerprint.go` — `responseFingerprint`（必要欄位 + JSON 型別）+ `warnFingerprint()`（變更即 warn）。
- 已接線：FinMind（envelope + dataset 必要欄位）、Yahoo `UnmarshalYahooChart`
  （`indicators.quote` 空 → typed `ErrSchema` + warn，防 4/5 消費層 panic）、TWSE `STOCK_DAY_ALL`
  （fields header / row 寬度 warn）、TAIFEX `FetchPCR`（raw row keys warn）。
- 語意：fingerprint 是 **warn-only 早期偵測**；硬 gate（typed ErrSchema、breaker trip）決定 fetch 是否失敗。

## 海關出口 CSV（P2-19）

- `fetchLatestTwoMonths` 改用共用 `fetchWithRetry`（429/5xx retry）。
- `parseCustomsCSV` 改 **header-driven 欄位對映**：必要欄位 `年度`/`月份`/`出口總值(新臺幣千元)`/
  `進口總值(新臺幣千元)`/`出入超(新臺幣千元)` 缺一 → typed `ErrSchema`。修正舊 fixed-index 把 row[3]
  （= 出口）誤當進口總值的潛在 bug。
