# 前端結構性重構規劃（P2）

## 背景

`frontend-zero-value-audit` PR #1087 已修復 P0/P1 的顯示錯誤與資料缺失問題。本 PR 承接其遺留的結構性問題，目標是避免同類 zero-value / missing-data bug 復發。

## 範圍

### 1. 統一數值格式化層

**檔案**：`shared_web/static/js/shared/format-metric.js`

- 定義標準格式化函式：
  - `fmtCurrency(value, options)` — 幣別，含千分位
  - `fmtPct(value, decimals)` — 百分比，不含正負號
  - `fmtSignedPct(value, options)` — 帶正負號百分比，避免 `-0.0%`
  - `fmtLargeNumber(value)` — 大數（萬 / 億）
  - `fmtDrawdown(value)` — 回撤，含語義標籤
- 移除各頁面硬編碼的 `toFixed(1/2/3)`
- 增加單元測試覆蓋邊界值：0、極小值、`null`、`NaN`、負數

### 2. API 合約文件

**新檔案**：

- `docs/specs/stock-api-contract-spec.md`
- `docs/specs/dashboard-api-contract-spec.md`

**內容**：

- 明確每個欄位語義（例如外資 `value` = 億元，`change_pct` = 日變動%）
- 定義缺失資料的回應格式：`null` vs `0` vs omit
- 定義錯誤回應格式：`{"error": "..."}`

### 3. Admin Dashboard 並發請求最佳化

**檔案**：`admin_web/static/js/main.js`

- 將 14 個並發請求分組：
  - 核心（必須先載入）：portfolio-state、system-health、macro snapshot
  - 非核心（可背景更新）：scheduler、risk、experiments、darwinian 等
- 每個 panel 獨立載入 / 錯誤 / 重試狀態
- 增加重試機制與明確錯誤提示，避免單一 API timeout 拖垮整頁

### 4. 全站資料缺失狀態統一

**檔案**：各 page render 函式、`shared_web/static/js/shared/app-utils.js`

- 引入 `renderMissingState(label, reason)` 元件
- 區分四種狀態：
  - 「載入中」
  - 「無資料」
  - 「資料待更新」
  - 「API 錯誤」
- 不再用 `0.0`、`—`、`NaN` 混用

## 與 P0/P1 的關係

- 本 PR **不改變** P0/P1 已修復的業務邏輯
- 只改前端結構、格式化層、API 消費模式與文件

## 驗收標準

- [ ] `format-metric.js` 有單元測試且覆蓋 0 / 極小值 / `null` / `NaN`
- [ ] 前端無硬編碼 `toFixed`（統一使用 `format-metric`）
- [ ] Admin 首頁無連續 timeout（非核心請求獨立背景更新）
- [ ] 資料缺失狀態顯示統一
- [ ] API 合約文件 merged

## 備註

- 本 PR 為 draft，會逐步拆分為多個 commit
- 建議 review 時優先看 `format-metric.js` 與 `admin_web/static/js/main.js`
