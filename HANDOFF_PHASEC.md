# Handoff — Atlas Phase C: Feature Expansion + SK-07 GLM Spline + SK-10 RandomForest

**移交日期**: 2026-06-02
**來源 workspace**: `/Users/kaecer/workspace/atlas` @ main (latest)
**目標**: 新開 Claude Code workspace 接手 Phase C

---

## Part 0: 你繼承了什麼（已完成，不要重做）

### 核心基礎設施（Phase A+B 全部完成）

| 模組 | 位置 | 狀態 |
|------|------|------|
| ML 模型 (4個) | `internal/ml/ols.go, pcr.go, pls.go, elasticnet.go` | ✅ 都有 New*() 建構子，滿足 `ml.Model` 介面 |
| 評估工具 | `internal/eval/importance.go, metrics.go, pdp.go` | ✅ PermutationImportance, PDP, OOSR2, Sharpe 全有 |
| KFoldSplitter | `internal/ml/trainer.go:27` | ✅ 支援 k-fold CV，隨機洗牌 |
| 特徵萃取 (7 features) | `internal/feature/feature.go` | ✅ close, volume, return_1d, return_5d, hl_ratio, ma_ratio, volume_ratio |
| CLI | `cmd/backtest-pipeline/main.go` (399行) | ✅ -model (ols/pcr/pls/elasticnet), -features, -synthetic, -out CSV, split params |
| FactorPredictor | `internal/experiment/importance.go` (78行) | ✅ 包裝 ml.OLS，滿足 eval.Predictor。ComputeImportance(X,y) + ComputeImportanceFromBars(bars) |
| Judge 接線 | `internal/experiment/judge.go` | ✅ OOS 通過後自動呼叫 computeAndAttachImportance() |
| 參數校準 | `cmd/atlas/main.go` | ✅ 7 個自我校準迴圈（auto_calibrate, risk_gate, factor_weight, ML retrain 等） |
| 數據管道 | 40 個 data provider files | ✅ Gateway, circuit breaker, rate limiting, health tracking |
| 覆蓋率 | 全部模組 > 40% | ✅ feature/ 87.1%, eval/ 98%, ml/ 65.3% |

### 關鍵介面（你要延伸的）

```go
// internal/ml/trainer.go — 所有模型實作這個
type Model interface {
    Fit(X [][]float64, y []float64) error
    Predict(X [][]float64) ([]float64, error)
}

// internal/eval/importance.go — FactorPredictor 用這個
type Predictor interface {
    Predict(X [][]float64) ([]float64, error)
}

// internal/feature/feature.go — 特徵註冊表（你主要改這個）
var Registry = map[string]Func{...}  // 目前 7 個特徵
type Func func(bar domain.DailyBar, idx int, bars []domain.DailyBar) float64
```

### 可用的數據欄位（DailyBar）

```go
type DailyBar struct {
    Date   time.Time  // 交易日
    Symbol string     // 如 "2330.TW"
    Open   float64
    High   float64
    Low    float64
    Close  float64
    Volume int64
}
```

> 注意：DailyBar 沒有基本面欄位（P/E, P/B, 市值等）。基本面需要從 FinMind/TWSE channel 另外取。

---

## Part 1: 你要做的工作 — Phase C

### 總目標
從目前的 7 個線性特徵擴張到 25+ 個多樣化特徵，然後實作 GLM spline (SK-07) 和 RandomForest (SK-10)。

### Phase C1: 特徵擴張（從 7 → 25+）

把以下特徵加入 `internal/feature/feature.go` 的 `Registry`。全部可從 DailyBar OHLCV 計算，**不需要新資料源**。

#### 第一批：15 個技術指標（全都可以從 OHLCV 計算）

| # | 特徵名 | 公式 | 最少 bars |
|---|--------|------|-----------|
| 1 | `rsi_14` | 100 - 100/(1 + avg_gain/avg_loss over 14) | 14 |
| 2 | `macd` | 12EMA(Close) - 26EMA(Close) | 26 |
| 3 | `macd_signal` | 9EMA of MACD | 35 |
| 4 | `bb_pct_b` | (Close - MA20) / (2 × std20) | 20 |
| 5 | `atr_14` | mean(true_range, 14) | 14 |
| 6 | `obv` | cumulative(if Close>prev_Close then +Volume else -Volume) | 2 |
| 7 | `adx_14` | smoothed DX over 14 | 28 |
| 8 | `volatility_20d` | std(log_return, 20) × sqrt(252) | 20 |
| 9 | `skewness_20d` | skew(log_return, 20) | 20 |
| 10 | `kurtosis_20d` | kurt(log_return, 20) | 20 |
| 11 | `amihud` | abs(return) / (Close × Volume) × 1e6 | 1 |
| 12 | `price_position` | (Close - MA20) / Close | 20 |
| 13 | `volume_trend` | MA(Volume, 5) / MA(Volume, 20) | 20 |
| 14 | `hl_range_pct` | (High - Low) / MA(Close, 20) | 20 |
| 15 | `return_autocorr` | corr(return[t], return[t-1]) over 20 | 21 |

每個特徵的實作模式參照現有的 `ma_ratio`（需要 window 的特徵在 idx < window 時回傳 preset fallback 值）。

#### 第二批：ml_scorer 特徵遷移（從 `internal/orchestrator/ml_scorer.go` 搬到 `feature.Registry`）

目前 `extractFeatures()` 有 4 個獨立特徵，不經過 `feature.Registry`。應加入統一註冊表：

| 特徵名 | 公式 | 來源 |
|--------|------|------|
| `momentum_intra` | (Close - Open) / Open | ml_scorer.go |
| `value_intra` | Close / Open | ml_scorer.go |
| `quality_intra` | 1 - (High-Low)/Close | ml_scorer.go |
| `liquidity` | log(1 + Volume) | ml_scorer.go |

#### 第三批：Derived cross-features（可選，加分）

- `return_1d × volume_ratio` (交互項)
- `volatility_20d / hl_ratio` (波動率歸一化的 range)

### Phase C2: SK-07 GLM Spline

GLM spline 的本質：對選定的特徵做 natural cubic spline basis expansion（1 個特徵 → k 個 basis functions），然後用 IRLS 擬合 GLM。

#### 實作檔案
新建 `internal/ml/spline.go`，實作 `ml.Model` 介面。

```go
// GLMSpline fits a Generalized Linear Model with spline basis expansion.
type GLMSpline struct {
    // Degree is the number of basis functions per feature (default 3).
    Degree int
    // Features is the indices of features to apply spline to (nil = all).
    Features []int
    // Family is "gaussian" (linear), "poisson" (count), or "gamma" (positive).
    Family string
    // FitIntercept prepends intercept column (default true).
    FitIntercept bool

    // internal state
    nFeatures int
    knotPositions [][]float64  // per-feature knot positions
    coeffs []float64
    fitted bool
}
```

#### 需要的子元件

1. **Spline basis 函數**: 輸入 `(x float64, knots []float64, degree int) → []float64`（degree+1 個 basis values）
   - natural cubic spline: knots = quantiles(x, degree-1)，邊界處線性延伸
2. **IRLS solver**: 輸入 `(X, y, family, maxIter, tol) → []float64`
   - Gaussian: 等價於 OLS（normal equation）
   - Poisson: weight = exp(Xβ)，iteratively reweighted LS
3. **CV for degree selection**: 用 `KFoldSplitter`（已有）對 degree ∈ [2,3,4,5] 做 cross-validation

#### 測試
- `TestGLMSpline_Gaussian` — Gaussian family 等價於 OLS
- `TestGLMSpline_DegreeCV` — CV 自動選 degree
- `TestGLMSpline_EmptyData` — 邊界條件

### Phase C3: SK-10 RandomForest

#### 實作檔案
新建 `internal/ml/randomforest.go`，實作 `ml.Model` 介面。

```go
type RandomForest struct {
    // NTrees is the number of trees in the forest (default 100).
    NTrees int
    // MaxDepth limits tree depth (0 = unlimited).
    MaxDepth int
    // MinSamplesSplit is the minimum samples to split a node (default 5).
    MinSamplesSplit int
    // MaxFeatures is the number of features to sample per split ("sqrt" or int).
    MaxFeatures string  // "sqrt" or "all"
    // Seed for reproducibility.
    Seed uint64

    // internal
    trees []*decisionTree
    oobPredictions [][]float64 // for OOB error
}
```

#### 需要的子元件

1. **CART decision tree**: `type decisionTree struct` — 遞迴二分，MSE impurity，max_depth 停止
2. **Bootstrap sampling**: 從 n 樣本中抽 n 個 with replacement（產生 OOB）
3. **Random subspace**: 每個 split 只考慮 `maxFeatures` 個隨機特徵
4. **Prediction**: 所有樹的平均（regression）或多數決（classification）

> Go 沒有 scikit-learn，所有子元件都得手刻。CART tree 是最複雜的部分（~150 行）。

#### 測試
- `TestRandomForest_Basic` — 合成資料，n_trees=10，驗證預測有效
- `TestRandomForest_OOB` — 驗證 OOB 樣本不被用於訓練
- `TestRandomForest_CompareOLS` — 非線性資料上 RF 應顯著優於 OLS

### Phase C4: 接進 CLI 和 Judge（用既有的 integration pattern）

1. 在 `newModel()` in `cmd/backtest-pipeline/main.go` 加入 `"glm"` → `&ml.GLMSpline{}` 和 `"rf"` → `&ml.RandomForest{}`
2. 跑 `--synthetic` 驗證兩者都能通過 basic smoke test
3. 可選：讓 `FactorPredictor` 支援切換模型（目前硬編碼 `ml.OLS`）

---

## Part 1.5: 重要架構注意事項

### feature.Registry vs FactorEngine — 兩條不同的特徵管線

```
feature.Registry (你負責擴張的)
  └── 從 DailyBar OHLCV 計算技術指標
  └── 用於 cmd/backtest-pipeline + FactorPredictor

FactorEngine (已存在，不要改)
  └── 從 FundamentalProvider + MacroDataSnapshot + HistoricalPrices 計算因子
  └── 用於 orchestrator 即時評分
```

**基本面資料（P/E, P/B, P/S, 股息率）** 不走 `feature.Registry`。
它們來自 `data/fundamentals.json`（靜態檔案，由 `FundamentalProvider` 載入），
透過 `FactorEngine.calculateValueDetail()` 計算。這條管線已經完整。

**巨集觀資料（T86 外資買賣超、融資餘額、產業指數）** 也不走 `feature.Registry`。
它們來自 `MacroDataSnapshot` → `FactorBridge` → `FactorEngine.calculateInstitutionalSentimentDetail()`。

**你的 Phase C1 特徵擴張** 應該集中在 `feature.Registry`，專注於可以從 OHLCV 計算的技術指標。
不要把基本面或巨集觀資料混進來 — 它們有自己的管線。

---

## Part 2: 實作指引

### 程式碼慣例（必須遵守）
- Go 1.25，模組名 `github.com/kaecer68/atlas-go`
- import 順序：stdlib → `gonum` → `github.com/kaecer68/atlas-go/...`
- 錯誤包裝：`fmt.Errorf("context: %w", err)`
- 新 `internal/` 模組需要 `doc.go` + MATURITY.md 更新
- 測試與原始碼同目錄同 package（`*_test.go`）

### TDD 要求
每個新特徵和模型必須測試先行（RED → GREEN）：
1. 先寫測試，確認它 fail（不是 syntax error，是 assertion fail）
2. 寫最小實作讓它 pass
3. 驗證 `gofmt -l` + `go vet` + `go test -cover`

### CI 指令
```bash
gofmt -l . && go build ./... && go test ./... && go vet ./...
```

### 驗收標準
1. `internal/feature/` 有 25+ 特徵，全部有測試，覆蓋率 > 80%
2. `internal/ml/spline.go` 存在，`TestGLMSpline_Gaussian` PASS
3. `internal/ml/randomforest.go` 存在，`TestRandomForest_Basic` PASS
4. `cmd/backtest-pipeline -synthetic -model glm` 可運行
5. `cmd/backtest-pipeline -synthetic -model rf` 可運行
6. 無破壞既有 18 個 ml/ 測試

---

## Part 3: 補充上下文

### Feature Registry 擴張模式

參考現有的 `ma_ratio` 實作 (`internal/feature/feature.go:43-55`)：
- 用 `idx` 參數判斷是否有足夠歷史數據
- window 不足時回傳 neutral 值（1.0 或 0.0）
- 避免 division by zero（檢查 `Close > 0`、`sum > 0`）
- 加入 `Registry` map，用 `"snake_case"` 命名

```go
// 加入 Registry 的模式:
"rsi_14": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
    if idx < 14 { return 50.0 }  // neutral RSI
    // ... compute RSI(14) ...
},
```

### 何時用 gonum

GLM spline 的 IRLS 和 spline basis 計算推薦使用 `gonum.org/v1/gonum/mat`（已在 go.mod 中）。但 CART tree 和 RF 不需要矩陣運算，用純 Go 即可。

### 何時該停

如果 spline 或 RF 的非線性 R² 不顯著優於 OLS（在既有 7 特徵上），這是**預期行為**，不是 bug。非線性模型的價值在特徵擴張之後才會顯現。先完成 Phase C1，再評估 C2/C3 的優先級。

