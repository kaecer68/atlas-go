---
name: atlas-mcp-integration
description: "Teach external AI agents how to connect to atlas-go via MCP protocol — configuration, authentication, first call, and common task patterns."
version: "1.1"
category: "feature"
auto_load: false
load_policy: "manual_only"
created: "2026-07-02"
updated: "2026-08-13"
target_audience: "developer"
---

# Atlas MCP Integration — 外部 AI Agent 接入指引

## 描述（Description）

本技能教任何 MCP-compatible AI Agent（Claude Desktop、Cursor、OpenCode、OpenClaw、Hermes 等）如何：
1. 配置 MCP client 接入 atlas-go 的 MCP 伺服器（`cmd/atlas-mcp`）
2. 進行第一次呼叫並驗證連通
3. 掌握常見投資研究任務的 tool 組合（daily briefing、risk review、experiment evaluation）

同時也是 **atlas-go 內部開發 agent 的 introspection 入口**：透過 `atlas://` resources 與
self-observability 工具（`audit_state`、`mcp_get_*`、`system_get_maturity` 等）自我檢查
系統結構、工作流與 MCP 用量，不必只靠 grep 原始碼（見 § 內部開發 agent 使用）。

本技能不包含 tool 的詳細清單 — tool 導覽請見 [`atlas-mcp-tool-tour`](../atlas-mcp-tool-tour/SKILL.md)。

## 何時觸發（When to Trigger）

- 當使用者說「幫我接上 atlas」「接入 atlas-mcp」「設定 atlas MCP server」
- 當外部 AI agent 第一次接觸 atlas-go 專案，需要知道「從哪裡開始呼叫」
- 當 agent 需要查詢台灣市場狀態、策略績效、風險指標、實驗結果
- 當 agent 看到 `ATLAS_BASE_URL`、`ATLAS_API_KEY` 環境變數但不知道如何配置 MCP client

## 核心概念（Core Concepts）

### MCP (Model Context Protocol)
- **定義**：標準化的 AI agent ↔ 伺服器協議（JSON-RPC 2.0），讓 agent 透過 tools/resources/prompts 與 atlas-go 互動
- **在 Atlas 系統中的實作位置**：`cmd/atlas-mcp/`（Go binary，stdio transport），透過 HTTP 轉發至 `cmd/atlas` 主程式的 REST API
- **與其他技能的關聯**：本技能是「外部 agent 入口」層，下游依賴 `atlas-mcp-tool-tour`（tool 導覽）

### Transport 與 Auth
- **stdio transport**（唯一生產路徑）：agent parent process 透過 stdin/stdout 與 atlas-mcp 通訊。process isolation 即安全邊界
- **TokenAuth**：`ATLAS_MCP_TOKEN` 環境變數（stdio 為 forward-looking；SSE/HTTP transport 啟用後強制）
- **API Key**：`ATLAS_API_KEY` 以 `X-API-Key` header 轉發至 atlas-go admin endpoints
- **安全邊界**：MCP server 只 bind 127.0.0.1；不應對外暴露

## 數據來源（Data Sources）

| 數據 | 模組/檔案 | 說明 |
|------|----------|------|
| MCP server binary | `cmd/atlas-mcp/main.go` | 入口，flag 解析，transport 啟動 |
| Tool 實作 | `cmd/atlas-mcp/server/tools_*.go`（26 個檔案）+ `cmd/atlas-mcp/server/tools.go`（核心 entry-point） | 117 個 tool 的 handler |
| Agent 文件 | `docs/investor/README.md`、`docs/reference/tool-catalog.md` | 入門 + tool catalog |
| MCP 規格 | `docs/specs/agent-mcp-server-spec.md` | 設計規格、安全邊界、JSON Schema |

## 實作位置（Implementation Locations）

| 概念 | 檔案路徑 | 關鍵函數/結構 |
|------|---------|-------------|
| MCP 入口 | `cmd/atlas-mcp/main.go` | `main()` |
| Transport | `cmd/atlas-mcp/server/server.go` | MCP server lifecycle |
| Auth | `cmd/atlas-mcp/server/auth.go` | `TokenAuth` |
| Tool 註冊 | `cmd/atlas-mcp/server/tools.go` | `registerTools()` |

## 使用範例（Usage Examples）

### 範例 1: 配置 Claude Desktop 接入 atlas

```json
// ~/.config/Claude Desktop/claude_desktop_config.json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/absolute/path/to/bin/atlas-mcp",
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "xxx"
      }
    }
  }
}
```

重新啟動 Claude Desktop 後，agent 即可使用 `regime_get_history`、`strategy_list_active` 等 117 個 tool。

### 範例 2: 配置 OpenCode 接入 atlas

```json
// opencode.json
{
  "mcp": {
    "atlas-mcp": {
      "type": "local",
      "command": ["/absolute/path/to/bin/atlas-mcp"],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "xxx"
      }
    }
  }
}
```

### 範例 3: 第一次呼叫 — 驗證連通 + 每日簡報

```
Agent: system_get_health()
→ atlas 回傳健康狀態 → 確認連通

Agent: regime_get_history() + crossmarket_get_status() + narrative_get_bundle()
→ 取得今日市場體制、跨市場狀態、編譯好的 briefing bundle
→ 用這三筆資料生成每日投資簡報
```

## 內部開發 agent 使用（Internal Introspection）

本節適用於 **atlas-go 內部開發 agent**（Claude Code、prime-agent、OpenCode 等）的自我檢查：
與其只 grep 原始碼，優先透過 atlas-mcp 的 introspection 能力掌握系統結構與工作流。

### 1. 用 resources 掌握靜態結構（不需 backend）

| Resource URI | 內容 | 何時用 |
|--------------|------|--------|
| `atlas://tools/catalog` | 全部 MCP tool 的分群清單（來源 `docs/reference/tool-catalog.md`） | 想確認「有哪些工具可用、該用哪個」 |
| `atlas://workflows/catalog` | WA-XXX 工作流地圖（來源 `docs/reference/workflow-map.md`） | 想把「意圖」對應到正確 Tool |
| `atlas://audit/recent` | 最近 50 筆本地 audit log（JSONL，最新在前） | 除錯近期 agent 活動，免另開 log query |

以上 resources 直接讀本地檔案/audit log，**backend（:18080）未啟動也可用**。

### 2. 用 self-observability 工具做內部檢查

| Tool | 用途 | backend 依賴 |
|------|------|-------------|
| `audit_state` | 憲章審計追蹤快照（22 audit items + F1-F5/M1-M6/X1-X3 governance） | ❌ 不需（embedded snapshot） |
| `mcp_get_call_stats` / `mcp_get_session_topology` / `mcp_get_top_slow_tools` / `mcp_get_tenant_usage` | MCP 呼叫統計、agent×tool 矩陣、最慢工具、tenant 用量 | ❌ 不需（讀本地 audit JSONL + in-memory 聚合） |
| `system_get_health` | 整體系統健康（HTTP `/api/dashboard/system-health`） | ✅ 需要 |
| `system_get_maturity` | 各模組成熟度評級（S/E/X/U） | ✅ 需要 |

> 已知限制：stdio transport 目前不會注入 agent_id，`mcp_get_session_topology` 的 agent 會顯示為 `anonymous`。

### 3. prime-agent 註冊範例（settings.json mcpServers）

```jsonc
// ~/.prime/agent/settings.json（或專案 .prime/agent/settings.json）
{
  "mcpServers": {
    "atlas-mcp": {
      "type": "stdio",
      "command": "/absolute/path/to/bin/atlas-mcp",
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "xxx"
      }
    }
  }
}
```

> 註：prime-agent 目前的 `McpIntegration` 層僅把 `type: "http"` 的 server 接進 kernel；stdio 條目遵循
> 標準 MCP client 格式（Claude Desktop / Cursor / OpenCode 同形），prime-agent 的 stdio kernel 接線仍在 roadmap。
> 若你的 client 不支援 stdio，可將 atlas-mcp 以 HTTP/SSE transport 啟動後改註冊 `type: "http"`。

### 4. backend 依賴與降級

- **靜態 introspection**（`atlas://tools/catalog`、`atlas://workflows/catalog`、`atlas://audit/recent`、`audit_state`、`mcp_get_*`）：backend 未啟動仍可用 — 資料在本地檔案/audit log。
- **live 資料**（`system_get_health`、`system_get_maturity`、市場/策略工具、`atlas://config/parameters` 等動態 resources）：需要 backend 於 `ATLAS_BASE_URL`（預設 `http://127.0.0.1:18080`）正常服務，否則回傳 upstream error。
- **建議順序**：先讀 `atlas://tools/catalog` + `atlas://workflows/catalog` 建立全局理解 → 依需求呼叫對應工具；backend 未啟動時先做靜態檢查（`audit_state`、`mcp_get_*`），等 backend 起來再做 live 檢查。

## 驗證規則（Validation Rules）

- [ ] MCP client 配置中 `ATLAS_BASE_URL` 指向運作中的 atlas-go HTTP API
- [ ] `ATLAS_API_KEY` 已設定（admin endpoints 需要）
- [ ] atlas-mcp binary 可獨立執行（`./bin/atlas-mcp --version` 回傳版本號）
- [ ] `system_get_health` tool 呼叫成功回傳（連通性驗證）

## 相關技能（Related Skills）

| 技能 | 關聯 |
|------|------|
| `atlas-mcp-tool-tour` | 117 個 tool 的分群導覽 — 接入成功後的下一步 |
| `atlas-pre-change-protocol` | 若需修改 atlas-mcp 程式碼，修改前必跑 |
| `atlas-risk-management` | 風險相關 tool（`risk_get_*`）的金融背景 |

> **任務→工具對照** — 接入完成後查 [`docs/reference/tool-catalog.md`](../../../docs/reference/tool-catalog.md) 末段「任務 → Tool 反向索引」（12 種典型任務 × 首選 tool）。

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.1 | 2026-08-13 | 新增「內部開發 agent 使用」— resources 導覽（tools/workflows catalog）、self-observability 工具（audit_state / mcp_get_* / system_get_maturity）、prime-agent stdio 註冊範例、backend 依賴與降級說明 |
| 1.0 | 2026-07-02 | 初版 — 涵蓋 stdio transport、Claude/Cursor/OpenCode 配置、首次呼叫範例 |
