# MCP 整合 — 本機模式（atlas-go + agent 在同一台機器）

> **適用場景**: 開發者本機或單一伺服器同時跑 atlas-go backend 與 AI agent（Hermes / OpenClaw / Claude Desktop / Cursor / OpenCode）。
>
> **前提**: atlas-go backend 已在本機 port 18080 跑起來；`bin/atlas-mcp` 已編譯完成。
>
> **雲端模式**: 如果你的 atlas-go 部署在遠端伺服器，雲端整合指南尚在規劃中；請先參考 [`docs/operations/mcp-deploy.md`](./operations/mcp-deploy.md) 與 `internal/apigateway/CONSTITUTION.md`。

---

## 1. 環境準備

### 1.1 啟動 atlas-go backend

```bash
# 啟動 docker deps + go run 原生 backend
make dev
# 或直接
go run ./cmd/atlas -api
```

驗證 backend 在跑：
```bash
curl -fsS http://127.0.0.1:18080/health
# 應回: {"status":"ok",...}
```

### 1.2 編譯 atlas-mcp binary

```bash
make build-mcp
# 產出: bin/atlas-mcp (~25MB)
ls -lh bin/atlas-mcp
```

### 1.3 健康檢查

```bash
make mcp-status
# 預期輸出:
#   ✓ binary:        bin/atlas-mcp (25M)
#   ✓ atlas-go:      http://127.0.0.1:18080 (status: ok)
#   ✓ LLM router:    http://127.0.0.1:18080/api/llm/health (router: v2.x)
```

---

## 2. 互動式設定（推薦）

```bash
make setup-mcp
```

Wizard 會：
1. 偵測已安裝的 MCP 客戶端（掃 `~/.hermes/` / `~/.openclaw/` / `~/Library/Application Support/Claude/` / `~/.cursor/` / `~/.config/opencode/`）
2. 列出找到的 client 讓你選
3. 跑 4 個 health probe
4. 寫入正確的 config snippet（mode 0600）
5. 印出後續步驟（重啟 / reload / 驗證指令）

詳細文件：[`cmd/atlas-mcp-setup/README.md`](../cmd/atlas-mcp-setup/README.md)

---

## 3. 手動設定（如果不想用 wizard）

每個 client 的設定檔位置不同，env var 統一是 `ATLAS_BASE_URL` / `ATLAS_API_KEY`。

### 3.1 Hermes（`~/.hermes/config.yaml`）

```yaml
mcp_servers:
  atlas-mcp:
    command: /absolute/path/to/atlas-go/bin/atlas-mcp
    env:
      ATLAS_BASE_URL: http://127.0.0.1:18080
      ATLAS_API_KEY: "your-admin-key"
    enabled: true
```

驗證：
```bash
hermes mcp reload
hermes mcp test atlas-mcp
# 預期: 列出 116 個 tool（範圍 [115, 118]）
```

### 3.2 OpenClaw（`~/.openclaw/mcp.json`）

```json
{
  "mcp": {
    "servers": {
      "atlas-mcp": {
        "type": "stdio",
        "command": "/absolute/path/to/atlas-go/bin/atlas-mcp",
        "env": { "ATLAS_BASE_URL": "http://127.0.0.1:18080" }
      }
    }
  }
}
```

驗證：
```bash
openclaw mcp reload
openclaw mcp list
```

### 3.3 Claude Desktop（macOS）

檔案：`~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/absolute/path/to/atlas-go/bin/atlas-mcp",
      "env": { "ATLAS_BASE_URL": "http://127.0.0.1:18080" }
    }
  }
}
```

**注意**: Claude Desktop 沒有 hot-reload MCP config。**完全退出並重新啟動** Claude Desktop 才會生效。

### 3.4 Cursor

檔案：使用者全域 `~/.cursor/mcp.json` 或專案 `.cursor/mcp.json`

```json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/absolute/path/to/atlas-go/bin/atlas-mcp",
      "env": { "ATLAS_BASE_URL": "http://127.0.0.1:18080" }
    }
  }
}
```

驗證：Cursor 命令面板執行 `MCP: Reload`。

### 3.5 OpenCode CLI

檔案：`~/.config/opencode/opencode.json`

```json
{
  "mcp": {
    "atlas-mcp": {
      "type": "local",
      "command": ["/absolute/path/to/atlas-go/bin/atlas-mcp"],
      "env": { "ATLAS_BASE_URL": "http://127.0.0.1:18080" }
    }
  }
}
```

驗證：
```bash
opencode mcp list
# 或互動驗證
opencode mcp debug atlas-mcp
```

---

## 4. 必設環境變數速查

| 變數 | 必要 | 預設 | 說明 |
|------|------|------|------|
| `ATLAS_BASE_URL` | 是 | `http://127.0.0.1:18080` | atlas-go HTTP API 位置 |
| `ATLAS_API_KEY` | 看 tier | 空 | admin API key（premium tier 需要） |
| `ATLAS_MCP_TOKEN` | stdio 選填 | 空 | stdio 模式的 token fallback |
| `ATLAS_MCP_AUDIT_LOG` | 否 | `$TMPDIR/atlas-mcp-audit.log` | JSONL audit log |
| `ATLAS_MCP_RATE_LIMIT_PER_MINUTE` | 否 | `120` | per-tool 限流；`0` = 停用 |

**已廢棄**（不要再用）：`ATLAS_WORK_DIR` / `ATLAS_DATABASE_URL` / `ATLAS_REDIS_URL` / `ATLAS_API_TOKEN`。

---

## 5. 三步驗證 SOP

```bash
# 1. binary 存在
ls -lh bin/atlas-mcp

# 2. backend 在跑
curl -fsS http://127.0.0.1:18080/health

# 3. MCP server 啟動時印出 tool count
bin/atlas-mcp 2>&1 | head -3
# 預期: 看到 "atlas-mcp: registered N tools" 且 N ∈ [115, 118]
```

---

## 6. 常見問題

| 症狀 | 解法 |
|------|------|
| 看不到 tools | 確認 client 重啟或 reload（見 §3 各 client 驗證步驟） |
| Tool count = 0 | atlas-go backend 沒跑或 `ATLAS_BASE_URL` 設錯 |
| 401 Unauthorized | 設 `ATLAS_API_KEY` 或用免費 tier 工具（`mcp_quickstart`/`macro_*`/`event_*`） |
| binary not found | 跑 `make build-mcp` |
| 連線失敗 | `curl http://127.0.0.1:18080/health` 先確認 backend 通 |

更多除錯：[`cmd/atlas-mcp/README.md`](../cmd/atlas-mcp/README.md) §常見問題。

---

## 7. 相關資源

- **5 分鐘 SOP（給 agent 用）**: [`.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`](../.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md)
- **完整 MCP 指南**: [`cmd/atlas-mcp/README.md`](../cmd/atlas-mcp/README.md)
- **116 個 tool 總覽**: [`docs/reference/tool-catalog.md`](./reference/tool-catalog.md)
- **Setup wizard 詳解**: [`cmd/atlas-mcp-setup/README.md`](../cmd/atlas-mcp-setup/README.md)

---

**版本**: v0.0.0.32+（116 tools）| **授權**: GNU AGPL v3
