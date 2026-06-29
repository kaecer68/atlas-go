# Atlas Agent Tools — 實戰指南

> **本文件**：給 AI agent 看的「何時該呼叫哪個 tool」決策表（非完整 schema）。
> **完整工具 schema / 安全 / 部署**：[`agent-mcp-server.md`](./specs/agent-mcp-server.md)
> **底層 workflow 對應**：[`WORKFLOW_MAP.md`](./WORKFLOW_MAP.md)

---

## 5 分鐘決策樹

```
你要做什麼？
├── 回答「現在市場怎樣？」
│   ├── 整體 regime  → regime_get_history, macro_get_snapshot_latest
│   ├── 跨市場狀態  → crossmarket_get_status, crossmarket_get_us_indices
│   └── 個股敘事    → narrative_get_events, narrative_get_stress_index_current
│
├── 查 portfolio 狀態
│   ├── 風險指標    → risk_get_metrics, risk_get_correlation_matrix
│   ├── Drawdown    → dashboard_get_drawdown
│   └── 持倉健康    → control_get_agent_health
│
├── 觸發動作（需 audit）
│   ├── 跑回測       → task_create (kind=backtest_run), task_poll until done
│   ├── 評分策略     → experiment_judge (low risk, needs P1 MCP token)
│   ├── 凍結/解凍 agent → control_pause_agent / control_resume_agent
│   └── 警報確認     → alert_acknowledge / alert_resolve
│
└── 監控系統健康
    ├── LLM router    → llm_get_health
    ├── 整體          → system_get_health, system_get_metrics
    ├── 資料品質      → dashboard_get_data_quality, macro_get_data_health
    └── 警報計數      → alert_get_stats
```

---

## 高頻工具 Top 15（按真實使用頻率）

| Tool | 觸發情境 | 範例呼叫 |
|------|---------|---------|
| `regime_get_history` | 用戶問「最近市場怎樣」、「今天體制？」 | `{"days": 30}` |
| `macro_get_snapshot_latest` | 用戶問「台股壓力指標 / 外资 / 法人動態」 | `{}` |
| `risk_get_metrics` | 用戶問「目前風險等級、可進場嗎？」 | `{}` |
| `control_get_agent_health` | 「哪些 agent 被靜音 / 哪些最健康」 | `{}` |
| `strategy_list_active` | 「現在上線哪些策略？」 | `{}` |
| `strategy_get` | 「策略 X 的 hit rate / Sharpe？」 | `{"id": "xxx"}` |
| `dashboard_get_reasoning_trace` | 「為什麼推薦 2330？」 | `{"symbol": "2330"}` |
| `narrative_get_events` | 「最近有什麼大事件？」 | `{"days": 7}` |
| `alert_get_unacknowledged` | 「有未確認警報嗎？」 | `{}` |
| `alert_acknowledge` | 確認警報 | `{"alert_id": "..."}` |
| `experiment_judge` | 在 P1 MCP 觸發下，評分候選 vs baseline | `{"experiment_id": "..."}` |
| `system_get_health` | 定期健康檢查 | `{}` |
| `task_create` | 觸發回測 / 重新計算 / 批次 ingest | `{"kind": "backtest", "args": {...}}` |
| `task_get` | 取得 task 進度 | `{"id": "..."}` |
| `crossmarket_get_us_indices` | 「NVDA 怎樣影響台股？」 | `{}` |

---

## 不可呼叫的工具（安全邊界）

詳見 [`agent-mcp-server.md`](./specs/agent-mcp-server.md) §3.2。重要邊界：

| 不暴露 | 原因 |
|--------|------|
| `*_api_keys_update` | 直接改 secrets |
| `*_silence` (alert) | 易被誤用屏蔽 alert |
| `*_set-model-weight` | 達爾文權重變更 |
| `*_revert` (experiment) | baseline 還原風險 |
| `*_ingest` (macro/channel) | 觸發重算 |
| `experiment_judge_*_silent` | 默默評分不 log |

這些只能透過 **graphical admin UI** (`/admin/`) 或人工 CLI 觸發。

---

## 典型對話流程（範例）

### 場景 A：用戶問「今天可以進場嗎？」
```
1. llm_get_health                    → router 是否正常
2. regime_get_history {days:7}       → 近期體制
3. risk_get_metrics                  → 當前風險
4. control_get_agent_health          → 被禁用的 agent
5. alert_get_unacknowledged          → 未處理警報
6. (如都通過) → 給出「可進場 / 觀望 / 減倉」建議 + 因果說明
```

### 場景 B：用戶問「為什麼推薦 2330？」
```
1. dashboard_get_reasoning_trace {symbol:"2330"}
2. strategy_get_attribution {id:"..."}  （如 trace 揭示用的是某策略）
3. narrative_get_events {symbol:"2330", days:3}
4. 組合說明：信號來源 + 觸發敘事 + 評分依據
```

### 場景 C：用戶要暫停某個失控的 agent
```
1. control_get_agent_health          → 確認該 agent 真的失控
2. control_pause_agent {agent_id, reason, duration_h}
3. alert_get_unacknowledged          → 確認有 alert 觸發 pause
4. （pause 動作已 log 到 audit-log，無需再描述）
```

---

## 錯誤處理慣例

atlas-mcp 對錯誤自動轉譯：

| atlas HTTP 狀態 | MCP 錯誤碼 | Agent 應該做 |
|----------------|------------|-------------|
| 401 / 403 | `auth/unauthorized` | 重新確認 token；不應繼續 retry |
| 404 | `tool/not-found` | URL/ID 錯誤，調整後重試 |
| 429 | `rate/limited` | 指數退避後重試 |
| 500+ | `internal/error` | 顯示「atlas 暫時無法回應」給用戶 |
| 400 | `params/invalid` | 修正參數格式 |

完整錯誤轉譯規則見 [`agent-mcp-server.md`](./specs/agent-mcp-server.md) §6。
