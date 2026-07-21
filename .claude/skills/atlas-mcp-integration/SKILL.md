---
name: atlas-mcp-integration
description: "Teach external AI agents how to connect to atlas-go via MCP protocol — configuration, authentication, first call, and common task patterns."
version: "1.0"
category: "feature"
auto_load: false
load_policy: "manual_only"
created: "2026-07-02"
updated: "2026-07-02"
target_audience: "developer"
---

# Atlas MCP Integration — 外部 AI Agent 接入指引

## 描述（Description）

本技能教任何 MCP-compatible AI Agent（Claude Desktop、Cursor、OpenCode、OpenClaw、Hermes 等）如何：
1. 配置 MCP client 接入 atlas-go 的 MCP 伺服器（`cmd/atlas-mcp`）
2. 進行第一次呼叫並驗證連通
3. 掌握常見投資研究任務的 tool 組合（daily briefing、risk review、experiment evaluation）

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
- **TokenAuth**：`MCPToken` 環境變數（stdio 為 forward-looking；SSE/HTTP transport 啟用後強制）
- **API Key**：`ATLAS_API_KEY` 以 `X-API-Key` header 轉發至 atlas-go admin endpoints
- **安全邊界**：MCP server 只 bind 127.0.0.1；不應對外暴露

## 數據來源（Data Sources）

| 數據 | 模組/檔案 | 說明 |
|------|----------|------|
| MCP server binary | `cmd/atlas-mcp/main.go` | 入口，flag 解析，transport 啟動 |
| Tool 實作 | `cmd/atlas-mcp/server/tools_*.go`（26 個檔案）+ `cmd/atlas-mcp/server/tools.go`（核心 entry-point） | 112 個 tool 的 handler |
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

重新啟動 Claude Desktop 後，agent 即可使用 `regime_get_history`、`strategy_list_active` 等 112 個 tool。

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

## 驗證規則（Validation Rules）

- [ ] MCP client 配置中 `ATLAS_BASE_URL` 指向運作中的 atlas-go HTTP API
- [ ] `ATLAS_API_KEY` 已設定（admin endpoints 需要）
- [ ] atlas-mcp binary 可獨立執行（`./bin/atlas-mcp --version` 回傳版本號）
- [ ] `system_get_health` tool 呼叫成功回傳（連通性驗證）

## 相關技能（Related Skills）

| 技能 | 關聯 |
|------|------|
| `atlas-mcp-tool-tour` | 112 個 tool 的分群導覽 — 接入成功後的下一步 |
| `atlas-pre-change-protocol` | 若需修改 atlas-mcp 程式碼，修改前必跑 |
| `atlas-risk-management` | 風險相關 tool（`risk_get_*`）的金融背景 |

> **任務→工具對照** — 接入完成後查 [`docs/reference/tool-catalog.md`](../../../docs/reference/tool-catalog.md) 末段「任務 → Tool 反向索引」（12 種典型任務 × 首選 tool）。

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-07-02 | 初版 — 涵蓋 stdio transport、Claude/Cursor/OpenCode 配置、首次呼叫範例 |
