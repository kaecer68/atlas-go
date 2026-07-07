# agents.md — web (Frontend)

> 修改 web 目錄下的程式碼前請先閱讀此文件。

## CSS 色彩語意系統

atlas-go 使用**台股紅漲綠跌**慣例（與國際相反），系統有兩套並行的色彩體系：

### 市場方向（價格/漲跌）
| Token | Utility Class | 用途 | 示例場景 |
|-------|--------------|------|----------|
| `--up` / `--bullish` | `.text-up` | 上漲/多頭/正值 | 報酬率 > 0、Sharpe > 1、regime RISK_ON |
| `--down` / `--bearish` | `.text-down` | 下跌/空頭/負值 | 報酬率 < 0、Sharpe < 0、regime RISK_OFF |
| `--warn` | `.text-warn` | 中性/警告 | Sharpe 介於 0~1、NEUTRAL regime |

### 資金流與信號 Token（新增於 PR #944，Phase 0）
| Token | Utility Class | 用途 | 示例場景 |
|-------|--------------|------|----------|
| `--capital-inflow` / `--capital-outflow` | `.text-capital-inflow` / `.text-capital-outflow` | 外資買賣超、資金流向 | foreign_investor_net > 0、外資賣超 |
| `--signal-bullish` / `--signal-bearish` | `.text-signal-bullish` / `.text-signal-bearish` | narrative event 信號晶片 | 利多事件 → --signal-bullish、利空事件 → --signal-bearish |

### 字體層級 Token（新增於 PR #944，Phase 0）
| Token | 用途 |
|-------|------|
| `--font-tabular` | 等寬數字字型堆疊（表格/數字對齊用） |
| `--text-lede` | 文章導言段落字級（hero 摘要） |

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
- **JS 端色彩判斷**：使用 `shared_web/static/js/shared/color-tokens.js` 提供的 `financialColor()` / `regimeColor()` / `severityColor()` / `confidenceColor()`，不要在各頁面重複寫 color 判斷邏輯。此為 Phase 0（PR #944）建立的色彩管理單一權威來源。
- **新增顏色**：一律使用現有 CSS 變數，不要寫死 hex/rgba。
- **inline style 用 `var(--...)`**，不要寫死色碼。
- `--up`/`--down` 目前留 118 處，語意正確無須急著一次性改完，但 touch 到時請順手遷移至 `financialColor()`。

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
  → JS 端：用 financialColor(value, 'trend') / financialColor(value, 'pnl')

要表達的語意是資金流入/流出（外資買賣超）？
  → 用 --capital-inflow/--capital-outflow
  → JS 端：用 financialColor(value, 'capital')

要表達的語意是系統狀態（API、guard、DB）？
  → 用 --color-success/--color-danger/--color-warning

要表達的是風險程度（VaR、波動）？
  → 用 --risk-high/--risk-low

要表達的是績效評估（Sharpe、hit rate）？
  → 用 --metric-good/--metric-bad

要表達的是 narrative event 信號（利多/利空）？
  → 用 --signal-bullish/--signal-bearish
  → JS 端：用 financialColor(value, 'signal')
```

## 重要參考檔案

| 檔案 | 內容 |
|------|------|
| `shared_web/static/css/base/variables.css` (canonical) | 所有 CSS 變數定義（色彩、字體、間距） |
| `shared_web/static/css/components/utilities.css` (canonical) | Utility class 定義 |
| `shared_web/static/js/shared/utils.js` (canonical) | Canvas 橋接函數 (`getThemeColor`, `hexToRgba`) |
| `shared_web/static/js/shared/color-tokens.js` (canonical) | JS 端統一色彩邏輯（`financialColor`, `regimeColor`, `severityColor`, `confidenceColor`），PR #944 引入 |
| `shared_web/static/js/shared/theme-labels.js` (canonical) | 24 個 narrative event theme → 中文標籤對映表（從 `internal/narrative/templates.go` 擷取） |
| `shared_web/static/js/components/event-calendar.js` | 市場行事曆組件（除權息、法說會、季節性事件），PR #945 引入 |

## 市場行事曆組件

`event-calendar.js` 串接後端 `/api/dashboard/calendar-events`（由 `internal/industry/event_calendar.go` 提供），在首頁以 responsive grid 顯示近期市場事件：

- **事件類型**：除權息、法說會、財報公布、股東會、MSCI 季度調整等 14 種類型（`TaiwanEventType` enum）
- **資料來源標記**：`default_rules`（硬編碼規則）／`twse_provider`（TWSE OpenAPI，已 deprecate 2026-06-30）／`finmind_provider`（FinMind API，待補強）
- **互動**：hover 顯示 description 全文，active 事件左側有 accent 邊框
- **evidence 等級**：`backtested` / `estimated` / `unverified` / `realtime` |
