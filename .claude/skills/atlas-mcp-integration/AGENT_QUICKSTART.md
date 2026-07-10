# atlas-mcp Agent Quickstart（5 分鐘接入 SOP）

> **你是第一次接入 atlas-mcp 的 AI agent？** 從這裡開始。本文件是 50 行 copy-paste-ready 設定範例，與 [`cmd/atlas-mcp/README.md`](../../../cmd/atlas-mcp/README.md)（source of truth）對齊。

## 1. 前置需求

- atlas-go backend 已啟動於 `http://127.0.0.1:18080`（`curl http://127.0.0.1:18080/health` 應回 `{"status":"ok"}`）
- 已編譯的 atlas-mcp binary 位於 `/absolute/path/to/bin/atlas-mcp`（執行 `make build-mcp` 或 `go build -o bin/atlas-mcp ./cmd/atlas-mcp/`）
- 環境變數 `ATLAS_API_KEY`（聯絡 atlas-go 管理員取得 admin key）

## 2. 必要環境變數

| 變數 | 必填 | 預設 | 用途 |
|------|------|------|------|
| `ATLAS_BASE_URL` | 否 | `http://127.0.0.1:18080` | atlas-go HTTP API 基底 URL |
| `ATLAS_API_KEY` | 是（admin endpoint） | — | 以 `X-API-Key` header 轉發至 atlas-go |
| `ATLAS_MCP_TOKEN` | SSE/HTTP 必填 | — | Bearer token；stdio 可選 |
| `ATLAS_MCP_AUDIT_LOG` | 否 | `$TMPDIR/atlas-mcp-audit.log` | JSONL audit log |

> 完整 env var 表（30+ 個）見 `cmd/atlas-mcp/README.md` §配置。

## 3. 五種 Client 設定範例（直接 copy-paste）

### A. Hermes（`~/.hermes/config.yaml`）

```yaml
mcp_servers:
  atlas-go:
    command: "/absolute/path/to/bin/atlas-mcp"
    env:
      ATLAS_BASE_URL: "http://127.0.0.1:18080"
      ATLAS_API_KEY: "your-api-key-here"
```

重啟 Hermes 後執行 `hermes mcp test atlas-go` 驗證。

### B. OpenClaw（`~/.openclaw/mcp.json`）

```json
{
  "atlas-go": {
    "type": "stdio",
    "command": "/absolute/path/to/bin/atlas-mcp",
    "env": {
      "ATLAS_BASE_URL": "http://127.0.0.1:18080",
      "ATLAS_API_KEY": "your-api-key-here"
    }
  }
}
```

### C. Claude Desktop（macOS：`~/Library/Application Support/Claude/claude_desktop_config.json`）

```json
{
  "mcpServers": {
    "atlas-go": {
      "command": "/absolute/path/to/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "your-api-key-here"
      }
    }
  }
}
```

需完全退出並重啟 Claude Desktop。

### D. Cursor（`~/.cursor/mcp.json`）

與 Claude Desktop 同樣 JSON 格式，wrapper 用 `mcpServers`。

### E. OpenCode CLI（`~/.config/opencode/opencode.json`）

```json
{
  "mcp": {
    "atlas-go": {
      "type": "local",
      "command": ["/absolute/path/to/bin/atlas-mcp"],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "your-api-key-here"
      }
    }
  }
}
```

注意 OpenCode 用 `mcp`（不是 `mcpServers`）且 `command` 是 array。

## 4. 三步驗證

```bash
# Step 1: 確認 binary 可執行
/absolute/path/to/bin/atlas-mcp --help    # 不應 crash

# Step 2: 確認 atlas-go backend 健康
curl -fsS http://127.0.0.1:18080/health

# Step 3: 確認 MCP 連線（從 client 端）
hermes mcp test atlas-go          # 應列出 91 個 tool
# 或在 Claude Desktop / Cursor 內嘗試調用任一 tool
```

## 5. 進階設定

- **TLS / Reverse proxy**：見 `cmd/atlas-mcp/README.md` §安全提醒
- **Token rotation**：DB TokenStore 模式（`DATABASE_URL` 啟用），見 `cmd/atlas-mcp/server/auth_db.go`
- **Rate limiting**：`ATLAS_MCP_RATE_LIMIT_PER_MINUTE=60`
- **Audit log retention**：`ATLAS_MCP_AUDIT_RETENTION_DAYS=90`

## 6. 常見問題

| 症狀 | 原因 | 解法 |
|------|------|------|
| `command not found` | binary path 錯誤 | 確認 `/absolute/path/to/bin/atlas-mcp` 存在且有執行權限 |
| 連線 401 | `ATLAS_API_KEY` 錯誤 | 跟管理員確認 admin key |
| 看不到 91 個 tool | client 還在用舊 config | 編輯 config 後**重啟 client**（Hermes 可用 `/reload-mcp`） |
| tool 呼叫 timeout | atlas-go backend 沒啟動 | `curl http://127.0.0.1:18080/health` 確認 |

## 7. 互動式 Wizard（推薦）

若不想手刻 config，可用 `make setup-mcp` 啟動互動式 wizard，自動偵測 client、產生 config、驗證連線。需先合併 PR #3（`cmd/atlas-mcp-setup`）。

---

**文件歷史**：v1.0 建立於 2026-07-10（PR #1 of atlas-mcp onboarding 2026 Q3）。
