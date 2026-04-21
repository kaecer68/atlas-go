# AGENTS.md — internal/orchestrator

本目錄是 `atlas-go` 的大腦核心，負責協調領域專家、控制層過濾與系統擴充外掛。

---

## 核心職責

- **流程協調** (`SystemCore`)：管理模擬生命週期，連結市場資料、實驗狀態與模擬引擎。
- **分層路由** (`executors.go`)：依序執行 `Context` (Regime) → `Sector/Style/Superinvestor` (Agent) → `Control` (CRO/CIO) 三層架構。
- **外掛託管** (`PluginHost`)：以統一介面處理 PRISM、Swarm、JANUS 等外部子系統的掛載與生命週期勾子。

---

## 關鍵介面與模式

| 介面 | 職責 | 模式 |
|------|------|------|
| `AgentExecutor` | 負責產生個股推薦 | `Supports(spec) bool` + `Recommend(...)` |
| `RegimeExecutor` | 負責判定當前盤勢 | `Supports(spec) bool` + `Score(...)` |
| `Plugin` | 生命週期外掛 | `Attach(core)` + `ProcessRecommendations` + `PostSimulation` |

### 分層執行順序
1. **Regime 判定**：由 `LayerContext` 代理（如 Macro）計算總分決定 Risk On/Off。
2. **個股篩選**：由 `Screener` 依據 `agents.json` 的 `screening_criteria` 進行靜默過濾。
3. **推薦收集**：由 `Sector/Style` 代理產生原始推薦。
4. **控制過濾**：由 `LayerControl` 代理（CRO/CIO）執行強制阻擋或權重調整。

---

## 開發慣例

- **Executor 註冊**：新增 Executor 需在 `plugin_registry.go` 的 `NewPluginRegistry()` 中手動加入對應陣列。
- **因子分數**：所有推薦在進入控制層前，必須經由 `CalculateFactorScoresWithBreakdown` 補完因子細節。
- **原子性**：`System.RunDailySimulation` 應保持無副作用，直到結果寫入 `ledger`。

---

## 陷阱與反模式

- **禁止跨層呼叫**：Executor 不應直接讀取 `SystemCore` 的私有欄位，應透過傳入的 `registry` 或 `quotes`。
- **ID 混淆**：控制層（CIO）輸出必須保留原始 Agent ID，不可覆寫，否則 `PassedGuards` 稽核會失效。
- **靜默過濾**：若標的不符合 `Screener` 門檻，將完全不會進入 `Recommend` 階段，開發時若發現「推薦消失」請優先檢查 `agents.json`。
- **Registry 變更**：修改 `AgentRegistry` 後，必須確認 `ExecuteRegistryResearch` 的路由邏輯能正確匹配新 Layer。
