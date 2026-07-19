# Audit Manifest: BL-CL5b point-in-time capital-flow snapshot endpoint (spec §18.3.2)

> **Audit source**: docs/specs/capital-flow-seven-dimension-spec.md §18.3.2（contract 已完整）
> **Goal**: 實作 `GET /api/capital-flow/historical-snapshot/{trading_date}` endpoint 以回傳單一交易日的七維錢潮快照
> **Scope**: LOW — handler 新 endpoint + route 註冊 + test（無 service/store 層變更，純 HTTP wrapper）
> **Created**: 2026-07-20
> **Status**: done

---

## 摘要

spec §18.3.2 的「未實作 — BL-CL5b」point-in-time snapshot endpoint 終於實作。既有 `HandleHistory` 回傳的是 rolling trend（多日 samples per dimension），新 endpoint 回傳單日快照（single sample per dimension or null）。

### 實作範圍

- `internal/capitalflow/handler.go`：
  - `HandleHistoricalSnapshot(r)` — 讀 `PathValue("trading_date")`，對 7 個 dimension 各自查 `store.History(...)`，filter trading date，回 `{trading_date, status, dimensions: {<force>: {...} | {"data_available": false, "missing_reason":"..."}}}`
  - `RegisterRoutes` 加 `GET /api/capital-flow/historical-snapshot/{trading_date}` route
- `internal/capitalflow/handler_test.go`：
  - `TestHandleHistoricalSnapshot_OK` — 2 dims 有資料 → status=partial
  - `TestHandleHistoricalSnapshot_MissingDate` — 無資料 → status=missing
  - `TestHandleHistoricalSnapshot_ValidationErrors` — 格式無效 → status=400
- `cmd/atlas/main.go`：production route 註冊（cfHandler.HandleHistoricalSnapshot）

### Status enum（對齊 spec §18.3.2）
- `complete` — 7 dims 全有資料
- `partial` — 部分有
- `missing` — 全無

### 不變的既有 helper
- `store.History()` — 直接使用（不重複 SQL/query logic）
- `RegisterRoutes(mux, provider)` — 仍用 `NewHandler`（test/legacy 路徑），與 main.go production 路徑分離

### 驗收
- `go build ./...` 全綠
- `go test ./internal/capitalflow/...` 8 tests 全綠（5 既有 + 3 新）
- `gofmt` PASS
- markdown link check PASS

---

**Commit**：
- `feat(manifest): BL-CL5b add point-in-time capital-flow snapshot endpoint (§18.3.2)` — handler + test + route

See also: docs/specs/capital-flow-seven-dimension-spec.md §18.3.2
