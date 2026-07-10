# MCP 整合 — 雲端模式（atlas-go 部署雲端，外部 agent 透過 reverse proxy）

> **適用場景**: atlas-go 部署在雲端（Kubernetes / VM / container），外部用戶電腦安裝的 AI agent（Hermes / OpenClaw / Claude Desktop / Cursor / OpenCode）需要從遠端接入。
>
> **本機模式**: 如果 atlas-go 與 agent 在同一台機器，請改讀 [`mcp-integration-LOCAL.md`](./mcp-integration-LOCAL.md)。
>
> **狀態**: **🚧 SCAFFOLD — 細節待補（預計 1-2 個月後）**

---

## 何時該用這份文件

- atlas-go 部署在雲端（不是本機 dev）
- 外部用戶透過 HTTPS 接入
- 需要 TLS / reverse proxy / token rotation
- 多個用戶共享同一個 atlas-go instance

---

## ⚠️ 本文件狀態

本文件是 **scaffold（骨架）**。雲端部署涉及的具體細節（K8s manifest / nginx config / Cloudflare tunnel / token rotation schedule / 監控告警）將在 atlas-go 雲端部署進入穩定階段後補寫。

**預計填寫時間**: 2026-08 ~ 2026-09（atlas-go 雲端部署跑出穩定績效後）

**在此之前**: 雲端部署請直接讀 `internal/apigateway/CONSTITUTION.md` + `docs/operations/l2-4-runbook.md` 作為 framework 入口，並諮詢 ops 同事。

---

## 規劃中的章節（TODO）

### 1. 架構總覽（TODO）

```
[外部 agent] → HTTPS → [reverse proxy: TLS + auth] → [atlas-mcp: streamable-HTTP] → [atlas-go HTTP API]
                                              ↓
                                         [audit log]
```

要點：
- atlas-mcp 支援 streamable-HTTP transport（Phase 4 已 ship，見 `cmd/atlas-mcp/server/transport.go`）
- 但 MCP server 仍 bind `127.0.0.1`（per `internal/apigateway/CONSTITUTION.md` 安全邊界）
- 雲端接入必須透過 reverse proxy（如 nginx / Caddy / Cloudflare Tunnel）

### 2. reverse proxy 設定（TODO）

> 待補：nginx / Caddy / Cloudflare Tunnel 三選一的設定範例。
>
> 關鍵要求：
> - TLS 終止在 reverse proxy
> - Bearer token 驗證
> - 限制來源 IP（per audit policy）
> - WebSocket upgrade（給 streamable-HHTTP 用）

### 3. Token rotation（TODO）

> 待補：cloud-side token store + client-side refresh 機制。
>
> 參考：`cmd/atlas-mcp/server/auth_db_pg.go`（PG-backed TokenStore）+ `auth.go` 的 3-tier fallback。

### 4. 監控告警（TODO）

> 待補：哪些 metric 要暴露給 Prometheus / 如何 alert / dashboard 配置。
>
> 參考：`docs/operations/l2-4-runbook.md` 的 observability 段落。

### 5. 已知限制（TODO）

- atlas-mcp 預設 bind `127.0.0.1:9090`（admin server），**不應**對外暴露
- 雲端接入必須用 streamable-HTTP transport + reverse proxy
- 大量並發用戶需要 token quota 機制（per-user rate limit）

---

## 權威來源（即便 scaffold 也先讀這些）

| 文件 | 用途 |
|------|------|
| [`internal/apigateway/CONSTITUTION.md`](../internal/apigateway/CONSTITUTION.md) | 數據源憲法：6 條文 + 3 附錄，所有外部 API 必須透過 Gateway |
| [`docs/specs/agent-mcp-server.md`](../docs/specs/agent-mcp-server.md) | MCP server 設計規格、安全邊界、JSON Schema |
| [`cmd/atlas-mcp/server/transport.go`](../cmd/atlas-mcp/server/transport.go) | 三種 transport 實作（stdio / SSE / streamable-HTTP） |
| [`docs/operations/l2-4-runbook.md`](../docs/operations/l2-4-runbook.md) | L2.4 觀察窗口 runbook（含部分雲端操作指引） |
| [`cmd/atlas-mcp/server/auth.go`](../cmd/atlas-mcp/server/auth.go) | 3-tier auth 鏈（DB → env → dev mode） |
| [`cmd/atlas-mcp/server/AGENTS.md`](../cmd/atlas-mcp/server/AGENTS.md) | MCP server 模組陷阱（66 個 .go 檔案的 hot-path） |

---

## 雲端部署 checklist（規劃中）

- [ ] 部署 atlas-go 到 K8s / VM
- [ ] 設定 reverse proxy + TLS
- [ ] 設定 PG-backed TokenStore（多用戶 token 管理）
- [ ] 設定 audit log retention + forwarding
- [ ] 設定 Prometheus metric scrape
- [ ] 設定告警（health 401 / circuit breaker / tool count drift）
- [ ] 設定 token rotation schedule
- [ ] 設定 per-user rate limit
- [ ] load testing（91 個 tool × N 個並發用戶）
- [ ] 安全 audit（OWASP Top 10 對照）

---

## 聯絡

- **Atlas ops 同事**: 雲端部署細節諮詢
- **Security team**: auth / token / 監控 review
- **Dev team**: MCP server 行為問題

**Owner of this doc**: 待指派（建議 ops lead）

---

**版本**: v0.0.0.32+ SCAFFOLD | **更新策略**: 雲端部署進入穩定階段後每 2 週更新
