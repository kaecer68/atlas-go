# Alert Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_risk_alert.go`(與 Risk 共檔)
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Alert](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a

## 1. 目的

對外暴露 alert 系統的查詢面(read-only):未確認警報、全部警報、統計、規則配置。

## 2. Tool 清單(4 個)

| Tool | Description | Handler | Backend |
|------|-------------|---------|---------|
| `alert_list_unacknowledged` | 未確認警報(Phase 1) | `handleAlertListUnacknowledged` | `GET /api/alerts/unacknowledged` |
| `alert_list` | 所有警報 | `handleAlertList` | `GET /api/alerts/list` |
| `alert_get_stats` | 警報統計(severity × source) | `handleAlertGetStats` | `GET /api/alerts/stats` |
| `alert_get_rules` | 警報規則配置 | `handleAlertGetRules` | `GET /api/alerts/rules` |

## 3. 規格重點

- **Alert store**: `internal/alerting/`(ack 寫入由 HTTP `/api/alerts/acknowledge` 處理,**不在 MCP 範圍**)
- **MCP 設計原則**: Alert 為 only-read-from-side;ack 走 atlas HTTP API 直接(per PR #1100 `alert_list_unacknowledged` doc comment)
- **DestructiveHint**: 全部 false
- **與 `mcp_anomaly_*` 關係**: anomaly 工具(self-observation)與業務 alert 不同層

## 4. 已知限制

- `alert_get_rules` 回傳的規則設計可能包含 SLB(secret literals as blocked-by-value),所以 post-processor 需脫敏
- 大量 backlog(>10K)時 `alert_list` 建議分頁(由 frontend 處理)

## 5. 測試

- `tools_risk_alert_test.go`(shape + ack 路徑對齊)
- e2e: `curl /api/alerts/unacknowledged` 200 + `[…]`

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版 |
