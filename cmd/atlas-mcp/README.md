# atlas-mcp

`atlas-mcp` 是 [atlas-go](https://github.com/kaecer68/atlas-go) 的 MCP (Model Context Protocol) 伺服器。讓任何 MCP-compatible AI Agent（Claude Desktop、Cursor、OpenCode、OpenClaw、Hermes 等）透過標準 JSON-RPC 2.0 協議查詢與輕度觸發 atlas-go 的台股投資研究能力。

> **Agent 入門** — 第一次使用？先讀 [`docs/AGENT_ONBOARDING.md`](../../docs/AGENT_ONBOARDING.md)（5 分鐘速讀），再看 [`docs/AGENT_TOOLS.md`](../../docs/AGENT_TOOLS.md)（80 個 tool 的決策樹與完整 catalog）。
> **完整規格** — 80 個 tool 的設計文件、安全邊界、JSON Schema 模板見 [`docs/specs/agent-mcp-server.md`](../../docs/specs/agent-mcp-server.md)。
> **開發者** — 若要在 `cmd/atlas-mcp/server/` 內新增或修改 tool，**必先讀** [`server/AGENTS.md`](./server/AGENTS.md)（模組陷阱文件）。

## 目前規模

| 面向 | 現狀 |
|------|------|
| MCP Tools | **80 個**（16 個 `tools_*.go` 檔案按領域分群 + 5 個核心 entry-point 在 `tools.go`） |
| Tool description | `auto-desc.gen.json`（713 行，由 `cmd/atlas-mcp/descgen/` 自動生成） |
| Transport | **stdio**（生產）；SSE / streamable-HTTP（程式碼就緒，待啟用，見 roadmap P1 殘留） |
| Auth | TokenAuth + DB TokenStore（`auth.go` / `auth_db.go` / `auth_db_pg.go`）+ admin HTTP API（127.0.0.1，`token_admin_handler.go`） |
| Audit | v2 schema（retention、cleanup、ArgsHash、SessionID、Transport，`audit_v2.go`；v1 `audit.go` 為向後相容 shim） |
| 擴充協議 | Resources（`resources.go`）、Prompts（`prompts.go`）、Elicitation（`elicitation.go`）、Sampling（`sampling.go`）、Roots（`roots.go`） |
| 觀測 | Rate limiting（`ratelimit.go`）、Metrics（`metrics.go`）、Anomaly detection（`tools_anomaly.go`） |
| 工具分類 | Macro（6）、Crossmarket（3）、Regime（1）、Narrative（7）、Risk（5）、Alert（4）、Strategy（5）、Experiment（3）、Synergy（3）、Control（4）、Scheduler/Task（4）、System/Health（9）、Data（4）、Universe（2）、LLM（2）、Trace（4）、PRISM/Swarm（6）、Report（4）、Audit（4）、Anomaly（2）= 80 |

## 快速啟動

```bash
# 建置
go build -o bin/atlas-mcp ./cmd/atlas-mcp/

# 啟動（stdio transport）
ATLAS_BASE_URL=http://127.0.0.1:8080 ATLAS_API_KEY=xxx ./bin/atlas-mcp
```

伺服器從 stdin 讀取 JSON-RPC 請求、往 stdout 寫入回應。`ATLAS_BASE_URL` 指向 atlas-go HTTP API（預設 `http://127.0.0.1:8080`）。

## 配置

全部透過環境變數：

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_BASE_URL` | `http://127.0.0.1:8080` | atlas-go HTTP API 基底 URL |
| `ATLAS_API_KEY` | （未設） | 以 `X-API-Key` header 轉發至 atlas-go admin endpoints |
| `ATLAS_MCP_TOKEN` | （未設） | MCP transport 層 token（stdio 為 forward-looking，SSE/HTTP 啟用後強制） |
| `ATLAS_MCP_AUDIT_LOG` | `/tmp/atlas-mcp-audit.log` | JSONL audit log 路徑。父目錄自動建立（mode 0700） |
| `ATLAS_MCP_ALLOWED_ROOTS` | （未設） | MCP client roots 白名單（CSV 格式路徑清單，未設時無限制，見 Phase 4 B roots 擴充協議） |

> **stdio 安全模型**：目前無 transport 層 token 強制執行。process isolation（僅 parent process 可觸及 stdin/stdout）即安全邊界。`TokenAuth` + DB TokenStore 已實作，SSE/streamable-HTTP transport 啟用後強制驗證。

## MCP Client 配置範例

### Claude Desktop（`~/.config/Claude Desktop/claude_desktop_config.json`）

```json
{
  "mcpServers": {
    "atlas-go": {
      "command": "/absolute/path/to/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:8080",
        "ATLAS_API_KEY": "xxx",
        "ATLAS_MCP_TOKEN": "yyy",
        "ATLAS_MCP_AUDIT_LOG": "/var/log/atlas-mcp/audit.log"
      }
    }
  }
}
```

### Cursor（Settings → MCP）

與 Claude Desktop 同樣 JSON 格式。透過 `+ Add new MCP server` 新增。

### OpenCode（`opencode.json`）

```json
{
  "mcp": {
    "atlas-go": {
      "type": "local",
      "command": ["/absolute/path/to/bin/atlas-mcp"],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:8080",
        "ATLAS_API_KEY": "xxx"
      }
    }
  }
}
```

## Tool 命名慣例

```
<area>_<verb>_<noun>?
例：regime_get_history  /  strategy_list_active  /  experiment_judge
```

全 snake_case（與 atlas-go 全專案 JSON tag 慣例一致）。`area` 與 `verb` 必填，`noun` 視 `verb` 是否需要區分對象而定。

## Audit Log 格式（JSONL）

每行一個 tool call：

```json
{"ts":"2026-06-30T08:00:13Z","tool":"regime_get_history","arg_keys":["days"],"status":"ok","duration_ms":42}
{"ts":"2026-06-30T08:00:14Z","tool":"experiment_judge","arg_keys":["experiment_id"],"status":"error","duration_ms":120,"error":"..."}
```

必填欄位：`ts`、`tool`、`status`（`ok` | `error` | `unauthorized`）、`duration_ms`。`arg_keys` 只記錄 key 名稱、不記錄值。`error` 僅在 `status != "ok"` 時輸出。

## License

GNU AGPL v3 — 與 atlas-go 一致（見根目錄 `LICENSE`）。
