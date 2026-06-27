---
name: atlas-strategy-evolution
description: "Use when working with the strategy evolution loop, model performance feedback, or dynamic agent adjustment. Triggers: evolution cycle, agent performance evaluation, mutation brief, weakest agent selection."
---

**版本**: 1.0  
**日期**: 2026-04-23  
**職責**: 投資模型的動態調整、績效回饋與持續進化  

---

## 核心哲學

> **「沒有永遠有效的投資模型，只有持續進化的策略系統」**

---

## 投資模型生態系

### 現有模型

| 模型ID | 名稱 | 適用情境 | 權重範圍 |
|--------|------|---------|---------|
| `ai_supercycle_model` | AI超級週期模型 | 結構性科技趨勢強勁 | 0.1 - 0.6 |
| `hawkish_fed_model` | 鷹派Fed模型 | 高利率+緊縮環境 | 0.1 - 0.5 |
| `geopolitical_hedge_model` | 地緣政治避險模型 | 戰爭/地緣風險升溫 | 0.1 - 0.6 |
| `semiconductor_cycle_model` | 半導體週期模型 | 半導體庫存週期 | 0.1 - 0.4 |
| `seasonal_model` | 季節性輪動模型 | 春節/除權息等季節效應 | 0.0 - 0.3 |

### 模型權重動態調整

```go
func EvolveModelWeights(macro MacroRiskAssessment, trends []StructuralTrend) {
    switch macro.Level {
    case Green:
        weights["ai_supercycle_model"] = 0.6
        weights["hawkish_fed_model"] = 0.2
        weights["geopolitical_hedge_model"] = 0.2
        
    case Yellow:
        if trends["AI"].Strength > 70 {
            weights["ai_supercycle_model"] = 0.5
            weights["hawkish_fed_model"] = 0.3
        } else {
            weights["ai_supercycle_model"] = 0.3
            weights["hawkish_fed_model"] = 0.4
        }
        
    case Orange/Red:
        weights["ai_supercycle_model"] = 0.1
        weights["geopolitical_hedge_model"] = 0.6
        weights["hawkish_fed_model"] = 0.3
    }
}
```

---

## MetaLearner 與 MiroFish Swarm 的連結

策略進化系統透過 `SubmitTrainingScenarios` 接收 Swarm 模擬產生的訓練情境，將魚群表現轉化為學習策略的演化素材：

```
MiroFish Swarm 模擬
  └── ExportTrainingData() → []TrainingScenario
        └── MetaLearner.SubmitTrainingScenarios(scenarios)
              └── scenarioToLearningData() → SwarmLearningData
                    └── SubmitSwarmData() → swarmData channel
                          └── processSwarmData() 更新策略績效
                                └── evolvePopulation() 執行遺傳演算法
```

- 每條魚的 `TrainingScenario` 包含其歷史狀態、預測、表現（Accuracy、SharpeRatio、MaxDrawdown）與預測規則
- `scenarioToLearningData` 將魚的準確率映射為學習率參數（accuracy>0.8 → 0.001；>0.6 → 0.01；否則 0.1）
- 批次提交後自動觸發一次 `evolvePopulation()`，執行 elite 選擇、crossover、mutation
- 頂級策略可透過 `GET /api/dashboard/swarm-strategies` 查詢

## 績效回饋機制

### 評估維度

```go
type ModelPerformance struct {
    ModelID       string
    Period        string
    PredictedFlow string    // 模型預測的資金流向
    ActualFlow    string    // 實際資金流向
    Accuracy      float64   // 預測準確率
    PnLImpact     float64   // 對組合損益的影響
}
```

> 注意：`ModelPerformance` 是投資模型層級的績效追蹤（用於 Darwinian 權重調整）。MetaLearner 內部使用 `StrategyPerformance`（SuccessCount、FailureCount、AvgImprovement、ConvergenceRate、StabilityScore）追蹤學習策略的表現。兩者位於不同抽象層級。

### 自動調整規則

```go
func EvaluateModels(performances []ModelPerformance) {
    for _, p := range performances {
        if p.Accuracy < 0.5 {
            ReduceModelWeight(p.ModelID, 0.1)
        } else if p.Accuracy > 0.8 {
            IncreaseModelWeight(p.ModelID, 0.1)
        }
    }
}
```

---

## 實驗生命週期

```
Propose → Execute → Judge → Promote/Revert
```

### 改良重點（v2.0）

1. **觀測值門檻提高**：level_1: 3→8, level_2: 8→12, level_3: 12→15
   > ⚠️ **Pending**: Thresholds remain at level_1: 3, level_2: 8, level_3: 12 (see `internal/experiment/judge.go` → `requiredObservationCountForMaturity()`). Requires data collection infrastructure upgrade.
2. **統計穩定性檢查**：Sharpe ratio 標準誤差 < 0.5
   > ✅ **Implemented**: `maintain_sharpe_like` gate uses `SharpeStabilityCheck()`.
3. **Out-of-sample 驗證**：Candidate 必須在第二個獨立窗口優於 Baseline
   > ✅ **Implemented**: `no_drawdown_spike` and `preserve_downside_protection` gates.
4. **透明度強化**：Darwinian 權重截斷事件記錄 + 連續截斷警告
   > ✅ **Implemented**: Logging in `constrainWeight()` at `darwinian_weights.go:315-319`.

---

## 關鍵檔案

- `internal/experiment/judge.go` - 實驗評判核心
- `internal/experiment/sharpe_stability.go` - Sharpe穩定性檢查
- `internal/experiment/oos_validator.go` - Out-of-sample驗證
- `internal/portfolio/darwinian_weights.go` - Darwinian權重管理
- `internal/narrative/knowledge_base.go` - 投資模型定義

---

## Wave 9 觀測整合

策略進化循環可透過 `EventPositionUpdate` 與 `EventRegimeChange` 事件串接 Wave 9 可觀測性。

---

## Agent Spawning 模組（internal/spawning）

自動偵測系統知識缺口並生成新 Agent，管理完整生命週期。

| 陷阱 | 說明 |
|------|------|
| **PromptsDir 必須為絕對路徑** | `SpawningConfig.PromptsDir` 預設 `"prompts"` 相對於 CWD；`ManualSpawn` 前需確認工作目錄 |
| **Agent 初始為 disabled** | `AgentFactory.CreateAgentForGap` 產生 `Enabled: false`，必須經 `AcceptAgent()` 才啟用 |
| **Extinction 閾值 20 天** | `CheckExtinction()` 預設權重維持在 DarwinianWeightMin(0.3) 以下 20 天即淘汰 |
| **ManualSpawn 不進內部 map** | `ManualSpawn()` 只建立並回傳 `SpawnedAgent`，不會自動加入 `spawnedAgents` map（測試有註解）|
| **Gap 優先排序用 bubble sort** | `prioritizeGaps()` 使用簡單排序（列表小，非效能瓶頸），嚴重程度加權後以年齡微調 |

## MetaLearner（internal/metalearning）

透過遺傳演算法根據 MiroFish swarm 回饋持續優化學習策略參數。

| 陷阱 | 說明 |
|------|------|
| **冷啟動分數為 0** | `calculateStrategyScore()` 在 `TotalApplications == 0` 時回傳 0，新策略初期排名墊底 |
| **通道滿載丟棄** | `SubmitSwarmData` 與 `SubmitTrainingResult` 使用緩衝 channel（容量 100），滿載時直接丟棄 |
| **pseudo-random 為 deterministic** | 使用自訂 LCG（`deterministicRand`），測試可重現但非加密安全 |
| **參數突變可能為負** | `mutateStrategy()` 對 `learning_rate`/`batch_size` 取絕對值，其他參數可能變為負數 |
| **Save/Load 只存 ID 引用** | `Save()` 將 population 與 elite 存為 ID 列表；`Load()` 需從 `strategies` map 還原引用 |

---

*技能版本: 1.2*  
*最後更新: 2026-06-27*
