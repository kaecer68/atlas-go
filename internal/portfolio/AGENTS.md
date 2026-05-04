# internal/portfolio AGENTS.md

## OVERVIEW
`internal/portfolio` 負責台股投資組合的權重管理與因子計算，是系統「模擬優先」與「稽核導向」的核心。

portfolio 不拆分程式碼（Direction C）：`FactorEngine` 被 11 個 consumer 使用，`DarwinianWeightManager` 被 9 個 consumer 使用，兩者共同服務同一 pipeline，拆分只會增加跨 package 耦合而不減少實際耦合。代わりに、加強此 AGENTS.md 內部文件以覆蓋所有職責區塊。

---

## KEY CONCEPTS

### 1. Darwinian Weights (達爾文權重管理)
- **範圍限制**：權重強制夾制於 `[0.3, 2.5]`。`0.3` 為 whisper (低語)，`2.5` 為 shout (高喊)。
- **動態調整**：`PerformDailyAdjustment` 依據 **Rolling Sharpe Ratio** (20天) 進行分層調整：
    - **Top 1/3**: 權重提升 (`TopQuartileMultiplier=1.05`) + Performance Bonus。
    - **Bottom 1/3**: 權重調降 (`BottomQuartileMultiplier=0.95`) + Risk Penalty (若波幅過高)。
- **套用機制**：`ApplyDarwinianWeights` 將 Agent 推薦的 Conviction 乘上權重，結果限制在 `[1, 250]`。
- **主要檔案**：`darwinian_weights.go`、`agent_weights.go`

### 2. FactorEngine (多因子計算引擎)
- **因子類型**：計算 Momentum (動能)、Value (價值)、Quality (品質)、Agent (代理人)、InstitutionalSentiment (機構情緒)、Liquidity (流動性) 六類因子。
- **透明決策鏈 (Audit Trail)**：回傳 `FactorScoreBreakdown` 包含：
    - `Formula`: 實際計算公式字串。
    - `RawInputs`: 原始輸入數值 (如 P/E, P/B, 20d Volatility)。
    - `IsFallback`: 標記是否因資料缺失而使用猜測值。
- **Fallback 行為**：當歷史資料不足時，Momentum 會回退至 intraday return；Value/Quality/Liquidity 則回退至固定常數 (`0.1`/`0.05`/`0.0`)。
- **主要檔案**：`factor_engine.go`

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
- **主要檔案**：功能已實作於 `factor_engine.go` 的 `CalculateInstitutionalSentimentScore()`

### 2.3. Liquidity (流動性因子)
- **計算方式**（Amihud ILLIQ proxy）：
    ```
    Liquidity = -log( |Return| / Volume )  // 標準化後
    ```
- **閾值**：
    - ILLIQ > 1.0：低流動性（因子權重調降）
    - ILLIQ < 0.1：高流動性（正常權重）
- **主要檔案**：功能已實作於 `factor_engine.go` 的 `CalculateLiquidityScore()`

### 2.4. FactorWeightEngine (動態權重引擎)
- **職責**：根據市場事件與 Regime 動態調整因子權重。
- **基礎權重配置**（6 因子）：
    | 因子 | 基礎權重 |
    |------|----------|
    | Momentum | 0.20 |
    | Value | 0.15 |
    | Quality | 0.15 |
    | Agent | 0.20 |
    | InstitutionalSentiment | 0.15 |
    | Liquidity | 0.15 |
- **事件驅動調整**：當 NarrativeEvent 觸發時，FactorWeightEngine 根據事件 Theme 調整相關因子權重。
- **Regime 感知**：不同 Regime 下採用不同的基礎權重配置。
- **主要檔案**：`factor_weight_engine.go`（待建立）

### 3. Agent Health Management (代理健康狀態管理)
- **狀態機**：Agent 有四種健康狀態 — `healthy`、`degraded`、`muted`、`recovering`。
- **啞元閾值**：`DefaultMuteThreshold=5` 次連續虧損自動 mute；`DefaultUnmuteThreshold=3` 次連續獲利自動 unmute。
- **自動恢復**：`DefaultAutoRecoverDays=7` — mute 超過 7 天後自動進入 recovering 狀態。
- **複合分數**：`CompositeScore` = Sharpe(40%) + HitRate(30%) + Streak(30%)，低於閾值時自動干預。
- **持久化**：`AgentHealthStore` 自動儲存健康狀態，重啟後可恢復。
- **主要檔案**：`agent_health.go`、`agent_health_store.go`

### 4. Optimizer (多因子組合優化器)
- **核心類型**：`FactorScore` (單因子評分)、`OptimizedPosition` (優化後倉位)、`Constraints` (組合約束)。
- **優化流程**：`aggregateRecommendations` → `calculateMultiFactorScores` → `allocateInitialWeights` → `applyConstraints` → `buildPositions`。
- **因子評分**：`symbolScore` 結構彙總 Momentum/Value/Quality/Agent 四因子，計算加權總分。
- **約束條件**：`MaxPositionPct=0.15`、`MaxSectorPct=0.40`、`CashReserve=0.05`、Beta range `[0.8, 1.2]`。
- **訂單轉換**：`OptimizeToOrders` 將優化倉位轉換為 `domain.Order` 陣列。
- **主要檔案**：`optimizer.go`

### 5. Capital Allocator (資本配置器)
- **核心類型**：`CapitalAllocation` — `TotalCapital`、`TotalDeployable`、`ReserveCash`、`PositionSizes`。
- **配置邏輯**：依 `CapitalPhaseConfig` 中的階段限制，按 Conviction 比例分配可部署資本。
- **零信念分配**：當 `totalConviction == 0` 時，平均分配給所有推荐。
- **稅務整合**：`ReallocateWithTax` 從各標的配置中扣除 TaxSnapshot 的 `TotalTax`。
- **主要檔案**：`capital_allocator.go`

### 6. Sector Rotator (行業輪動管理器)
- **核心類型**：`SectorAllocation` (目標配置)、`SectorRotationPlan` (輪動計劃)、`RebalancingTrade` (再平衡交易)。
- **宏觀驅動**：根據 `MacroRiskAssessment.Level` (Green/Yellow/Orange/Red) 調整行業配置。
    - **Green**: 維持基準配置。
    - **Yellow**: 輕度防御 (+defensive +cash, -ai_supply_chain -semiconductor)。
    - **Orange**: 中度防御 (+10% defensive +8% cash +5% gold, -8% ai_supply_chain -8% semiconductor)。
    - **Red**: 極度風險回避 (+25% cash +15% defensive +10% gold)。
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
- **Kelly Criterion**：`f* = (p*b - q) / b`，套用 `KellyFraction=0.25` 保守係數。
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

## VERIFICATION

```bash
go build ./internal/portfolio/...
go test ./internal/portfolio/...
go vet ./internal/portfolio/...
test -z "$(gofmt -l internal/portfolio/)"
```