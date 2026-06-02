# Atlas 模型評估框架 Skill

> **實作狀態**：⚠️ 部分實作 — 核心評估指標已實作於 `internal/eval/`，部分進階功能（交互效應、獎勵一致性）尚未實作  
> **最後審計**：2026-06-02  
> **實際檔案結構**：`metrics.go`（OOS R²、Sharpe、MaxDD）、`importance.go`（Permutation Importance）、`pdp.go`（Partial Dependence Plot）

**版本**: 1.0  
**日期**: 2026-05-29  
**職責**: 模型評估框架 — 樣本外 R²、夏普比率、排列重要性、部分相依圖

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | Atlas 對應 | 狀態 |
|---------|---------|-----------|------|
| SK-12 | OOS R² / Sharpe / cumulative return | `metrics.go`（`OOSR2`、`SharpeRatio`、`EvalResult`） | ✅ 已實作 |
| SK-13 | Permutation importance | `importance.go`（`PermutationImportance`） | ✅ 已實作 |
| SK-14 | PDP (Partial Dependence Plot) | `pdp.go`（`ComputePDP`、`PDPResult`） | ✅ 已實作 |
| SK-15 | Interaction effects (Friedman's H) | 新建評估器 | ❌ 未實作 |
| SK-28 | Reward mismatch (SL/RL 一致性) | 新建一致性檢查 | ❌ 未實作 |
| SK-30 | Quantum stability (參數穩定性) | 擴充現有閘門 | ❌ 未實作 |

---

## 核心哲學

> **「Atlas 有完善的實驗評判（Judge），但缺少模型層面的診斷工具。只知道『好不好』，不知道『為什麼好』。」**

---

## 評估架構

```
ML Model (from atlas-fin-ml-pipeline)
    ├── SK-12: OOS 預測力度量  ✅
    ├── SK-13: 因子重要性      ✅
    ├── SK-14: 部分相依        ✅
    ├── SK-15: 交互效應        ❌
    ├── SK-28: 獎勵一致性      ❌
    └── SK-30: 參數穩定性      ❌
```

---

## 各評估模組實作指引

### SK-12: OOS 預測力度量

已實作：`internal/eval/metrics.go` — `OOSR2(yTrue, yPred []float64) float64`  
已實作：`SharpeRatio(returns []float64, riskFreeRate float64) float64`  
輸出結構：`EvalResult{R2OOS, Sharpe, CumReturn, MaxDD}`

### SK-13: Permutation Importance

已實作：`internal/eval/importance.go` — `PermutationImportance(model, X, y, metric, nRepeats)`  
用於量化各因子的邊際貢獻。

### SK-14: Partial Dependence Plot (PDP)

已實作：`internal/eval/pdp.go` — `ComputePDP(model, X, featureIdx, nGrid)`  
回傳 `PDPResult{X, Y, FeatureName}` 用於視覺化因子效應。

### SK-15: Interaction Effects (Friedman's H)

`internal/eval/interaction.go` — **尚未實作**。  
計算 Friedman's H-statistic 以量化因子交互效應。

### SK-28: Reward Mismatch (SL/RL 一致性)

`internal/eval/reward_consistency.go` — **尚未實作**。  
檢查監督學習（minimize MSE）和強化學習（maximize Sharpe）目標的一致性。

---

## 實作位置（實際檔案對照）

| 元件 | 技能文件原名 | 實際檔案 | 狀態 |
|------|-------------|---------|------|
| OOS 度量 | `internal/eval/oos_metrics.go` | `internal/eval/metrics.go` | ✅ 已實作（名稱不同） |
| Permutation Importance | `internal/eval/permutation.go` | `internal/eval/importance.go` | ✅ 已實作（名稱不同） |
| PDP | `internal/eval/pdp.go` | `internal/eval/pdp.go` | ✅ 已實作（名稱一致） |
| Interaction Effects | `internal/eval/interaction.go` | — | ❌ 未實作 |
| Reward Consistency | `internal/eval/reward_consistency.go` | — | ❌ 未實作 |
| 模組文件 | — | `internal/eval/doc.go` | ✅ 成熟度: evolving |

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

### EvalResult 結構（已實作）

```go
// internal/eval/metrics.go

// EvalResult bundles key out-of-sample evaluation metrics.
type EvalResult struct {
    R2OOS     float64 `json:"r2_oos"`
    Sharpe    float64 `json:"sharpe"`
    CumReturn float64 `json:"cum_return"`
    MaxDD     float64 `json:"max_dd"`
}

// OOSR2 computes the out-of-sample R-squared.
func OOSR2(yTrue, yPred []float64) float64

// SharpeRatio computes the annualized Sharpe ratio from daily returns.
func SharpeRatio(returns []float64, riskFreeRate float64) float64
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
