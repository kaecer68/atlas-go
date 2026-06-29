# Atlas MCP Server — 設計規格

> **Audience**：Go 工程師 + AI Agent 作者。定義 atlas-go 對 AI Agent 暴露能力的 MCP (Model Context Protocol) 伺服器設計。
> **狀態**：P1 設計階段（Stage 3 產出）
> **範圍決策**：「完整暴露」atlas-go 能力（使用者於 2026-06-30 確認）
> **關聯文件**：
> - [`WORKFLOW_MAP.md`](../../WORKFLOW_MAP.md)（workflow 總覽）
> - [`AGENTS.md`](../../AGENTS.md)（跨工具 AI 共用指引）
> - [`../specs/agent-loop-state-machine.md`](../specs/agent-loop-state-machine.md)（AgentLoop 細節）

---

## 1. 設計目標

讓任何 **MCP-compatible AI agent**（如 Cursor、Claude Desktop、OpenCode、自建 agent）能透過標準 MCP protocol：

1. **查詢** atlas-go 內部狀態（市場體制、agent 健康、風險指標、macro data）
2. **觸發** 已知安全邊界的動作（回測、批次任務、查詢類實驗操作）
3. **訂閱** event bus 事件流（drawdown breach、risk gate rejected、regime change）

**不做**：繞過現有 apigateway Constitution（6 條文 + 3 附錄）的存取。

---

## 2. 架構總覽

```
┌─────────────┐  stdio/SSE/streamable-HTTP  ┌──────────────────┐
│  AI Agent   │ ◀─────────────────────────▶ │  atlas-mcp       │
│ (Claude,    │   JSON-RPC 2.0              │  (Go native)     │
│  Cursor…)   │                              │  cmd/atlas-mcp/  │
└─────────────┘                              └────────┬─────────┘
                                                     │ HTTP (localhost only)│
                                                     ▼
                                              ┌──────────────┐
                                              │  atlas       │
                                              │  (現有 API)   │
                                              └──────────────┘
```

**關鍵決策**：
- **語言**：Go native（atlas-go 是 Go，避免語言生態污染）
- **MCP SDK**：✅ **Spike 完成（[`docs/spikes/mcp-go-sdk-spike.md`](../spikes/mcp-go-sdk-spike.md)）— 採用 OFFICIAL [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) v1.6.1**
  - **理由**：(1) Go 1.25.0 為下限（atlas-go 正式版為 1.26，OFFICIAL SDK 涵蓋升級後環境）— mark3labs 需要 1.25.5；(2) 已 v1.x stable，API locked；(3) 官方 + Google 合作，前向相容 MCP 2026-07-28 spec；(4) Apache-2.0 license（與 atlas-go 相容）；(5) 內建 `auth`/`oauthex` 套件便於未來擴充
  - **Trade-off**：star 數比 mark3labs 少（4.7k vs 8.9k）但官方 SDK 受 spec 維護組織認可
- **併入**：作為 `cmd/atlas-mcp/main.go` 子命令，可與現有 `cmd/atlas` 共用內部套件
- **Transport**：預設 **stdio**（最廣泛相容）；透過 flag 提供 SSE / streamable-HTTP（多客戶端）
- **Auth**：透過環境變數 `ATLAS_MCP_TOKEN`，呼叫端必須提供
- **範圍綁定**：MCP server 只 bind 127.0.0.1，避免對外暴露

---

## 3. MCP Tools 清單

下表為 atlas-go 全量 HTTP endpoints → MCP tool 映射。所有工具依照 **Workflow Area (WA-XXX)** 分群，與 [`WORKFLOW_MAP.md`](../../WORKFLOW_MAP.md) §3 對齊。

### 3.1 MCP Tools 全清單

**統一數字聲明**：本規格涵蓋 **約 70 個 MCP tool 候選**（最終暴露數）。上限 102（含未篩選的衍生 endpoints）；下限約 50（剔除所有 Admin-only / 副作用後的純讀取工具）。Phase 1 MVP 實作 **5 個核心 tool**。

> 為何是約 70：grep 結果共 268 個 `mux.Handle` / `mux.HandleFunc` match，去重後 HTTP endpoint 約 100-110 個。扣除 §3.2 列出的 10 個 Admin-only / 副作用 endpoint，再扣除 `#/static` 路由、`/admin/`、`/client/` SPA、`/metrics` Prometheus scrape、`/docs` swagger 等非業務 endpoint，最終候選數約 **70**。

| WA | 群組 | Tool 數 | 主要用途 |
|----|------|---------|---------|
| WA-101 資料源 | `data_*` | 4 | channel 健康、data pipeline 監控 |
| WA-102 標的宇宙 | `universe_*` | 2 | session 列表、universe 重疊分析 |
| WA-103 Macro 鏈 | `macro_*` | 6 | stress index、capital flow、macro snapshot |
| WA-200 體制 | `regime_*` | 1 | regime 歷史 |
| WA-201 敘事 | `narrative_*` | 9 | events/chains/models/templates/stress-index |
| WA-202 跨市場 | `crossmarket_*` | 3 | status/correlation/us-indices |
| WA-301 LLM Loop | `llm_*` | 2 | health、cost |
| WA-302 推理/Trace | `trace_*` | 4 | sim-latest、agent observatory、reasoning |
| WA-400 風險 | `risk_*` | 5 | gate 狀態、correlation、risk-calibration |
| WA-500 策略 | `strategy_*` | 8 | list/get/validate/annotate/summary |
| WA-501 達爾文 | `synergy_*` | 6 | darwinian + l2-4 schedule |
| WA-503 實驗 | `experiment_*` | 5 | promote/revert/judge/diff/history |
| WA-505 報告/稅務 | `report_*`、`tax_*`、`perf_*` | 4 | report + tax snapshot + perf |
| WA-601 警報 | `alert_*` | 8 | list/stats/rules/ack/resolve/silence |
| WA-603 控制平面 | `control_*` | 8 | pause/resume/ban/approve/reject/audit |
| WA-604 排程/任務 | `scheduler_*`、`task_*` | 9 | schedule status、task CRUD |
| WA-606 系統健康 | `system_*`、`health_*`、`metrics_*` | 12 | health/metrics/data-integrity/circuit |
| WA-700 PRISM | `prism_*` | 1 | training-results |
| WA-701 Swarm | `swarm_*` | 5 | status/consensus/anomalies/scenarios/strats |
| **總計** | | **~102** | （其中 32 個為 Admin-only / §3.2 排除；70 個為 MCP tool 候選） |

### 3.2 不暴露的 endpoints（安全邊界）

以下 endpoints **不在 MCP 範圍**：

| Endpoint | 原因 |
|----------|------|
| `/admin/reload-config` | 需 AdminPost 且有副作用；由 CLI `atlas reload-config` 處理 |
| `/api/admin/calibrate-thresholds` | 同上 |
| `/api/dashboard/api-keys/update` | 直接修改 secrets，必須人類觸發 |
| `/api/control/sector-ban` | 變動策略凍結，需 Admin 審計 |
| `/api/synergy/l2-4-schedule/start\|stop\|reset\|update` | L2.4 觀察窗口控制（避免腳本誤用） |
| `/api/alerts/silence` | 抑制 alert,易被誤用 |
| `/api/scheduler/toggle` | 全域排程開關 |
| `/api/experiment/revert` | baseline 還原,風險高 |
| `/api/control/set-model-weight` | 達爾文權重變更 |
| `/api/macro/ingest` | 觸發 macro 重算 |

這些仍可透過 **graphical admin UI** (`/admin/`) 或 **直接 HTTP 呼叫**（Admin 後台）進行，但**不會**暴露給外部 AI Agent。

### 3.3 MCP Tool 命名慣例

```
<area>_<verb>_<noun>?
e.g.   regime_get_history
       strategy_list_active
       experiment_judge
       alert_acknowledge
```

規則：
- 使用 snake_case（與 snake_case JSON tag 慣例一致）
- `area` 與 `verb` 必填
- `noun` 可選，取決於 verb 是否區分對象
- `*_list` 與 `*_get` 區分（list = 集合、get = 單筆）

---

## 4. JSON Schema 設計範本

每個 MCP tool 必須附帶完整 JSON Schema（LLM 用於 function calling 推論）。範本如下：

```json
{
  "name": "regime_get_history",
  "description": "取得指定期間的市場體制歷史（RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL）。觸發時機：當 agent 需要理解市場環境演變以決策策略倉位時。回傳 sorted by date desc。",
  "inputSchema": {
    "type": "object",
    "properties": {
      "start_date": {
        "type": "string",
        "format": "date",
        "description": "起始日期 (YYYY-MM-DD)，預設 = today - 30 days"
      },
      "end_date": {
        "type": "string",
        "format": "date",
        "description": "結束日期 (YYYY-MM-DD)，預設 = today"
      },
      "symbol": {
        "type": "string",
        "description": "可選，限制為單一標的"
      }
    },
    "required": []
  },
  "annotations": {
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": false
  }
}
```

每個 schema 必須遵守：
- `readOnlyHint` / `destructiveHint` 標註清楚（MCP protocol annotation）
- `description` 必須含「**觸發時機**」與「**與其他 tool 的區別**」（讓 LLM 知道何時該用這個 tool）
- 所有可選參數在 `required: []` 內只列必填

---

## 5. 架構設計

```
cmd/atlas-mcp/
├── main.go            // 入口 + flag 解析 + stdio / SSE 啟動
├── server/
│   ├── server.go      // MCP server lifecycle
│   ├── tools.go       // 70 個候選 tool 註冊（依 §3 定義）
│   ├── transport.go   // stdio / SSE / streamable-HTTP 切換
│   └── auth.go        // ATLAS_MCP_TOKEN 驗證
├── tools/
│   ├── regime.go      // regime_* tools (對應 /api/dashboard/regime-history)
│   ├── strategy.go    // strategy_* tools
│   ├── ...            // 共 ~20 個檔案，每檔一類
│   └── helpers.go     // 共用 HTTP 呼叫 + error 轉譯
└── README.md          // 部署 + 使用說明
```

### 5.1 共用 Tool Base（helpers.go）

每個 tool handler 透過單一 helper 呼叫內部 HTTP API：

```go
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)

func callAtlasAPI(
    ctx context.Context,
    method, path string,
    body, result any,
) error {
    // 統一呼叫 atlas-go (http://127.0.0.1:8080)
    // 自動注入 ATLAS_API_KEY（複用 admin API key 機制）
    // 自動錯誤轉譯（atlas 回傳的 error JSON → MCP error code）
}
```

**好處**：所有 tool 不用各自重寫 HTTP call、MCP server 與 atlas 主程式鬆耦合。

### 5.2 工具註冊（tools.go）

```go
func RegisterAllTools(srv *mcp.Server) {
    regime.Register(srv)         // WA-200
    strategy.Register(srv)      // WA-500
    experiment.Register(srv)    // WA-503
    risk.Register(srv)          // WA-400
    alert.Register(srv)         // WA-601
    control.Register(srv)       // WA-603 (limited subset)
    // ...
}
```

### 5.3 Transport 切換（transport.go）

```go
func RunServer(transport string) error {
    switch transport {
    case "stdio":
        return runStdio()
    case "sse":
        return runSSE(":9090")
    case "streamable-http":
        return runStreamableHTTP(":9090")
    default:
        return fmt.Errorf("unknown transport: %s", transport)
    }
}
```

預設 `stdio`（最廣泛相容於 Claude Desktop、Cursor 等 IDE）；可在 config 指定切換。

---

## 6. 安全模型

### 6.1 認證

```
┌──────────┐                  ┌────────────┐                 ┌──────────┐
│  Agent   │  Bearer ATLAS_   │  atlas-mcp │  ATLAS_API_KEY  │  atlas   │
│  client  │ ──MCP_TOKEN──▶  │            │ ──(loopback)─▶ │  server  │
│          │                  │ validates  │                 │          │
└──────────┘                  │ token &    │                 └──────────┘
                               │ whitelist  │
                               │ allowed_   │
                               │ IPs        │
                               └────────────┘
```

- **Token 1**：`ATLAS_MCP_TOKEN`（MCP 端）—— Agent 提供給 atlas-mcp 驗證
- **Token 2**：`ATLAS_API_KEY`（admin 端）—— atlas-mcp 內部呼叫 atlas 時使用（如要呼叫 admin 端點）

`ATLAS_MCP_TOKEN` 與 `ATLAS_API_KEY` **分離**，即使 MCP token 洩漏也不直接取得 atlas admin 存取權。

### 6.2 網路邊界

- `atlas-mcp` 只 listen `127.0.0.1`（絕不對外）
- 即使走 SSE / streamable-HTTP 也不綁 `0.0.0.0`
- 預設拒絕所有 IP（除 127.0.0.1），可在 config 開放（如要塞內其他 agent）

### 6.3 危險操作保護

對任何 `destructiveHint=true` 的 tool，預設啟用兩階段確認：

```json
{
  "name": "experiment_judge",
  "description": "觸發 LLM judge 評分候選策略 vs baseline。⚠️ 副作用：寫入 experiment history。",
  "inputSchema": { ... },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": true
  }
}
```

MCP server 在執行前 log warning 到本地檔（`atlas-mcp-audit.log`），便於人類稽核。

---

## 7. 部署

### 7.1 編譯

```bash
go build -o bin/atlas-mcp ./cmd/atlas-mcp/
```

### 7.2 啟動範例（stdio）

```bash
ATLAS_MCP_TOKEN=xxx ATLAS_API_KEY=yyy ./bin/atlas-mcp --transport=stdio
```

Cursor / Claude Desktop 的 `mcp.json`：

```json
{
  "mcpServers": {
    "atlas-go": {
      "command": "/path/to/atlas-mcp",
      "args": ["--transport=stdio"],
      "env": {
        "ATLAS_MCP_TOKEN": "xxx",
        "ATLAS_API_KEY": "yyy"
      }
    }
  }
}
```

### 7.3 啟動範例（SSE）

```bash
ATLAS_MCP_TOKEN=xxx ./bin/atlas-mcp \
  --transport=sse \
  --bind=127.0.0.1:9090
```

Client 端（任意 SSE 相容 agent）：
- POST `http://127.0.0.1:9090/mcp`
- Header: `Authorization: Bearer xxx`

---

## 8. 實作計畫（給後續 PR）

### Phase 1：MVP（建議第 1 個 PR）
- [ ] `cmd/atlas-mcp/` 雛型 + stdio transport
- [ ] 認證 + audit log
- [ ] 5 個核心 tool（regime_get_history, strategy_list, experiment_judge, alert_list, health_check）
- [ ] README + 自訂測試（任何 LLM agent 跑得起來）

### Phase 2：核心擴展（建議第 2-3 個 PR）
- [ ] SSE + streamable-HTTP transport
- [ ] macro/narrative/risk 群組
- [ ] schema 完整化（含每個 tool 的 `description` + annotation）

### Phase 3：完整覆蓋
- [ ] 所有 **70 個候選** tool 全部實作（依 §3 定義；Admin-only 已剔除）
- [ ] 整合測試（用 mock LLM 模擬 agent 行為）
- [ ] 效能基準（latency、token 使用量）

---

## 9. 驗收標準

1. **功能**：列出所有 70 個候選 tool（含 description + JSON Schema）
2. **單元測試**：每個 tool handler 都有 `*_test.go`，覆蓋率 ≥80%
3. **整合測試**：3 個典型 agent scenario（如 daily briefing、portfolio QA、risk check）
4. **安全**：Admin 端點 list 與 MVP 一致，audit log 路徑清楚
5. **文件**：`cmd/atlas-mcp/README.md` 完整（部署、token 管理、troubleshooting）

---

## 10. 開放議題（待使用者決策）

1. **是否要支援 GraphQL 而非 JSON-RPC？** 現 MCP 標準為 JSON-RPC，GraphQL 需要額外 stdio adapter
2. **是否要支援 streaming tool（長時執行）？** 範例：`backtest_run` 觸發後等 1 分鐘回傳。預設不支援，透過 polling
3. **`atlas-mcp` 與 `atlas` 是否要併入同一 binary？** 預設是（單一 binary，多子命令），但需評估 binary 大小
4. **是否要 binary-internal version（取代現有 HTTP API）？** 預設否，HTTP 仍為主，MCP 為附加層
5. **audit log 保留期？** 預設 30 天（合規最低），可調
