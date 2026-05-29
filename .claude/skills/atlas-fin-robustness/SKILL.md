# Atlas 穩健性檢驗套件 Skill

**版本**: 1.0
**日期**: 2026-05-29
**職責**: 穩健性檢驗套件 — 規模分組、排除仙股、消去法、對抗性訓練

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 現有基礎 | 需擴充 |
|---------|---------|---------------|--------|
| SK-20 | Size group analysis (規模分組) | ❌ 無 | 新建 |
| SK-21 | Penny stock exclusion (仙股排除) | ⚠️ screener 有 PE/PB/成交量過濾 | 擴充 |
| SK-22 | Ablation study (消去法) | ❌ 無 | 新建 |
| SK-31 | SL vs RL comparison | ❌ 無 | 新建 |
| SK-32 | Reward sensitivity (獎勵敏感度) | ❌ 無 | 新建 |

---

## 核心哲學

> **「模型在 Top 100 股票上表現好 ≠ 真的穩健。」**

---

## 穩健性檢驗架構

```
Trained Model / Strategy
    ├── SK-21: 資料層 — Penny stock exclusion
    ├── SK-20: 子群體層 — Size group analysis
    ├── SK-22: 因子層 — Ablation study
    ├── SK-31: 目標層 — SL vs RL comparison
    └── SK-32: 敏感度層 — Reward sensitivity
```

---

## 各檢驗模組實作指引

### SK-21: Penny Stock Exclusion
排除股價 < NT$10 或市值 < NT$1B 的標的。

### SK-20: Size Group Analysis
策略必須在三個規模群體中分別檢驗（大/中/小市值）。

### SK-22: Ablation Study
逐因子移除，量化每個因子的邊際貢獻。

### SK-31: SL vs RL Comparison
監督學習（minimize MSE）和強化學習（maximize Sharpe）的目標可能不一致。

### SK-32: Reward Sensitivity
變動 RL reward function 的參數，觀測策略行為的穩定性。

---

## 與現有 adversarial 模組的整合

現有基礎：`internal/adversarial/adversarial_trainer.go` 已實作 Red Team / Blue Team 對抗訓練。將穩健性檢驗作為 adversarial training 的前置步驟。

---

## 交叉參考

- **atlas-fin-ml-pipeline**: ML 模型訓練
- **atlas-fin-model-eval**: 模型評估
- **atlas-fin-backtest-engine**: 回測引擎
- **atlas-strategy-evolution**: 實驗生命週期
- **atlas-risk-management**: 風險整合

---

## Go 實作骨架

### 與 adversarial 模組的整合橋接點

```go
// internal/robustness/bridge.go

// RobustnessGate runs all robustness checks and feeds results into
// the adversarial training pipeline for scenario generation.

type RobustnessGate struct {
    AblationResults   []AblationResult
    SizeGroupResults  []SizeGroupResult
    AdversarialBridge *AdversarialBridge
}

// GenerateAdversarialScenarios converts robustness findings into
// targeted adversarial attack scenarios.
//
// Example: if ablation shows Quality factor is critical,
// produce a "quality_factor_manipulation" scenario that tests
// model behavior when financial reporting data is corrupted.
func (rg *RobustnessGate) GenerateAdversarialScenarios() []domain.AdversarialScenario
```

### 整合流程

```
RobustnessGate.Check(model, data)
  ├── Ablation (SK-22) → 找出關鍵因子
  ├── SizeGroup (SK-20) → 找出弱勢規模群
  └── bridge.GenerateAdversarialScenarios()
        └── AdversarialTrainer.Train(blueTeam, scenarios)
              └── BattleResult → model robustness score
```

*技能版本: 1.0*
*最後更新: 2026-05-29*
