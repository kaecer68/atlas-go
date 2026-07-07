# atlas-mcp 整合指南

## 總覽

`cmd/atlas-mcp` 是 atlas-go 的 MCP (Model Context Protocol) server，提供 80+ tools 供外部 AI Agent 調用。支援 **stdio** transport（已實作）及 **SSE/streamable-HTTP**（開發中）。

## 快速開始

### Claude Desktop

編輯 `claude_desktop_config.json`：

```json
{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/path/to/atlas-go/cmd/atlas-mcp",
      "args": [],
      "env": {
        "ATLAS_WORK_DIR": "/path/to/atlas-go",
        "ATLAS_DATABASE_URL": "postgres://user:pass@localhost:5432/atlas?sslmode=disable",
        "ATLAS_REDIS_URL": "redis://localhost:6379",
        "ATLAS_API_TOKEN": "your-mcp-token"
      }
    }
  }
}
```

### OpenClaw / Hermes

在 `~/.openclaw/mcp.json` 或 `~/.hermes/mcp.json`：

```json
{
  "atlas-mcp": {
    "type": "stdio",
    "command": "/path/to/atlas-go/cmd/atlas-mcp",
    "env": {
      "ATLAS_WORK_DIR": "/path/to/atlas-go",
      "ATLAS_API_TOKEN": "your-mcp-token"
    }
  }
}
```

### OpenCode CLI

在 `.opencode/opencode.json` 內 MCP server 區新增：

```json
{
  "name": "atlas-mcp",
  "command": "/path/to/atlas-go/cmd/atlas-mcp"
}
```

## 工具分類

| 分類 | 工具 | 用途 |
|------|------|------|
| **市場總覽** | `mcp_quickstart`、`macro_get_snapshot_latest`、`crossmarket_get_us_indices`、`capital_flow_daily`、`capital_flow_summary` | 快速取得當前市場快照 |
| **策略推薦** | `strategy_ranker`、`strategy_list_active`、`strategy_get`、`strategy_get_attribution` | 策略績效排名與進出場判斷 |
| **風險管理** | `risk_get_metrics`、`risk_get_drawdown`、`risk_get_correlation_matrix`、`risk_get_commentary` | VaR、回撤、相關係數矩陣 |
| **事件監控** | `event_calendar`、`event_flow_prediction`、`narrative_get_events`、`alert_list_unacknowledged` | 事件日曆、資金流預測、警報 |
| **回測分析** | `experiment_history`、`experiment_diff`、`strategy_get_attribution` | 實驗對比、策略歸因 |
| **系統狀態** | `system_get_health`、`system_get_circuit_breaker`、`data_get_channels`、`llm_get_health` | 服務健康、資料管線、LLM 路由 |

## Prompt 模板

MCP server 提供 6 個預設 Prompt 模板，外部 AI 可按名稱調用：

| Prompt | 用途 |
|--------|------|
| `taiwan_quick_look` | 台股今日快覽：宏觀快照 + 資金流向 + 壓力指數 + 事件 |
| `strategy_advice` | 策略建議：策略排名 + 風險評論 + 盤勢歷史 |
| `stock_health_check` | 持股健檢：輸入股票代號（`symbol="2330"`） |
| `daily_market_briefing` | 每日市場簡報（英文） |
| `risk_check` | 投資組合風險評估 |
| `regime_interpretation` | 盤勢解讀（`regime="RISK_ON"`） |

## MCP Resources

| URI | 內容 |
|-----|------|
| `atlas://strategies/active` | 當前活躍策略定義（JSON） |
| `atlas://market/regime` | 最新盤勢分類與壓力指數 |
| `atlas://events/today` | 今日事件清單 |

## 常用任務組合

### 每日摘要

```
1. 調用 mcp_quickstart → 取得當前市場快照 + 策略排名 + 壓力指數 + 事件
2. 調用 capital_flow_daily → 七大資金勢力分解
3. 調用 narrative_get_events → 宏觀事件敘事
```

### 策略訊號判斷

```
1. 調用 strategy_ranker → 取得排名
2. 調用 strategy_get(id="all_weather") → 取得詳細定義
3. 調用 risk_get_metrics → 確認風險閾值
4. 調用 event_flow_prediction → 確認事件驅動方向
```

### 個股健檢

```
1. 調用 trace_get_decision_chain → 該股因果鏈
2. 調用 universe_get_sessions → 歷史模擬 session
3. 調用 strategy_get(id="growth") → 對應策略詳情
```

## 認證與 Tier 權限

- **Public tier**：mcp_quickstart、macro_get_snapshot_latest、event_calendar、capital_flow_summary
- **Registered tier**：+ strategy_ranker、risk_get_metrics、narrative_get_events
- **Premium tier**：全部 80+ tools

Token 透過 `ATLAS_API_TOKEN` 環境變數或在 settings 中傳入 `Authorization: Bearer <token>`。

## 常見問題

### tools 列表顯示不完整

確認 token 的 tier 層級是否有足夠權限。premium token 可看到全部 tools。

### 連接失敗

1. 確認 `ATLAS_WORK_DIR` 指向正確的 atlas-go 根目錄
2. 確認 PostgreSQL + Redis 運行中
3. 確認 `ATLAS_API_TOKEN` 與 token store 內一致
4. 查看 server log：`atlas-mcp` 啟動時會輸出 tool count

### 數據為空

1. 確認 Gateway 初始化成功（log 會顯示 `[Gateway] initialized with N channels`）
2. `crossmarket_get_us_indices` 使用 Yahoo Finance 即時資料，需要網路連線
3. 盤中資料可能延遲 5-15 分鐘
