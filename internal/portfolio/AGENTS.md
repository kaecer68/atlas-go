# internal/portfolio AGENTS.md

## OVERVIEW
`internal/portfolio` 負責台股投資組合的權重管理與因子計算，是系統「模擬優先」與「稽核導向」的核心。

**Same-package 拆分原則（Direction C 補充 — 2026-06 修訂）**：`FactorEngine` 被 11 個 consumer 使用，`DarwinianWeightManager` 被 9 個 consumer 使用。**跨 package 拆分**會增加耦合而不減少實際耦合，因此禁止。但 **同 package 內的多檔案拆分**（依職責分離）是被允許且鼓勵的，只要符合以下條件：
- 公開 API 完全不變（Layer 3 snapshot `TestPortfolio_PublicAPI` 仍 417-symbol 對齊）
- 所有 consumer 無需修改（11 個外部 + 1 個 internal caller 零修改）
- Factor Change Protocol (§12) 不被破壞
- PR #684 (executors.go 1284 → 11 檔) 是此原則的範本

目前 `FactorEngine` 已拆分為 12 個檔案（見 §2.0）。

---

## KEY CONCEPTS

### 1. Darwinian Weights (達爾文權重管理)
- **Agent 權重範圍限制**：權重強制夾制於 `[0.3, 2.5]`。`0.3` 為 whisper (低語)，`2.5` 為 shout (高喊)。
- **套用後信念範圍限制**：經過權重調整後的信念值會再夾制於 `[ConvictionClampMin, ConvictionClampMax]`（預設 `[1, 250]`），防止信念極端化。
- **動態調整**：`PerformDailyAdjustment` 依據 **Rolling Sharpe Ratio** (20天) 進行分層調整：
    - **Top 1/3**: 權重提升 (`TopQuartileMultiplier=1.05`) + Performance Bonus。
    - **Bottom 1/3**: 權重調降 (`BottomQuartileMultiplier=0.95`) + Risk Penalty (若波幅過高)。
- **套用機制**：`ApplyDarwinianWeights` 將 Agent 推薦的 Conviction 乘上權重，結果先做權重夾制再進行信念夾制。
- **主要檔案**：`darwinian_weights.go`、`agent_weights.go`

### 2. FactorEngine (多因子計算引擎)
- **因子類型**：計算 Momentum (動能)、Value (價值)、Quality (品質)、Agent (代理人)、InstitutionalSentiment (機構情緒)、Liquidity (流動性) 六類因子。
- **透明決策鏈 (Audit Trail)**：回傳 `FactorScoreBreakdown` 包含：
    - `Formula`: 實際計算公式字串。
    - `RawInputs`: 原始輸入數值 (如 P/E, P/B, 20d Volatility)。
    - `IsFallback`: 標記是否因資料缺失而使用猜測值。
- **Fallback 行為**：當歷史資料不足時，Momentum 會回退至 intraday return；Value/Quality/Liquidity 則回退至固定常數 (`0.1`/`0.05`/`0.0`)。
- **主要檔案**：見 §2.0 檔案地圖（12 個同 package 檔案，按職責分離）。`factor_engine.go` 為 entry stub。

### 2.0 FactorEngine 檔案地圖（12 個同 package 檔案）

2026-06 完成 `factor_engine.go` (1289 → 12 行 entry stub) 拆分為 12 個檔案（1 entry stub + 11 職責分離檔案）。每個檔案獨立可編譯、無 consumer 修改、Layer 3 snapshot 仍對齊 417 個 public symbol。

| 檔案 | 行數（約） | 職責 | 公開符號 |
|------|-----------|------|---------|
| `factor_engine.go` | 12 | Entry stub — package doc 指向各檔 | （無） |
| `factor_engine_types.go` | 81 | 公開型別 + 介面 + struct | `QuoteProvider`, `CorporateActionProvider`, `FactorEngine`, `PreciousMetalsContext`, `PMContextProvider`, `NarrativeProviderFunc`, `IndustryCycleProviderFunc`, `LinkageProviderFunc`, `TSMCProviderFunc` |
| `factor_engine_constructors.go` | 181 | 建構與注入入口 | `NewFactorEngine`, `WithHistoricalPrices`, `WithFundamentalProvider`, `WithParameters`, `WithNarrativeProvider`, `WithIndustryCycleProvider`, `WithLinkageProvider`, `WithTSMCProvider`, `WithCorporateActionProvider`, `SetCycleTracker`, `SetCycleCardBuilder`, `WithPreciousMetalsProvider`, `WithETFAnalyzer`, `GetETFAnalyzer` |
| `factor_engine_helpers.go` | 103 | Helpers + TTL 冪等快取 + 產業分類 | `isFinite`, `preciousMetalsRegistry`, `isPreciousMetal`, `IsPreciousMetal`, `ensureAdjusted`, `classifyIndustry` |
| `factor_engine_momentum.go` | 74 | Momentum 因子 | `CalculateMomentumScore`, `calculateMomentumDetail` |
| `factor_engine_value.go` | 158 | Value 因子（SCOR-02/03） | `CalculateValueScore`, `calculateValueDetail` |
| `factor_engine_quality.go` | 94 | Quality 因子 | `CalculateQualityScore`, `calculateQualityDetail` |
| `factor_engine_institutional.go` | 48 | Institutional Sentiment 因子 | `CalculateInstitutionalSentimentScore` |
| `factor_engine_liquidity.go` | 54 | Liquidity 因子（Amihud ILLIQ） | `CalculateLiquidityScore` |
| `factor_engine_aggregate.go` | 324 | Aggregate 計算 + SCOR-04 fallback reduction | `CalculateAllScores`, `CalculateAllScoresWithBreakdown` |
| `factor_engine_etf.go` | 55 | ETF 因子 + NAV 刷新（public，被 cmd/atlas 呼叫） | `CalculateETFScore`, `RefreshETFNAV` |
| `factor_engine_precious_metals.go` | 301 | Precious Metals 因子 + 9 sub-factor（Erb & Harvey 2013） | `CalculatePreciousMetalsScore` (+ 9 private `pm*Score`/`getPMContext`/`normalizeImportYoY`) |

**進入任一 `factor_engine_*.go` 檔案修改前必讀 §10.2**（Corporate Action TTL 冪等性 + 24h TTL 邏輯）。`ensureAdjusted` 不可在重構時破壞 `adjustedSymbols map` 的 TTL 語意，否則 `TestEnsureAdjustedCachesWithinTTL` / `TestEnsureAdjustedRefetchesAfterTTL` 會失敗。

### 2.1. FactorBridge (宏觀數據橋接器)
- **職責**：將 MacroDataSnapshot（monitoring 數據）轉換為可用於因子計算的輸入。
- **MacroDataSnapshot 來源**：
    - TWSEBalanceProvider → 散戶資金流向
    - TWSECapitalFlowProvider → 外資/法人數據
    - TaiwanStressIndex → 市場壓力指數
- **輸出結構**：
    - `ForeignFlowScore`: 外資買賣超標準化分數 [-1, 1]
    - `MarginBalanceScore`: 券資比標準化分數 [-1, 1]
    - `RetailSentimentScore`: 散戶情緒分數 [-1, 1]
    - `StressLevel`: 市場壓力等級 (0-100)
- **主要檔案**：`factor_bridge.go`（已實作）

### 2.2. InstitutionalSentiment (機構情緒因子)
- **計算方式**：
    ```
    InstitutionalSentiment = 0.50 × ForeignFlowScore
                          + 0.30 × DomesticFlowScore
                          + 0.20 × MarginBalanceScore
    ```
- **資料來源**：
    - 外資：`TWSECapitalFlowProvider.GetForeignInvestment()`
    - 法人：`TWSECapitalFlowProvider.GetDomesticInstitutional()`
    - 券資比：`TWSEBalanceProvider.GetMarginBalance()`
- **主要檔案**：功能已實作於 `factor_engine_institutional.go` 的 `CalculateInstitutionalSentimentScore()`

### 2.3. Liquidity (流動性因子)
- **計算方式**（Amihud ILLIQ proxy）：
    ```
    Liquidity = -log( |Return| / Volume )  // 標準化後
    ```
- **閾值**：
    - ILLIQ > 1.0：低流動性（因子權重調降）
    - ILLIQ < 0.1：高流動性（正常權重）
- **主要檔案**：功能已實作於 `factor_engine_liquidity.go` 的 `CalculateLiquidityScore()`

### 2.4. FactorWeightEngine (動態權重引擎)
- **職責**：根據市場事件與 Regime 動態調整因子權重，並確保權重總和為 1.0。
- **配置化**：所有因子權重、Regime 調整、事件 delta 以及策略調整均已透過 `ParametersConfig.FactorWeight` 進行配置化。`internal/config/parameters.go` 與 `configs/parameters.json` 是唯一的權威來源。
- **基礎權重配置**（**12 因子** - **預設值，可透過 parameters.json 覆蓋**）：
    | 因子 | 基礎權重 | 說明 |
    |------|----------|------|
    | Momentum | 0.25 | 動能因子,價格趨勢強度 |
    | Value | 0.20 | 價值因子,本益比/股價淨值比等估值指標 |
    | Quality | 0.20 | 品質因子,ROE/毛利率/盈餘穩定性 |
    | Agent | 0.15 | 代理因子,Agent 信心度與歷史績效 |
    | InstitutionalSentiment | 0.10 | 法人情緒,外資/投信/融資券流向 |
    | Liquidity | 0.05 | 流動性因子,Amihud ILLIQ proxy |
    | Narrative | 0.05 | 敘事因子,宏觀事件對市場情緒的影響 |
    | IndustryCycle | 0.00 | 產業週期因子,景氣位置與產業輪動評分 |
    | PreciousMetals | 0.00 | 貴金屬因子,僅黃金/白銀類股啟用 |
    | ETF | 0.00 | ETF 因子,僅 ETF 標的啟用 |
    | Linkage | 0.00 | 連動因子,需 WithLinkageProvider 注入 |
    | TSMC | 0.05 | 台積電因子,需 WithTSMCProvider 注入 |
- **新因子條件啟用**：
    - **PreciousMetals**:僅對貴金屬類股(`isPreciousMetal(symbol)` 判定)計算,其他標的權重為 0。
    - **ETF**:僅對 ETF 標的計算,個股不適用。
    - **Linkage**:需透過 `WithLinkageProvider` 注入 Provider,未注入時權重為 0。
    - **TSMC**:需透過 `WithTSMCProvider` 注入 Provider,未注入時權重為 0。
- **Known Issue (2026-06-15)**:`CalculateAllScoresWithBreakdown` 路徑曾漏算 TSMC 因子(回傳 `FactorScoreItem{}` 空值),已於同期 W2-T1 修補。`FactorTSMC` 仍由 `calculateMultiFactorScores`(optimizer.go:391-395)獨立計算,兩條路徑現在已對齊。
- **事件驅動調整**：當 NarrativeEvent 觸發時，FactorWeightEngine 根據事件 Theme 與 Severity 調整相關因子權重。
    - `AI_capex_surge`：提升 Quality (+delta) 與 Momentum (+delta)
    - `US_rates_up`：提升 Value (+delta)，降低 InstitutionalSentiment (-delta)
    - `oil_price_shock`：降低 Liquidity (-delta) 與 Momentum (-delta)
    - Severity 對應 delta：`critical=0.10`, `high=0.05`, `medium=0.02`, `low=0.01`
- **Regime 感知**：不同 Regime 下採用不同的基礎權重偏移
    - `bull`：Momentum +0.05, Quality -0.03, Value -0.02
    - `bear`：Quality +0.05, Value +0.03, Momentum -0.05
    - `high_vol`：Liquidity +0.05, Momentum -0.03, InstitutionalSentiment -0.02
- **權重保護機制**：
    - 單一因子權重 clamp 於 [0.02, 0.50]
    - 自動正規化 (normalize) 確保總和 = 1.0
    - 過期事件 (faded/expired) 自動從活躍列表移除
- **主要檔案**：`factor_weight_engine.go`
- **現況**：已實作。`Optimizer` 在 `Optimize()` 中自動呼叫 `factorWeightEngine.GetWeights(regime)` 取得動態權重，若未附加則回退至靜態 `factorWeights`。

### 3. Agent Health Management (代理健康狀態管理)
- **狀態機**：Agent 有四種健康狀態 — `healthy`、`degraded`、`muted`、`recovering`。
- **啞元閾值**：`DefaultMuteThreshold=5` 次連續虧損自動 mute；`DefaultUnmuteThreshold=3` 次連續獲利自動 unmute。
- **自動恢復**：`DefaultAutoRecoverDays=7` — mute 超過 7 天後自動進入 recovering 狀態。
- **複合分數**：`CompositeScore` = Sharpe(40%) + HitRate(30%) + Streak(30%)，低於閾值時自動干預。
- **持久化**：`AgentHealthStore` 自動儲存健康狀態，重啟後可恢復。
- **主要檔案**：`agent_health.go`、`agent_health_store.go`

### 4. Optimizer (多因子組合優化器) — P0-1 Covariance Upgrade
- **核心類型**：`FactorScore` (單因子評分)、`OptimizedPosition` (優化後倉位)、`Constraints` (組合約束)。
- **優化流程**：`aggregateRecommendations` → `calculateMultiFactorScores` → `allocateInitialWeights` → `applyConstraints` → `buildPositions`。
- **因子評分**：`symbolScore` 結構彙總 Momentum/Value/Quality/Agent 等因子，計算加權總分。
- **約束條件**：`MaxPositionPct=0.15`、`MaxSectorPct=0.40`、`CashReserve=0.05`、Beta range `[0.8, 1.2]`。
- **訂單轉換**：`OptimizeToOrders` 將優化倉位轉換為 `domain.Order` 陣列。
- **主要檔案**：`optimizer.go`

#### 4.1. 共變異數優化（P0-1 新增 — 2026-05）
- **權重分配已從單純線性歸一化升級為馬可維茲均值-變異數優化。**
- **兩路徑設計**：
  1. **有歷史資料** → 共變異數矩陣 + Ledoit-Wolf shrinkage + Active-set QP 求解最小變異數組合
  2. **無歷史資料** → Fallback 至 `linearWeights()`（保留原有線性歸一化行為）
- **共變異數估計**（Ledoit & Wolf, 2004）：
  - `sampleCov()` 計算樣本共變異數矩陣 S (N×N)
  - `ledoitWolfShrink()` 計算 shrunken estimator: Σ = (1-δ)·S + δ·ν·I
  - Shrinkage intensity δ 由 pi, rho, gamma 動態計算，非硬編碼
- **QP 求解器**（active-set method）：
  - `activeSetQP()` 求解 minimize w'Σw s.t. Σw=1, 0 ≤ wᵢ ≤ w_max
  - KKT 系統：`[2Σ_FF, A'; A, 0][w_F; λ] = [ -2Σ_FA·w_A; b - A·w_A ]`
  - 使用 `gonum.org/v1/gonum/mat.SolveVec` 解線性系統
  - `isOptimal()` 檢查 KKT 條件（∇f ≥ 0 at lb, ∇f ≤ 0 at ub）
  - `gradientProjection()` 為 fallback（KKT 系統 singular 時）
- **有效前沿**：`GetEfficientFrontier()` 計算 20 點均值-變異數前沿，使用雙等式約束（Σw=1, μ'w=r_target）
- **參數**: `lookbackDays=60`, `riskFreeRate=0.015`（年化）
- **⚠️ 公約**：不可改 `Optimize()`, `OptimizeToOrders()`, `GetEfficientFrontier()` 的公開簽章
- **⚠️ 陷阱**：`activeSetQP` 的 KKT 檢查對上界：需 ∇f ≤ 0 才 optimal（非 ≥ 0）。N=2 且 w_max = 1/N 時初始點命中兩邊界，需確保 w_init 不全部撞邊界

### 5. Capital Allocator (資本配置器)
- **核心類型**：`CapitalAllocation` — `TotalCapital`、`TotalDeployable`、`ReserveCash`、`PositionSizes`。
- **配置邏輯**：依 `CapitalPhaseConfig` 中的階段限制，按 Conviction 比例分配可部署資本。
- **零信念分配**：當 `totalConviction == 0` 時，平均分配給所有推荐。
- **稅務整合**：`ReallocateWithTax` 從各標的配置中扣除 TaxSnapshot 的 `TotalTax`。
- **主要檔案**：`capital_allocator.go`

### 6. Sector Rotator (行業輪動管理器)
- **核心類型**：`SectorAllocation` (目標配置)、`SectorRotationPlan` (輪動計劃)、`RebalancingTrade` (再平衡交易)。
- **配置化**：宏觀調整 (`SectorRotationMacroAdjustments`) 與流向調整 (`SectorRotationFlowAdjustments`) 偏移值已配置化至 `ParametersConfig.Orchestrator`。
- **宏觀驅動**：根據 `MacroRiskAssessment.Level` (Green/Yellow/Orange/Red) 調整行業配置。
    - **Green**: 維持基準配置。
    - **Yellow**: 輕度防御 (+defensive +cash, -ai_supply_chain -semiconductor)。
    - **Orange**: 中度防御 (+10% defensive +8% cash +5% gold, -8% ai_supply_chain -8% semiconductor)。
    - **Red**: 極度風險回避 (+25% cash +15% defensive +10% gold)。
- **註**：上述偏移值為系統預設值；運行時可透過 `parameters.json` 進行動態覆蓋。
- **Primary Flow**：risk_off / carry_trade_unwind / sector_rotation 三種主流流向。
- **再平衡觸發**：`RebalanceThreshold` 以下的變動被忽略，大於閾值才生成交易。
- **Drawdown 整合**：`CanExecuteRotation` 檢查 `MacroAwareDrawdownDecision` — emergency/severe 停止輪動，moderate 以上允許。
- **主要檔案**：`sector_rotator.go`

### 7. Risk Manager (風險管理器)
- **核心類型**：`RiskMetrics` (風險指標)、`RiskAlert` (風險警報)、`Position` (倉位追蹤)。
- **警報類型**：`AlertDrawdown`、`AlertPositionSize`、`AlertDailyLoss`、`AlertVolatility`、`AlertConcentration`。
- **警報級別**：`LevelInfo`、`LevelWarning`、`LevelCritical`、`LevelEmergency`。
- **最大回撤控制**：預設 8% (`maxDrawdownPct`)，觸發時生成 `LevelCritical` 警報。
- **日虧損控制**：預設 3% (`maxDailyLossPct`)，超過時生成 `LevelWarning`。
- **倉位追蹤**：維護 `positions map[string]*Position`，自動計算未實現盈虧。
- **停損/止盈**：每個 position 的 `Unrealized` 低於 -5% 觸發停損警報，高於 +20% 觸發止盈警報。
- **緊急停止**：`ShouldStopTrading` — drawdown 超過 150% max 或 3+ 個 critical/emergency 警報時返回 true。
- **主要檔案**：`risk_manager.go`、`volatility_manager.go`

### 8. Sizer (倉位規模計算器)
- **核心類型**：`Signal` (交易信號)、`RiskParameters` (風險參數)、`PositionSizingResult` (計算結果)。
- **Kelly Criterion**：`f* = (p*b - q) / b`，套用半 Kelly 係數 `KellyFraction=0.5`（Thorp, 2006）。波動率估計不穩定時，half-Kelly 比 quarter-Kelly 提供更好的成長-回撤權衡。
- **波動率調整**：目標波動率 20%，實際波動率越高調整越大 (調整範圍 0.25x - 2.0x)。
- **ATR 止損**：每筆交易承擔組合 1% 風險，`RiskAmount = Shares * ATR * ATRMultiplier(2.0)`。
- **流動性限制**：`MaxPositionByADV = 0.01` (日成交量的 1%)。
- **相關性懲罰**：平均相關性越高懲罰越大，最高 70%。
- **主要檔案**：`sizing.go`

### 9. Post-Trade Analyzer (盤後分析器)
- **核心類型**：`TradeRecord` (交易記錄)、`PerformanceMetrics` (業績指標)、`AnalysisReport` (完整報告)。
- **業績指標**：WinRate、AvgWin、AvgLoss、ProfitFactor、MaxDrawdown、SharpeRatio。
- **歸因分析**：按 Agent、按 Symbol 兩個維度計算 PnL 貢獻。
- **Agent 統計**：`AgentStats` — TotalTrades、WinCount、TotalPnL、WinRate、SharpeLike。
- **執行質量**：`ExecutionQuality` — AvgSlippage、AvgCommission、FillRate、AvgExecutionTime。
- **改進建議**：`ImprovementSuggestion` — 勝率過低、盈亏比失衡、Agent 表現不佳、回撤控制不佳、持倉時間過長。
- **主要檔案**：`analysis.go`

### 10. Historical Prices & Fundamental Provider (資料提供者)
- **職責**：提供歷史價格與基本面資料，用於 FactorEngine 的因子計算。
- **主要檔案**：`historical_prices.go`、`fundamental_loader.go`

#### 10.1 Corporate Action Adjustment (P1-2-β)
- `AdjustForCorporateActions(actions []domain.CorporateAction) error`: 對歷史價格進行除權息/減資的向後調整。運算為冪等（調用兩次結果一致）。
- `ActionEffects(symbol string) []shared.ActionEffect`: 回傳已套用的調整清單供下游（FactorEngine、reporters）使用。
- **調整因子計算**：
  - ReferencePrice > 0: factor = ReferencePrice / postEventRawPrice（優先）
  - CashDividend > 0: factor = (postEventPrice - CashDividend) / postEventPrice
  - StockDividend > 0: factor = (10.0 - StockDividend) / 10.0（面額 10 元）
  - CapitalReductionRatio > 0: factor = 1.0 - CapitalReductionRatio
- **冪等性**: 偵測 pre-event price 已符合調整後數值時跳過。
- **行動排序**: caller 負責按 ExDate 升序排序；未知 symbol 靜默忽略。

#### 10.2 Corporate Action Integration in FactorEngine (P1-2-γ)
- **Interface**: `CorporateActionProvider` defined in `factor_engine_types.go:19-21` — single method `GetCorporateActions(ctx context.Context, symbol string, start, end time.Time) ([]domain.CorporateAction, error)`.
- **Injection**: `WithCorporateActionProvider(CorporateActionProvider)` setter attaches a provider to `FactorEngine`.
- **Internal state**:
  - `corpActions CorporateActionProvider` — the attached provider (nil = skip adjustment)
  - `adjustedMu sync.Mutex` — protects the cache maps
  - `adjustedSymbols map[string]time.Time` — tracks which symbols have been adjusted + when
  - `adjustmentTTL 24 * time.Hour` — TTL after which a symbol is re-adjusted
- **`ensureAdjusted(ctx, symbol string) error`**:
  1. If `corpActions` is nil → skip (no-op)
  2. Check `adjustedSymbols[symbol]`: if exists and `time.Since(lastAdjusted) < adjustmentTTL` → skip (fresh)
  3. Otherwise, use a 365-day lookback window (`now-365d` → `now`)
  4. Call `corpActions.GetCorporateActions(ctx, symbol, start, end)`
  5. If actions found and `>0`, call `hp.AdjustForCorporateActions(actions)`
  6. On success: store `adjustedSymbols[symbol] = time.Now()`
  7. On fetch failure: log `logging.Warn(...)` with the error (non-fatal)
- **Integration points**:
  - `calculateMomentumDetail()` calls `ensureAdjusted(ctx, symbol)` at the top
  - `calculateQualityDetail()` calls `ensureAdjusted(ctx, symbol)` at the top
  - Uses `ctx` from caller (now propagated through detail methods)
- **Failure mode**: If corporate action fetch fails, the symbol is NOT marked adjusted. Next cycle retries. If no provider attached, adjustment is silently skipped.
- **Dependency**: Uses `HistoricalPrices` (`hp` field) which must have `AdjustForCorporateActions` method.

### 11. Conviction Normalizer & Regime/Style (信念正規化)
- **Conviction Normalizer**：將不同來源的信念分數正規化至統一尺度。
- **Regime/Style**：市場體制識別 (Bull/Bear/Neutral/HighVol) 與風格標籤 (Growth/Value/Quality)。
- **Regime 判斷條件**：
    | Regime | 判斷條件 |
    |--------|----------|
    | Bull | VIX < 15, TrendUp |
    | Bear | VIX > 25, TrendDown |
    | Neutral | VIX 15-25 |
    | HighVol | VIX > 30 |
- **RegimeChange 觸發時**：
    1. 記錄 `PreviousRegime` / `CurrentRegime`
    2. 計算 Regime 持續時間
    3. 觸發 FactorWeightEngine 重新計算權重
    4. 發送 `RegimeChangedEvent` 到因果鏈
- **主要檔案**：`conviction_normalizer.go`、`regime.go`、`style.go`

---

## ANTI-PATTERNS (高危陷阱)

- **Silent Clamping (靜默夾制)**：權重調整在 `constrainWeight` 中靜默完成，外部調用者若不檢查 `adjustments` 回傳值將無法得知是否觸碰邊界。
- **Ignoring IsFallback (忽略回退標記)**：在進行實驗評價 (Judge) 或 決策鏈審查時，必須檢查 `IsFallback`，否則會誤信低品質的估算數據。
- **Mutable Slice Reuse (切片重用)**：`ApplyDarwinianWeights` 會生成新的 `Recommendation` 切片，切勿直接修改傳入的原切片。
- **不檢查 AgentHealth 就放行**：若 Agent 處於 `muted` 狀態，其推薦不應進入 pipeline。`IsAgentHealthy()` 會對 unknown agent 預設返回 true，但啞元 agent (從未出現過) 也視為 healthy。
- **Optimizer 未 Attach FactorEngine**：直接建立 `NewOptimizer()` 而不呼叫 `WithFactorEngine()` 會導致 Momentum/Value/Quality 因子計算失敗，回退至 fallback 值。
- **FactorWeightEngine 未正規化**：`GetWeights()` 回傳前必須經過 `normalizeWeights()`，否則權重總和不為 1.0 會導致評分偏差。
- **忽略過期事件**：`Update()` 必須定期呼叫以移除 faded/expired 事件，否則權重調整會永久殘留。
- **Sizer 的 ATR 預設值為 0**：`getATR` 未快取時返回 0，導致 `adjustForATR` 調整失效，倉位可能過大。

---

## DATA FLOW

```
Market Data (quotes, fundamentals)
         ↓
FactorEngine (CalculateMomentum/Value/Quality)
         ↓
Optimizer.Optimize() ← FactorEngine + AgentWeights + Constraints
         ↓
CapitalAllocator.Allocate() ← CapitalPhaseConfig + Conviction weights
         ↓
Sizer.CalculateSize() ← Kelly + Volatility + ATR + Liquidity + Correlation
         ↓
RiskManager.AddPosition() / UpdatePosition()
         ↓
PostTradeAnalyzer.Record() / CalculateMetrics()
```

**Darwinian Weights 平行於整條 pipeline**：
```
Agent Recommendations → ApplyDarwinianWeights() → Modified Conviction → Optimizer
```

**Agent Health 監控整條 pipeline**：
```
RecordOutcome() per agent → evaluateInterventions() → mute/unmute/recover
```

**Sector Rotation 在宏觀層面影響配置**：
```
MacroRiskAssessment → SectorRotator.GeneratePlan() → CapitalAllocator 的 sector constraints
```

---

## KEY TYPES (public 結構體)

| 結構體 | 檔案 | 用途 |
|--------|------|------|
| `DarwinianWeightManager` | darwinian_weights.go | 達爾文權重動態調整 |
| `FactorEngine` | factor_engine.go | 多因子評分計算 |
| `AgentHealthManager` | agent_health.go | 代理健康狀態追蹤 |
| `Optimizer` | optimizer.go | 組合優化核心 |
| `CapitalAllocator` | capital_allocator.go | 資本配置 |
| `SectorRotator` | sector_rotator.go | 行業輪動 |
| `RiskManager` | risk_manager.go | 風險控制 |
| `Sizer` | sizing.go | 倉位規模計算 |
| `PostTradeAnalyzer` | analysis.go | 盤後分析 |

---

## 12. Factor Change Protocol (因子變更協議)

新增、刪除或改名任何 `FactorType` 時，**必須順序更新以下 8 個位置**：

| Step | 位置 | 變更內容 |
|------|------|---------|
| 1 | `optimizer.go:15-27` | FactorType 常數宣告 |
| 2 | `factor_weight_engine.go:41-56` | `defaultBaseWeights` map |
| 3 | `internal/domain/shared/shared.go:48-62` | `FactorScoreBreakdown` struct（含 json tag） |
| 4 | `internal/domain/shared/shared.go:64-79` | `FactorScores` struct |
| 5 | `optimizer_pipeline.go:28-45` | `symbolScore` struct |
| 6 | `optimizer_pipeline.go:138-164` | `calculateMultiFactorScores` totalScore 計算 |
| 7 | `factor_engine_aggregate.go` | `CalculateAllScoresWithBreakdown` breakdown 建構（2026-06 從 `factor_engine.go` 拆分；如重新合併請保留行號） |
| 8 | `factor_weight_engine.go` | `applyEventAdjustment` / `strategyDeltas` / `GetWeights` 中的因子引用 |

> 行號會隨重構漂移，請以「該檔案中對應語義區塊」為準。

**完成後必須執行**：

```bash
go generate .                                    # 同步前端型別
bash scripts/ci/verify_factor_integrity.sh        # G1-G10 完整性驗證
go build ./... && go test ./internal/portfolio/... # 編譯 + 測試
```

**CI 強制**：`quality.yml` 的 `factor-integrity` job 會在每個 PR 自動執行 `verify_factor_integrity.sh`，違反者 CI 失敗。

---

## VERIFICATION

```bash
go build ./internal/portfolio/...
go test ./internal/portfolio/...
go vet ./internal/portfolio/...
test -z "$(gofmt -l internal/portfolio/)"
```
## 13. 因子權重校準器 (FactorWeightCalibrator)

`CalibrateWeights(ctx, orders)` 提供全自動的因子權重校準：
- 從 session ledger 載入歷史推薦資料（含因子分數與 forward return）
- 使用 Bayesian optimizer (Gaussian Process + RBF kernel) 搜尋最優權重
- 自審計（train/validation split）：改善 >3% 則自動套用，退化則自動拒絕
- 結果寫入 `parameters.json` → `GetWeights()` 即時讀取（hot-reload）
- 背景任務 `factor_weight_calibrate` 每 24h 自動執行

### 校準結果即時同步

`GetWeights()` 會合併 `baseWeights` 與 `config.FactorWeight.BaseWeights`。校準器寫入 config 後，**無需重啟**即可生效。

## 14. CycleTracker 橋接

`FactorEngine.SetCycleTracker(ct)` 接受來自 monitoring 服務的共用 CycleTracker。當即時數據（FinMind/Fubon）更新 CycleTracker 時，`cycleProv` 自動反映最新產業週期位置。
