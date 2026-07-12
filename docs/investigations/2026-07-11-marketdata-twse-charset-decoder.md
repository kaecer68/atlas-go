# TWSE / TAIFEX Charset 解碼與 Calendar API 棄用調查

> **文件角色**：數據源調查與 root-cause 分析紀錄。  
> **來源**：原記載於 `internal/marketdata/AGENTS.md`，因屬歷史 RCA 與調查性質，搬遷至此。  
> **權威狀態**： charset 規則仍適用；TWSE Calendar API 棄用狀態截至 2026-06-30。

---

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

---

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

---

## 已知未 refactor 的外部 source（follow-up）

- **Fugle、FinMind、Fubon、TEJ、Yahoo、BDI、frankfurter 等**：目前仍用 raw `json.Unmarshal` / `json.NewDecoder` 解析 HTTP body。這些 provider 目前回 UTF-8 故無 bug，但若日後發現 mojibake 應比照 TWSE / TAIFEX 套用 `DecodeJSON`。

測試覆蓋：`charset_decoder_test.go` 含 11 個 helper 測試（含 Big5 round-trip、未知 charset error、mojibake 防護），既有 8 個 provider 的 mock test 全部沿用 UTF-8 payload 故向後相容。

---

## 相關規則

- `internal/marketdata/AGENTS.md`：charset 規則與 provider 優先級速查。
- `internal/marketdata/charset_decoder.go`：`DecodeJSON` 實作。
- `internal/apigateway/CONSTITUTION.md`：數據源優先級與 fallback 規範。
