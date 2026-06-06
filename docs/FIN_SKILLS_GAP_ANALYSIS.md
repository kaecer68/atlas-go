# Fin-Skills → Atlas Gap Analysis

**版本**: 1.0 | **日期**: 2026-05-29 | **作者**: Kaecer
**用途**: 將 Fin-Skills SK-01 至 SK-32 的學術能力要求與 atlas-go 工程實作做精確對映，標示覆蓋缺口並提出實施優先序。

---

## Executive Summary

Atlas 是一套模擬優先、稽核導向的台股投資研究系統，目前已實現了生產級投組管理基礎設施。然而，與 Fin-Skills 的完整學術規格相比，atlas 在**模型訓練、特徵工程、正式評估、穩健性檢驗和強化學習**等五個面向存在顯著落差。

截至 2026 年 5 月，32 項 Fin-Skills 的覆蓋狀況為：**4 項已覆蓋（✅）**、**6 項部分覆蓋（⚠️）**、**22 項完全缺失（❌）**。

優先實施建議：Phase A 聚焦 SK-03（時間序列切割）和 SK-12（樣本外評估）；Phase B 擴充 SK-01（因子庫）和 SK-06（彈性網）；Phase C 則逐步納入強化學習與穩健性檢驗。

---

## Coverage Matrix

| SK-ID | Fin-Skill 名稱 | Atlas 模組 | 覆蓋 | 目前已有 | 尚缺 | 優先級 |
|-------|---------------|-----------|------|---------|------|--------|
| SK-01 | 建構多元預測因子庫 | `internal/portfolio/factor_engine.go` | ⚠️ Partial | 11 factors | 75/86 factors 未實作 | B |
| SK-02 | 特徵擴充：股票-總經交互 | `internal/narrative/ingestor.go` | ❌ Missing | Macro ingestion pipeline exists | 無 86×12=1,118 交互特徵的結構化擴充邏輯 | B |
| SK-03 | 時間序列滾動切割 | `internal/backtest/window.go` | ❌ Missing | 簡單 Window.Run 視窗回測 | 無正式的 train/valid/test split | A |
| SK-04 | Huber 損失異常值處理 | — | ❌ Missing | — | 無 Huber loss（ξ=0.9）實作 | B |
| SK-05 | OLS 基準線性模型 | — | ❌ Missing | gonum/mat 可用 | 無 OLS pipeline | B |
| SK-06 | 彈性網正則化模型 | — | ❌ Missing | — | 無 L1+L2 混合正則化 | B |
| SK-07 | 廣義線性模型（樣條） | — | ❌ Missing | — | 無樣條基底展開 | C |
| SK-08 | 主成分迴歸（PCR） | — | ❌ Missing | — | 無 PCA 降維 | B |
| SK-09 | 偏最小平方法（PLS） | — | ❌ Missing | — | 無 PLS supervised 降維 | B |
| SK-10 | 隨機森林模型 | — | ❌ Missing | — | 無 RF（500 trees, max_depth=2） | B |
| SK-11 | 多層神經網路 | — | ❌ Missing | — | 無 NN 訓練框架 | C |
| SK-12 | 樣本外評估 | `internal/experiment/oos_validator.go` | ⚠️ Partial | Sharpe computation | 缺 R²_oos 公式 | A |
| SK-13 | 排列重要性 | — | ❌ Missing | — | 無 permutation importance | C |
| SK-14 | 部分相依圖 | — | ❌ Missing | — | 無 PDP marginal effects | C |
| SK-15 | 雙特徵交互作用 | — | ❌ Missing | — | 無 two-way interaction heatmap | C |
| SK-16 | 多空十分位數投資組合 | `internal/screener/screener.go` | ⚠️ Partial | Screener ranking exists | 無正式 decile portfolio 建構 | B |
| SK-17 | 加權方式 | `internal/portfolio/darwinian_weights.go` | ⚠️ Partial | Darwinian weights | 缺 formal equal-weight / value-weight | B |
| SK-18 | 因子模型風險調整 | — | ❌ Missing | — | 無 FF5+MOM 迴歸 | B |
| SK-19 | 交易成本與稅務調整 | `internal/tax/taiwan_tax.go` | ⚠️ Partial | TaiwanTaxCalculator (0.3%) | 交易成本 0.00654 未實作 | B |
| SK-20 | 規模分組穩健性檢驗 | `internal/screener/screener.go` | ❌ Missing | 僅 market cap filtering | 無 Big/Small 分組穩健性檢驗框架 | C |
| SK-21 | 排除仙股穩健性檢驗 | — | ❌ Missing | — | 無 exclude bottom 20% by price 的檢驗協定 | C |
| SK-22 | 消去法 | — | ❌ Missing | — | 無 ablation analysis | C |
| SK-23 | 產業輪動環境建構 | `internal/industry/` | ⚠️ Partial | 47 industries | 未格式化為 RL environment | C |
| SK-24 | PPO 強化學習訓練框架 | — | ❌ Missing | — | 無 RL framework | C |
| SK-25 | 獎勵函數設計與評估 | — | ❌ Missing | — | 無 reward functions | C |
| SK-26 | 經典策略網路 | — | ❌ Missing | — | 無 LSTM/Transformer policy nets | C |
| SK-27 | 量子增強策略網路 | — | ❌ Missing | — | 無 QNN/QRWKV/QASA | C |
| SK-28 | 獎勵-績效錯配診斷 | — | ❌ Missing | — | 無 Spearman correlation 診斷 | C |
| SK-29 | 滾動窗口回測模擬 | `internal/backtest/window.go` | ⚠️ Partial | Window.Run exists | 無 rolling rebalance framework | B |
| SK-30 | 量子模型訓練穩定性 | — | ❌ Missing | — | 無 gradient variance 監控 | C |
| SK-31 | 監督學習 vs. 強化學習比較 | `internal/experiment/` | ⚠️ Partial | Experiment framework | 無 SL/RL comparison protocol | C |
| SK-32 | 獎勵函數敏感性分析 | — | ❌ Missing | — | 無 reward variant testing framework | C |

**統計**: ✅ 4 (12.5%) | ⚠️ 6 (18.8%) | ❌ 22 (68.8%)

---

## Detailed Gap Analysis

### 1. Data Preparation & Feature Engineering (SK-01 ~ SK-04, SK-23)

#### SK-01: 建構多元預測因子庫
- **覆蓋**: ⚠️ Partial
- **Atlas 現狀**: FactorEngine 在 `internal/portfolio/factor_engine.go` 中計算 6 類核心因子。
- **落差**: Fin-Skills 規範 86 個股票層級因子，atlas 僅覆蓋約 11 個（~13%）。
- **建議行動**: 擴充 FactorEngine 支援從 FundamentalProvider 讀取財報資料。

#### SK-02: 特徵擴充：股票-總經交互作用
- **覆蓋**: ❌ Missing
- **Atlas 現狀**: 宏觀資料獲取管道完整 — `internal/narrative/ingestor.go` 處理 12 個宏觀變數。
- **落差**: 完全缺少 86 股票因子 × 12 宏觀變數 = 1,118 個交互特徵的結構化擴充邏輯。
- **建議行動**: 實作 `internal/feature_engineering/interaction_expander.go`。
- **MVP Spec**: 實作 `internal/feature_engineering/interaction_expander.go` — 從 FactorEngine 取得最多 5 個 stock-level factors，從 Narrative 取得 3 個 macro variables，自動生成 5×3=15 交互項矩陣。僅支援數值型交互（factor × macro），不處理類別型。預估工時：2-3 天。依賴：SK-01 FactorEngine 可輸出 feature matrix、Narrative Ingestor 可輸出 MacroDataSnapshot。

#### SK-03: 時間序列滾動切割
- **覆蓋**: ❌ Missing — **Phase A 最優先**
- **Atlas 現狀**: `internal/backtest/window.go` 的 `Window.Run` 提供單純的時間視窗回測。
- **落差**: Fin-Skills 規範年度步進式的 rolling split。
- **建議行動**: 在 `internal/backtest/` 中實作 `RollingSplitter` 型別。

#### SK-04: Huber 損失異常值處理
- **覆蓋**: ❌ Missing
- **Atlas 現狀**: 無任何 robust loss function 實作。
- **落差**: 缺 Huber loss（ξ=0.9）的 Go 實作。
- **建議行動**: 實作 `internal/ml/loss.go` 提供 HuberLoss 函數。
- **MVP Spec**: 實作 `HuberLoss(x, xi float64) float64` 單一函數於 `internal/ml/loss.go`。xi=0.9（Fin-Skills 預設）。不整合進任何訓練 pipeline（留待後續任務）。預估工時：0.5 天。依賴：無（純函數，無模組依賴）。

#### SK-23: 產業輪動環境建構
- **覆蓋**: ⚠️ Partial
- **Atlas 現狀**: 產業生態系（`internal/industry/`）極其完整。
- **落差**: 未格式化為 RL environment。
- **建議行動**: 若 RL 路線被採納，在 `internal/industry/` 中新增 `rl_environment.go`。

---

### 2. Model Building & Training (SK-05 ~ SK-11, SK-24, SK-26, SK-27)

#### SK-05: OLS 基準線性模型
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/ols.go` — gonum/mat SVD 求解 β。
- **MVP Spec**: 實作 `internal/ml/ols.go` — 使用 gonum/mat SVD 求解 β=(X'X)⁻¹X'y，含 `FitOLS(X, y *mat.Dense) (*mat.VecDense, error)`。僅支援 dense matrix，不含 intercept handling（由呼叫方手動加入常數欄）。預估工時：1 天。依賴：gonum.org/v1/gonum/mat（已整合）。

#### SK-06: 彈性網正則化模型
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/elastic_net.go` — coordinate descent with soft-thresholding。
- **MVP Spec**: 實作 `internal/ml/elastic_net.go` — coordinate descent with soft-thresholding，支援 L1+L2 混合正則化。`FitElasticNet(X, y *mat.Dense, lambda1, lambda2 float64) ([]float64, error)`。使用 cycling coordinate descent，不收斂至 tolerance 1e-4 或 max 1000 iterations。預估工時：3 天。依賴：gonum/mat（矩陣運算）。

#### SK-07: 廣義線性模型（樣條）
- **覆蓋**: ❌ Missing
- **建議行動**: 需 spline basis expansion + group lasso solver。
- **MVP Spec**: 實作 `internal/ml/glm_spline.go` — 三次自然樣條基底展開（knots=3），再對展開後特徵執行 OLS。`FitSplineGLM(X, y *mat.Dense, nKnots int) (*SplineModel, error)`。不含 Group Lasso 正則化（留待後續）。預估工時：3 天。依賴：SK-05 OLS 作為基底回歸器。

#### SK-08: 主成分迴歸（PCR）
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/pcr.go` — PCA via SVD → top-K components → OLS。
- **MVP Spec**: 實作 `internal/ml/pcr.go` — PCA via gonum/mat SVD，取 top-K components 再執行 OLS。`FitPCR(X, y *mat.Dense, nComponents int) (*PCRAsset, error)`。K 預設為 min(10, nFeatures)。預估工時：1.5 天。依賴：gonum/mat SVD、SK-05 OLS。

#### SK-09: 偏最小平方法（PLS）
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/pls.go` — NIPALS algorithm。
- **MVP Spec**: 實作 `internal/ml/pls.go` — NIPALS algorithm for weight vectors。`FitPLS(X, y *mat.Dense, nComponents int) (*PLSAsset, error)`。收斂門檻 1e-6，max 100 iterations per component。預估工時：2 天。依賴：gonum/mat（向量/矩陣運算）。

#### SK-10: 隨機森林模型
- **覆蓋**: ❌ Missing
- **建議行動**: Python bridge 方案 — `internal/ml/rf_bridge.go`。
- **MVP Spec**: Python bridge 方案 — `internal/ml/rf_bridge.go` 透過 `os/exec` 呼叫 Python sklearn RandomForestRegressor（n_estimators=500, max_depth=2），經 JSON stdin/stdout 傳遞資料與結果。`FitRF(X, y []float64) (*RFPredictor, error)`。不含 Go 原生 RF 實作。預估工時：2 天。依賴：Python 3 + sklearn 環境。

#### SK-11: 多層神經網路
- **覆蓋**: ❌ Missing
- **建議行動**: gorgonia bridge 方案。
- **MVP Spec**: gorgonia bridge 方案 — `internal/ml/nn.go` 實作 1-3 hidden layer feedforward NN，使用 ReLU activation。`BuildMLP(nFeatures, nHidden int, nLayers int) (*gorgonia.ExprGraph, error)`。不含自動微分訓練循環（留待後續）。預估工時：3 天。依賴：gorgonia.org/gorgonia（需新增）。

#### SK-24: PPO 強化學習訓練框架
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。建立 `internal/rl/` 模組。
- **MVP Spec**: 實作 `internal/rl/ppo.go` — PPO agent with actor-critic architecture。`PPOAgent` 含 policy net + value net + GAE advantage estimation + clip objective (ε=0.2) + mini-batch updates。使用 gonum/mat 做前向傳播（不依賴 gorgonia）。預估工時：5 天。依賴：gonum/mat（矩陣運算）、SK-23 RLEnvironment 介面。

#### SK-25: 獎勵函數設計與評估
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。
- **MVP Spec**: 實作 `internal/rl/rewards.go` — 三種獎勵函數：`top10_hit`（top 10 預測是否有實際報酬前 10%）、`continuous_rank`（Spearman rank correlation）、`risk_penalty`（-drawdown）。`CompositeReward` 以可配置權重組合。預估工時：1.5 天。依賴：SK-24 PPO Agent。

#### SK-26: 經典策略網路
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。
- **MVP Spec**: 實作 `internal/rl/networks.go` — 2-layer LSTM policy network for sequential sector allocation。`LSTMPolicyNet` 使用手刻 LSTM cell（forget/input/output gates）而非外部依賴。輸入為產業狀態向量，輸出為 action probabilities。預估工時：4 天。依賴：gonum/mat（矩陣運算）。

#### SK-27: 量子增強策略網路
- **覆蓋**: ❌ Missing
- **建議行動**: 放在最末優先級或完全跳過。
- **MVP Spec**: 不實施。Fin-Skills 論文顯示 QNN/QRWKV/QASA 在台股實驗中 underperforms 經典 LSTM，且量子模擬在古典硬體上無加速優勢。建議以 SK-26 LSTM 為 RL 策略網路基準。

---

### 3. Model Evaluation & Interpretation (SK-12 ~ SK-15, SK-28, SK-30)

#### SK-12: 樣本外評估
- **覆蓋**: ⚠️ Partial — **Phase A 最優先**
- **建議行動**: 在 `internal/experiment/oos_validator.go` 中新增 ComputeR2OOS 函數。

#### SK-13: 排列重要性
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/interpretability.go` 中實作 PermutationImportance。
- **MVP Spec**: 實作 `internal/eval/permutation.go` — `PermutationImportance(model ml.Model, X, y, factorNames, nRepeats=5)`。對每個因子 shuffle 後重算 OOS R²，回傳重要性排序。MVP 僅支援 dense matrix，nRepeats 固定 5（非 SK 建議的 10+）。預估工時：1.5 天。依賴：SK-05 Model 介面、SK-12 OOSMetrics.ComputeOOSMetrics。

#### SK-14: 部分相依圖
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/interpretability.go`。
- **MVP Spec**: 實作 `internal/eval/pdp.go` — `ComputePDP(model ml.Model, X *mat.Dense, factorIdx int, nGrid=20)`。對 grid points 平均預測值，回傳 PDPCurve。不含 ICE curves。預估工時：1 天。依賴：SK-05 Model.Predict 介面。

#### SK-15: 雙特徵交互作用
- **覆蓋**: ❌ Missing
- **建議行動**: `internal/ml/interpretability.go`。
- **MVP Spec**: 實作 `internal/eval/interaction.go` — `ComputeFriedmanH(model ml.Model, X *mat.Dense, factorA, factorB int)`。計算 H-statistic，回傳 InteractionResult。MVP 僅支援 pairwise（兩個因子），不含 global interaction matrix。預估工時：2 天。依賴：SK-14 PDP 計算函數。

#### SK-28: 獎勵-績效錯配診斷
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。
- **MVP Spec**: 實作 `internal/eval/reward_consistency.go` — `CheckRewardMismatch(models, X, y, prices)`。計算 Spearman rank correlation between SL MSE ranking and RL Sharpe ranking，回傳 `RewardMismatchReport`。預估工時：1 天。依賴：SK-05 Model 介面、SK-12 ComputeSharpeOOS。

#### SK-30: 量子模型訓練穩定性
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C（低優先級）。僅在 SK-27 被實作時需要。
- **MVP Spec**: 不實施（量子相關，與 SK-27 聯動）。若 SK-27 不實施則本項自動跳過。

---

### 4. Strategy Construction & Performance (SK-16 ~ SK-19, SK-25, SK-29)

#### SK-16: 多空十分位數投資組合
- **覆蓋**: ⚠️ Partial
- **建議行動**: 在 `internal/portfolio/decile_portfolio.go` 中實作。

#### SK-17: 加權方式
- **覆蓋**: ⚠️ Partial
- **建議行動**: 提供 WeightingScheme enum。

#### SK-18: 因子模型風險調整
- **覆蓋**: ❌ Missing
- **建議行動**: 建立 `internal/factor_model/` 模組。
- **MVP Spec**: 實作 `internal/factor_model/ff5.go` — 載入台灣 FF5+MOM 因子資料（從 TEJ/FinMind CSV），執行 OLS 迴歸分解策略 alpha。`RunFF5Regression(strategyReturns, factorReturns) (*FF5Regression, error)`。不含 Newey-West HAC SE（留待後續）。預估工時：2 天。依賴：SK-05 OLS 或 gonum/mat 線性回歸。

#### SK-19: 交易成本與稅務調整
- **覆蓋**: ⚠️ Partial
- **建議行動**: 在 `internal/tax/` 中新增 TaiwanCostModel。

#### SK-29: 滾動窗口回測模擬
- **覆蓋**: ⚠️ Partial
- **建議行動**: 與 SK-03 的 RollingSplitter 整合。

---

### 5. Robustness Checks (SK-20 ~ SK-22)

#### SK-20: 規模分組穩健性檢驗
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。實作 RobustnessCheckSize。
- **MVP Spec**: 實作 `internal/robustness/size_group.go` — `RunSizeGroupAnalysis(model, universe)`。依市值中位數 split 為 Big/Small 兩組，分別計算 OOS R² 並比較。MVP 僅支援二分法（非三組），不含 statistical test。預估工時：1.5 天。依賴：SK-05 Model 介面、SK-12 OOSMetrics。

#### SK-21: 排除仙股穩健性檢驗
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。實作 RobustnessCheckPennyStock。
- **MVP Spec**: 實作 `internal/robustness/exclusion.go` — `ExclusionFilter{MinPrice: 10.0, MinMarketCap: 1e9}`。Apply(universe) 回傳過濾後的 universe + 排除原因列表。僅支援 price/marketCap 過濾，不含全額交割股檢查。預估工時：0.5 天。依賴：無（純資料過濾，無模組依賴）。

#### SK-22: 消去法
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。實作 AblationAnalysis。
- **MVP Spec**: 實作 `internal/robustness/ablation.go` — `RunAblationStudy(data, factorNames, trainFn)`。從 full model 開始，逐一移除因子重新訓練，記錄 OOS R² 變化量（ΔR²）。MVP 僅支援線性模型（OLS/ElasticNet），每因子只訓練一次（非 bootstrap）。預估工時：2 天。依賴：SK-05 Model 介面、SK-12 OOSMetrics。

---

### 6. Cross-Technique Comparison (SK-31 ~ SK-32)

#### SK-31: 監督學習 vs. 強化學習比較
- **覆蓋**: ⚠️ Partial
- **建議行動**: Phase C。建立 `internal/experiment/cross_paradigm.go`。
- **MVP Spec**: 實作 `internal/experiment/cross_paradigm.go` — `ComparisonProtocol` struct 定義統一的 benchmark 配置（相同 universe、時間範圍、成本假設）。`CompareSLvsRL(slModel, rlAgent, benchmark) (*ComparisonResult, error)`。含 Sharpe、max drawdown、turnover、hit rate 四項指標。預估工時：2 天。依賴：SK-12 OOSMetrics、SK-24 PPO Agent。

#### SK-32: 獎勵函數敏感性分析
- **覆蓋**: ❌ Missing
- **建議行動**: Phase C。實作 RewardSensitivityAnalysis。
- **MVP Spec**: 實作 `internal/rl/reward_sensitivity.go` — `RunRewardSensitivity(trainFn, evalFn, paramGrid)`。對 3 種 reward weight 組合執行 grid search（每種 3 個 level = 27 combinations），產出 Sharpe sensitivity heatmap。預估工時：1.5 天。依賴：SK-24 PPO Agent、SK-25 CompositeReward。

---

## Implementation Roadmap

### Phase A: Critical (must have for production) — SK-03, SK-12

| Priority | SK | Action | Files to Create/Modify | Effort |
|----------|-----|--------|------------------------|--------|
| A1 | SK-03 | 實作 RollingSplitter | Create `internal/backtest/rolling_split.go` | 3-5 days |
| A2 | SK-12 | 擴充 OOS 評估 pipeline | Modify `internal/experiment/oos_validator.go` | 2-3 days |

### Phase B: Important (significant improvement) — SK-01, SK-02, SK-05, SK-06, SK-08, SK-09, SK-16, SK-18, SK-19, SK-29

| Priority | SK | Action | Files to Create/Modify | Effort |
|----------|-----|--------|------------------------|--------|
| B1 | SK-01 | 擴充 FactorEngine 至 30-40 factors | Create `internal/factor_registry/` | 7-10 days |
| B2 | SK-02 | 實作交互特徵擴充 | Create `internal/feature_engineering/interaction_expander.go` | 3-4 days |
| B3 | SK-05 | 實作 OLS baseline | Create `internal/ml/ols.go` | 1-2 days |
| B4 | SK-06 | 實作 ElasticNet | Create `internal/ml/elastic_net.go` | 3-4 days |
| B5 | SK-19 | 完成交易成本計算 | Modify `internal/tax/taiwan_tax.go` | 1-2 days |
| B6 | SK-08 | 實作 PCR | Create `internal/ml/pcr.go` | 1-2 days |
| B7 | SK-09 | 實作 PLS | Create `internal/ml/pls.go` | 2-3 days |
| B8 | SK-18 | 實作 FF5+MOM + Newey-West | Create `internal/factor_model/` | 4-5 days |
| B9 | SK-16 | 實作 decile portfolio | Create `internal/portfolio/decile_portfolio.go` | 2-3 days |
| B10 | SK-29 | Rolling window 回測整合 | Modify `internal/backtest/window.go` | 2-3 days |

### Phase C: Nice-to-have (research/experimental) — SK-04, SK-07, SK-10, SK-11, SK-13~15, SK-17, SK-20~32

| Priority | SK | Group | Effort |
|----------|-----|-------|--------|
| C1 | SK-04 | Huber loss | 1 day |
| C2 | SK-10 | Random Forest | 5-7 days |
| C3 | SK-13~15 | Model interpretability tools | 4-6 days |
| C4 | SK-17 | Weighting scheme formalization | 1-2 days |
| C5 | SK-20~22 | Robustness checks | 4-6 days |
| C6 | SK-23~32 | RL ecosystem | 20-30 days |
| C7 | SK-07 | GLM + Splines | 5-7 days |
| C8 | SK-11 | Neural Networks | 5-7 days |
| C9 | SK-27, SK-30 | Quantum experiments | 10-15 days (OPTIONAL) |

---

## Appendix: Atlas Module Maturity & Risk Assessment

| Module | Tier | Risk of Change | Notes |
|--------|------|---------------|-------|
| `internal/portfolio/` | E (evolving) | Medium | FactorEngine 擴充不影響 optimizer 公開 API |
| `internal/backtest/` | E (evolving) | Low | RollingSplitter 為新增型別 |
| `internal/experiment/` | S (stable) | Low | OOS 擴充為 additive |
| `internal/tax/` | E (evolving) | Low | TaiwanCostModel 為新增 |

---

## Appendix: Key References

| Reference | File | Relevance |
|-----------|------|-----------|
| Factor Engine | `internal/portfolio/factor_engine.go` | SK-01 核心實作位置 |
| OOS Validator | `internal/experiment/oos_validator.go` | SK-12 partial coverage |
| Backtest Window | `internal/backtest/window.go` | SK-03/SK-29 base |
| Taiwan Tax | `internal/tax/taiwan_tax.go` | SK-19 partial; 0.3% transaction tax |
| Constitution (API Gateway) | `internal/apigateway/CONSTITUTION.md` | Architecture constraints |
| Maturity Registry | `internal/MATURITY.md` | Module tier tracking |

(End of file)
