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
- SDK init 前 TCP pre-flight check（`neoapi.fbs.com.tw:443`），不可達時回 503。
