# AGENTS.md — admin_web

管理後台前端。

## 陷阱

- **市場行事曆組件**：`shared_web/static/js/components/event-calendar.js` 串接 `/api/dashboard/calendar-events`。事件資料來源標記可能為 `default_rules` / `twse_provider`（已 deprecate 2026-06-30）/ `finmind_provider`；下游須處理空 events。
- **esbuild fallback**：`admin_web/static/` 找不到的 import 會 fallback 到 `shared_web/static/js/`。不要建立空 stub shadow `shared_web` 實作。

## 重要參考檔案

| 檔案 | 內容 |
|------|------|
| `shared_web/static/css/base/variables.css` | CSS 變數定義 |
| `shared_web/static/js/shared/color-tokens.js` | JS 端統一色彩邏輯 |
| `shared_web/static/js/shared/utils.js` | Canvas 橋接函數 |
| `shared_web/static/js/components/event-calendar.js` | 市場行事曆組件 |
