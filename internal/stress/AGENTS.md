# AGENTS.md — stress（壓力測試）

> 合併 `stress` 模組的陷阱與 API 文件。

---

## 模組用途

提供**單一場景壓力測試 API**，對給定投資組合快照施加「歷史重現式」市場 Shock，計算模擬期間的每日投資組合價值路徑 `V(t)`，產出 MDD、VaR、Sortino、Recovery Days 等風險指標。

用於 **orchestrator SystemCore 的 live risk evaluation**（ runtime 即時評估，非批次）。

---

## 公開 API

### 核心型別

```go
// Scenario：單一歷史壓力場景（不可變）
type Scenario struct {
    ID, Name, Description string
    Date        time.Time
    Quotes      []domain.Quote   // VIX / DXY / US10Y / OIL
    Regime      domain.Regime    // 場景觸發後的 Regime
    WindowDays  int              // 模擬天數（場景日期後）
}

// ScenarioResult：單一場景模擬結果
type ScenarioResult struct {
    ScenarioID             string
    ScenarioName           string
    TotalReturn            float64
    MaxDrawdown            float64  // 最大回落（正值）
    SharpeRatio            float64
    SortinoRatio           float64
    VaR95                  float64
    TradeCount             int
    FinalRegime            domain.Regime
    MomentumDisabled       bool
    RecoveryDays           int      // −1 = 未恢復
    MaxConsecutiveLossDays int
    DailyValues            []float64 // V(0)=1.0 正規化
}

// Report：多場景彙總報告
type Report struct {
    ScenarioResults []ScenarioResult
    BaselineResult  *ScenarioResult
    WorstDrawdown   float64
    WorstVaR        float64
    AvgReturn       float64
}

// Runner：執行器
type Runner struct {
    registry    domain.AgentRegistry
    policy      domain.ExecutionPolicy
    covMatrix   [][]float64        // 共變異數矩陣（可選）
    covSymbols  []string
    portWeights map[string]float64 // 可選
}
```

### 主要函式

| 函式 | 說明 |
|------|------|
| `NewRunner(registry, policy)` | 建立 Runner |
| `(*Runner).RunScenario(scenario, quotes, recs)` | 單一場景模擬，核心 API |
| `(*Runner).RunAll(quotes, recs)` | 對所有內建場景執行模擬 |
| `(*Runner).SetCovariance(matrix, symbols)` | 設定共變異數矩陣（增強相關性模型） |
| `(*Runner).SetPortfolioWeights(weights)` | 設定個股权重（覆蓋 Runner 預設） |
| `FormatReport(Report)` | 格式化報告字串（除錯用） |
| `AllScenarios()` | 回傳所有 10 個內建場景 |
| `GetScenarioByID(id)` | 以 ID 查詢場景，找不到回 error |
| `(Scenario).MergeQuotes(stockQuotes)` | 合併宏觀報價與個股報價 |
| `(Scenario).VIXLevel()` | 從 Quotes 取出 VIX 值 |

### 內建場景（10 個）

| ID | 名稱 | Regime | WindowDays |
|----|------|--------|-----------|
| `covid_crash_2020` | COVID-19 Market Crash | RiskOff | 40 |
| `fed_hikes_2022` | Fed Aggressive Rate Hikes | RiskOff | 30 |
| `ai_bubble_2024` | AI Semiconductor Bubble | RiskOn | 20 |
| `taiwan_tension_2022` | Taiwan Geopolitical Tension | RiskOff | 15 |
| `normal_market_2024` | Normal Market Conditions | Neutral | 20 |
| `stagflation_2023` | Stagflation | RiskOff | 20 |
| `em_contagion_2018` | 新興市場傳染風險 2018 | RiskOff | 20 |
| `liquidity_crunch_2008` | 金融海嘯流動性緊縮 2008 | RiskOff | 20 |
| `hualien_earthquake_2024` | Hualien Earthquake 2024 | RiskOff | 10 |
| `china_lockdown_2022` | China Lockdown 2022 | RiskOff | 90 |
| `level3_alert_2021` | Taiwan Level 3 Alert 2021 | RiskOff | 90 |

---

## 關鍵陷阱

| 陷阱 | 說明 |
|------|------|
| **`stress` vs `risktest` 差異** | `stress` 是 **runtime 即時單場景 API**（由 orchestrator SystemCore 的 live risk evaluation 呼叫）。`risktest` 是 **批次多場景 CLI**（`cmd/stress-test`），兩者不可互換。 |
| **Shock decay 固定** | decay factor 使用 `λ=0.15/day`（`e^(−0.15·t)`），意即 10 天後仍有 20% shock，20 天後 5%。若需調整需同步改 `decayFactor()` 與參數文件。 |
| **VIX 作為宏觀 Shock 代理** | 場景宏觀條件以 VIX / DXY / US10Y / OIL 四個 Symbol 表徵；真實交易所時間序列另有 `domain.Quote` feed，不應與 Scenario Quotes 混淆。 |
| **DailyValues 為正規化值** | `DailyValues[0] ≡ 1.0`（起始值 1.0），非貨幣金額；最終貨幣價值需乘上初始組合價值。 |
| **RecoveryDays = −1 表示未恢復** | 在 `WindowDays` 內未見新高則回 −1；可用於判定「多久內需干預」。 |
| **Covariance Matrix 需對稱正定** | `SetCovariance` 傳入的矩陣若非正定，Cholesky 分解會 panic；建議在設定前做特徵值檢查或用 `runScenarioCov` 的降級處理。 |
| **Momentum 紀律** | 當 `Scenario.Regime == RegimeRiskOff` 時，`RunScenario` 會停用 momentum 策略（`MomentumDisabled=true`），視為逃生模式。 |
| **內建場景 Quotes 為固定值** | 每個 Scenario 的 `Quotes` 為建構期常數，非動態市場報價；真實宏觀資料需透過 `MergeQuotes` 或外部 feed 注入。 |
| **Portfolio Weights 需先設定** | 若未呼叫 `SetPortfolioWeights`，Runner 內部無個股权重，模擬路徑可能不準確（取決於 `recs` 內含的持倉）。 |

---

## 依賴關係

```
stress
├── domain          — AgentRegistry, ExecutionPolicy, Regime, Quote, Recommendation
├── domain/shared   — 共享型別
└── portfolio       — 組合相關型別
```

**被依賴於**：
- `orchestrator/SystemCore` — live risk evaluation 即時呼叫 `RunScenario`
- `cmd/stress-test` — 參考 `risktest` 批次執行（注意區分）

---

## 測試

- `TestAllScenariosCount` — 內建場景數量為 10
- `TestGetScenarioByID` / `TestGetScenarioByIDNotFound`
- `TestScenarioMergeQuotes`
- `TestRunnerRunScenario` / `TestRunnerRunScenarioMomentumDisabled`
- `TestRunnerRunAll`
- `TestFormatReport`
- `TestMultiDayStress_*`（COVID / FedHiking / AllScenarios）
- `TestCholeskyDecompose`
- `TestCovarianceDrivenScenarios`
- `TestStressSortinoComputed`
