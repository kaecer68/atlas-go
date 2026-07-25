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
| atlas-go 服務 | 同一台機或可連線的 HTTP endpoint（port 18080）|
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
| `ATLAS_BASE_URL` | atlas-go HTTP API 基準 URL | `http://127.0.0.1:18080` |
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
| `ATLAS_MCP_ROOTS_ALLOWED` | 逗號分隔 `file://` URI 清單;當 client 未宣告 roots 時作為 fallback allow-list | （未設時無 fallback）|
| `ATLAS_MCP_ROOTS_READ_SIZE_CAP` | 單次 `mcp_roots_read_file` 最大讀取位元組 | `1048576`（1 MiB）|
| `ATLAS_MCP_ROOTS_ALERT_ON_CHANGE` | client 端 roots 變動時發出 `security_roots_changed` 事件至 `internal/alerting` Publisher | `true` |
| `ATLAS_MCP_PARAMS` | 自訂 `parameters.json` 路徑;未設時 fallback 到 `configs/parameters.json` | （未設時使用預設路徑）|

### 4.3 Roots（Phase 4 Direction B）

> **Phase 4 Direction B 新增**：client 透過 MCP `RootsV2` capability 宣告 `file://` 根目錄，atlas-mcp 對其下的檔案提供 **唯讀** 讀取（`mcp_roots_list` + `mcp_roots_read_file`）。

**唯讀邊界（read-only boundary）**：根據 [`internal/apigateway/CONSTITUTION.md` 附錄 D](../../internal/apigateway/CONSTITUTION.md) — Phase 4 B 的 narrow exception，**禁止**對宣告根目錄內的任何檔案進行寫入、修改、刪除、重新命名；任何 write flag 或 query fragment（`?write=`、`#delete`）會被 `handleMCPRootsReadFile` 拒絕並回傳 explicit error。

**啟用方式**：預設 roots 工具 **永遠註冊**（依 client capability 決定是否啟用）。client 必須在 MCP `initialize` 階段宣告 `RootsV2` capability 才能使用 roots 系列工具。

**配置優先級（top wins）**：
1. 環境變數 `ATLAS_MCP_ROOTS_ALLOWED`、`ATLAS_MCP_ROOTS_READ_SIZE_CAP`、`ATLAS_MCP_ROOTS_ALERT_ON_CHANGE`
2. `configs/parameters.json` 的 `mcp.roots` section
3. 內建預設（size cap `1048576`、無 allowed roots、alert on change `true`）

**自訂 params 檔案位置**：透過 `ATLAS_MCP_PARAMS` env var 覆寫；未設時使用 `configs/parameters.json`（相對於 atlas-mcp binary 啟動時的工作目錄）。

**安全特性**：
- **Path-traversal hardening**：`filepath.Clean` + `filepath.Abs` + `filepath.EvalSymlinks`（symlink escape 防護）— `isUnderRoots` 比對前套用
- **檔案大小上限**：預設 1 MiB；`info.Size()` 雙重把關 + `io.LimitReader`
- **常規檔案限制**：僅允許 `Mode().IsRegular()`，拒絕 device/socket/FIFO/symlink-to-non-regular
- **Audit 強制**：每次讀取寫入 audit log JSONL（含 `path`、`size_bytes`、`ts`、`tenant_id`）
- **綁定位址**：SSE/HTTP 模式必綁 `127.0.0.1`
- **Capability gate**：client 未宣告 `RootsV2` 時回傳 explicit error，不走 soft fallback

**告警整合**：當 `ATLAS_MCP_ROOTS_ALERT_ON_CHANGE=true`（預設），server 端的 `RootsListChangedHandler` 會透過 `internal/alerting.Publisher` 發出 `security_roots_changed` 事件（成功為 `SeverityInfo`、失敗為 `SeverityWarning`），下游 Alertmanager 或 webhook 訂閱者可即時收到 roots 變動通知。

**完整規格**：[`docs/specs/agent-mcp-phase4-spec.md` §6.2.B](../specs/agent-mcp-phase4-spec.md)、[`internal/apigateway/CONSTITUTION.md` 附錄 D](../../internal/apigateway/CONSTITUTION.md)

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
| `ATLAS_BASE_URL` | `http://atlas-go:18080` |
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
      - "18080:18080"
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
    ports: ["18080:18080"]

  atlas-mcp:
    image: atlas-go:latest
    environment:
      - ROLE=mcp
      - ATLAS_BASE_URL=http://atlas-go:18080
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
    "atlas-mcp": {
      "command": "/usr/local/bin/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
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
    "atlas-mcp": {
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
- `mcp_anomaly_emitted_total{tenant_id, anomaly_type, severity}` — anomaly 發射累計（Phase 4 T1.4 新增）

`/metrics` 僅 bind `127.0.0.1`，不對外暴露；若需外部 Prometheus scrape，請透過 reverse proxy / TLS / WAF。

### 9.4 Anomaly Tools（Phase 4 Direction A）

- `mcp_anomaly_get_recent`：列出最近 N 條 anomaly event
- `mcp_anomaly_ack`：透過 `/api/alerts/acknowledge` 確認 anomaly alert

### 9.5 T1.4 Alert / Eventbus 整合（Phase 4 Direction A）

**目的**：把 anomaly detector 的輸出，從「只在 process 內部可見」升級為「可在 Alertmanager 收單 + 跨進程 SSE 訂閱 + Prometheus 計算 rate」。

**資料流（偵測 → 四個 sink）**：

```
audit entry ─► detector (3 baselines) ─► ring-buffer Store
                                            │
                                            ▼
                                       Emitter (poll 1s)
                                            │
                ┌─────────────────┬──────────┼──────────┐
                ▼                 ▼          ▼          ▼
           alert publisher   ack store   event bus   metrics
           (Alertmanager)   (MemoryStore) (SSE bus)  (Prometheus)
```

每一個 anomaly 事件都會被 fan-out 到四個目的地（依序，alert publisher 失敗不擋其他三個）：

| Sink | 介面 / 套件 | 預設行為 | 上線後行為 |
|------|------------|---------|-----------|
| Alert publisher | `internal/alerting.Publisher` | `NoOpPublisher`（空 URL） | `WebhookPublisher` POST 到 `alert_webhook_url` |
| Ack store | `internal/mcp/anomaly.AnomalyStore` | `MemoryStore` 容量 1000 | 容量 1000，operator dashboard 用 `ListUnacked` 拉未確認事件 |
| Event bus | `internal/eventbus.ChannelEventBus` | （獨立 atlas-mcp process 無 bus） | `EventMCPAnomalyDetected` 事件帶 `MCPAnomalyEventPayload` 給 SSE 訂閱者 |
| Metrics | `cmd/atlas-mcp/server.Metrics` | 既有 | `mcp_anomaly_emitted_total{tenant_id, anomaly_type, severity}` counter 累計 |

**設定（`configs/parameters.json` 的 `mcp_anomaly` 段）**：

| 鍵 | 預設 | 說明 |
|----|------|------|
| `alert_webhook_url` | `""` | Alertmanager 風格 webhook URL。空字串 = NoOpPublisher，僅本地有 ack store + metrics。**生產環境強烈建議設為 `http://alertmanager:9093/api/v1/alerts`**。 |
| `alert_http_timeout_seconds` | `5` | 每次 POST 給 webhook 的 timeout。**設計理由**：避免 Alertmanager 緩慢時拖慢 emitter poll loop。 |
| `emitter_interval_seconds` | `1` | Emitter 多久 poll 一次 detector 的 ring buffer。**設計理由**：1s 是 operator alerting 可接受的最大 lag；更低 = 換 CPU 換 latency。 |
| `ack_store_capacity` | `1000` | 記憶體 ack store 容量上限。**設計理由**：1000 筆 ≈ 1Hz burst 持續 17 分鐘，遠超過任何 operator 合理的確認回應時間。 |

**Alertmanager 端的建議 rule**：

```yaml
# prometheus rules fed by mcp_anomaly_emitted_total
groups:
  - name: mcp_anomaly
    rules:
      - alert: MCPAnomalyBurst
        expr: rate(mcp_anomaly_emitted_total{severity="high"}[5m]) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "MCP 高嚴重度異常持續觸發"
          description: "tenant {{ $labels.tenant_id }} 在 5 分鐘內多次觸發 {{ $labels.anomaly_type }}"
```

**失敗模式與容錯**：

| 失敗 | 行為 | 設計理由 |
|------|------|---------|
| Webhook 5xx | 該次 publish 記 `logging.Warn`，但**仍寫入 ack store + 更新 metrics + 發 bus 事件** | 一個慢/壞的 alert sink 不能把整個 pipeline 拖死；operator 仍能在 dashboard 看到事件 |
| Webhook 連線拒絕 | 同上 | 同上 |
| Context cancel | emitter goroutine 在下一次 tick 退出 | 對齊 server graceful shutdown |
| `mcp_anomaly_ack` 收到未知 id | 回 `ErrAnomalyNotFound` (HTTP 400) | 與 T1.5 contract 一致 |
| 同一個 event 重複 poll | 5 次 `ProcessOnce` 仍只 POST 1 次 | dedup by `TS \| type \| tenant \| tool` |

**SRE 操作 SOP**：

1. **確認 alert 收到**：deploy 後主動製造 burst（送 20 個 audit entry 給同 tenant 同 tool），觀察 `mcp_anomaly_emitted_total` 與 Alertmanager 是否同時更新。
2. **Ack 驗證**：在 `mcp_anomaly_get_recent` 看到事件後用 `mcp_anomaly_ack` 確認，確認後 `ListUnacked` 應少 1、`ListAll` 仍顯示 1 筆（保留 audit）。
3. **降級**：若 Alertmanager 完全不可用，把 `alert_webhook_url` 設回 `""` 切回 NoOpPublisher，ack store + metrics 仍可用，operator 不會失明。

**測試覆蓋**：

- 單元測試：`internal/alerting/` 4 個 test、`internal/mcp/anomaly/` 30+ 個 test（含 emitter 6 個、severity mapping、MemoryStore 12 個）
- 整合測試：`internal/mcp/anomaly/integration_test.go` 三個情境（full pipeline、idempotency、ack list semantics）
- 全部 `-race` 通過；publisher 失敗不擋其他 sink 是用 `t.Run` 內的失敗注入驗證

### 9.6 升級流程

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

- 完整 tool catalog（116 tools）：[`docs/reference/tool-catalog.md`](../reference/tool-catalog.md)
- 設計規格（Phase 2.2 狀態）：[`docs/specs/agent-mcp-server-spec.md`](#)
- 系統 workflow 對應（WA-XXX）：[`docs/reference/workflow-map.md`](../reference/workflow-map.md)
- 計劃藍圖： [`docs/specs/agent-mcp-server-spec.md`](#)（canonical spec,roadmap v2 snapshot 詳見 PR #876 `feat/atlas-mcp` 歷史 commit;CI 不追蹤 `.omo/` 內路徑故不列入連結）
- Go core coding rules： [`.github/instructions/go-core.instructions.md`](../../.github/instructions/go-core.instructions.md)
- Live trading guardrails： [`.github/instructions/live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md)
- Constitution： [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md)
