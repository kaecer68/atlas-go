# AGENTS.md — internal/marketdata

本目錄負責台股行情與總體經濟指標的資料獲取與抽象化。

---

## OVERVIEW

`marketdata` 套件定義了資料提供者的介面，並實作多種適配器以對接外部 API。

- **核心介面**：
    - `Provider` (`provider.go`)：個股行情介面，要求實作 `GetQuotes`。
    - `MacroDataProvider` (`macro_provider.go`)：總經指標介面，要求實作 `FetchSnapshot`。
- **資料流**：
    `External API (Fugle/TWSE) → Client → Provider/Adapter → domain.Quote / MacroDataSnapshot`

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
