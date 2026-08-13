# atlas-mcp

`atlas-mcp` 是 [atlas-go](https://github.com/kaecer68/atlas-go) 的 MCP (Model Context Protocol) 伺服器。讓任何 MCP-compatible AI Agent（Claude Desktop、Cursor、OpenCode、OpenClaw、Hermes 等）透過標準 JSON-RPC 2.0 協議查詢與輕度觸發 atlas-go 的台股投資研究能力。

> **Agent 入門** — 第一次使用？先讀 [`docs/investor/README.md`](../../docs/investor/README.md)（5 分鐘速讀），再看 [`docs/reference/tool-catalog.md`](../../docs/reference/tool-catalog.md)（tool 決策樹與完整 catalog，為 tool 數量與清單的權威來源）。
> **完整規格** — 設計文件、安全邊界、JSON Schema 模板見 [`docs/specs/agent-mcp-server-spec.md`](../../docs/specs/agent-mcp-server-spec.md)。
> **開發者** — 若要在 `cmd/atlas-mcp/server/` 內新增或修改 tool，**必先讀** [`server/AGENTS.md`](./server/AGENTS.md)（模組陷阱文件）。

## For AI Agent Operators（Hermes / OpenClaw / Claude Desktop / Cursor / OpenCode）

> **你是替投資人用戶接入 atlas-mcp 的 agent operator？從一行 installer 開始。**

```bash
# 一行安裝（不需 Go toolchain、不需 clone repo）
curl -fsSL https://raw.githubusercontent.com/kaecer68/atlas-go/main/scripts/install-atlas-mcp-from-release.sh | bash

# 或鎖定版本
curl -fsSL ... | bash -s -- --version v0.0.0.33
```

自動下載預編譯 binary + SHA256 驗證 + 安裝到 `~/.local/bin/atlas-mcp`。

### 短期共用 dev key（**推薦**給 hermes / openclaw agent 短期使用）

atlas-go 商業化前（見 [#1068](https://github.com/kaecer68/atlas-go/issues/1068)），為降低維護成本，目前所有 hermes / openclaw agent **共用一組 dev key** 放在 `~/.config/atlas-go/.env`（已 gitignore）。

```bash
# 一條龍：自動 source env + 安裝 + 設定 + 驗證 hermes
make setup-mcp-agent

# 單獨驗證 hermes 真的能用
make verify-mcp-setup
```

**短期限制**（等 #1068 解）：
- 這是 dev key，**不要拿來做 live trading**
- 商業化後改用個人 key：`hermes mcp add atlas-mcp --env ATLAS_API_KEY=$YOUR_PERSONAL_KEY`

**沒有 `make` 環境時的 fallback**（純 hermes CLI）：
```bash
# 1. 讀 dev key 進當前 shell
set -a; . ~/.config/atlas-go/.env; set +a

# 2. 一條龍加入 hermes（自動 enable 全部 tools）
printf "Y\n" | hermes mcp add atlas-mcp \
  --command "$(command -v atlas-mcp)" \
  --env ATLAS_BASE_URL="${ATLAS_BASE_URL:-http://127.0.0.1:18080}" \
  --env ATLAS_API_KEY="${ATLAS_API_KEY}" \
  --connect-timeout 30
hermes mcp configure atlas-mcp --enable-all 2>/dev/null || true
```

MCP client config 路徑：

| Client | Config 檔 |
|--------|-----------|
| Hermes | `~/.hermes/config.yaml` |
| OpenClaw | `~/.openclaw/mcp.json` |
| Claude Desktop | `~/Library/Application Support/Claude/claude_desktop_config.json`（macOS）|
| Cursor | `~/.cursor/mcp.json` |
| OpenCode | `~/.config/opencode/opencode.json` |

> **重要：設定檔的 server entry key 是 `'atlas-mcp'`，不是 `'atlas-go'`**（與本 repo 早期 PR 系列混用 'atlas-go' 為 server key 對齊 — 詳見本 README §MCP Client 配置範例）。

**你的投資人用戶可以問**（呼叫 `mcp_quickstart` 開始）：

- `「2330 現在多少？」` → `stock_get_quote {symbol: "2330"}`
- `「現在市場風險怎樣？」` → `risk_get_metrics`
- `「今天要關注什麼？」` → `narrative_get_events`
- 更多自然語言範本見 [`docs/operations/stock-mcp-query-templates.md`](../../docs/operations/stock-mcp-query-templates.md)

或用 [`make setup-mcp`](../atlas-mcp-setup/) 啟動互動式 wizard 自動偵測已安裝的 client 並寫入 config。

## 目前規模

| 面向 | 現狀 |
|------|------|
| MCP Tools | **117 個 tool**（預設；+2 sampling/elicitation feature-gated 全開時 119；啟動期 assert [115, 121]；權威清單見 [`docs/reference/tool-catalog.md`](../../docs/reference/tool-catalog.md)） |
| Tool description | `auto-desc.gen.json`（由 `cmd/atlas-mcp/descgen/` 自動生成） |
| Transport | **stdio**（預設，向後相容）；**SSE + streamable-HTTP**（Phase 4 啟用，Bearer auth 強制） |
| Auth | TokenAuth + DB TokenStore（`auth.go` / `auth_db.go` / `auth_db_pg.go`）+ admin HTTP API（127.0.0.1，`token_admin_handler.go`） |
| Audit | v2 schema（retention、cleanup、ArgsHash、SessionID、Transport，`audit_v2.go`；v1 `audit.go` 為向後相容 shim） |
| 擴充協議 | Resources（`resources.go`）、Prompts（`prompts.go`）、Elicitation（`elicitation.go`）、Sampling（`sampling.go`）、Roots（`roots.go`） |
| 觀測 | Rate limiting（`ratelimit.go`）、Metrics（`metrics.go`）、Anomaly detection（`tools_anomaly.go`） |
| 工具分類 | Regime（1）、Macro（6）、Capital Flow（4）、Crossmarket（3）、Narrative（7）、Events（2）、Risk（5）、Alert（4）、Strategy（6）、Recommendation（1）、Experiment（5）、Synergy（3）、Control（7）、Scheduler/Task（4）、System（7）、LLM（2）、Trace（4）、Data（4）、Stock（4）、Parameters（5）、Backtest（2）、Industry Extension（5）、Sector Canonical（2）、Universe（2）、Report（4）、Briefing（2）、Protocol Extensions（4，其中 2 個 feature-gated 預設關閉）、MCP Audit / Observability（6）、Prism（1）、Template Detector（2） |

## 快速啟動

**路徑 A — 一行 installer（推薦，給 agent operator）**：

```bash
curl -fsSL https://raw.githubusercontent.com/kaecer68/atlas-go/main/scripts/install-atlas-mcp-from-release.sh | bash
```

詳見上面 §For AI Agent Operators。

**路徑 B — 開發者（已 clone repo + 有 Go）**：

```bash
# 建置
go build -o bin/atlas-mcp ./cmd/atlas-mcp/

# 啟動（stdio transport — 預設，Claude Desktop / Cursor / OpenCode 用）
ATLAS_BASE_URL=http://127.0.0.1:18080 ATLAS_API_KEY=xxx ./bin/atlas-mcp

# 啟動（streamable-HTTP transport，Bearer auth 強制）
ATLAS_MCP_TRANSPORT=streamable-http \
ATLAS_MCP_ADDR=127.0.0.1:9090 \
ATLAS_MCP_TOKEN=$(openssl rand -hex 32) \
ATLAS_BASE_URL=http://127.0.0.1:18080 \
ATLAS_API_KEY=xxx \
./bin/atlas-mcp

# 啟動（SSE transport，Bearer auth 強制；deprecated by MCP spec，但保留相容）
ATLAS_MCP_TRANSPORT=sse \
ATLAS_MCP_ADDR=127.0.0.1:9090 \
ATLAS_MCP_TOKEN=$(openssl rand -hex 32) \
ATLAS_BASE_URL=http://127.0.0.1:18080 \
ATLAS_API_KEY=xxx \
./bin/atlas-mcp
```

stdio 模式從 stdin 讀取 JSON-RPC 請求、往 stdout 寫入回應。`ATLAS_BASE_URL` 指向 atlas-go HTTP API（預設 `http://127.0.0.1:18080`）。

streamable-HTTP / SSE 模式 bind `ATLAS_MCP_ADDR`（預設 `127.0.0.1:9090`），所有請求需帶 `Authorization: Bearer <ATLAS_MCP_TOKEN>` header，否則回傳 401。dev mode（`ATLAS_MCP_TOKEN=""`）允許無 token 存取，僅供本機開發使用。

> **安全提醒**：HTTP transport 一律 bind 127.0.0.1，不要對外暴露。若需遠端使用，請放在具備 TLS 終止與速率限制的 reverse proxy（例如 nginx、Caddy）後方。

## 配置

全部透過環境變數（CLI flag > env > 預設值優先級，見 `cmd/atlas-mcp/main.go`）：

### 連線 / Atlas 後端

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_BASE_URL` | `http://127.0.0.1:18080` | atlas-go HTTP API 基底 URL |
| `ATLAS_API_KEY` | （未設） | 以 `X-API-Key` header 轉發至 atlas-go admin endpoints |
| `DATABASE_URL` | （未設） | PostgreSQL DSN；啟用後自動跑 migration 並切換至 PG-backed `TokenStore`（v2 schema） |

### MCP Server 本體

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_MCP_TRANSPORT` | `stdio` | 傳輸層：`stdio` / `sse` / `streamable-http` |
| `ATLAS_MCP_ADDR` | `127.0.0.1:9090` | 監聽位址，僅 `sse` / `streamable-http` 使用 |
| `ATLAS_MCP_TOKEN` | （未設） | Bearer token；SSE/HTTP transport 啟用後強制驗證（401 if missing/wrong）；DB token store 啟用時作為 env-fallback |
| `ATLAS_MCP_ADMIN_TOKEN` | （未設） | Admin HTTP API token（token management）；未設 = 該 API 停用 |
| `ATLAS_MCP_ADMIN_ADDR` | `127.0.0.1:9090`（僅在 `ADMIN_TOKEN` 有設時） | Admin HTTP 監聽位址 |
| `ATLAS_MCP_METRICS_ADDR` | （未設） | Prometheus `/metrics` 監聽位址；典型 `127.0.0.1:9091`；未設 = 不暴露 metrics |

### Audit / Logging

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_MCP_AUDIT_LOG` | `$TMPDIR/atlas-mcp-audit.log` | JSONL audit log 路徑；父目錄自動建立（mode 0700） |
| `ATLAS_MCP_AUDIT_RETENTION_DAYS` | `30` | 超過 N 天的 audit 條目自動 prune；`0` = 停用 retention |

### Rate limiting

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_MCP_RATE_LIMIT_PER_MINUTE` | `120` | 每 `(tool, tenant)` 每分鐘允許的請求數；`0` = 不限流 |
| `ATLAS_MCP_RATE_LIMIT_BURST` | = `RATE_LIMIT_PER_MINUTE` | 瞬間允許的 burst 量 |

> **Phase 6 hardening**: 預設從 `0` 改為 `120`，防止外部 agent（Hermes / OpenClaw / HTTP transport）發生 runaway loop 時壓垮 atlas-go。stdio 模式同樣適用此預設；本機 Claude Desktop / Cursor / OpenCode 若需更高頻率，可顯式設 `ATLAS_MCP_RATE_LIMIT_PER_MINUTE=0`（僅限本機開發）。HTTP/SSE transport 在 production 部署中**不應**關閉限流。

### 擴充協議開關（Phase 4 B）

| 變數 | 預設值 | 用途 |
|------|--------|------|
| `ATLAS_MCP_SAMPLING_ENABLED` | `false` | 啟用 `mcp_sample_llm`（透過 atlas LLM router 抽樣） |
| `ATLAS_MCP_ELICITATION_ENABLED` | `false` | 啟用 `mcp_elicit_user`（向使用者請求結構化輸入） |
| `ATLAS_MCP_ROOTS_ALLOWED` | （未設） | 當 client 未宣告 roots 時，server 預設允許的 `file://` 白名單（CSV） |
| `ATLAS_MCP_ROOTS_READ_SIZE_CAP` | `1048576`（1 MiB） | 單次 `mcp_roots_read_file` 讀取上限（bytes） |
| `ATLAS_MCP_ROOTS_ALLOW_UNSAFE` | `0` | escape hatch（不建議）：`1` = 跳過 root path 驗證（僅限 dev） |
| `ATLAS_MCP_ROOTS_ALERT_ON_CHANGE` | `false` | client 宣告的 roots 變動時是否 alert |

> **stdio 安全模型**：stdio 模式預設不強制 Bearer token（process isolation 是本機單用戶場景的安全邊界），但仍接受 token 標頭作為多租戶 routing。**若 atlas-mcp 被多個 agent / 多個使用者共用，或運行在共享主機上，請務必設定 `ATLAS_MCP_TOKEN`**，否則任何能啟動 process 的程式都能呼叫全部 110+ 個 tool。SSE / streamable-HTTP 模式 `Authorization: Bearer <token>` **必填**，未帶或錯誤回傳 401。

## MCP Client 配置範例

### Claude Desktop（`~/.config/Claude Desktop/claude_desktop_config.json`）

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

### Cursor（Settings → MCP）

與 Claude Desktop 同樣 JSON 格式。透過 `+ Add new MCP server` 新增。

### OpenCode（`opencode.json`）

```json
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

## 開發者場景（已有本地 binary）

適用：已 clone atlas-go repo、Go build 出 `bin/atlas-mcp`、想接到 hermes 驗證開發。

```bash
# 1. 確認 binary 可執行
ls -la ~/workspace/atlas/bin/atlas-mcp

# 2. 確認 dev key 已 source
set -a; . ~/.config/atlas-go/.env; set +a

# 3. 加進 hermes（可選：用 --enable-all 或後續手動 --enable 開特定 tool）
hermes mcp add atlas-mcp \
  --command "$HOME/workspace/atlas/bin/atlas-mcp" \
  --env ATLAS_BASE_URL="${ATLAS_BASE_URL}" \
  --env ATLAS_API_KEY="${ATLAS_API_KEY}" \
  --env ATLAS_MCP_AUDIT_LOG="$HOME/.hermes/logs/atlas-mcp-audit.log" \
  --connect-timeout 30

# 4. 驗證
hermes mcp list
# 在 hermes session 內呼叫 mcp__atlas_mcp__mcp_quickstart 確認
```

> 若 `~/.config/atlas-go/.env` 存在，hermes 自動 source，**可省 `--env ATLAS_API_KEY=...`**；但顯式帶上避免路徑耦合（見下節「升級 SOP」）。

## atlas-go 升級 SOP（更新源碼後重啟 hermes 端）

當你更新了 `atlas-go` 源碼（不管是改 `cmd/atlas-mcp/` 或 `internal/`），需要讓 hermes 端的 atlas-mcp 看到新 binary：

```bash
# 1. 重新編譯
cd ~/workspace/atlas && make build-mcp

# 2. 重啟 hermes 端的 atlas-mcp
hermes mcp restart atlas-mcp

# 3. 驗證
hermes mcp list  # 確認 tool 數量在 tool-catalog 範圍內
```

> 預期：若改了 `cmd/atlas-mcp/`，tool 數量或 signature 可能微調（啟動期 `RegisteredToolCount ∈ [115, 118]` assert 強制；詳見 [`docs/reference/tool-catalog.md`](../../docs/reference/tool-catalog.md)）。重啟 hermes session 後才會看到新 tool。若 binary 與 source 對不上（`stat bin/atlas-mcp mtime < git log -1 -- cmd/atlas-mcp/`），重啟前先 `make build-mcp`。

## License

GNU AGPL v3 — 與 atlas-go 一致（見根目錄 `LICENSE`）。
