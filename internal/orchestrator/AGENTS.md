# AGENTS.md — internal/orchestrator

協調領域專家、控制層過濾與系統擴充外掛。

## 核心職責

- **流程協調** (`SystemCore`)：管理模擬生命週期，連結市場資料、實驗狀態與模擬引擎。
- **分層路由** (`executor_*.go`)：依序執行 Context (Regime) → Sector/Style/Superinvestor (Agent) → Control (CRO/CIO)。
- **外掛託管** (`PluginHost`)：統一介面處理 PRISM、Swarm、JANUS 等外部子系統。

## 關鍵介面與模式

| 介面 | 職責 | 模式 |
|------|------|------|
| `AgentExecutor` | 產生個股推薦 | `Supports(spec) bool` + `Recommend(...)` |
| `RegimeExecutor` | 判定當前盤勢 | `Supports(spec) bool` + `Score(...)` |
| `Plugin` | 生命週期外掛 | `Attach(core)` + `ProcessRecommendations` + `PostSimulation` |

### 分層執行順序
1. Regime 判定
2. 個股篩選（Screener）
3. 推薦收集
4. 控制過濾（CRO/CIO）

## 開發慣例

- 新增 Executor 需在 `plugin_registry.go` 的 `NewPluginRegistry()` 中手動加入。
- 推薦進入控制層前須經 `CalculateFactorScoresWithBreakdown`。
- `System.RunDailySimulation` 應保持無副作用，直到結果寫入 `ledger`。
- `LLMSectorAgentsEnabled=true` 但 driver 為 `nil` 時 plugin 為 no-op。
- `SectorAgentLLMDriver` 必須包裝 `PlanDriver` + `ReflectDriver`。

## 陷阱與反模式

- 禁止跨層直接讀取 `SystemCore` 私有欄位，應透過傳入的 `registry` 或 `quotes`。
- CIO aggregator 會覆寫 `finalRecs[].Agent`，但 `PassedGuards` 查核改用 symbol-only key。
- 靜默過濾：標的不符合 `Screener` 門檻將不進入 `Recommend`，「推薦消失」時優先檢查 `agents.json`。
- 修改 `AgentRegistry` 後，確認 `ExecuteRegistryResearch` 路由邏輯能匹配新 Layer。
