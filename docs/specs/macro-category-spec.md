# Macro Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_macro.go`
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Macro](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a — 補獨立 spec

## 1. 目的

對外暴露總經 macro snapshot、stress index 與 ingest pipeline 狀態,給 agent 與 human investor 取用市場/六維度總體狀態。

## 2. Tool 清單（6 個）

| Tool | Description | Handler | Backend endpoint |
|------|-------------|---------|------------------|
| `macro_get_snapshot_latest` | 最新 macro snapshot(6 維度) | `handleMacroGetSnapshotLatest` | `GET /api/macro/snapshot/latest` |
| `macro_get_snapshot_history` | Macro snapshot 歷史(`days` 參數) | `handleMacroGetSnapshotHistory` | `GET /api/macro/snapshot/history?days=N` |
| `macro_get_stress_index_current` | 當前 stress index | `handleMacroGetStressIndexCurrent` | `GET /api/stress/index/current` |
| `macro_get_stress_index_history` | Stress index 歷史 | `handleMacroGetStressIndexHistory` | `GET /api/stress/index/history?days=N` |
| `macro_get_capital_flow_latest` | 外資/法人/散戶資金流 snapshot | `handleMacroGetCapitalFlowLatest` | `GET /api/capital/flow/latest` |
| `macro_get_ingest_status` | 通道 ingest 狀態 | `handleMacroGetIngestStatus` | `GET /api/ingest/status` |

## 3. 規格重點

- **Snapshot 來源**:`internal/monitoring/service/crossmarket.go::FetchSnapshot` 包 internal/apigateway(`apigateway.FetchResult`)
- **Stress index**:`internal/narrative/trj.go`,詳見 [`docs/specs/janus-regime-detection-spec.md`](../specs/janus-regime-detection-spec.md)
- **資本流 snapshot**:見 [`internal/capitalflow/`](../reference/tool-catalog.md) (cross-reference `capital_flow_daily` / `capital_flow_summary`)
- **Ingest 狀態**:`internal/macro/ingestor.go`

## 4. 已知限制

- snapshot 在 5-min refresh cycle 內重複呼叫會回 cached 值(見 `internal/monitoring/service/crossmarket.go::getCachedSnapshot`)
- `days` 參數預設 30、上限 365

## 5. 測試

- `cmd/atlas-mcp/server/tools_macro_test.go`(tools 列表 + schema)
- e2e: `curl /api/macro/snapshot/latest`

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版(Macro 獨立 spec) |
