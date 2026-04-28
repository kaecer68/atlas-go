# Skill: atlas-event-driven-weights

## 描述

**事件驅動動態因子權重系統** - 將巨集觀敘事事件（narrative）與因子計算（portfolio）連接，實現真正的數據交叉點。

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
FactorEngine (6因子)
```

**輸入來源**：
- TWSEBalanceProvider → 散戶資金流向
- TWSECapitalFlowProvider → 外資/法人數據
- TaiwanStressIndex → 市場壓力指數

### 2. 六因子系統

| 因子 | 基礎權重 | 說明 |
|------|----------|------|
| Momentum | 0.20 | 動能因子 |
| Value | 0.15 | 價值因子 |
| Quality | 0.15 | 品質因子 |
| Agent | 0.20 | 代理人因子 |
| InstitutionalSentiment | 0.15 | 機構情緒因子 |
| Liquidity | 0.15 | 流動性因子 |

### 3. InstitutionalSentiment 因子

計算方式：
```
InstitutionalSentiment = 0.50 × ForeignFlowScore
                        + 0.30 × DomesticFlowScore
                        + 0.20 × MarginBalanceScore
```

### 4. Liquidity 因子

計算方式（Amihud ILLIQ proxy）：
```
Liquidity = -log( |Return| / Volume )  // 標準化後
```

### 5. FactorWeightEngine（動態權重引擎）

根據事件動態調整因子權重：

```go
type FactorWeightConfig struct {
    BaseWeights      map[string]float64  // 基礎權重
    EventAdjustments map[string]EventWeightAdjustment
}

type EventWeightAdjustment struct {
    ThemePattern string
    WeightDelta  map[string]float64  // 因子 → 權重變化
    RegimeFilter string             // 僅在特定 regime 生效
}
```

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
| FactorBridge | `internal/portfolio/factor_bridge.go` | 待建立 |
| InstitutionalSentiment | `internal/portfolio/factor_institutional_sentiment.go` | 待建立 |
| Liquidity | `internal/portfolio/factor_liquidity.go` | 待建立 |
| FactorWeightEngine | `internal/portfolio/factor_weight_engine.go` | 待建立 |
| RegimeChange | `internal/portfolio/regime_change.go` | 待建立 |

## 擴展現有程式碼

### 修改 factor_engine.go

1. 新增 `CalculateInstitutionalSentiment()` 方法
2. 新增 `CalculateLiquidity()` 方法
3. 擴展 `CalculateAllScoresWithBreakdown()` 支援 6 因子

### 修改 NarrativeEvent (internal/narrative/types.go)

新增欄位：
- `Duration time.Duration`
- `ExpiresAt *time.Time`
- `Severity string`
- `Status string`

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
