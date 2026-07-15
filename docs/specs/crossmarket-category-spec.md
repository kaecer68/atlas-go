# Crossmarket Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_crossmarket.go`
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Crossmarket](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a

## 1. 目的

即時鏡像 US 市場指數 / 個股供 agent 與 web cross-market 頁面讀取;補 L4 資料可見性(8 個零值判定)在 `internal/monitoring/service/crossmarket.go` 已實作。

## 2. Tool 清單(3 個)

| Tool | Description | Handler | Backend |
|------|-------------|---------|---------|
| `crossmarket_get_status` | 跨市場資料源狀態 | `handleCrossMarketStatus` | `GET /api/crossmarket/status` |
| `crossmarket_get_correlation` | 台股 sector vs US indices 相關性 | `handleCrossMarketCorrelation` | `GET /api/crossmarket/correlation` |
| `crossmarket_get_us_indices` | S&P 500 / NASDAQ / Dow / SOX / NVDA / AAPL / MSFT / TSM ADR 即時 snapshot | `handleCrossMarketUSIndices` | Yahoo Finance live fetch |

## 3. 規格重點

- **US indices 即時 fetch**:不走 atlas HTTP API,直接從 Yahoo Finance(同 domain 外部資源,需 apigateway channel 註冊)
- **Correlations**:見 `internal/orchestrator/crossmkt_correlator.go`
- **Status 與 degraded 旗標**:4 層資料可見性機制(L1 Gateway → L2 Adapter → L3 Service → L4 Frontend),見 [`atlas-data-visibility` skill](../../.claude/skills/atlas-data-visibility/SKILL.md)

## 4. 已知限制

- Yahoo Finance rate limit 不可預期;若 8 個欄位全 `symbol=""` → data_status = "degraded"
- Tool 對應 `internal/monitoring/CrossMarketService`,fallback 需 SQLite backend(目前 jsonl backend 為 Stage 8 follow-up)

## 5. 測試

- `tools_crossmarket_test.go`(shape)
- e2e: `curl /api/crossmarket/status` + verify `data_status` 欄位

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版 |
