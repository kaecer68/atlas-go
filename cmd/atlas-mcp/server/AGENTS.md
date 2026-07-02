# AGENTS.md — atlas-mcp/server

> 本模組為 MCP stdio server 實作，負責把 atlas-go HTTP API 包裝成 MCP tool。所有回覆與修改必須使用繁體中文。

## 快速路由

| 檔案 | 責任 |
|------|------|
| `server.go` | 生命週期、設定驗證、rate limiter / metrics / anomaly emitter 啟動 |
| `tools.go` | 總註冊入口與 `withAudit`/`withAuditExtra` 包裝 |
| `tools_*.go` | 各領域 tool 註冊與 handler（control / audit / anomaly / crossmarket ...） |
| `audit.go` | 寫入模型 `AuditEntry` + `AuditWriter`（v2 格式為主） |
| `audit_v2.go` | 讀取模型 `AuditEntryV2` + 聚合函式 |
| `auth.go` | `TokenAuth`：env token / DB store fallback / dev mode |
| `auth_db.go` | `TokenStore` 介面與錯誤定義 |
| `auth_db_pg.go` | PostgreSQL 實作，只存 SHA-256 hash |
| `metrics.go` | Prometheus 註冊與 `ObserveCall`/`ObserveAnomaly` |
| `auto-desc.gen.go` | descgen 產出，禁止手動編輯 |

## 命名陷阱

- Tool 名稱慣例為 `area_verb_noun`：例如 `regime_get_history`、`control_get_active_overrides`、`mcp_anomaly_get_recent`。
- `tools.go` 裡 `registerTools` 會再呼叫 `registerMacroTools`、`registerControlTools` 等；新增領域時要在這裡加一行，否則 tool 不會掛載。
- handler 簽名固定為 `func (s *server) handleXXX(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`；目前實作一律回傳 `nil` 作為第一個值，由 go-sdk 自動轉 `Out` 為 JSON。

## 相依陷阱

- `tools_anomaly.go` 沒有直接 import `metrics.go`，但 `withAudit` 會呼叫 `observeAuditEntry`，後者把 `AuditEntry` 餵給 `s.metrics.ObserveCall` 與 `s.detector.Observe`。
- `server.go` 建立 `anomaly.NewDetector(..., metrics, nil)`，所以 detector 已經綁定 metrics；異常分數會寫入 `mcp_anomaly_score` / `mcp_anomaly_emitted_total`。
- 任何繞過 `withAudit` 的 handler 都不會被 rate limit、metrics、detector 觀測到，視為 blind spot。

## 生成程式碼陷阱

- `auto-desc.gen.go` 頂端標示 `DO NOT EDIT`，由 `cmd/atlas-mcp/descgen` 根據 `tools_*.go` 產生。
- `tools.go` 第一行 `//go:generate go run ../descgen -out ../auto-desc.gen.json -pkgdir .` 只產 JSON；`auto-desc.gen.go` 的 byte slice 由建置流程嵌入。
- 要改 tool 描述請改各 `tools_*.go` 的 fallback 字串或 descgen 輸入，不要直接改 `auto-desc.gen.go`。

## 稽核陷阱

- **寫入用 `audit.go`**：`AuditEntry` 是 v2 寫入模型，`Write()` 會自動補 `schema_version=2`、`transport="stdio"`、`args_hash`；失敗時不應蓋過原始錯誤（見 `withAuditExtra`）。
- **讀取用 `audit_v2.go`**：`AuditEntryV2` 是讀取/聚合模型，`ParseAuditEntry` 會把無 `schema_version` 的舊紀錄標為 v1，並從 `duration_ms` 回填 `latency_ms`。
- `tools_audit.go` 的 `readAuditEntriesV2` 會鎖 `w.mu` 再讀檔，與 `AuditWriter.Cleanup` 互斥；新增自訂讀取時必須複製同樣的鎖策略，否則可能與 cleanup 競爭。
- `AuditWriter.Cleanup` 會先 `Sync()`、`Close()`、原子 rename，最後重新 open；呼叫後底層 fd 已換，外部不可再持有舊 fd。

## 認證陷阱

- 三層鏈：`auth.go`（決策層）→ `auth_db.go`（介面層）→ `auth_db_pg.go`（儲存層）。
- `TokenAuth.Authenticate` 優先查 `TokenStore`：
  - `ErrDBUnavailable` → fail closed，回傳 `ErrUnauthorized`。
  - `ErrRevoked` / `ErrExpired` → 直接拒絕。
  - `ErrTokenNotFound` → 才 fallback 到 env token（`MCPToken`）。
- env token 為空時是 dev mode：`Authenticate` 直接回傳原 context，不檢查 bearer。
- `auth_db_pg.go` 永遠只存 `hashTokenRaw(raw)`，回傳的 raw token 只會出現在 `Register` / `Rotate` 回傳值，不會再出現第二次。
- `AdminAddr` 啟用時，`server.go` 強制要求 `TokenStore != nil` 且 `AdminToken != ""`，且只能 bind `127.0.0.1`。

## 測試陷阱

- 單元測試通常不透過 `Run()` 建立 server，而是像 `tools_anomaly_test.go` 的 `newAnomalyTestHarness` 手動組 `server{cfg, audit, cli, metrics, detector}`。
- 手動組 server 時若沒設 `metrics` 與 `detector`，`withAudit` 裡的 `observeAuditEntry` 會因 nil check 跳過，但不會 panic。
- `mcp_session_test.go` 的 helper 會改寫 `cfg.AuditLogPath` 到 temp dir；測試結束要關閉 transport 與 audit writer，避免洩漏 goroutine 或 fd。
- 比對 audit log 時建議用 `strings.Contains` 檢查 `"tool":"<name>"`，不要直接 unmarshal 整個 JSONL。
