# AGENTS.md — internal/adversarial

對抗性（Adversarial）訓練模組。Red Team 模擬極端市場情境攻擊 agent，Blue Team 學習防禦。輸出 `BattleResult` 與 `StressTestResult`。

 Maturity: **X-tier** — experimental。詳見 MATURITY.md §adversarial。

## 模組用途

- 對市場情境扰动（perturbation）下的 agent 穩健性壓力測試
- 生成 edge cases 供 backtest validation
- 評估 agent 對 distribution shift 的抵抗力
- 識別弱點 agent 並回傳 `Vulnerability` 列表供 Phase 3 优化

## 公開 API

### 核心型別

| 型別 | 用途 |
|------|------|
| `TeamType` | Red（攻擊方）或 Blue（防守方） |
| `ScenarioType` | 攻擊情境：`FlashCrash` / `LiquidityCrisis` / `CorrelationSpike` / `FlashRally` / `SectorRotation` |
| `SeverityLevel` | 嚴重程度：`Critical` / `High` / `Medium` / `Low` |
| `AdversarialScenario` | 單一情境定義（ID / Name / Type / Severity / Description） |
| `AdversarialAgent` | 紅隊或藍隊參與者（ID / Name / Team / Strategies / WinRate） |
| `AttackStrategy` | 具體攻防策略（ID / Name / Team / Effectiveness） |
| `BattleResult` | 單次對戰結果（Winner / RedScore / BlueScore / KeyEvents / Duration / Rounds） |
| `BattleEvent` | 對戰中重大事件（Round / Team / Event / Impact） |
| `Vulnerability` | 系統弱點（Type / Occurrences / Severity / Recommendation） |
| `ScenarioResult` | 單一情境測試結果（ScenarioType / Score / Passed / Details） |
| `StressTestResult` | 對特定 agent 壓力測試彙總（AgentID / OverallScore / Passed / Scenarios / Vulnerabilities） |
| `AdversarialReport` | 完整訓練報告 |
| `TrainingSummary` | 整體訓練結果彙總 |

### 工廠函式

```go
func NewAdversarialTrainer(config *AdversarialConfig) *AdversarialTrainer
func DefaultAdversarialConfig() *AdversarialConfig
```

### AdversarialTrainer 公開方法

| 方法 | 用途 |
|------|------|
| `RunTraining() *TrainingSummary` | 執行完整對抗訓練循環 |
| `StressTestAgent(agentID string, agent domain.AgentSpec) *StressTestResult` | 對指定 agent 執行所有情境壓力測試 |
| `GetVulnerabilities() []Vulnerability` | 分析紅隊成功模式，回傳系統弱點列表 |
| `GenerateReport() *AdversarialReport` | 產生完整訓練分析報告 |

### 預設組態

| 欄位 | 預設值 |
|------|--------|
| `RedTeamSize` | 5 |
| `BlueTeamSize` | 5 |
| `BattleRounds` | 10 |
| `TrainingCycles` | 3 |
| `MinEffectivenessThreshold` | 0.3 |
| `AdaptationRate` | 0.1 |

## 依賴關係

```
internal/adversarial
  └─ depends on: internal/domain (AgentSpec)
                  internal/logging

internal/orchestrator/adversarial_executor.go
  └─ uses: adversarial.StressTestResult, ScenarioResult, ScenarioType
  └─ 封裝: AdversarialScenarioRunner (包裝 adversarial 介面，串接 replay dataset)

internal/orchestrator/phase3_controller.go
  └─ uses: adversarial, adversarial_executor
  └─ 持有: advRunner *AdversarialScenarioRunner, lastAdvResult *adversarial.StressTestResult
  └─ 呼叫: runAdversarialStressTests(), GetLastAdversarialResult()

internal/orchestrator/plugin_adapters.go
  └─ 呼叫: WithAdversarialRunner(), runAdversarialStressTests()
```

## ⚠️ 關鍵陷阱

### 矛盾：experimental 標記與 runtime 實際使用

`doc.go` 明確標示 `Maturity: experimental` / `X-tier`，但此模組已被 `orchestrator` runtime 實際依賴：

- `phase3_controller.go` 在 production 流程中呼叫 `runAdversarialStressTests()`
- `adversarial_executor.go` 的 `AdversarialScenarioRunner` 直接操作真實 replay dataset
- `plugin_adversarial.go` 在 System 啟動時自動注入

**風險**：若日後 `adversarial` 進行破壞性變更，會直接影響 Phase 3 最佳化流程。

**緩解**：目前 `adversarial` 屬於內部隔離實作（`AdversarialScenarioRunner` 在 `orchestrator/adversarial_executor.go` 而非直接暴露），但介面型別（`StressTestResult` 等）仍來自 `adversarial` package。

### 測試樞紐依賴

`adversarial_test.go` 中的測試仰賴 `domain.AgentSpec` 欄位存在（`ID` / `Skill` / `Enabled`）。若 `domain.AgentSpec` 結構改變，測試會斷裂。

## ANTI-PATTERNS

- 不可直接實例化 `AdversarialTrainer` 而不通過 `NewAdversarialTrainer`（內部需初始化團隊與情境）。
- `StressTestAgent` 預設 pass threshold = 0.6；若 agent 穩健性要求提高，threshold 需同步調整。
- `GetVulnerabilities` 目前實作為 simplified stub（不走真實 replay），真實弱點偵測依賴 `AdversarialScenarioRunner` 的 `RunStressTest`。

## 與其他模組的界線

| 模組 | 關係 |
|------|------|
| `orchestrator` | Consumer — `phase3_controller` 呼叫 adversarial 壓力測試 |
| `replay` | `AdversarialScenarioRunner` 消費 replay dataset 進行真實報價突變 |
| `reflexivity` | 各自獨立；adversarial 測 market shock，reflexivity 測價格自反性動態 |
| `forecast` | 無依賴關係 |
