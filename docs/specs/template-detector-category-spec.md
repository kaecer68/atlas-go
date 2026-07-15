# Template Detector Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_template_detector.go`
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Template Detector](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a

## 1. 目的

暴露 template trigger detector scan 的兩類結果給 agent:
1. 過去 scan ledger 的「最近 N 次」結果(`template_detector_status`)
2. 註冊表中所有 24 個 detector 的啟用/停用狀態(`detector_registry_list`)

## 2. Tool 清單(2 個)

| Tool | Description | Handler | Backend |
|------|-------------|---------|---------|
| `template_detector_status` | 最近(limit,預設 100)次 trigger theme scan 結果 | `handleTemplateDetectorStatus` | `GET /api/detector/scan/status?limit=N` |
| `detector_registry_list` | 24 個 template trigger detector 的 theme + enable/disable | `handleDetectorRegistryList` | `GET /api/detector/registry/list` |

## 3. 規格重點

- **Source code**: `internal/narrative/detector.go`(24 個 detector)
- **Ledger**: `ledger.detector_scan_log`(SQLite 寫入;`detector_scan` 端點仍用 jsonl backend,Stage 8 follow-up)
- **Auth**: 兩 tool 皆需 `ATLAS_API_KEY`(backend `/api/detector/*` 設為 protected)
- **DestructiveHint**: 兩 tool 都 false(都是 read-only)

## 4. 已知限制

- jsonl backend 下 `template_detector_status` 可能回 503(store unavailable),待 SQLite 升級
- `detector_registry_list` 用 atlas HTTP API

## 5. 測試

- `tools_template_detector_test.go` — handler shape
- e2e: `curl -H "X-API-Key: $ATLAS_API_KEY" /api/detector/registry/list`(回 200 + 24 detectors JSON)

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版(Stage 5 PR#4 Stage B 補為 MCP reachability + Round 3a doc) |
