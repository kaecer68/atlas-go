# Atlas ML/DL 模型訓練管線 Skill

**版本**: 1.0
**日期**: 2026-05-29
**職責**: ML/DL 模型訓練管線 — 從 Fin-Skills 監督學習模型到 atlas Go 實作

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | 對應 Go 實作層級 |
|---------|---------|-----------------|
| SK-03 | Rolling split (時間序列交叉驗證) | 資料分割層 |
| SK-05 | OLS 線性回歸 | 線性模型層 |
| SK-06 | ElasticNet 正則化回歸 | 線性模型層 |
| SK-07 | GLM with spline (廣義線性模型) | 非線性模型層 |
| SK-08 | PCR (主成分回歸) | 降維層 |
| SK-09 | PLS (偏最小平方法) | 降維層 |
| SK-10 | RandomForest (隨機森林) | 集成學習層 |
| SK-11 | NeuralNet 1-5 layers (神經網路) | 深度學習層 |

---

## 核心哲學

> **「Atlas 已有 FactorEngine 計算 11 個因子，但缺少 ML 訓練層來從因子預測報酬。」**

---

## 訓練管線架構

```
FactorEngine (11 factors) → Train/Valid/Test Split → Model Fit → Predict → Evaluate
```

### Phase 1: 資料準備 (Data Split — SK-03)

不使用隨機 shuffle。金融資料必須用 rolling window split。

### Phase 2: 線性模型 (SK-05 ~ SK-09)

與現有 FactorEngine 的整合方式：現有 `FactorEngine` 輸出為 `map[string]float64`（symbol → totalScore）。ML 訓練層改為從歷史資料學習 β。

### Phase 3: 集成與深度學習 (SK-10 ~ SK-11)

RandomForest 建議 Python bridge 方案，NeuralNet 建議 gorgonia。

---

## 與現有模組的整合點

| 現有模組 | 整合方式 | 新增介面 |
|---------|---------|---------|
| `internal/portfolio/factor_engine.go` | 讀取 11 factors 作為模型輸入 X | `FactorEngine.FeatureMatrix()` |
| `internal/portfolio/optimizer.go` | ML 預測替代 handcrafted 加權分數 | `Optimizer.WithMLModel(m Model)` |
| `internal/experiment/judge.go` | ML 模型作為 Candidate variant | `MutationBrief.Type = "ml_model"` |
| `internal/replay/` | 載入歷史資料做 training set | `replay.LoadForML(start, end)` |

---

## Go 套件依賴建議

```bash
gonum.org/v1/gonum        # mat, stat, optimize — 線代、統計、優化
gorgonia.org/gorgonia      # 神經網路（SK-11）
```

---

## 新模組結構建議

```
internal/ml/
├── doc.go
├── split.go
├── ols.go
├── elasticnet.go
├── model.go
└── bridge.go
```

---

## 交叉參考

- **atlas-fin-model-eval**: 模型評估框架
- **atlas-core-architecture**: 整體架構與模組邊界
- **atlas-strategy-evolution**: 實驗生命週期
- **atlas-risk-management**: 風險整合

---

## Go 實作骨架

### Model 介面定義

```go
// internal/ml/model.go — Model 介面

// Model defines the unified interface for all supervised learning models
// in the atlas ML pipeline. Models are trained on factor matrices (X) and
// forward returns (y), then produce predictions for OOS evaluation.
type Model interface {
    Fit(X, y *mat.Dense) error
    Predict(X *mat.Dense) (*mat.VecDense, error)
    Name() string
    Coefficients() []float64  // for audit trail transparency
}
```

### Trainer 結構

```go
// internal/ml/trainer.go — CrossValidate

// Trainer orchestrates model training with rolling-window cross-validation.
type Trainer struct {
    Splitter *RollingSplitter  // SK-03: train/valid/test split
    Registry map[string]Model  // model name → constructor
}

// CrossValidate trains all registered models on each split's training window,
// selects the best model by validation OOS R², and returns test-set predictions.
func (tr *Trainer) CrossValidate(factors, returns *mat.Dense, dates []time.Time) (*CVResult, error)
```

### 與 FactorEngine 整合點

```go
// FactorEngine.CalculateAllScoresWithBreakdown() → 特徵矩陣 X
// replay.LoadForML() → forward return 向量 y
// Trainer.CrossValidate(X, y, dates) → 最佳模型預測
```

*技能版本: 1.0*
*最後更新: 2026-05-29*
