# Atlas 模型評估框架 Skill

**版本**: 1.0
**日期**: 2026-05-29
**職責**: 模型評估框架 — 樣本外 R²、夏普比率、排列重要性、部分相依圖

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 對應 |
|---------|---------|-----------|
| SK-12 | OOS R² / Sharpe / cumulative return | 現有 Judge 擴充 |
| SK-13 | Permutation importance | 新建評估器 |
| SK-14 | PDP (Partial Dependence Plot) | 新建評估器 |
| SK-15 | Interaction effects (Friedman's H) | 新建評估器 |
| SK-28 | Reward mismatch (SL/RL 一致性) | 新建一致性檢查 |
| SK-30 | Quantum stability (參數穩定性) | 擴充現有閘門 |

---

## 核心哲學

> **「Atlas 有完善的實驗評判（Judge），但缺少模型層面的診斷工具。只知道『好不好』，不知道『為什麼好』。」**

---

## 評估架構

```
ML Model (from atlas-fin-ml-pipeline)
    ├── SK-12: OOS 預測力度量
    ├── SK-13: 因子重要性
    ├── SK-14: 部分相依
    ├── SK-15: 交互效應
    ├── SK-28: 獎勵一致性
    └── SK-30: 參數穩定性
```

---

## 各評估模組實作指引

### SK-12: OOS 預測力度量

需擴充：`internal/eval/oos_metrics.go` — ComputeOOSMetrics(yTrue, yPred, dates) → R²_OOS, Sharpe_OOS, HitRate

### SK-13: Permutation Importance

`internal/eval/permutation.go` — ComputePermutationImportance(model, X, y, factorNames, nRepeats)

### SK-14: Partial Dependence Plot (PDP)

`internal/eval/pdp.go` — ComputePDP(model, X, factorIdx, nGrid)

### SK-15: Interaction Effects (Friedman's H)

`internal/eval/interaction.go` — ComputeFriedmanH(model, X, factorA, factorB)

### SK-28: Reward Mismatch (SL/RL 一致性)

`internal/eval/reward_consistency.go` — CheckRewardMismatch(models, X, y, prices)

---

## 與現有模組的整合

| 現有模組 | 整合方式 |
|---------|---------|
| `internal/experiment/judge.go` | Evaluate() 中自動執行 OOSMetrics + Stability check |
| `internal/experiment/oos_validator.go` | 擴充為完整的 OOS 度量 |
| `internal/portfolio/optimizer.go` | Permutation importance 回饋到 FactorWeightEngine |

---

## 交叉參考

- **atlas-fin-ml-pipeline**: ML 模型訓練
- **atlas-strategy-evolution**: 實驗生命週期
- **atlas-risk-management**: 風險整合
- **atlas-core-architecture**: 整體模組邊界

---

## Go 實作骨架

### OOSMetrics 函數簽名

```go
// internal/eval/oos_metrics.go

// OOSMetrics holds out-of-sample evaluation metrics for a trained model.
type OOSMetrics struct {
    R2OOS         float64   // Campbell-Thompson OOS R²
    SharpeOOS     float64   // 年化策略 Sharpe ratio
    CumulativeRet []float64 // 每日累積報酬曲線
    HitRate       float64   // 方向正確率
    RMSE          float64   // 均方根誤差
}

// ComputeOOSMetrics calculates all OOS metrics from predictions and actual returns.
func ComputeOOSMetrics(yTrue, yPred []float64, dates []time.Time) OOSMetrics
```

### SharpeRatio 函數簽名

```go
// ComputeSharpeOOS computes annualized Sharpe ratio from a series of daily returns.
// Returns are assumed to be excess returns (strategy - risk-free).
func ComputeSharpeOOS(dailyReturns []float64) float64 {
    // Sharpe = mean(daily_ret) / std(daily_ret) * sqrt(252)
}
```

### 與 judge.go 的整合點

```go
// internal/experiment/judge.go — Evaluate() 擴充

// 對 ML model variant 自動執行：
//   metrics := eval.ComputeOOSMetrics(yTrue, yPred, dates)
//   if metrics.R2OOS <= 0 {
//       result.PassedGates["oos_r2_positive"] = false
//   }
```

*技能版本: 1.0*
*最後更新: 2026-05-29*
