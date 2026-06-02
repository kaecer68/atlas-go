# Atlas 穩健性檢驗套件 Skill

> ⚠️ **此技能描述的功能部分實作，但整體仍為藍圖階段**  
> **實作狀態**：⚠️ 部分實作 — `internal/robustness/` 已有三個檢驗模組，但 bridge.go 與多個 SK 規格未實作  
> **最後審計**：2026-06-02  
> **現有基礎**：`ablation.go`、`penny_exclusion.go`、`size_group.go`（含測試）、`internal/adversarial/adversarial_trainer.go`

**版本**: 1.0
**日期**: 2026-05-29
**職責**: 穩健性檢驗套件 — 規模分組、排除仙股、消去法、對抗性訓練

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 現有基礎 | 需擴充 |
|---------|---------|---------------|--------|
| SK-20 | Size group analysis (規模分組) | ✅ `internal/robustness/size_group.go` 已實作 | — |
| SK-21 | Penny stock exclusion (仙股排除) | ✅ `internal/robustness/penny_exclusion.go` 已實作 | — |
| SK-22 | Ablation study (消去法) | ✅ `internal/robustness/ablation.go` 已實作 | — |
| SK-31 | SL vs RL comparison | ❌ 無 | 新建 |
| SK-32 | Reward sensitivity (獎勵敏感度) | ❌ 無 | 新建 |

---

## 核心哲學

> **「模型在 Top 100 股票上表現好 ≠ 真的穩健。」**

---

## 穩健性檢驗架構

```
Trained Model / Strategy
    ├── SK-21: 資料層 — Penny stock exclusion ✅
    ├── SK-20: 子群體層 — Size group analysis ✅
    ├── SK-22: 因子層 — Ablation study ✅
    ├── SK-31: 目標層 — SL vs RL comparison ❌
    └── SK-32: 敏感度層 — Reward sensitivity ❌
```

---

## 各檢驗模組實作指引

### SK-21: Penny Stock Exclusion（✅ 已實作）
`internal/robustness/penny_exclusion.go` 已實作。排除股價 < NT$10 或市值 < NT$1B 的標的。

### SK-20: Size Group Analysis（✅ 已實作）
`internal/robustness/size_group.go` 已實作。策略在三個規模群體中分別檢驗（大/中/小市值）。

### SK-22: Ablation Study（✅ 已實作）
`internal/robustness/ablation.go` 已實作。逐因子移除，量化每個因子的邊際貢獻。

### SK-31: SL vs RL Comparison（❌ 未實作）
監督學習（minimize MSE）和強化學習（maximize Sharpe）的目標可能不一致。待建立。

### SK-32: Reward Sensitivity（❌ 未實作）
變動 RL reward function 的參數，觀測策略行為的穩定性。待建立。

---

## 實際檔案結構

| 元件 | 實際檔案 | 狀態 |
|------|---------|------|
| Ablation Study | `internal/robustness/ablation.go` + `ablation_test.go` | ✅ 已實作 |
| Penny Exclusion | `internal/robustness/penny_exclusion.go` + `penny_exclusion_test.go` | ✅ 已實作 |
| Size Group | `internal/robustness/size_group.go` + `size_group_test.go` | ✅ 已實作 |
| Robustness Bridge | `internal/robustness/bridge.go` | ❌ 未實作 |
| Adversarial Trainer | `internal/adversarial/adversarial_trainer.go` | ✅ 已實作 |

## 與現有 adversarial 模組的整合

現有基礎：`internal/adversarial/adversarial_trainer.go` 已實作 Red Team / Blue Team 對抗訓練。將穩健性檢驗作為 adversarial training 的前置步驟（透過 `bridge.go` 連接，尚未實作）。

---

## 交叉參考

- **atlas-fin-ml-pipeline**: ML 模型訓練
- **atlas-fin-model-eval**: 模型評估
- **atlas-fin-backtest-engine**: 回測引擎
- **atlas-strategy-evolution**: 實驗生命週期
- **atlas-risk-management**: 風險整合

---

## Go 實作骨架

### 與 adversarial 模組的整合橋接點（待實作）

```go
// internal/robustness/bridge.go — 尚未實作

// RobustnessGate runs all robustness checks and feeds results into
// the adversarial training pipeline for scenario generation.

type RobustnessGate struct {
    AblationResults   []AblationResult
    SizeGroupResults  []SizeGroupResult
    AdversarialBridge *AdversarialBridge
}

// GenerateAdversarialScenarios converts robustness findings into
// targeted adversarial attack scenarios.
func (rg *RobustnessGate) GenerateAdversarialScenarios() []domain.AdversarialScenario
```

### 整合流程（設計）

```
RobustnessGate.Check(model, data)
  ├── Ablation (SK-22) → 找出關鍵因子
  ├── SizeGroup (SK-20) → 找出弱勢規模群
  └── bridge.GenerateAdversarialScenarios()
        └── AdversarialTrainer.Train(blueTeam, scenarios)
              └── BattleResult → model robustness score
```

*技能版本: 1.0*
*最後更新: 2026-06-02*
*狀態: 部分實作 — 三個核心檢驗已實作，橋接層與 SL/RL 比較待建立*
