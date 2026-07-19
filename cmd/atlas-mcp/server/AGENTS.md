# AGENTS.md — atlas-mcp/server

> 本模組為 MCP stdio server 實作，負責把 atlas-go HTTP API 包裝成 MCP tool。
> 通用規則（語言、ACI、workspace close）見專案根目錄 `AGENTS.md` 與全域 `~/.config/opencode/AGENTS.md`。

## 快速路由

| 檔案 | 責任 |
|------|------|
| `server.go` | 生命週期、設定驗證、rate limiter / metrics / anomaly emitter 啟動、transport 分派 |
| `transport.go` | Transport 實作：`ServeStdio` / `ServeSSE` / `ServeStreamableHTTP` + 共用 `BearerAuth` middleware |
| `tools.go` | 總註冊入口、`countedAddTool` 計數包裝、`RegisteredToolCount`、`withAudit`/`withAuditExtra` 包裝 |
| `tools_*.go` | 各業務領域 tool 註冊與 handler（macro / crossmarket / narrative / risk_alert / strategy / capitalflow / recommendation / stock / strategy_ranker / experiment / synergy / control / scheduler_task / system / llm_trace / data_universe / report_prism / anomaly / briefing / events 等），共 20+ 個檔案 |
| `sampling.go` | MCP protocol 層：`mcp_sample_llm`（feature-gated，`SamplingEnabled` 控制） |
| `roots.go` | MCP protocol 層：`mcp_roots_list` + `mcp_roots_read_file`（filesystem boundary） |
| `elicitation.go` | MCP protocol 層：`mcp_elicit_user`（feature-gated，`ElicitationEnabled` 控制） |
| `audit.go` | 寫入模型 `AuditEntry` + `AuditWriter`（v2 格式為主） |
| `audit_v2.go` | 讀取模型 `AuditEntryV2` + 聚合函式 |
| `auth.go` | `TokenAuth`：env token / DB store fallback / dev mode |
| `auth_db.go` | `TokenStore` 介面與錯誤定義 |
| `auth_db_pg.go` | PostgreSQL 實作，只存 SHA-256 hash |
| `metrics.go` | Prometheus 註冊與 `ObserveCall`/`ObserveAnomaly` |
| `auto-desc.gen.go` | descgen 產出，禁止手動編輯 |
| `elicitation_validate.go` | elicit_user schema server-side pre-validation（大小/屬性/外部 $ref 過濾）|

## 工具計數

> **權威來源**：[`docs/reference/tool-catalog.md`](/docs/reference/tool-catalog.md) 記錄最終對 agent 暴露的 tool 名稱與分類；本節只保留實作層面的計數規則。

- 目前 `registerTools()` 內掛載的業務 tool handler 數量為 **106 個**（sampling / elicitation 兩個 feature-gated tool 關閉時）；`SamplingEnabled` / `ElicitationEnabled` 各再 +1，最高 **108 個**。
- `registerAuditTools()` 另外掛載 **4 個**自我觀測 tool，不計入 `registerTools()` 的範圍。
- `server.Run()` 在所有 tool 註冊完成後 assert `RegisteredToolCount` 在 **111–114** 範圍內；**若不在範圍內直接 return error 阻止啟動**，防止文件↔程式碼漂移。此範圍對應 `docs/reference/tool-catalog.md` 所列對 agent 暴露的 tool 總數。
- `countedAddTool[In, Out any]()` 是 `mcp.AddTool` 的泛型包裝，自動累加 `RegisteredToolCount`；所有 tool 註冊都必須經過它。
- 新增 tool 時：`countedAddTool()` 會自動遞增計數器，但仍應同步更新 `server.go` 與 `tools_transport_sse_test.go` 的上下界，並確認 `docs/reference/tool-catalog.md` 已納入新 tool。
- `registerTokenAdminTools`（admin.go）不計入，因為它用獨立的 `mcp.Server` 實例。

## 命名陷阱

- Tool 名稱慣例為 `area_verb_noun`：例如 `regime_get_history`、`control_get_active_overrides`、`mcp_anomaly_get_recent`。
- `tools.go` 裡 `registerTools` 會再呼叫 **17 個 registerXXX**（14 個 `tools_*.go` + `sampling.go` + `roots.go` + `elicitation.go`）；新增領域時要在這裡加一行，否則 tool 不會掛載。
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

## Transport 陷阱（Phase 4）

- `transport.go` 提供三個 dispatcher：
  - `ServeStdio(ctx, mcpSrv)` — 預設，`mcpSrv.Run(ctx, &mcp.StdioTransport{})` 的薄包裝，向後相容 Claude Desktop / Cursor / OpenCode。
  - `ServeSSE(ctx, mcpSrv, addr, auth)` — MCP 2024-11-05 SSE 規格；**spec 已標 deprecation**，僅作相容保留。
  - `ServeStreamableHTTP(ctx, mcpSrv, addr, auth)` — MCP 2025-03-26 規格，當前 MCP 標準；新部署優先採用。
- HTTP transports 一律走 `BearerAuth(auth)` middleware，token 錯誤回 401；`TokenAuth` 在 dev mode（`MCPToken == ""`）會放行所有請求。
- `extractBearer` 嚴格區分 `Bearer ` prefix（大小寫敏感）；其他 scheme（Basic / Digest / bare）一律回空 token 走 401 路徑。
- HTTP transports 一律 bind `127.0.0.1:`（推薦 `127.0.0.1:9090`），不暴露 0.0.0.0；遠端使用請放 reverse proxy 後。
- `listenHTTP` graceful shutdown 用 5s timeout 處理 ctx cancellation，確保 in-flight request 結束後才退出。
- 設定 `Config.Transport` 為空字串時 fallback 至 `TransportStdio`（向後相容舊部署）。
- HTTP transport 不修改 audit 的 `transport` 欄位邏輯（v2 schema 已預留欄位，目前由 `withAudit` 從 ctx 取值，stdio 為 `"stdio"`，HTTP 路徑在 audit 中標為 `"http"`）。
- **Rate limit 預設 120 req/min**（Phase 6 hardening）：`Config.RateLimitPerMinute` 預設 `120`，`Burst` 預設等於 `PerMinute`。HTTP/SSE 部署不應關閉；stdio 開發若需無限流可設 `ATLAS_MCP_RATE_LIMIT_PER_MINUTE=0`。
- **stdio auth 要求**：stdio 模式雖不強制 Bearer token，但若 process 可被多個 agent / 多個使用者觸發，必須設定 `ATLAS_MCP_TOKEN` 並在 MCP client config 中帶入，否則等同無認證暴露全部 tool。

## 測試陷阱

- 單元測試通常不透過 `Run()` 建立 server，而是像 `tools_anomaly_test.go` 的 `newAnomalyTestHarness` 手動組 `server{cfg, audit, cli, metrics, detector}`。
- 手動組 server 時若沒設 `metrics` 與 `detector`，`withAudit` 裡的 `observeAuditEntry` 會因 nil check 跳過，但不會 panic。
- `mcp_session_test.go` 的 helper 會改寫 `cfg.AuditLogPath` 到 temp dir；測試結束要關閉 transport 與 audit writer，避免洩漏 goroutine 或 fd。
- 比對 audit log 時建議用 `strings.Contains` 檢查 `"tool":"<name>"`，不要直接 unmarshal 整個 JSONL。
