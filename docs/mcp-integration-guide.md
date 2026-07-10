# atlas-mcp 整合指南

> **⚠️ DEPRECATED（2026-07-10）**：本文件保留作為歷史參考，但**內容已分散到下方兩份權威文件**。Agent 與開發者請改讀新文件：
>
> - **部署、配置、env var 速查**：[`cmd/atlas-mcp/README.md`](../cmd/atlas-mcp/README.md)（source of truth — 與 `cmd/atlas-mcp/main.go` 對齊）
> - **Agent 5 分鐘設定 SOP**：[`.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`](../.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md)（50 行 copy-paste-ready 範例）
>
> 本文件存在的理由：避免外部 bookmark 404；內部已知內容過時，新整合請用上方兩份文件。

---

## 總覽（歷史快照，內容已過時）

`cmd/atlas-mcp` 是 atlas-go 的 MCP (Model Context Protocol) server，提供 **91 個** tools（業務 87 + audit 4，編譯期 assert ∈ [89, 91]），透過 JSON-RPC 2.0 與外部 AI Agent 通訊。支援 **三種 transport**：

- **stdio**（預設，向後相容 Claude Desktop / Cursor / OpenCode）
- **SSE**（已 ship，Phase 4；deprecated by MCP spec 但保留相容）
- **streamable-HTTP**（已 ship，Phase 4；推薦用於 HTTP transport）

## 歷史設定（已過時，僅供考古）

> **下方 JSON 範例的 env var 名稱全部錯誤**。正確的 env var 見 [`cmd/atlas-mcp/README.md` §配置](../cmd/atlas-mcp/README.md#配置)。歷史紀錄保留是為了讓曾經照這份文件設定的人能快速找到「我哪裡做錯了」。

### Claude Desktop（歷史錯誤版本）

```json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/path/to/atlas-go/cmd/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_WORK_DIR": "/path/to/atlas-go",                      // ❌ 不存在
        "ATLAS_DATABASE_URL": "postgres://...",                     // ❌ 應為 DATABASE_URL
        "ATLAS_REDIS_URL": "redis://localhost:6379",               // ❌ 不存在
        "ATLAS_API_TOKEN": "your-mcp-token"                        // ❌ 應為 ATLAS_API_KEY
      }
    }
  }
}
```

**修正後**（見 `cmd/atlas-mcp/README.md` line 108-123）：

```json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/absolute/path/to/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "xxx",
        "ATLAS_MCP_TOKEN": "yyy",
        "ATLAS_MCP_AUDIT_LOG": "/var/log/atlas-mcp/audit.log"
      }
    }
  }
}
```

### OpenClaw / Hermes（歷史錯誤版本）

```json
{
  "atlas-mcp": {
    "type": "stdio",
    "command": "/path/to/atlas-go/cmd/atlas-mcp",
    "env": {
      "ATLAS_WORK_DIR": "/path/to/atlas-go",        // ❌ 不存在
      "ATLAS_API_TOKEN": "your-mcp-token"          // ❌ 應為 ATLAS_API_KEY
    }
  }
}
```

### OpenCode CLI（歷史錯誤版本）

```json
{
  "name": "atlas-mcp",
  "command": "/path/to/atlas-go/cmd/atlas-mcp"     // ❌ OpenCode 應為 type/command 結構
}
```

> 上述三個 client 的**正確** JSON/YAML 範例見 [`AGENT_QUICKSTART.md`](../.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md)。

## 為什麼這份文件被棄用

2026-07-10 hermes agent 嘗試按本文件設定時，發現所有 env var 都是舊版殘留（`ATLAS_WORK_DIR` / `ATLAS_DATABASE_URL` / `ATLAS_REDIS_URL` / `ATLAS_API_TOKEN`），但 `cmd/atlas-mcp/main.go` 實際讀的是 `ATLAS_BASE_URL` / `ATLAS_API_KEY` / `ATLAS_MCP_TOKEN`。文件未跟上 Phase 4 transport 實作（仍寫「開發中」）、工具數從 80+ 增到 91 卻未更新、且未指向 `cmd/atlas-mcp/README.md`（source of truth）。

修訂：見 PR 系列「atlas-mcp onboarding 2026 Q3」。

## 給文件的維護者

- **新增 MCP 設定說明**：請改寫到 `cmd/atlas-mcp/README.md`（與 main.go 同 package，最容易保持同步）
- **新增 client 範例**：請改寫到 `.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`
- **不建議**更新本文件（已標 deprecated，留作歷史）

---

**檔案歷史**：建立於 v0.0.0.32（2026-06-30），於 v0.0.0.32 補遺（2026-07-10）標記 deprecated。
