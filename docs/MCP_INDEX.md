# Atlas MCP Index｜給外部 AI Agent 的入口指南

> **本文件**：外部 MCP 客戶端（Cursor、Claude Desktop、OpenCode、Copilot、OpenClaw 等）連線 `atlas-mcp` 的第一份必讀文件。讀完即可知道能做什麼、如何驗證連線、邊界在哪、後續要看哪份深入文件。
> **目標**：5 分鐘內取得四個問題的答案：能做什麼、怎麼連、邊界在哪、接下來讀哪。
> **不涵蓋**：內部 tool 開發細節、codegen、CI 流程。

> ⚠️ **重要提示：atlas-mcp 的 tool 定義是 compile-time 產生的**，反映的是 `atlas-mcp` binary **建置時**的 Go 程式碼狀態。若 Go 程式碼變更後未重新 `go build` atlas-mcp binary，MCP client 看到的 tool schema 可能與實際系統行為有落差。agent 若發現 tool 行為與文件不符，請先檢查 atlas-mcp binary 是否已重新建置。
>
> 程式碼智慧工具（GitNexus / codebase-memory / codegraph）則不受此限制 — 它們是**即時索引**，隨時反映程式碼最新狀態。工具選擇指引見 [`docs/TOOLS.md`](./TOOLS.md)。

---

## 一句話：atlas-mcp 是什麼

`atlas-mcp` 是 atlas-go 台股投資研究系統的官方 MCP (Model Context Protocol) 伺服器。透過標準 JSON-RPC 2.0 與 MCP 工具描述,讓任何外部 AI Agent 可查詢市場體制、敘事事件、portfolio 風險、回測任務與系統健康度,並對已知安全邊界的動作（觸發回測、查詢實驗結果、警報確認、寫入控制平面 read-only 指令）發出輕度指令。每次呼叫皆寫入稽核日誌。

---

## 快速開始

### 1. 啟動 atlas-mcp 二進位

```bash
go build -o bin/atlas-mcp ./cmd/atlas-mcp/
ATLAS_BASE_URL=http://127.0.0.1:18080 ATLAS_API_KEY=your-key \
  ./bin/atlas-mcp
```

預設走 stdio transport,從 stdin 讀取 JSON-RPC 請求、往 stdout 寫回應。`ATLAS_BASE_URL` 指向已啟動的 atlas-go HTTP API。

### 2. 註冊到 MCP Client

#### OpenCode（`opencode.json`）

```json
{
  "mcp": {
    "atlas-mcp": {
      "type": "local",
      "command": ["/absolute/path/to/bin/atlas-mcp"],
      "env": {
        "ATLAS_BASE_URL": "http://127.0.0.1:18080",
        "ATLAS_API_KEY": "your-key"
      }
    }
  }
}
```

Claude Desktop 與 Cursor 採用相同 JSON schema,放置於各自的 MCP 設定檔中。完整設定範例與可用環境變數清單見 [`cmd/atlas-mcp/README.md`](../cmd/atlas-mcp/README.md)。

### 3. 確認連通

呼叫 `system_get_health`。成功回應應包含 LLM provider 狀態、router 版本、系統整體健康度等欄位。

若 timeout 或 4xx：`ATLAS_BASE_URL` / `ATLAS_API_KEY` 不對、或 atlas HTTP API 未啟動。請先確認 `curl http://127.0.0.1:18080/health` 回 `ok`。

---

## 能做什麼

`atlas-mcp` 約 84 個 tool,依任務群組織如下：

| 任務群組 | 入口 tool |
|---------|----------|
| **市場體制與敘事** | `regime_get_history`、`narrative_get_events`、`macro_get_snapshot_latest` |
| **跨市場與資料品質** | `crossmarket_get_status`、`data_get_quality` |
| **投資組合與風險** | `risk_get_metrics`、`risk_get_drawdown`、`strategy_list_active` |
| **策略與實驗** | `experiment_judge`、`strategy_get`、`synergy_list` |
| **任務、排程、控制** | `task_list`、`task_get_events`、`control_list_actions`、`audit_get_logs` |
| **系統健康與警報** | `system_get_health`、`llm_get_health`、`alert_list` |

Tool 命名遵循 `<area>_<verb>_<noun?>` 慣例,全 snake_case。完整 catalog 與調用決策樹見 [`AGENT_TOOLS.md`](./AGENT_TOOLS.md)。

---

## 架構邊界

| 面向 | 邊界設定 |
|------|---------|
| **Transport** | stdio 為生產預設。SSE 與 streamable-HTTP 程式碼已就緒,待啟用。 |
| **Auth** | `ATLAS_MCP_TOKEN`(stdio 為 forward-looking,SSE/HTTP 啟用後強制驗證)、DB TokenStore、`TokenAuth` 機制已實作。 |
| **Loopback** | atlas-mcp 只 bind 127.0.0.1,不對外暴露。 |
| **Audit** | 每個 tool call 寫入 `ATLAS_MCP_AUDIT_LOG` JSONL,必填欄位：`ts`、`tool`、`status`、`duration_ms`。`arg_keys` 只記錄 key 名稱、不記值。 |
| **底層 Gateway** | 不得繞過 [`internal/apigateway/CONSTITUTION.md`](../internal/apigateway/CONSTITUTION.md)（6 條文 + 3 附錄：統一入口、限流、熔斷、環境變數治理、背景任務排程）。 |
| **可觸發動作** | 回測、批次任務、實驗查詢/評分、控制平面 read-only 指令、警報確認/解除。 |
| **不可觸發** | 未授權下單、live trading、admin token 管理繞道、未受稽核的副作用操作。 |

外部 agent 看不到任何 atlas-go 內部 Go 套件,所有能力都透過 MCP tool 暴露。這是刻意為之的最小權限邊界。

---

## 文件路由

| 想深入了解... | 讀這份 |
|--------------|--------|
| 84 個 tool 完整 catalog、5 分鐘決策樹、調用範例 | [`docs/AGENT_TOOLS.md`](./AGENT_TOOLS.md) |
| atlas-mcp 建置、所有環境變數、Claude/Cursor/OpenCode 配置範例、audit log 格式 | [`cmd/atlas-mcp/README.md`](../cmd/atlas-mcp/README.md) |
| MCP server 設計原理、JSON Schema 模板、安全邊界、可擴充協議（Resources / Prompts / Sampling / Elicitation / Roots） | [`docs/specs/agent-mcp-server.md`](./specs/agent-mcp-server.md) |
| 21 個 workflow（WA-001 ~ WA-701）與 MCP tool 對應、流程設計 | [`docs/REFERENCE/WORKFLOW_MAP.md`](./WORKFLOW_MAP.md) |
| 第一次使用 atlas 的 5 分鐘速讀（市場＋策略＋MCP 整體脈絡） | [`docs/AGENT_ONBOARDING.md`](./AGENT_ONBOARDING.md) |
| 所有 atlas 文件分類與路由（含本檔） | [`docs/DOCUMENTATION_MAP.md`](./DOCUMENTATION_MAP.md) |

---

**最後更新**：2026-07-03（Wave 11+,atlas-go v0.0.0.27+,atlas-mcp Phase 2.2 全部 84 tool 上線）
