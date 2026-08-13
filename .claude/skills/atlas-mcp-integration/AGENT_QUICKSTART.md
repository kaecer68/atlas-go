# atlas-mcp Agent Quickstart（5 分鐘接入 SOP）

> **你是第一次接入 atlas-mcp 的 AI agent？** 從這裡開始。本文件是 50 行 copy-paste-ready 設定範例，與 [`cmd/atlas-mcp/README.md`](../../../cmd/atlas-mcp/README.md)（source of truth）對齊。

## 1. 前置需求

- atlas-go backend 已啟動於 `http://127.0.0.1:18080`（`curl http://127.0.0.1:18080/health` 應回 `{"status":"ok"}`）。若你只是 hermes/openclaw agent 的 operator 且 backend 不在你這台，跳過此項 —— 由 atlas-go 管理員負責 backend。
- **atlas-mcp binary**（擇一）：
  - **路徑 A — 一行 installer（推薦，給沒有 Go toolchain 的 agent operator）**：
    ```bash
    curl -fsSL https://raw.githubusercontent.com/kaecer68/atlas-go/main/scripts/install-atlas-mcp-from-release.sh | bash
    # 或鎖定版本
    curl -fsSL ... | bash -s -- --version v0.0.0.33
    ```
    自動下載預編譯 binary + SHA256 驗證 + 安裝到 `~/.local/bin/atlas-mcp`。
  - **路徑 B — 開發者（已 clone repo + 有 Go）**：
    ```bash
    make build-mcp
    make install-mcp    # 或直接用 bin/atlas-mcp
    ```
- 環境變數 `ATLAS_API_KEY`（**短期可省**，見下面「短期共用 dev key」段；正式商業化見 [#1068](https://github.com/kaecer68/atlas-go/issues/1068)）

## 1.5 短期共用 dev key（推薦給 hermes / openclaw agent 短期使用）

atlas-go 商業化前（見 [#1068](https://github.com/kaecer68/atlas-go/issues/1068)），為了降低維護成本與摩擦點，目前使用**一組共用 dev key** 放在 `~/.config/atlas-go/.env`（這檔已 gitignore）。短期**所有 hermes / openclaw agent 都用同一組 key**：

```bash
# 一條龍安裝 + 設定 + 驗證
make setup-mcp-agent
# 內部自動 source ~/.config/atlas-go/.env → 取得 ATLAS_API_KEY →
# 用 hermes mcp add --env ATLAS_API_KEY=... 寫入 ~/.hermes/config.yaml
```

**底層細節**（給好奇的 agent）：

```bash
# 1. 確認 dev key 存在
test -f ~/.config/atlas-go/.env && echo "OK" || echo "❌ 找不到 .env"

# 2. 看 dev key 內容（脫敏顯示）
grep '^ATLAS_API_KEY=' ~/.config/atlas-go/.env | sed 's/=.*/=***REDACTED***/'

# 3. 一條龍（自動 source env + hermes mcp add 含所有 env 變數）
make setup-mcp-agent

# 4. 驗證 hermes 真的能用
make verify-mcp-setup

# 4.5 若不想用 make，可手動 hermes mcp add（顯式帶 env，自包含不依賴 .env 自動 source）
hermes mcp add atlas-mcp \
  --command "$(command -v atlas-mcp)" \
  --env ATLAS_BASE_URL="${ATLAS_BASE_URL:-http://127.0.0.1:18080}" \
  --env ATLAS_API_KEY="${ATLAS_API_KEY}" \
  --env ATLAS_MCP_AUDIT_LOG="$HOME/.hermes/logs/atlas-mcp-audit.log" \
  --connect-timeout 30
hermes mcp configure atlas-mcp --enable-all 2>/dev/null || true
```

**限制**：
- 這是 dev key（標記為 not-for-prod），不要拿來做 live trading
- 等 #1068 商業化後改用個人 key：`hermes mcp add atlas-mcp --env ATLAS_API_KEY=$YOUR_PERSONAL_KEY`

## 2. 必要環境變數

| 變數 | 必填 | 預設 | 用途 |
|------|------|------|------|
| `ATLAS_BASE_URL` | 否 | `http://127.0.0.1:18080` | atlas-go HTTP API 基底 URL |
| `ATLAS_API_KEY` | 是（admin endpoint） | — | 以 `X-API-Key` header 轉發至 atlas-go |
| `ATLAS_MCP_TOKEN` | — | — | **不使用**：TokenAuth 直接從 `MCPToken` 設定(stdio 為 dev mode,無 env)；admin API 由 `ATLAS_API_KEY` 經 X-API-Key 轉發 |
| `ATLAS_MCP_AUDIT_LOG` | 否 | `$TMPDIR/atlas-mcp-audit.log` | JSONL audit log |

> 完整 env var 表（30+ 個）見 `cmd/atlas-mcp/README.md` §配置。

## 3. 五種 Client 設定範例（直接 copy-paste）

### A. Hermes（`~/.hermes/config.yaml`）

```yaml
mcp_servers:
  atlas-mcp:
    command: "/absolute/path/to/bin/atlas-mcp"
    env:
      ATLAS_BASE_URL: "http://127.0.0.1:18080"
      ATLAS_API_KEY: "your-api-key-here"
```

重啟 Hermes 後執行 `hermes mcp test atlas-go` 驗證。

### B. OpenClaw（`~/.openclaw/mcp.json`）

```json
{
  "atlas-mcp": {
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
    "atlas-mcp": {
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
    "atlas-mcp": {
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
hermes mcp test atlas-mcp         # 應列出 117 個 tool
# 或在 Claude Desktop / Cursor 內嘗試調用任一 tool
```

# Step 4: 確認 admin tools 可用（需要 ATLAS_API_KEY）
hermes mcp call atlas-mcp llm_get_cost  # 若 503 = KimiClient 未啟動（正常，dev 環境）


## 4.5 First Contact SOP（首次接入後必跑 3 call）

```bash
# 1. 確認系統健康
hermes mcp call atlas-mcp system_get_health

# 2. 取得今日市場速覽
hermes mcp call atlas-mcp mcp_quickstart

# 3. 看今天為什麼漲跌
hermes mcp call atlas-mcp explain_market_move
```

這三步確認連通後，即可開始查詢：
- **趨勢方向**：`regime_get_history` → `capital_flow_daily`
- **個股資訊**：`stock_get_quote` / `stock_get_fundamentals` / `stock_get_chips` / `stock_get_technical`
- **事件預測**：`event_flow_prediction`
- **策略建議**：`get_recommendations` → `strategy_ranker`
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
| 看不到 117 個 tool | client 還在用舊 config | 編輯 config 後**重啟 client**（Hermes 可用 `/reload-mcp`） |
| tool 呼叫 timeout | atlas-go backend 沒啟動 | `curl http://127.0.0.1:18080/health` 確認 |

## 7. 互動式 Wizard（推薦）

若不想手刻 config，可用 `make setup-mcp` 啟動互動式 wizard，自動偵測 client、產生 config、驗證連線。

---

**文件歷史**：v1.0 建立於 2026-07-10（PR #1 of atlas-mcp onboarding 2026 Q3）。
