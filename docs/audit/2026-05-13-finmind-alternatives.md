# FinMind 替代方案調查報告

> **日期**: 2026-05-13
> **目的**: FinMind API 免費額度用盡（HTTP 402），需替換 TaiwanStockMonthRevenue（台積電月營收）之數據源
> **受影響檔案**: `internal/marketdata/tsmc_revenue_provider.go`、`internal/marketdata/finmind_client.go`

---

## 現狀分析

### 目前呼叫鏈

```
TSMCRevenueProvider.FetchSnapshot()
  └─ fetchWithFallback()
       ├─ client.GetMonthRevenue("2330", year, month)   ← FinMind API
       └─ client.GetMonthRevenue("2330", year-1, month)  ← FinMind API
  └─ loadLatestSnapshot() (fallback: 從本地 JSON 檔載入上次成功爬取的數據)
```

### FinMind 限制

| 項目 | 值 |
|------|-----|
| 錯誤碼 | HTTP 402 |
| 錯誤訊息 | "Requests reach the upper limit" |
| 免費額度 | 600 req/hr |
| Key 輪替 | 每 7 天需更換一次 |
| 替代可行性 | 可付費升級 Backer/Sponsor，但不建議依賴 |

---

## 方案 1: TWSE OpenAPI `/opendata/t187ap05_L` ✅ **推薦方案**

### 可行性: ✅ 完全可行

TWSE 官方 OpenAPI 提供**上市公司每月營業收入彙總表**，無需 API Key，無速率限制（合理使用下）。

### 端點資訊

| 項目 | 值 |
|------|-----|
| **端點 URL** | `https://openapi.twse.com.tw/v1/opendata/t187ap05_L` |
| **文件** | `https://openapi.twse.com.tw/v1/swagger.json` |
| **方法** | `GET` |
| **認證** | 無（完全免費開放） |
| **回傳格式** | JSON Array（全部上市公司，無分頁） |
| **更新頻率** | 每日更新（次月 10 日前公告當月營收） |
| **法律風險** | ✅ 無 — 政府開放資料，授權條款 `政府資料開放授權條款-第1版` |

### 回傳範例（2330 台積電, 2026年4月）

```json
{
  "出表日期": "1150512",
  "資料年月": "11504",
  "公司代號": "2330",
  "公司名稱": "台積電",
  "產業別": "半導體業",
  "營業收入-當月營收": "410725118",
  "營業收入-上月營收": "415191699",
  "營業收入-去年當月營收": "349566940",
  "營業收入-上月比較增減(%)": "-1.075787644781405",
  "營業收入-去年同月增減(%)": "17.495412466636576",
  "累計營業收入-當月累計營收": "1544828558",
  "累計營業收入-去年累計營收": "1188820604",
  "累計營業收入-前期比較增減(%)": "29.946314254829318",
  "備註": "-"
}
```

### 欄位對應

| TWSE 欄位 | FinMind 對應 | 型別 | 說明 |
|-----------|-------------|------|------|
| `公司代號` | `symbol` | string | 2330 |
| `營業收入-當月營收` | `revenue` | string (數字) | 單位：仟元 |
| `營業收入-去年當月營收` | — | string (數字) | 自行計算 YoY% |
| `營業收入-去年同月增減(%)` | — | string (數字) | 直接可用 |

### ⚠️ 注意事項

1. **日期格式**: `資料年月` 使用**民國年**（如 `11504` = 2026年4月），需轉換
2. **無分頁/無過濾**: API 回傳 **全部上市公司**（約 900+ 筆），需在 client 端過濾 `公司代號` = "2330"
3. **欄位值皆為字串**: 所有數值欄位以 string 傳回，需 `strconv.ParseFloat` 轉換
4. **部分欄位可能為空**: 如新上市公司某些月份可能無資料
5. **資料延遲**: 營收公告截止日為次月 10 日，查詢當月資料可能尚未上架

### 現有基礎

系統已有完整的 `TWSEClient`（`internal/marketdata/twse_openapi.go`），含 rate limiter、HTTP client、錯誤處理。可**直接擴充**而非從零實作。

### 實作方式

新增 `TWSEOpenAPIProvider.GetMonthRevenue()` 方法於 `twse_openapi.go`，或新增 `twse_month_revenue_provider.go` 獨立檔案。詳見下方「實現計畫」。

---

## 方案 2: 公開資訊觀測站 MOPS 網頁爬取 ⚠️ **不建議**

### 可行性: ⚠️ 技術上可行但有風險

MOPS（Market Observation Post System）提供月營收查詢，但僅有 HTML 頁面，無公開 JSON API。

### 端點資訊

| 項目 | 值 |
|------|-----|
| **查詢 URL** | `https://mops.twse.com.tw/mops/web/t05st10_ifrs` |
| **查詢方式** | 需 POST form data（公司代號、年度、月份） |
| **回傳格式** | HTML table |
| **認證** | 無 |
| **法律風險** | ⚠️ **高** — 需要網頁爬蟲解析 HTML，可能違反使用條款 |

### 缺點

1. **HTML 解析脆弱** — 台灣政府網站經常改版（2026 年剛改版），爬蟲程式需頻繁維護
2. **非結構化資料** — 需用 `goquery` 或 regex 解析 HTML table
3. **POST 請求** — 非標準 REST API，需要額外處理 session/cookie
4. **無 SLA** — MOPS 無正式 API 文件，無法保證服務可用性
5. **IP 封鎖風險** — 頻繁爬取可能被防火牆封鎖

### 適用場景

僅建議作為最終備援（last resort），不作為主要數據源。

---

## 方案 3: TEJ API（台灣經濟新報）⚠️ **部分可行**

### 可行性: ⚠️ 需要 API Key 且有每日額度限制

系統已有 `TEJClient`（`internal/marketdata/tej_provider.go`），但目前僅實作 `GetStockPriceDaily()` 和 `GetFinancialStatements()`。TEJ 的免費試用方案為每日 500 次呼叫。

### TEJ 月營收端點

| 項目 | 值 |
|------|-----|
| **端點** | `https://api.tej.com.tw/api/datatables/TWN/AFINB.json` |
| **文件** | `https://api.tej.com.tw/` |
| **認證** | API Key（query param `api_key`） |
| **免費額度** | 500 calls/day（trial tier） |
| **付費額度** | 2000 calls/day |
| **回傳格式** | JSON（tejResponse wrapper） |

### 缺點

1. **需要 API Key** — 不是完全開放資料
2. **每日額度限制** — 500 calls/day 可能不夠其他功能共用
3. **與 TEJ 股價查詢共享額度** — 若已用 TEJ 做股價查詢，會更快耗盡配額
4. **資料集代碼不明確** — `TWN/AFINB` 是否包含月營收需進一步確認
5. **歷史資料不一定比 FinMind 更完整**

### 適用場景

僅作為備援。若系統已有 TEJ API Key 且尚未用滿每日配額，可以低成本加入。

---

## 方案 4: Yahoo Finance 網頁爬取 ⚠️ **不建議**

### 可行性: ⚠️ 技術上可行，但不可靠

Yahoo Finance 的 [營收頁面](https://tw.stock.yahoo.com/quote/2330.TW/revenue) 以 HTML 呈現月營收表格，但**無公開 API**。

### 現有基礎

系統已有 `YahooFinanceMacroProvider`，用於抓取總經指標（DXY, US10Y, VIX 等），但其使用 Yahoo Finance v8 chart API（
`/v8/finance/chart/{ticker}`），此 API **不提供月營收數據**。

### 缺點

1. **無結構化 API** — Yahoo 月營收數據僅在 HTML 頁面中顯示
2. **UI 改版頻繁** — HTML selector 隨改版失效
3. **IP 封鎖** — 現有 `YahooFinanceMacroProvider` 已需使用 User-Agent 輪換繞過限制
4. **資料延遲** — Yahoo 更新時間不確定
5. **法律爭議** — 爬蟲可能違反 Yahoo 服務條款

### 適用場景

**不建議採用**。如果需要 Yahoo 資料，可考慮僅作為 cross-check 而非主要數據源。

---

## 方案 5: TSMC 自結營收新聞稿（公司官網）⚠️ **理論上可行**

### 可行性: ⚠️ 理論上可行，但實務上複雜

TSMC 每月 10 日前會在 [tsmc.com](https://tsmc.com) 發布月營收新聞稿，但無結構化 API。

### 缺點

1. **HTML/PDF 格式** — 新聞稿為非結構化網頁或 PDF
2. **無歷史批量查詢** — 只能逐月抓取
3. **僅限 TSMC** — 無法用於其他個股
4. **維護成本高** — 網頁結構變化需更新解析邏輯

### 適用場景

完全排除。除非 FinMind、TWSE、TEJ 全部失效，否則不考慮。

---

## 方案 6: FinMind 付費升級 💰 **商業決策**

### 可行性: ✅ 技術上完全可行，但需付費

| 方案 | 費用 | 額度 | 功能 |
|------|------|------|------|
| Backer | 月付 | 較高 | + "backer" 標記資料集 |
| Sponsor | 月付 | 最高 | 全部資料含即時/分鐘級 |

### 優點
- 無需修改現有程式碼
- 現有 fallback 邏輯仍可保留

### 缺點
- 每月花費
- 仍有 7 天 Key 輪替限制
- 付費後仍非 SLA 保證

### 適用場景

若團隊評估月營收資料對系統至關重要且不願投入開發時間替換，短期可考慮。

---

## 推薦方案

### 🥇 TWSE OpenAPI（方案 1）— 強烈推薦

**理由：**
1. **完全免費、無認證** — 政府開放資料，授權明確
2. **回傳 JSON** — 無需解析 HTML，直接 `json.Unmarshal`
3. **更新頻率高** — 每日更新，次月 10 日即可取得最新營收
4. **覆蓋率高** — 包含所有上市公司
5. **現有基礎良好** — 系統已完整實作 `TWSEClient`，可擴充使用
6. **無每日額度限制** — 對 `/opendata/` 路徑的呼叫無 rate limit

### 🥈 TEJ API（方案 3）— 備援方案

若 TWSE OpenAPI 維護或暫時不可用，可 fallback 到 TEJ API（需額外申請 API Key）。

---

## 實現計畫

### 修改目標

在 `internal/marketdata/twse_openapi.go` 中為 `TWSEClient` 新增月營收查詢方法，並修改 `TSMCRevenueProvider` 的底層 client。

### 具體步驟

#### Step 1: 在 `TWSEClient` 新增 `GetMonthRevenueList()`

新增結構和查詢方法：

```go
// TWSEMonthRevenueRow 上市公司月營收一筆記錄
type TWSEMonthRevenueRow struct {
    ROCYearMonth   string  `json:"資料年月"`       // 民國年月, e.g. "11504"
    StockCode      string  `json:"公司代號"`
    CompanyName    string  `json:"公司名稱"`
    CurrentRevenue float64 `json:"-"`             // 當月營收（仟元）
    LastYearRevenue float64 `json:"-"`            // 去年當月營收（仟元）
    YoYChangePct   float64 `json:"-"`             // 年增率(%)
}

// TWSEMonthRevenueResponse TWSE 月營收 API 回應
type TWSEMonthRevenueResponse struct {
    Data []map[string]string `json:"data"`
}

func (c *TWSEClient) GetMonthRevenue(ctx context.Context, symbol string, year, month int) (float64, error)
```

#### Step 2: 資料轉換（民國年 → 西元年）

TWSE 使用民國年（`11504` = 2026年4月），轉換邏輯：

```go
func toROCDate(year, month int) string {
    return fmt.Sprintf("%03d%02d", year-1911, month)
}
```

查詢時需過濾 `資料年月` 與 `公司代號` 同時匹配的記錄。

#### Step 3: 效益考量 — 全部抓取 vs 快取

`/opendata/t187ap05_L` 回傳約 900+ 筆記錄（約 500KB JSON），考慮：
- **首次請求**: 從 TWSE API 抓取全部資料 → 在 client 端過濾
- **後續請求（同一 session）**: 可考慮短期快取（5-15 分鐘），避免重複下載大 payload
- **跨日請求**: 依賴 `資料年月` 欄位判斷數據是否更新

#### Step 4: 修改 `TSMCRevenueProvider`

```go
type TSMCRevenueProvider struct {
    twseClient  *TWSEClient    // 取代 FinMindClient
    tejClient   *TEJClient     // 備援
    storageDir  string
}
```

實作時使用**策略模式**：先嘗試 `TWSEClient.GetMonthRevenue()`，失敗則 fallback 到本地 JSON 檔案（`loadLatestSnapshot()` 現有邏輯）。

### 不和諧變更提醒

- `NewTSMCRevenueProvider()` 的簽名從 `(apiKey string)` 變更 — 影響 `cmd/atlas` 中建立 provider 的程式碼
- `TSMCRevenueProvider.client` 欄位從 `*FinMindClient` 變更為 `*TWSEClient` — 影響所有呼叫端

### 建議的建立工廠

```go
func NewTSMCRevenueProvider() *TSMCRevenueProvider {
    return &TSMCRevenueProvider{
        twseClient: NewTWSEClient(),
    }
}
```

不再需要 FinMind API Key 來取得月營收！

---

## 驗證方式

```bash
# 直接測試 TWSE OpenAPI 端點
curl -s https://openapi.twse.com.tw/v1/opendata/t187ap05_L | python3 -c "
import json, sys
data = json.load(sys.stdin)
for item in data:
    if item['公司代號'] == '2330':
        print(json.dumps(item, ensure_ascii=False, indent=2))
"
```

預期輸出：
```json
{
  "出表日期": "1150512",
  "資料年月": "11504",
  "公司代號": "2330",
  "公司名稱": "台積電",
  "營業收入-當月營收": "410725118",
  ...
}
```

---

## 附錄: 比較總表

| 方案 | 免費 | API Key | 速率限制 | 資料格式 | 維護成本 | 建議 |
|------|------|---------|---------|---------|---------|------|
| **TWSE OpenAPI** | ✅ 完全免費 | 無需 | 極低（開源放行） | JSON | 低 | **🥇 主要數據源** |
| **MOPS 爬蟲** | ✅ 免費 | 無需 | 中等（IP 風險） | HTML | **高** | ❌ 不建議 |
| **TEJ API** | ⚠️ 免費試用 | 需要 | 500 calls/day | JSON | 中 | 🥈 備援方案 |
| **Yahoo Finance** | ✅ 免費 | 無需 | 低（反爬風險） | HTML | **高** | ❌ 不建議 |
| **FinMind 付費** | ❌ 付費 | 需要 | 依方案 | JSON | 無 | 💰 商業決策 |

---

*調查完成日期: 2026-05-13*
*調查者: AI Agent（使用 TWSE OpenAPI swagger.json 即時驗證）*
