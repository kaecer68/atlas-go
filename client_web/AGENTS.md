# AGENTS.md — client_web

投資人介面前端。

## 陷阱

- **API 欄位合約**：前端 schema 必須與後端 struct 對齊。source of truth：後端 struct + `go generate .` 產出的 `shared_web/static/js/shared/field_types.ts`。
- **shared_web fallback**：`client_web/static/` 找不到的 import 會 fallback 到 `shared_web/static/js/`。不要建立空 stub shadow `shared_web` 實作；共用型別兩邊存在時必須同步修改。

## 重要參考檔案

| 檔案 | 內容 |
|------|------|
| `shared_web/static/css/base/variables.css` | CSS 變數定義 |
| `shared_web/static/js/shared/color-tokens.js` | JS 端統一色彩邏輯 |
| `shared_web/static/js/schemas/*.schema.json` | API response 欄位合約 |
| `shared_web/static/js/shared/field_types.ts` | 共用 TypeScript interface |
