# MCP 對外整合修正與 2026-07-28 規格遷移 Roadmap

> **Status**: Roadmap document. Captures (1) the atlas-mcp HTTP integration gap discovered during the 2026-08-01 hermes investigation, and (2) the migration plan for the MCP `2026-07-28` specification (stateless core), which GA'd on 2026-07-28.
> **Owner**: atlas-go maintainers. **Created**: 2026-08-01.

---

## 1. 背景：盤查發現的事實

### 1.1 事故：`daily-atlas-health.py` 呼叫失敗（已修）

hermes 的 cron 腳本 `~/.hermes/scripts/daily-atlas-health.py` 的 `mcp_call()` 向
`http://127.0.0.1:18080/mcp` POST JSON-RPC，期望呼叫 atlas-mcp 的 tool。**這個 path 不存在**：

- atlas-go 主程式（port 18080）**沒有** `/mcp` route（grep `cmd/atlas/` + curl 實測 404 `page not found`）
- atlas-mcp 是獨立 process，hermes 以 **stdio transport** 方式 spawn（`~/.hermes/config.yaml` → `mcp_stdio_watchdog.py`），MCP JSON-RPC 走 stdin/stdout pipe，**不對外開放 HTTP**
- atlas-mcp 的 streamable-HTTP transport 存在（`-transport streamable-http`，預設 port 9090），但 **hermes 從未啟用**，且 port 9090 已被 Prometheus 佔用

**根因**：hermes 誤解「MCP server 一定有 HTTP 端點」。atlas-go 文件（`cmd/atlas-mcp/README.md`、`docs/mcp-integration-local.md`）均正確描述 stdio 為預設 transport、HTTP 需顯式啟用；**文件無誤導**。這是 hermes 側的記憶錯誤。

**Phase 0 修法（已完成，2026-08-01）**：腳本改直連 atlas-go REST API，繞過 MCP 層。atlas-mcp 本身就是 MCP→REST 翻譯層，腳本直接呼叫 REST 少一層轉發。對應關係：

| 原 MCP tool | REST endpoint | 驗證 |
|---|---|---|
| `system_get_health` | `GET /api/dashboard/system-health` | 200 ✓ |
| `scheduler_get_status` | `GET /api/scheduler/status` | 200 ✓（回傳 JSON array）|
| `system_get_data_pipeline` | `GET /api/dashboard/data-pipeline` | 200 ✓ |
| `universe_get_sessions` | `GET /api/dashboard/sessions` | 200 ✓ |
| `daily_report` | `GET /api/reports/latest` | 200 ✓ |

auth：atlas-go 主程式 `AuthMiddleware` 接受 `Authorization: Bearer <key>` 或 `X-API-Key: <key>`（`internal/monitoring/api/shared/handler.go:125`）。`/api/dashboard/system-health` 為 `auth_required: false`，其餘需 key。

### 1.2 盤查暴露的技術債（本 roadmap 的實質內容）

hermes 發現的問題本身不是設計缺陷，但盤查暴露了 atlas-mcp 的真實技術債：

| # | 項目 | 現況 | 新規格狀態 | 嚴重度 |
|---|---|---|---|---|
| T1 | go-sdk 版本 | `v1.6.1`（`go.mod`）| 新規格需 `v1.7.0-pre.1+`（beta）| 中 |
| T2 | Roots 支援 | 實作中（`roots.go`，active）| **Deprecated**（12 個月）| 高 |
| T3 | Legacy HTTP+SSE transport | `ServeSSE` 實作中 | **Deprecated**（一年 offramp）| 低 |
| T4 | `initialize`/session | 依賴舊 SDK 自動處理 | 移除（stateless core）| 低（SDK 升級自動）|
| T5 | `Mcp-Method`/`Mcp-Name` headers | 未實作 | 新要求 | 低（SDK 升級自動）|
| T6 | Auth hardening（RFC 9207 iss）| 未實作 | 新要求 | 中（僅 OAuth 啟用時）|
| T7 | streamable-HTTP 從未啟用 | 可啟用未啟用 | 官方 transport | 決策點 |

---

## 2. 2026-07-28 規格遷移評估

### 2.1 時間壓力評估（沒有想像中急迫）

官方公告（GA blog post + SDK beta post）明確三點：

1. **7/28 是規格文字發布日，不是斷電日**：現有 client/server 不受影響（"nothing breaks today, and nothing breaks on July 28 either"）
2. **12 個月最小 deprecation window**：roots/sampling/logging 仍可用至少一年
3. **Go SDK 遷移是顯式 opt-in**：`StreamableHTTPOptions.Stateless = true` 才說 `2026-07-28`，不設則協商降到 `2025-11-25`；新 client 遇舊 server 自動 fallback 到 `initialize` handshake

**結論**：至少 12 個月緩衝期，無須緊急遷移。但 deprecated 項目（T2/T3）需要在 offramp 結束前決策。

### 2.2 遷移成本

Go SDK 遷移路徑最溫和（官方明說 "no package split and no rework of the API you use day to day"）：同 module path、同 API。

```bash
go get github.com/modelcontextprotocol/go-sdk@v1.7.0-pre.1   # 或等 stable
```

---

## 3. 改善計劃（Phases）

### Phase 0 — 立即（2026-08-01 已完成）

**Goal**: 修復 `daily-atlas-health.py`，消除對不存在 `/mcp` endpoint 的依賴。

- [x] 腳本 `mcp_call()` → `rest_get()`，直連 5 個 REST endpoints
- [x] 解析邏輯對應 REST 扁平結構（MCP handler 的 `{result, info}` wrapper 不存在）
- [x] 端到端實測：5 段報告全部正常產出
- [ ] **hermes 側記憶更正**（kaecer 在 hermes 端執行）：MCP server 預設 stdio、無 HTTP 端點；腳本/程式呼叫 atlas-go 用 REST API 而非 MCP

**Acceptance criteria**:
- `python3 ~/.hermes/scripts/daily-atlas-health.py` 在交易日產出完整報告，無 `[FAILED]`
- 腳本不再引用 `/mcp` path

### Phase 1 — 本季度（2026 Q3）

**Goal**: 確立 MCP 對外策略 + deprecated 項目決策。

| 任務 | 內容 | 決策點 |
|---|---|---|
| P1-1 | **MCP 對外 HTTP 策略**：是否啟用 streamable-http | 有外部 HTTP client 需求才啟用；hermes 走 stdio 正常運作，無迫切需求。若啟用：`ATLAS_MCP_TRANSPORT=streamable-http` + `ATLAS_MCP_ADDR=127.0.0.1:9091`（9090 屬 Prometheus）+ `ATLAS_MCP_TOKEN` |
| P1-2 | **Roots 去留**（T2）| 評估 roots 實際使用者（hermes roots? CLI?）；無 → 排入移除；有 → 文件化 12 個月內遷移 |
| P1-3 | **SSE 去留**（T3）| 確認無 client 依賴 SSE → 排入移除清單 |
| P1-4 | **腳本 REST 化的知識固化**：在 `docs/operations/` 加「REST vs MCP 呼叫」指引 | 防止其他 agent 重蹈 hermes 覆轍 |

**Acceptance criteria**:
- P1-1: 決策記錄（啟用或明確不啟用）+ 理由
- P1-2/P1-3: roots/SSE 的保留或移除決策，附證據
- P1-4: 指引文件 merge

### Phase 2 — 6-12 個月（2026 Q4 – 2027 Q2）

**Goal**: 完成 2026-07-28 規格遷移。

| 任務 | 內容 | 前置條件 |
|---|---|---|
| P2-1 | go-sdk 升級 `v1.7+`（stable）| v1.7 stable 釋出 |
| P2-2 | 視 hermes client 版本決定切 `Stateless = true` | hermes 側 MCP client 支援新規格 |
| P2-3 | 移除 deprecated roots（若 P1-2 決策移除）| P1-2 完成 + offramp 前 |
| P2-4 | 移除 SSE transport（若 P1-3 決策移除）| P1-3 完成 + offramp 前 |
| P2-5 | OAuth/EMA 啟用時處理 RFC 9207 auth hardening | 僅當啟用 OAuth |

**Acceptance criteria**:
- `go.mod` go-sdk ≥ v1.7 stable
- `make check-binaries` + `make ci-full` 通過
- atlas-mcp 啟動 log 顯示預期 transport 與工具數（115-118）
- 若切 Stateless：新規格 client 端到端呼叫通過；舊 client fallback 仍可用

---

## 4. 明確不做的事（Anti-goals）

- **不現在切 Stateless**：go-sdk v1.7 仍 beta，hermes client 未驗證新規格支援。官方建議 critical workload 用 stable。
- **不為此啟用 streamable-http**：除非有真實外部 HTTP client 需求。現有 hermes stdio 運作正常。
- **不改 hermes 的 stdio 架構**：stdio 是新舊規格共同的官方 transport。

---

## 5. 時間線

| 時間 | 里程碑 |
|---|---|
| 2026-08-01 | Phase 0 完成（腳本修復）|
| 2026-08-15 | Phase 1 決策完成（P1-1 至 P1-3）|
| 2026-09-30 | P1-4 指引 merge；Phase 1 close-out |
| 2026 Q4 – 2027 Q2 | Phase 2 執行（SDK 升級 + deprecated 項目移除）|
| 2027-07-28 前 | 所有 deprecated 項目關閉（12 個月 offramp 到期）|

---

## 6. 參考

- [MCP 2026-07-28 GA 公告](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/blog/content/posts/2026-07-28-spec-ga/index.md)
- [SDK beta 公告（遷移細節）](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/blog/content/posts/2026-06-29-sdk-betas-for-2026-07-28.md)
- [MCP Transport Future（stateless roadmap）](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/blog/content/posts/2025-12-19-mcp-transport-future.md)
- `cmd/atlas-mcp/README.md` — atlas-mcp 官方文件（stdio 預設、HTTP 顯式啟用）
- `cmd/atlas-mcp/main.go` — transport 選擇邏輯（CLI flag > env > stdio）
- `cmd/atlas-mcp/server/transport.go` — ServeStdio / ServeSSE / ServeStreamableHTTP
- `internal/monitoring/api/shared/handler.go` — atlas-go auth middleware（Bearer / X-API-Key）
