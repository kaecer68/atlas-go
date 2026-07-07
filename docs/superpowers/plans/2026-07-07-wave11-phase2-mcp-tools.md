# Wave 11 Phase 2: MCP Tool 暴露與 Prompt 對齊

> 目標：讓 atlas-mcp 完整覆蓋 PR #971–#975 新增的後端能力（capital flow、recommendation、event flow prediction），並修正 prompt 引用不存在 tool 的問題。

## 背景

PR #971–#975 重構後新增/強化了數個後端模組：

- `internal/capitalflow`：提供 `/api/capital-flow/daily` 與 `/api/capital-flow/summary`
- `internal/recommender`：提供 `/api/recommendations`
- `internal/eventdriven`：已在 `/api/events/flow-prediction` 註冊，並由 `tools_events.go` 暴露 `event_flow_prediction`
- `cmd/atlas-mcp/server/prompts.go` 中的 `taiwan_quick_look` 與 `strategy_advice` 卻引用尚未存在的 `capital_flow_daily` 與 `strategy_ranker`

本階段要將上述缺口補齊，並同步更新文件與測試。

## 階段範圍

**屬於 Phase 2**：

1. 新增 `capital_flow_daily` 與 `capital_flow_summary` tool（`tools_capitalflow.go`）
2. 新增 `get_recommendations` tool（`tools_recommendation.go`）
3. 調整 `server.go` tool count assertion（79–83，視新增 tool 數）
4. 修正 `prompts.go`：移除不存在 tool 的引用，改用已存在或可立即提供的 tool
5. 補齊測試：`tools_capitalflow_test.go`、`tools_recommendation_test.go`、`tools_events_test.go`、`tools_briefing_test.go`
6. 更新 `docs/AGENT_TOOLS.md`、`cmd/atlas-mcp/server/AGENTS.md` 的 tool 總數與分類
7. commit → push → PR #977

**不屬於 Phase 2**（留待 Phase 3+）：

- 前端欄位合約與頁面生命週期
- 後端資料提供者 stub 移除
- 安全強化與端到端測試

## 實作步驟

### 2.1 新增 `tools_capitalflow.go`

- 註冊 `capital_flow_daily`（GET `/api/capital-flow/daily`）
- 註冊 `capital_flow_summary`（GET `/api/capital-flow/summary`）
- 兩者皆為無參數 read-only tool，回傳 `map[string]any`

### 2.2 新增 `tools_recommendation.go`

- 註冊 `get_recommendations`（GET `/api/recommendations`）
- 暫不處理 JWT：透過 MCP 呼叫時由 atlas-go 後端根據 context 決定 tier，此處僅單純代理

### 2.3 調整 `prompts.go`

- `taiwan_quick_look`：將 `capital_flow_daily` 改為 `capital_flow_summary`（更簡潔），並保留 `event_calendar` 與 `mcp_quickstart`
- `strategy_advice`：`strategy_ranker` 目前不存在，改以 `strategy_list_active` + `strategy_get_summary` 取代，輸出改為「先列出活躍策略，再細看個別策略摘要」

### 2.4 調整 `server.go` assertion

- 原範圍 79–81，新增 2 個 capital flow + 1 個 recommendation = +3 tool
- 新範圍：82–84（base +3；sampling/elicitation 仍各 +0–1，但不影響 min/max 計算，因為它們原本就在 79–81 的彈性區間）
- 實際驗證：啟動 server 後確認 `RegisteredToolCount`

### 2.5 補測試

- `tools_capitalflow_test.go`：驗證 path 與非空結果
- `tools_recommendation_test.go`：驗證 path
- `tools_events_test.go`：驗證 `event_calendar` 與 `event_flow_prediction` path
- `tools_briefing_test.go`：驗證 `mcp_quickstart` 與 `daily_report` path

### 2.6 更新文件

- `docs/AGENT_TOOLS.md`：
  - 總數改為 83（假設新增 3 個）
  - 在「資金面 / 推薦」區塊加入 `capital_flow_daily`、`capital_flow_summary`、`get_recommendations`
- `cmd/atlas-mcp/server/AGENTS.md`：
  - 更新 tool 總數與註冊檔案清單
  - 標註 prompt 引用規則：prompt 只允許引用已註冊 tool

## 驗收標準

- [ ] `go test ./cmd/atlas-mcp/server/...` 全綠
- [ ] `go run ./cmd/atlas-mcp` 不會因 tool count assertion 失敗
- [ ] `docs/AGENT_TOOLS.md` 與實際註冊 tool 名稱一致
- [ ] `prompts.go` 無引用不存在 tool
- [ ] PR #977 已開出並連回本計畫

## 風險與注意

- `capitalflow` 的 `/api/capital-flow/daily` 會呼叫 `FetchSnapshot`，本地若無 provider 會回 503；MCP tool 只負責代理，不負責資料可用性
- `get_recommendations` 目前後端是 stub（固定回傳 market light），但不影響 tool 暴露
- 後續 Phase 4 才會把後端 stub 替換為真實資料
