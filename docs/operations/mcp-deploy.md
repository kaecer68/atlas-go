# MCP Server 部署指南（`cmd/atlas-mcp`）

> **目的**：把 `cmd/atlas-mcp` binary 部署到生產環境，作為外部 AI Agent（Claude Desktop、Cursor、OpenCode 等）對話的 backend。
> **目標讀者**：DevOps / SRE / 想整合 atlas-go 的開發者。

---

## 1. 架構位置

`cmd/atlas-mcp` 是 `atlas-go` repo 內的一個子 binary。它**依附於** `cmd/atlas`（atlas-go HTTP API），本身不持久化任何狀態——只做 protocol 轉譯 + auth 過濾 + audit log 寫檔。

```
AI Agent (Claude Desktop etc.)
  │
  │ stdio / SSE / streamable-HTTP
  ▼
atlas-mcp binary (this doc)
  │
  │ X-API-Key + HTTPS (loopback in container)
  ▼
atlas-go HTTP API (cmd/atlas)
  │
  ▼
PostgreSQL / Redis
```

部署上常見兩種拓樸：
- **同容器**：atlas-go + atlas-mcp 跑同一個 container（簡單、開發/單機用）
- **sidecar**：atlas-mcp 是獨立 container/service，atlas-go 是另一個（生產建議，role 清晰）

---

## 2. 環境需求

| 需求 | 版本 / 規格 |
|------|-------------|
| Go（build-time）| 1.26.4+（atlas-go 同版）|
| Docker（optional）| multi-stage build 支援 1.18+ |
| atlas-go 服務 | 同一台機或可連線的 HTTP endpoint（port 8080）|
| filesystem | audit log 寫入路徑（`$TMPDIR` 預設）需可寫 |
| Network | stdio 無需 port；SSE/HTTP bind `127.0.0.1`（不對外） |

---

## 3. 構建

### 3.1 本機 Go build

```bash
go build -o bin/atlas-mcp ./cmd/atlas-mcp/
```

會產出 11 MB 左右的靜態 binary（CGO disabled）。

### 3.2 Docker image（已整合進 atlas-go 的 multi-stage Dockerfile）

`Dockerfile`（atlas-go repo 根目錄）現在 build 4 個 binary：

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always) -X main.buildTime=$(date -u +%Y%m%d%H%M%S)" \
    -o atlas-go \
    ./cmd/atlas
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o daily-replay-sync ./cmd/daily-replay-sync
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o backfill-replay ./cmd/backfill-replay
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o atlas-mcp ./cmd/atlas-mcp
```

```bash
docker build -t atlas-go:latest .
```

binary 落在 image `/app/atlas-mcp`。

### 3.3 驗證 build

```bash
go test -count=1 -race ./cmd/atlas-mcp/...   # 99 tests, -race 綠
```

---

## 4. 環境變數

### 必要

| 變數 | 用途 | 預設 |
|------|------|------|
| `ATLAS_BASE_URL` | atlas-go HTTP API 基準 URL | `http://127.0.0.1:8080` |
| `ATLAS_MCP_AUDIT_LOG` | JSONL audit log 路徑 | `$TMPDIR/atlas-mcp-audit.log` |

### 選用

| 變數 | 用途 | 預設 |
|------|------|------|
| `ATLAS_API_KEY` | 轉發為 `X-API-Key` header 給 atlas-go admin endpoints | （未設時停用）|
| `ATLAS_MCP_TOKEN` | Bearer token required by HTTP/SSE 傳輸 | （未設時 dev mode）|
| `ATLAS_MCP_METRICS_ADDR` | Prometheus metrics listen address | （未設時停用；建議 `127.0.0.1:9091`）|
| `TMPDIR` | audit log 預設目錄 | 系統預設 |

| `ATLAS_MCP_SAMPLING_ENABLED` | 啟用 Sampling (server→client LLM 呼叫) | `false` |
| `ATLAS_MCP_ELICITATION_ENABLED` | 啟用 Elicitation (server 主動向使用者提問) | `false` |

### 註冊狀態

`ATLAS_BASE_URL`、`ATLAS_API_KEY`、`ATLAS_MCP_AUDIT_LOG` 已於 [`configs/allowed_env_vars.md`](../../configs/allowed_env_vars.md) 白名單；`ATLAS_MCP_TOKEN` 為 Phase 2 新增，正在本 doc 標頭 commit 同步註冊。

---

## 5. 啟動

### 5.1 預設（stdio）

```bash
./atlas-mcp
# 預設監聽 stdin/stdout
```

### 5.2 SSE（port 9090, dev mode 無 token）

```bash
./atlas-mcp --transport=sse --bind=127.0.0.1:9090
```

### 5.3 streamable-HTTP（port 9090）

```bash
./atlas-mcp --transport=http --bind=127.0.0.1:9090
```

### 5.4 帶 Bearer auth（production）

```bash
ATLAS_MCP_TOKEN=$(openssl rand -hex 32) \
  ./atlas-mcp --transport=http --auth=required --bind=127.0.0.1:9090
```

所有 client request 必須帶 `Authorization: Bearer <ATLAS_MCP_TOKEN>`，否則 401。

### 5.5 完整環境變數

| 變數 | 範例 |
|------|------|
| `ATLAS_BASE_URL` | `http://atlas-go:8080` |
| `ATLAS_API_KEY` | `<atlas-go admin API key>` |
| `ATLAS_MCP_AUDIT_LOG` | `/var/log/atlas-mcp/audit.log` |
| `ATLAS_MCP_TOKEN` | `<32-byte hex>`（必帶 `--auth=required`）|

---

## 6. Docker compose（同容器模式）

```yaml
# docker-compose.yml
services:
  atlas-go:
    build: .
    image: atlas-go:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://atlas:...@db/atlas
      - REDIS_URL=redis://cache:6379
    depends_on: [db, cache]

  # atlas-mcp 與 atlas-go 同容器：以 ENTRYPOINT 多進程，或改 image ENTRYPOINT 為 wrapper
  # 推薦：改 Dockerfile ENTRYPOINT 為 wrapper script
```

`Dockerfile` 改法（推薦 sidecar 模式）：

```dockerfile
# Final stage — 兩 binary 共享 image，啟動時挑一個
# ENTRYPOINT 改為 wrapper script，根據環境變數決定跑哪個
COPY --from=builder /build/scripts/atlas-mcp-wrapper.sh /app/
RUN chmod +x /app/atlas-mcp-wrapper.sh
ENTRYPOINT ["/app/atlas-mcp-wrapper.sh"]
CMD []
```

`atlas-mcp-wrapper.sh`（範例）：

```bash
#!/bin/sh
# 根據 ROLE 環境變數決定跑哪個 binary
# ROLE=api (default) → 跑 atlas-go API server
# ROLE=mcp             → 跑 atlas-mcp sidecar
ROLE="${ROLE:-api}"
if [ "$ROLE" = "mcp" ]; then
  exec /app/atlas-mcp
fi
exec /app/atlas-go "$@"
```

然後在 compose：

```yaml
services:
  atlas-go:
    image: atlas-go:latest
    command: ["-api"]
    environment:
      - ROLE=api
    ports: ["8080:8080"]

  atlas-mcp:
    image: atlas-go:latest
    environment:
      - ROLE=mcp
      - ATLAS_BASE_URL=http://atlas-go:8080
      - ATLAS_API_KEY=${ATLAS_API_KEY}
      - ATLAS_MCP_TOKEN=${ATLAS_MCP_TOKEN}
      - ATLAS_MCP_AUDIT_LOG=/var/log/atlas-mcp/audit.log
    volumes:
      - mcp-audit:/var/log/atlas-mcp
    # stdio：無需 ports。SSE/HTTP：改 entrypoint 為 atlas-mcp --transport=http --bind=0.0.0.0:9090
```

---

## 7. Claude Desktop 設定範例

`~/.config/Claude Desktop/claude_desktop_config.json`：

```json
{
  "mcpServers": {
    "atlas-go": {
      "command": "/usr/local/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:8080",
        "ATLAS_API_KEY": "<your-key>",
        "ATLAS_MCP_AUDIT_LOG": "/tmp/atlas-mcp-audit.log"
      }
    }
  }
}
```

注意：stdio 模式不適用於 HTTP transport 環境變數的 `--transport` flag——Claude Desktop 啟動時預設 stdio。

---

## 8. Cursor 設定範例

`~/.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "atlas-go": {
      "url": "http://127.0.0.1:9090/mcp",
      "transport": "streamable-http",
      "headers": {
        "Authorization": "Bearer <your-token>"
      }
    }
  }
}
```

對應的 atlas-mcp 啟動方式：

```bash
ATLAS_MCP_TOKEN=<your-token> \
  ./atlas-mcp --transport=http --auth=required --bind=127.0.0.1:9090
```

---

## 9. 監控 / 維運

### 9.1 Audit log

JSONL 格式（每行一筆 tool 呼叫）：

```json
{"ts":"2026-06-30T13:50:00Z","tool":"regime_get_history","arg_keys":["days"],"status":"ok","duration_ms":12}
{"ts":"2026-06-30T13:50:01Z","tool":"experiment_judge","arg_keys":["experiment_id"],"status":"error","duration_ms":120,"error":"atlas HTTP 500"}
```

欄位：`ts` (RFC3339), `tool` (string), `arg_keys` (array of input key names; never values), `status` (`ok`/`error`/`unauthorized`), `duration_ms` (int), `error` (only when status != ok).

**無 fsync**：寫到 OS page cache，crash 可能遺失最近幾秒。對 audit-driven 系統未來可改 `Sync()` 強制 disk flush（Phase 3 hardening）。

### 9.2 Health check

stdio 模式無 HTTP endpoint，傳統 curl-based healthcheck 不適用。SSE/HTTP 模式可加 `/healthz`（Phase 3 規劃中）。

### 9.3 Prometheus Metrics（Phase 4 Direction A）

設定 `ATLAS_MCP_METRICS_ADDR=127.0.0.1:9091` 後，atlas-mcp 會在同 process 內啟動獨立的 Prometheus HTTP endpoint：

```bash
ATLAS_MCP_METRICS_ADDR=127.0.0.1:9091 ./atlas-mcp
```

Scrape：

```bash
curl -s http://127.0.0.1:9091/metrics
```

暴露的 metrics：

- `mcp_calls_total{tool, transport, status}` — 各 tool 呼叫次數
- `mcp_call_duration_seconds{tool, transport}` — tool 呼叫 latency 分佈
- `mcp_active_sessions{transport}` — 當前活躍 session 數
- `mcp_token_usage_total{tenant_id}` — 成功 token 驗證次數
- `mcp_anomaly_score{tenant_id, anomaly_type}` — anomaly detector 即時分數

`/metrics` 僅 bind `127.0.0.1`，不對外暴露；若需外部 Prometheus scrape，請透過 reverse proxy / TLS / WAF。

### 9.4 Anomaly Tools（Phase 4 Direction A）

- `mcp_anomaly_get_recent`：列出最近 N 條 anomaly event
- `mcp_anomaly_ack`：透過 `/api/alerts/acknowledge` 確認 anomaly alert

### 9.5 升級流程

1. `git pull` + `go build`（本機）或 `docker build`（容器）
2. graceful shutdown：atlas-mcp 在收到 SIGTERM 後等當前 in-flight tool call 完成（≤ 30s timeout）— Phase 3 強化
3. 部署新 binary / image
4. 觀察 audit log 確認新 tool version 上線（透過 `tool` 欄或加 version field）

---

## 10. 故障排除

| 症狀 | 可能原因 | 修正 |
|------|---------|------|
| `atlas-mcp: server: AtlasBaseURL is required` | `ATLAS_BASE_URL` 未設 | export 環境變數 |
| 401 Unauthorized（SSE/HTTP）| `ATLAS_MCP_TOKEN` 未配對 | 確認 client header 帶正確 token；token 比較 case-insensitive |
| 工具呼叫回 `atlas HTTP 500` | atlas-go 服務失敗 | 看 atlas-go log / healthz |
| audit log 沒出現 | 環境變數 `ATLAS_MCP_AUDIT_LOG` 設到不可寫目錄 | 檢查路徑權限 |
| stdio 卡住無輸出 | 環境變數 `ATLAS_MCP_TOKEN` 為空且 `--auth=required`（stdio 應該是 dev mode 不檢查）| 確認 stdio 不傳 auth |
| SSE client 連不上 | bind address 不是 `0.0.0.0`（預設 `127.0.0.1`）| container port forward 設對，或改 `--bind=0.0.0.0:9090` |
| `constitution check` 失敗（CI）| 新 env var 未在 `configs/allowed_env_vars.md` 白名單 | 加上白名單 |

---

## 11. 安全注意事項

1. **`ATLAS_MCP_TOKEN` 必須保密**：等同 admin token。用 `openssl rand -hex 32` 產生；不要 commit 到 git。
2. **SSE/HTTP bind 預設 `127.0.0.1`**：不要為求方便改 `0.0.0.0` 暴露到公網（除非有 reverse proxy + TLS）。
3. **audit log 寫檔路徑**：確認在安全位置，避免 symlink attack（不要用 `/tmp` 當 production 位置）。
4. **stdio 模式無 transport-level auth**：依賴 process isolation（只有父 process 能 reach stdin/stdout）。Container 化時要確保 stdin/stdout 沒被暴露給多個 process。
5. **API key（`ATLAS_API_KEY`）**：admin 權限，保管嚴格；mcp binary 會 forward 給 atlas-go。

---

## 12. 對應的文件

- 完整 tool catalog（74 tools）：[`docs/AGENT_TOOLS.md`](../AGENT_TOOLS.md)
- 設計規格（Phase 2.2 狀態）：[`docs/specs/agent-mcp-server.md`](../specs/agent-mcp-server.md)
- 系統 workflow 對應（WA-XXX）：[`docs/WORKFLOW_MAP.md`](../WORKFLOW_MAP.md)
- 計劃藍圖： [`docs/plans/agent-interface-roadmap.md`](../plans/agent-interface-roadmap.md)
- Go core coding rules： [`.github/instructions/go-core.instructions.md`](../../.github/instructions/go-core.instructions.md)
- Live trading guardrails： [`.github/instructions/live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md)
- Constitution： [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md)
