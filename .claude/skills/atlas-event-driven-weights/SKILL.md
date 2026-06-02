# Skill: atlas-event-driven-weights

> **實作狀態**：⚠️ 部分實作 — 核心元件已實作，部分功能已整合至現有模組  
> **最後審計**：2026-06-02  
> **實際檔案結構**：`factor_bridge.go`（已實作）、`factor_weight_engine.go`（已實作，8 因子），其餘因子計算已內建於 `factor_engine.go`

## 描述

**事件驅動動態因子權重系統** — 將巨集觀敘事事件（narrative）與因子計算（portfolio）連接，實現真正的數據交叉點。

## 任務觸發

當 AI 代理需要：
- 實作或修改因子權重邏輯
- 新增/修改因子類型
- 將監測數據（monitoring）轉換為因子輸入
- 實作 RegimeChange 機制

## 核心概念

### 1. FactorBridge（宏觀數據橋接器）

將 MacroDataSnapshot 轉換為因子計算輸入：

```
MacroDataSnapshot (monitoring)
         ↓
FactorBridge
         ↓
ForeignFlowScore, MarginBalanceScore, RetailSentimentScore, StressLevel
         ↓
FactorEngine (8因子)
```

**輸入來源**：
- TWSEBalanceProvider → 散戶資金流向
- TWSECapitalFlowProvider → 外資/法人數據
- TaiwanStressIndex → 市場壓力指數

### 2. 八因子系統（已擴充為 8 因子）

| 因子 | 基礎權重 | 說明 | 狀態 |
|------|----------|------|------|
| Momentum | 0.25 | 動能因子 | ✅ 已實作（factor_engine.go） |
| Value | 0.20 | 價值因子 | ✅ 已實作（factor_engine.go） |
| Quality | 0.20 | 品質因子 | ✅ 已實作（factor_engine.go） |
| Agent | 0.15 | 代理人因子 | ✅ 已實作（factor_engine.go） |
| InstitutionalSentiment | 0.10 | 機構情緒因子 | ✅ 已實作（factor_engine.go `CalculateInstitutionalSentimentScore()`） |
| Liquidity | 0.05 | 流動性因子 | ✅ 已實作（factor_engine.go `CalculateLiquidityScore()`） |
| Narrative | 0.05 | 敘事因子 | ✅ 已實作 |
| IndustryCycle | 0.00 | 產業週期因子 | ✅ 已實作 |

### 3. InstitutionalSentiment 因子

計算方式：
```
InstitutionalSentiment = 0.50 × ForeignFlowScore
                        + 0.30 × DomesticFlowScore
                        + 0.20 × MarginBalanceScore
```
已實作於 `factor_engine.go` 的 `CalculateInstitutionalSentimentScore()` 方法。

### 4. Liquidity 因子

計算方式（Amihud ILLIQ proxy）：
```
Liquidity = -log( |Return| / Volume )  // 標準化後
```
已實作於 `factor_engine.go` 的 `CalculateLiquidityScore()` 方法。

### 5. FactorWeightEngine（動態權重引擎）

根據事件動態調整因子權重：

```go
type FactorWeightEngine struct {
    mu                 sync.RWMutex
    baseWeights        map[FactorType]float64
    eventWeights       map[string]map[FactorType]float64
    activeEvents       map[string]*narrative.NarrativeEvent
    lifecycle          *narrative.EventLifecycleManager
    strategyAdjustment map[FactorType]float64
    weightSource       string
    currentRegime      string
}
```

**事件驅動調整**：
- `AI_capex_surge`：提升 Quality + Momentum
- `US_rates_up`：提升 Value，降低 InstitutionalSentiment
- `oil_price_shock`：降低 Liquidity + Momentum
- Severity 對應 delta：`critical=0.10`, `high=0.05`, `medium=0.02`, `low=0.01`

### 6. RegimeChange 機制

| Regime | 判斷條件 |
|--------|----------|
| Bull | VIX < 15, TrendUp |
| Bear | VIX > 25, TrendDown |
| Neutral | VIX 15-25 |
| HighVol | VIX > 30 |

**觸發時**：
1. 記錄 `PreviousRegime` / `CurrentRegime`
2. 計算 Regime 持續時間
3. 觸發 FactorWeightEngine 重新計算權重
4. 發送 `RegimeChangedEvent` 到因果鏈

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| FactorBridge | `internal/portfolio/factor_bridge.go` | ✅ 已實作（含 RSI-tw 計算器整合） |
| InstitutionalSentiment | `internal/portfolio/factor_institutional_sentiment.go` | ⚠️ 未建立獨立檔案 — 功能已內建於 `factor_engine.go` |
| Liquidity | `internal/portfolio/factor_liquidity.go` | ⚠️ 未建立獨立檔案 — 功能已內建於 `factor_engine.go` |
| FactorWeightEngine | `internal/portfolio/factor_weight_engine.go` | ✅ 已實作（8 因子，配置化，含策略調整） |
| RegimeChange | `internal/portfolio/regime_change.go` | ⚠️ 未建立獨立檔案 — 功能由 `factor_weight_engine.go` + `regime.go` 覆蓋 |
| Regime/Style 定義 | `internal/portfolio/regime.go` | ✅ 已實作（RegimeConfig、StyleAllocation） |

## 擴展現有程式碼

### 已實作：factor_engine.go

1. ✅ `CalculateInstitutionalSentiment()` — 已整合於 `factor_engine.go`
2. ✅ `CalculateLiquidity()` — 已整合於 `factor_engine.go`
3. ✅ `CalculateAllScoresWithBreakdown()` — 已擴展支援 8 因子

## 驗證要求

```bash
go test ./internal/portfolio/...      # 因子計算測試
go test ./internal/narrative/...      # 事件系統測試
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

## 與其他技能整合

- `atlas-dynamic-correlation`：動態閾值依賴 Regime 判斷
- `atlas-multi-strategy`：多策略框架需要動態因子權重
- `atlas-news-sentiment`：新聞情緒作為因子輸入之一

## 數據來源

- TWSE 開放資料 API：`https://openapi.twse.com.tw/v1/`
- TWSE Data E-Shop：15+ 年歷史數據
- EOD Historical Data API：支援台股（1314 檔標的）

## 設計原則

1. **權重不應固定**：根據歷史數據統計出的規律，隨事件動態調整
2. **向後相容**：`executeOrder` / `ExecuteOrder` 兩個方法都需要保留
3. **不使用全域可變狀態**：執行期協調使用 context
4. **不過度擬合**：限制最小調整幅度，建立回測驗證框架
