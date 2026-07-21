# Atlas MCP Server — 設計規格

> **Audience**：Go 工程師 + AI Agent 作者。定義 atlas-go 對 AI Agent 暴露能力的 MCP (Model Context Protocol) 伺服器設計。
> **狀態**：P1 設計階段（Stage 3 產出）
> **範圍決策**：「完整暴露」atlas-go 能力（使用者於 2026-06-30 確認）
> **關聯文件**：
> - [`workflow-map.md`](../reference/workflow-map.md)（workflow 總覽）
> - [`AGENTS.md`](../../AGENTS.md)（跨工具 AI 共用指引）
> - [`agent-loop-state-machine.md`](./agent-loop-state-machine.md)（AgentLoop 細節）

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
  - **理由**：(1) Go 1.25.0 為下限（atlas-go 正式版為 1.26，OFFICIAL SDK 涵蓋升級後環境）— mark3labs 需要 1.25.5；(2) 已 v1.x stable，API locked；(3) 官方 + Google 合作，前向相容 MCP 2026-07-28 spec；(4) 上游 Apache-2.0（獨立第三方授權，與 atlas-go AGPL v3 並存；僅作相依評估，不影響 atlas-go 本體授權）；(5) 內建 `auth`/`oauthex` 套件便於未來擴充
  - **Trade-off**：star 數比 mark3labs 少（4.7k vs 8.9k）但官方 SDK 受 spec 維護組織認可
- **併入**：作為 `cmd/atlas-mcp/main.go` 子命令，可與現有 `cmd/atlas` 共用內部套件
- **Transport**：預設 **stdio**（最廣泛相容）；透過 flag 提供 SSE / streamable-HTTP（多客戶端）
- **Auth**：透過環境變數 `ATLAS_MCP_TOKEN`，呼叫端必須提供
- **範圍綁定**：MCP server 只 bind 127.0.0.1，避免對外暴露

---

## 3. MCP Tools 清單

下表為 atlas-go 全量 HTTP endpoints → MCP tool 映射。所有工具依照 **Workflow Area (WA-XXX)** 分群，與 [`workflow-map.md`](../reference/workflow-map.md) §3 對齊。

### 3.1 MCP Tools 全清單

**統一數字聲明**：最終對 agent 暴露的 tool 名稱、數量與分類，以 [`docs/reference/tool-catalog.md`](../reference/tool-catalog.md) 為單一權威來源。啟動期權威計數來自 [`cmd/atlas-mcp/auto-desc.gen.json`](../../cmd/atlas-mcp/auto-desc.gen.json)（`server.Run()` 在啟動時 assert `RegisteredToolCount ∈ [110, 112]`，防止文件↔程式碼漂移；110 = 業務 102 + roots 2 + template_detector 2 + audit 4，post-Round-2 dedup 基線；sampling/elicitation feature-gated 各 +1 達 112）。

**當前實際**：**110 個 tool**(業務 102 + template_detector 2 + roots 2 + audit 4；sampling / elicitation feature-gated 各 +1 達 112，但**預設 OFF**)。本節保留 high-level 群組對照；單一 tool 名稱請以 `auto-desc.gen.json` 為準。

| WA | 群組 | Tool 數 | 主要用途 |
|----|------|---------|---------|
| WA-101 資料源 | `data_*` | 4 | channel 健康、data pipeline 監控 |
| WA-102 標的宇宙 | `universe_*` | 2 | session 列表、universe 重疊分析 |
| WA-103 Macro 鏈 | `macro_*` | 6 | stress index、capital flow snapshot、macro snapshot |
| WA-104 個股 | `stock_*` | 4 | quote、fundamentals、chips、technical |
| WA-105 資金流 | `capital_flow_*` | 2 | daily Z-score 分解、summary |
| WA-200 體制 | `regime_*` | 1 | regime 歷史 |
| WA-201 敘事 / 事件 | `narrative_*`、`event_*` | 9 | events/chains/models/templates/stress-index + event calendar/prediction |
| WA-202 跨市場 | `crossmarket_*` | 3 | status/correlation/us-indices |
| WA-301 LLM Loop | `llm_*` | 2 | health、cost |
| WA-302 推理/Trace | `trace_*` | 4 | sim-latest、agent observatory、reasoning |
| WA-400 風險 | `risk_*` | 5 | gate 狀態、correlation、risk-calibration |
| WA-500 策略 | `strategy_*`、`strategy_ranker` | 6 | list/get/attribution/summary + ranked strategies |
| WA-501 達爾文 | `synergy_*` | 3 | darwinian + l2-4 schedule |
| WA-503 實驗 | `experiment_*` | 3 | judge/diff/history |
| WA-505 報告/稅務 | `report_*` | 4 | report + tax snapshot + perf + export |
| WA-520 推薦 | `get_recommendations` | 1 | tier-gated 投資組合推薦（v0.0.0.31 新；需 JWT） |
| WA-601 警報 | `alert_*` | 4 | list/stats/rules/unacknowledged |
| WA-603 控制平面 | `control_*` | 4 | approve/reject/audit/overrides |
| WA-604 排程/任務 | `scheduler_*`、`task_*` | 4 | schedule status、task CRUD |
| WA-606 系統健康 | `system_*` | 7 | health/metrics/trends/thresholds/pipeline/circuit/maturity |
| WA-700 PRISM | `prism_*` | 1 | training-results |
| MCP 自我觀測 | `mcp_get_*`、`mcp_anomaly_*` | 6 | session topology、call stats、tenant usage、slow tools + anomaly |
| MCP 協議擴充 | `mcp_roots_*`、`mcp_elicit_user`、`mcp_sample_llm` | 4 | Phase 4 protocol extensions（roots/elicitation/sampling）|
| Daily Briefing | `mcp_quickstart`、`daily_report` | 2 | 一站式摘要、每日報告 |
| **總計（v0.0.0.31）** | | **91** | 87 業務 + 4 audit（`mcp_get_*`） |

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

## 3.4 MCP Prompts 清單（v0.0.0.31 PR #972 新增）

6 個預設 Prompt 模板供外部 AI 按名稱調用，分散於 `cmd/atlas-mcp/server/prompts.go`：

| Prompt 名稱 | 用途 | 調用工具 |
|-----------|------|---------|
| `taiwan_quick_look` | 台股今日快覽：宏觀快照 + 資金流向 + 壓力指數 + 事件 | macro + capitalflow + stress + events |
| `strategy_advice` | 策略建議：策略排名 + 風險評論 + 盤勢歷史 | strategy_ranker + risk + regime |
| `stock_health_check` | 持股健檢：輸入股票代號（`symbol="2330"`） | trace + universe + strategy |
| `daily_market_briefing` | 每日市場簡報（英文版） | macro + capitalflow + events |
| `risk_check` | 投資組合風險評估 | risk + darwinian |
| `regime_interpretation` | 盤勢解讀（`regime="RISK_ON"`） | regime + narrative |

## 3.5 MCP Resources 清單（v0.0.0.31 PR #972 新增）

3 個 MCP Resources 提供結構化資料存取：

| URI | 內容 | Handler |
|-----|------|---------|
| `atlas://strategies/active` | 當前活躍策略定義（JSON） | `internal/strategy` 直接讀取 |
| `atlas://market/regime` | 最新盤勢分類 + 壓力指數 | `internal/regime.GetCurrent` |
| `atlas://events/today` | 今日事件清單 | `internal/industry.EventCalendar` |

## 3.6 v0.0.0.31 新增 MCP Tools（合計 +12）

本節彙整 v0.0.0.31 全部新增工具（已併入 §3.1 表格的對應 WA 群組）；§3.7（個股/資金流/排名）已併入 §3.6，依「主題分類」重新編排為四個小節，便於按場景查找。

**一站式摘要 + 每日報告（2 個）**：

| Tool 名稱 | 用途 | 對應 API |
|---------|------|---------|
| `mcp_quickstart` | 一站式開機摘要：macro + 策略 + 壓力 + 事件 + 資金流向 | 多源聚合 |
| `daily_report` | 最新每日市場報告完整 JSON | `/api/reports/latest` |

**事件日曆與事件驅動資金流（2 個）**：

| Tool 名稱 | 用途 | 對應 API |
|---------|------|---------|
| `event_calendar` | 近期事件列表（14 日 forward） | `/api/events/calendar` |
| `event_flow_prediction` | 5 日事件驅動資金流預測 | `/api/events/prediction` |

**個股四件套（4 個）**：

| Tool 名稱 | 用途 | 對應 API |
|---------|------|---------|
| `stock_get_quote` | 個股即時報價（Fugle） | `/api/stock/quote?symbol={symbol}` |
| `stock_get_fundamentals` | 個股基本面（PE/PB/PS/Yield/Sector） | `/api/stock/fundamentals?symbol={symbol}` |
| `stock_get_chips` | 個股籌碼面（外資/投信/自營商） | `/api/stock/chips?symbol={symbol}&date={date}` |
| `stock_get_technical` | 個股技術面（SMA20/50/RSI14） | `/api/stock/technical?symbol={symbol}&days={days}` |

**資金流日報 + 策略排名 + tier-gated 推薦（4 個）**：

| Tool 名稱 | 用途 | 對應 API |
|---------|------|---------|
| `capital_flow_daily` | 全市場七維錢潮雷達（3+2+2 分層）共振分析：官方法人 / 行為代理 / 領先＋跨市場訊號 — 詳見 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 | `/api/capital-flow/daily` |
| `capital_flow_summary` | 資金流摘要（給 morning briefing） | `/api/capital-flow/summary` |
| `strategy_ranker` | 策略排名（依勝率 + tier 標 free/registered/premium） | `/api/strategy-ranker/rank` |
| `get_recommendations` | tier-gated 投資組合推薦（**需 JWT**） | `/api/recommendations` |

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
│   ├── tools.go       // 註冊所有業務 tool（詳見 §3 與 `docs/reference/tool-catalog.md`）
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
    // 統一呼叫 atlas-go (http://127.0.0.1:18080)
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
    "atlas-mcp": {
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

---

## 11. Phase 2.2 Implementation Status（2026-06-30）

**狀態：✅ COMPLETE** — 14 batches 全部 force-push 到 PR #834（`feat/atlas-mcp-phase2`）。

### 工具擴展
- Phase 1 baseline：5 tools（stdio only，process isolation 為 security boundary）
- Phase 2.2 擴展：69 新 tools，分 14 個小 batch 推上
- **PR #834 累計：74 tools / 99 tests / `-race` 綠**

### 完整 Tool Catalog（74，per spec §3.1 候選清單全部上線）

| Area | Tools | Count |
|------|-------|------|
| Phase 1 core | `regime_get_history`, `strategy_list_active`, `experiment_judge`, `alert_list_unacknowledged`, `system_get_health` | 5 |
| Macro | `macro_get_snapshot_latest`, `macro_get_snapshot_history`, `macro_get_stress_index_current`, `macro_get_stress_index_history`, `macro_get_capital_flow_latest`, `macro_get_ingest_status` | 6 |
| Crossmarket | `crossmarket_get_status`, `crossmarket_get_correlation`, `crossmarket_get_us_indices` | 3 |
| Narrative | `narrative_get_events`, `narrative_get_chains`, `narrative_get_models`, `narrative_get_templates`, `narrative_get_seasonal`, `narrative_get_bundle`, `narrative_stress_index_thresholds` | 7 |
| Risk | `risk_get_metrics`, `risk_get_correlation_matrix`, `risk_get_drawdown`, `risk_get_calibration`, `risk_get_commentary` | 5 |
| Alert (new) | `alert_list`, `alert_get_stats`, `alert_get_rules` | 3 |
| Strategy (new) | `strategy_get_layers`, `strategy_get`, `strategy_get_attribution`, `strategy_get_summary` | 4 |
| Experiment (new) | `experiment_diff`, `experiment_history` | 2 |
| Synergy | `synergy_get_darwinian_status`, `synergy_get_darwinian_trend`, `synergy_get_l2_4_schedule` | 3 |
| Control (read-only) | `control_get_audit_log`, `control_get_active_overrides`, `control_approve_recommendation`, `control_reject_recommendation` | 4 |
| Scheduler/Task | `scheduler_get_status`, `task_list`, `task_get`, `task_get_events` | 4 |
| System (new) | `system_get_metrics`, `system_get_metrics_trend`, `system_get_thresholds`, `system_get_data_pipeline`, `system_get_circuit_breaker`, `system_get_maturity` | 6 |
| LLM | `llm_get_cost`, `llm_get_health` | 2 |
| Trace | `trace_get_sim_latest`, `trace_get_agent_observatory`, `trace_get_decision_chain`, `trace_get_reasoning` | 4 |
| Data | `data_get_channels`, `data_get_channel_detail`, `data_get_quality`, `data_get_field_contract` | 4 |
| Universe | `universe_get_sessions`, `universe_get_universe_overlap` | 2 |
| Report | `report_get_daily_summary`, `report_get_performance`, `report_get_tax_snapshot`, `report_get_export_link` | 4 |
| Prism | `prism_get_training_results` | 1 |

### Admin / destructive 排除清單（per §3.2）

以下端點**故意不暴露**給 MCP（需 admin 權限或 destructive）：

- `/admin/reload-config`, `/api/admin/calibrate-thresholds`, `/api/dashboard/api-keys/update`
- `/api/control/{sector-ban, set-model-weight, pause-agent, resume-agent}`
- `/api/experiment/{promote, revert}`
- `/api/alerts/{acknowledge, acknowledge-bulk, resolve, silence}`
- `/api/synergy/l2-4-schedule/{start, stop, reset, update}`
- `/api/scheduler/toggle`
- `/api/strategies/{id}/annotate`
- `/api/tasks` (POST), `/api/tasks/{id}/{cancel, retry, confirm}`

這些操作仍可透過 `/admin/` 管理後台或直接 HTTP 呼叫觸發（需 apigateway 認證）。

### Transport 演進

- **Phase 1**：stdio only，process isolation 為 security boundary
- **Phase 2.1**：新增 streamable-HTTP + SSE transports；Bearer auth 可強制（`--auth=required`）
  - stdio 仍 permissive（process isolation）
  - HTTP/SSE 經 `Authorization: Bearer <ATLAS_MCP_TOKEN>` 驗證
- **Phase 2.2**：74 tools 全部支援所有 3 個 transport

### 驗證（截至 2026-06-30）

- `go build ./cmd/atlas-mcp/...` exit 0
- `go test -count=1 -race ./cmd/atlas-mcp/...`：**99 PASS / 0 FAIL**
- PR #834：1 個 Phase 2.1 commit + 1 個 fix commit + 14 個 batch commits（每 batch 一個 commit）
- 全部 14 batch 透過 `--force-with-lease` 推上

---

## 12. Phase 4 B — Protocol Extensions（2026-07-01）

**狀態：✅ IMPLEMENTED** — PR #B（`feat/mcp-protoext`，Phase 4 B 單一 commit，1421 insertions）。

### 新增 Primitives

| Primitive | MCP SDK 1.6.1 | Tool | Default | 說明 |
|-----------|---------------|------|---------|------|
| **Sampling** | `CreateMessage` / `CreateMessageWithTools` | `mcp_sample_llm` | `false` | server 端向 client LLM 發起取樣請求；需 ATLAS_MCP_SAMPLING_ENABLED=true |
| **Roots** | `ListRoots` / `RootsListChangedHandler` | `mcp_roots_list`、`mcp_roots_read_file` | always on | read-only filesystem access；強制 O_RDONLY + path traversal hardening + symlink escape 防護 + per-read audit + 1MB size cap |
| **Elicitation** | `Elicit` with Mode="form" | `mcp_elicit_user` | `false` | server 向使用者主動提問；需 ATLAS_MCP_ELICITATION_ENABLED=true |

### 安全邊界

| 機制 | 說明 |
|------|------|
| Roots read-only | `os.OpenFile(O_RDONLY)` only, write flag → error |
| Path traversal | `filepath.EvalSymlinks` + `filepath.Clean` + prefix check |
| Symlink escape | RED test `TestMCPRootsReadFile_SymlinkEscape_Rejected` (CI 強制) |
| Symlink TOCTOU | `OpenFile` 使用 `O_NOFOLLOW` flag 關閉 re-resolve TOCTOU 視窗（#901/#902） |
| AllowedRoots gate | Server-side `validateAllowedRoots` 拒絕系統根目錄（`/`、`/etc`、`/proc` 等），`ATLAS_MCP_ROOTS_ALLOW_UNSAFE=1` escape hatch（#903） |
| Audit per read | `withAuditExtra` 記錄 `{path, size_bytes, ts, tenant_id}` |
| Size cap | 1MB (`io.LimitReader`) |
| Sampling opt-in | Default OFF, env flag required |
| Elicitation opt-in | Default OFF, env flag required |
| Elicitation schema validate | Server-side `validateElicitSchema` rejects oversized (>16KB), external `$ref`, >20 properties, >64-char property names — defense-in-depth over SDK's own structural check |
| Capability miss | Explicit error (no silent fallback) |

### 新增檔案

```
cmd/atlas-mcp/server/
├── sampling.go + _test.go      (Sampling: 124 + 176 LOC)
├── roots.go + _test.go         (Roots: 239 + 165 LOC)
├── elicitation.go + _test.go   (Elicitation: 103 + 135 LOC)
└── mcp_session_test.go         (SDK session accessor test: 63 LOC)

internal/apigateway/
└── constitution.md 附錄 D 新增 Roots Sanctioned Exception
```

### 驗證（2026-07-01）

- `go test -count=1 -race ./cmd/atlas-mcp/...`：**121 PASS / 0 FAIL**
- `go vet` / `gofmt -l .` / `staticcheck`：clean
- Coverage：63.3%（≥ 60%)
- oracle audit：P0（CONSTITUTION 附錄 D）+ 3×P1 → **ALL FIXED → READY TO MERGE**

### 對應的 MCP server 程式碼地圖（新增項目）

```
cmd/atlas-mcp/server/
├── sampling.go + _test.go       (B.1 Sampling — opt-in LLM call)   ← NEW
├── roots.go + _test.go          (B.2 Roots — read-only fs)         ← NEW
├── elicitation.go + _test.go    (B.3 Elicitation — user prompt)     ← NEW
└── mcp_session_test.go          (SDK helper coverage)               ← NEW
```

```
cmd/atlas-mcp/
├── main.go                 (env wiring, flag parsing, dispatch entry)
├── e2e_test.go             (real subprocess stdio JSON-RPC test)
├── main_test.go            (package compile smoke)
├── README.md               (deployment + client config)
└── server/
    ├── server.go           (Run dispatch + Config)
    ├── auth.go             (TokenAuth — Phase 2.1)
    ├── audit.go            (JSONL writer)
    ├── http_client.go      (atlas HTTP bridge)
    ├── metrics.go          (Phase 4A: Prometheus metrics)
    ├── tools_anomaly.go    (Phase 4A: anomaly tools)
    ├── transport_stdio.go  (Phase 1, stdio transport)
    ├── transport_http.go   (Phase 2.1, streamable-HTTP)
    ├── transport_sse.go    (Phase 2.1, SSE)
    ├── transport_auth.go   (Phase 2.1, Bearer middleware)
    ├── tools.go             (registerTools dispatch)
    ├── tools_phase1_core.go (Phase 1's 5 tools inline)
    ├── tools_macro.go + _test.go
    ├── tools_crossmarket.go + _test.go
    ├── tools_narrative.go + _test.go
    ├── tools_risk_alert.go + _test.go
    ├── tools_strategy.go + _test.go
    ├── tools_experiment.go + _test.go
    ├── tools_synergy.go + _test.go
    ├── tools_control.go + _test.go
    ├── tools_scheduler_task.go + _test.go
    ├── tools_system.go + _test.go
    ├── tools_llm_trace.go + _test.go
    ├── tools_data_universe.go + _test.go
    └── tools_report_prism.go + _test.go
```

## 12. Phase 4 Direction A — Observability（2026-07-01）

**狀態：✅ IMPLEMENTED** — 內建 Prometheus `/metrics` + 自訂 anomaly detector + alert integration。

### 新增元件

| 元件 | 位置 | 職責 |
|------|------|------|
| Prometheus Exporter | `cmd/atlas-mcp/server/metrics.go` | `/metrics` endpoint；暴露 5 種 metrics |
| Anomaly Detector | `internal/mcp/anomaly/` | rolling-window z-score + per-tool/per-tenant error rate |
| Anomaly Tools | `cmd/atlas-mcp/server/tools_anomaly.go` | `mcp_anomaly_get_recent`、`mcp_anomaly_ack` |

### 新增 Tools

| Tool | 類型 | 說明 |
|------|------|------|
| `mcp_anomaly_get_recent` | read | 列出最近 N 條 anomaly event |
| `mcp_anomaly_ack` | destructive | 透過 `/api/alerts/acknowledge` 確認 anomaly |

### Metrics

- `mcp_calls_total{tool, transport, status}` — Counter
- `mcp_call_duration_seconds{tool, transport}` — Histogram
- `mcp_active_sessions{transport}` — Gauge
- `mcp_token_usage_total{tenant_id}` — Counter
- `mcp_anomaly_score{tenant_id, anomaly_type}` — Gauge

### 驗證

- `go test ./cmd/atlas-mcp/... ./internal/mcp/anomaly/...`：**23 個新增測試 PASS / 0 FAIL**
- `/metrics` 綁定 `127.0.0.1`，獨立於 MCP transport port
- anomaly detector 3 種基線 unit test 通過

---

## 13. 後續版本演進（v0.0.0.30+ → v0.0.0.31+）

> §11 與 §12 為 PR #834 / Phase 4 當時的快照（截至 2026-07-01）。v0.0.0.31 起持續擴展，本節追蹤增量。

### 13.1 v0.0.0.31（2026-07-06 PR #945–#950+）

**Tool 演進**：74（§11 末）→ 87（v0.0.0.31 baseline，含 Phase 4 B extensions）→ **91（當前）**

| 變更類型 | 增量 | 對應 WA / Primitive |
|---------|------|--------------------|
| 新增 | 4 | `capital_flow_*` (WA-105)、`event_*` (WA-201) |
| 新增 | 4 | `stock_*` 四件套 (WA-104) |
| 新增 | 2 | `mcp_quickstart`、`daily_report` (Daily Briefing 群組) |
| 新增 | 1 | `strategy_ranker` (WA-500) |
| 新增 | 1 | `get_recommendations` (WA-520，需 JWT) |
| 新增 | 4 | `mcp_get_*` audit metrics（Phase 4 A 補入） |
| 已含 | 3 | `mcp_roots_*` (2) + `mcp_elicit_user` (1)（Phase 4 B extensions） |
| 已含 | 1 | `mcp_sample_llm`（Phase 4 B，opt-in via `ATLAS_MCP_SAMPLING_ENABLED=true`） |
| 已含 | 2 | `mcp_anomaly_*`（Phase 4 A） |

**計數規則**（v0.0.0.31 起）：

- 業務 tool：87 個（不含 audit）
- audit tool：4 個（`mcp_get_call_stats`、`mcp_get_session_topology`、`mcp_get_tenant_usage`、`mcp_get_top_slow_tools`）
- **總計**：108 個
- 編譯期 assert：`server.Run()` 啟動時檢查 `RegisteredToolCount ∈ [108, 110]`（`cmd/atlas-mcp/server/server.go`），防止文件↔程式碼漂移

**Module 演進**（同步參考 `internal/AGENTS_INDEX.md`）：59 模組（22 S / 23 E / 9 X / 5 U），新增 7 個 v0.0.0.31 模組（`capitalflow`、`eventdriven`、`recommender`、`dailyreport`、`strategy_ranker`、`strategy_validator`、`subscription`）。

**Protocol extensions 啟用狀態**：

- Sampling：預設 OFF，`ATLAS_MCP_SAMPLING_ENABLED=true` 啟用
- Elicitation：預設 OFF，`ATLAS_MCP_ELICITATION_ENABLED=true` 啟用
- Roots：always on（`ATLAS_MCP_ROOTS_ALLOW_UNSAFE=1` 為系統根目錄 escape hatch）
- Anomaly detector：always on（Phase 4 A）

### 13.2 規範變更（v0.0.0.31）

- §3.1 表格新增 `WA-520 推薦` 與「MCP 協議擴充」群組（4 個：roots/elicitation/sampling）
- §3.6 重構為「按主題分類」的四小節；§3.7（個股/資金流/排名）合併入 §3.6
- §3.4 MCP Prompts（6 個）、§3.5 MCP Resources（3 個）為 v0.0.0.31 起的標準配備
- §3.2 不暴露 endpoint 維持原樣（live broker、calibrate、sector-ban、revert 等）

### 13.3 參考

- 工具單一名稱 / 數量：`cmd/atlas-mcp/auto-desc.gen.json`（編譯期權威）
- 模組成熟度：`internal/MATURITY.md`
- 模組索引：`internal/AGENTS_INDEX.md`
- 工具操作導覽（agent 友善）：`docs/reference/tool-catalog.md`
- 部署與客戶端配置：`cmd/atlas-mcp/README.md`

