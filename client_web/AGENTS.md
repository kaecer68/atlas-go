# AGENTS.md — client_web（投資人介面）

> 修改 client_web 目錄下的程式碼前請先閱讀此文件。
> CSS 色彩語意系統的完整規範見 **[`docs/guides/frontend-architecture.md`](../docs/guides/frontend-architecture.md)**。本節僅列 client_web 特有陷阱。

## CSS 色彩語意系統

atlas-go 使用**台股紅漲綠跌**慣例（與國際相反），系統有兩套並行的色彩體系：

### 市場方向（價格/漲跌）
| Token | Utility Class | 用途 | 示例場景 |
|-------|--------------|------|----------|
| `--up` / `--bullish` | `.text-up` | 上漲/多頭/正值 | 報酬率 > 0、Sharpe > 1、regime RISK_ON |
| `--down` / `--bearish` | `.text-down` | 下跌/空頭/負值 | 報酬率 < 0、Sharpe < 0、regime RISK_OFF |
| `--warn` | `.text-warn` | 中性/警告 | Sharpe 介於 0~1、NEUTRAL regime |

### 系統狀態（成功/錯誤/警告 — 國際通用）
| Token | Utility Class | 用途 |
|-------|--------------|------|
| `--color-success` | `.text-success` | API 正常、guard 放行 |
| `--color-danger` | `.text-danger` | API 錯誤、guard 阻擋 |
| `--color-warning` | `.text-warn` | 部分過濾、部分成功 |

### 金融語意 Token（新，長期目標取代 `--up`/`--down`）
| Token | Utility Class | 取代 `--up`/`--down` 場景 |
|-------|--------------|---------------------------|
| `--pnl-profit` / `--pnl-loss` | `.text-pnl-profit` / `.text-pnl-loss` | 報酬率、盈虧金額 |
| `--trend-bullish` / `--trend-bearish` | `.text-trend-bullish` / `.text-trend-bearish` | regime、市場方向、產業輪動 |
| `--metric-good` / `--metric-bad` | `.text-metric-good` / `.text-metric-bad` | Sharpe、hit rate、信心指數 |
| `--risk-high` / `--risk-low` | `.text-risk-high` / `.text-risk-low` | 風險指標 |

### 關鍵規則
- **修改 `--up`/`--down` 使用場景時**：順手遷移到對應的金融語意 token（對照上表）。
- **新增顏色**：一律使用現有 CSS 變數，不要寫死 hex/rgba。
- **inline style 用 `var(--...)`**，不要寫死色碼。
- `--up`/`--down` 目前留 118 處，語意正確無須急著一次性改完，但 touch 到時請順手遷移。

## Canvas 繪圖色彩橋接

Canvas 無法直接讀取 CSS 變數，使用 `utils.js` 提供的橋接函數：

```javascript
// 讀取 CSS 變數（主題感知）
const color = getThemeColor('--pnl-profit', '#ef4444');
ctx.fillStyle = color;

// 透明度處理（替代手動 rgba hack）
ctx.strokeStyle = hexToRgba(getThemeColor('--trend-bullish'), 0.3);
```

原則：Canvas 繪圖一律透過 `getThemeColor()` + `hexToRgba()`，不直接寫死 hex。

## 色彩選擇決策樹

```
要表達的語意是漲跌方向（股價、報酬）？
  → 用 --up/--down（舊）或 --trend-bullish/--pnl-profit（新）

要表達的語意是系統狀態（API、guard、DB）？
  → 用 --color-success/--color-danger/--color-warning

要表達的是風險程度（VaR、波動）？
  → 用 --risk-high/--risk-low

要表達的是績效評估（Sharpe、hit rate）？
  → 用 --metric-good/--metric-bad
```

## 前端 API 欄位合約（source of truth: Go backend）

以下 schema / TypeScript type 必須與後端 struct 對齊，修改前請先確認後端欄位：

| API | Schema | 後端 Struct | 關鍵欄位 |
|-----|--------|-------------|----------|
| `/api/macro/snapshot/latest` | `shared_web/static/js/schemas/macro-snapshot.schema.json` | `internal/marketdata.MacroDataSnapshot` | `recorded_at` (int64), `data_status` (omitempty), indicator `timestamp` (int64) |
| `/api/dashboard/us-indices` | `shared_web/static/js/schemas/us-indices.schema.json` | `internal/monitoring/service.USIndicesResponse` | `recorded_at` (int64), `generated_at` (string), `indices[]/tech_stocks[]` 物件陣列 |
| `/api/taiwan/stress-index` | `shared_web/static/js/schemas/stress-index.schema.json` | `internal/narrative.TaiwanStressIndex` | `score` (number), `regime` (string), `timestamp` (int64) |

### Wave 11 Phase 3 已對齊的變更
- `macro-snapshot.schema.json`：`required` 改為 `recorded_at`；移除 `updated_at`；各 indicator `timestamp` 改為 `number`。
- `us-indices.schema.json`：`required` 改為 `recorded_at/generated_at/indices/tech_stocks/data_status`；`generated_at` 為 `string`；`failed_channels` / `stale_channels` optional。
- `stress-index.schema.json`：`required` 改為 `score/regime/timestamp`；移除舊欄位 `index` / `updated_at`。
- `CrossMarketStatus` 中的跨市場指標（`spx`, `ndx`, `dji`, `sox`, `nvda`, `aapl`, `msft`, `tsm_adr`, `vix`, `dxy`, `usd_twd`, `us10y`）已從 `string` 改為 `CrossMarketIndex` 物件（含 `symbol/value/change_pct/timestamp`）。
- `USIndicesResponse.indices` / `tech_stocks` 已從 `string[]` 修正為 `USIndexItem[]` 物件陣列。

## shared_web fallback 機制

`client_web/` 與 `admin_web/` 並不複製所有頁面與型別檔案。esbuild shared plugin 會在解析不到本機檔案時自動 fallback 到 `shared_web/static/js/`。因此：

- **不要**在 `client_web/static/js/` 建立空 stub 去 shadow `shared_web` 的實作。
- 共用型別（如 `field_types.ts`）若兩邊都存在，必須同步修改，避免型別漂移。
- 頁面模組優先在 `shared_web/static/js/pages/` 維護；`client_web` 僅放客製化覆寫。

## 重要參考檔案

| 檔案 | 內容 |
|------|------|
| `shared_web/static/css/base/variables.css` (canonical) | 所有 CSS 變數定義（色彩、字體、間距） |
| `shared_web/static/css/components/utilities.css` (canonical) | Utility class 定義 |
| `shared_web/static/js/shared/utils.js` (canonical) | Canvas 橋接函數 (`getThemeColor`, `hexToRgba`) |
| `shared_web/static/js/schemas/*.schema.json` (canonical) | API response 欄位合約 |
| `shared_web/static/js/shared/field_types.ts` (canonical) | 共用 TypeScript interface |
