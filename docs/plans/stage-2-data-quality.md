# Stage 2 — 資料品質與反污染機制 — 規劃

> 完整規劃見 `~/workspace/atlas-notes/05-decisions/2026-07-13-stage-2-data-quality.md`。
> 本文件是 in-repo summary + PR 規劃。

## 範圍

| Stage | 內容 | 工作量 |
|-------|------|--------|
| 2.1 | 事件資料品質把關 (5 rules: date, dedup, required, source, confidence) | 4-6 hr |
| 2.2 | 垃圾資料防護 (3 rules: sanitize title, cross-source verify, backfill mark) | 3-4 hr |
| 2.3 | 反污染審計 (2 rules: template + model weight) | 3-4 hr |
| 總計 | | 10-14 hr |

## 紅線

- [ ] 不改 audit log schema
- [ ] 不關閉既有事件偵測器
- [ ] 不在 production 直接改
- [ ] 不寫死資料進資料庫（測試資料 `synthetic=true`）
- [ ] 不繞過品質檢查
- [ ] 不改既有 91 個 MCP tool 介面

## 關鍵設計決定

1. **不動既有 struct schema**：`CalendarEvent` / `EventCalendarItem` 不加新欄位，改在 ingest 端做 wrapper/validation layer
2. **新建 `internal/eventquality` package**（4 個檔案：validator, sanitizer, cross_source_store, quality_log）
3. **audit_v2.go 加 tool registration**：`narrative_template_modified` + `narrative_model_weight_modified`（新 tool，schema 不變）
4. **整合點**：`event_calendar.go` (RefreshEvents + UpdateFromProvider), `eventdriven/handler.go`, `narrative/templates.go`, `narrative/models.go`
