# Atlas ML/DL 模型訓練管線 Skill

> **實作狀態**：✅ 已實作核心模型 — `internal/ml/` 模組（成熟度 `evolving`）  
> **最後審計**：2026-06-02  
> **實際檔案結構**：`ols.go`、`elasticnet.go`、`pcr.go`、`pls.go`、`trainer.go`（含 `Model` 介面 + `KFoldSplitter` + `CrossValidate`）

**版本**: 1.0  
**日期**: 2026-05-29  
**職責**: ML/DL 模型訓練管線 — 從 Fin-Skills 監督學習模型到 atlas Go 實作

---

## Academic Foundation

基於 Fin-Skills 論文規格：

| SK 編號 | 論文主題 | 對應 Go 實作 | 狀態 |
|---------|---------|-------------|------|
| SK-03 | Rolling split (時間序列交叉驗證) | `trainer.go`（`KFoldSplitter`、`RollingWindowSplit` 於 `backtest/rolling_split.go`） | ✅ 已實作 |
| SK-05 | OLS 線性回歸 | `ols.go`（`OLSModel`） | ✅ 已實作 |
| SK-06 | ElasticNet 正則化回歸 | `elasticnet.go`（`ElasticNetModel`，座標下降法） | ✅ 已實作 |
| SK-07 | GLM with spline (廣義線性模型) | — | ❌ 未實作 |
| SK-08 | PCR (主成分回歸) | `pcr.go`（`PCRModel`，SVD 分解） | ✅ 已實作 |
| SK-09 | PLS (偏最小平方法) | `pls.go`（`PLSModel`，NIPALS 演算法） | ✅ 已實作 |
| SK-10 | RandomForest (隨機森林) | — | ❌ 未實作（建議 Python bridge） |
| SK-11 | NeuralNet 1-5 layers (神經網路) | — | ❌ 未實作（建議 gorgonia） |

---

## 核心哲學

> **「Atlas 已有 FactorEngine 計算 11 個因子，可從歷史資料學習 β。」**

---

## 訓練管線架構

```
FactorEngine (11 factors) → Train/Valid/Test Split → Model Fit → Predict → Evaluate
```

### Phase 1: 資料準備 (Data Split — SK-03)

已實作於 `trainer.go` 的 `KFoldSplitter`。rolling window split 已實作於 `internal/backtest/rolling_split.go`。

### Phase 2: 線性模型 (SK-05 ~ SK-09)

已實作：OLS（`ols.go`）、ElasticNet（`elasticnet.go`）、PCR（`pcr.go`）、PLS（`pls.go`）。

與現有 FactorEngine 的整合方式：現有 `FactorEngine` 輸出為 `map[string]float64`（symbol → totalScore）。ML 訓練層從歷史資料學習 β。

### Phase 3: 集成與深度學習 (SK-10 ~ SK-11)

RandomForest 建議 Python bridge 方案，NeuralNet 建議 gorgonia — **尚未實作**。

---

## 實作位置（實際檔案對照）

| 元件 | 技能文件建議名稱 | 實際檔案 | 狀態 |
|------|----------------|---------|------|
| Model 介面 | `internal/ml/model.go` | `internal/ml/trainer.go` | ✅ 已實作（內建於 trainer.go） |
| 資料分割 | `internal/ml/split.go` | `internal/ml/trainer.go`（`KFoldSplitter`） | ✅ 已實作（內建於 trainer.go） |
| OLS | `internal/ml/ols.go` | `internal/ml/ols.go` | ✅ 已實作 |
| ElasticNet | `internal/ml/elasticnet.go` | `internal/ml/elasticnet.go` | ✅ 已實作 |
| PCR | — | `internal/ml/pcr.go` | ✅ 已實作（SVD 分解） |
| PLS | — | `internal/ml/pls.go` | ✅ 已實作（NIPALS） |
| Trainer | `internal/ml/trainer.go` | `internal/ml/trainer.go` | ✅ 已實作（`CrossValidate`） |
| Bridge | `internal/ml/bridge.go` | — | ❌ 未實作 |
| 模組文件 | — | `internal/ml/doc.go` | ✅ 成熟度: evolving |

## 與現有模組的整合點

| 現有模組 | 整合方式 | 新增介面 |
|---------|---------|---------|
| `internal/portfolio/factor_engine.go` | 讀取 11 factors 作為模型輸入 X | `FactorEngine.FeatureMatrix()` |
| `internal/portfolio/optimizer.go` | ML 預測替代 handcrafted 加權分數 | `Optimizer.WithMLModel(m Model)` |
| `internal/experiment/judge.go` | ML 模型作為 Candidate variant | `MutationBrief.Type = "ml_model"` |
| `internal/replay/` | 載入歷史資料做 training set | `replay.LoadForML(start, end)` |
| `internal/backtest/backtest_pipeline.go` | Rolling window backtest with ML Model | `BacktestPipeline` 使用 `Model` 介面 |

---

## Go 套件依賴建議

```bash
gonum.org/v1/gonum        # mat, stat, optimize — 線代、統計、優化
gorgonia.org/gorgonia      # 神經網路（SK-11）— 尚未整合
```

---

## 實際模組結構

```
internal/ml/
├── doc.go           # 成熟度: evolving
├── ols.go           # SK-05: OLS 線性回歸
├── ols_test.go
├── elasticnet.go    # SK-06: ElasticNet（座標下降法）
├── elasticnet_test.go
├── pcr.go           # SK-08: PCA + OLS（SVD 分解）
├── pcr_test.go
├── pls.go           # SK-09: PLS（NIPALS 演算法）
├── pls_test.go
└── trainer.go       # Model 介面 + KFoldSplitter + CrossValidate
```

---

## 交叉參考

- **atlas-fin-model-eval**: 模型評估框架
- **atlas-core-architecture**: 整體架構與模組邊界
- **atlas-strategy-evolution**: 實驗生命週期
- **atlas-risk-management**: 風險整合

---

## Go 實作骨架

### Model 介面定義（已實作於 trainer.go）

```go
// internal/ml/trainer.go — Model 介面

// Model is the interface that all supervised learning models must implement.
type Model interface {
    Fit(X [][]float64, y []float64) error
    Predict(X [][]float64) ([]float64, error)
}
```

### KFoldSplitter（已實作於 trainer.go）

```go
// KFoldSplitter implements k-fold cross-validation splitting.
type KFoldSplitter struct {
    K    int
    Seed uint64
}

func (s *KFoldSplitter) Split(nSamples int) [][2][]int
```

### 與 FactorEngine 整合點

```go
// FactorEngine.CalculateAllScoresWithBreakdown() → 特徵矩陣 X
// replay.LoadForML() → forward return 向量 y
// Trainer.CrossValidate(X, y, dates) → 最佳模型預測
```

*技能版本: 1.0*  
*最後更新: 2026-05-29*
