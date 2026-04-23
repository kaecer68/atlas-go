# FactorEngine 操作手冊

## 概述

FactorEngine 提供多因子評分計算，支援動能、價值、品質、Agent 四個維度。

## 因子類型

| 因子 | 權重預設 | 計算依據 |
|------|----------|----------|
| Momentum | 30% | 20 日漲跌幅 |
| Value | 25% | P/E、P/B、股息率 |
| Quality | 25% | 財務指標 |
| Agent | 20% | 推薦信念 |

## 使用方法

### 初始化

```go
engine := portfolio.NewFactorEngine()
engine.WithHistoricalPrices(historicalPrices)
engine.WithFundamentalProvider(fundamentalProvider)
```

### 計算單一因子

```go
momentumScore := engine.CalculateMomentumScore("2330", quotes)
valueScore := engine.CalculateValueScore("2330", quotes)
qualityScore := engine.CalculateQualityScore("2330", quotes)
```

### 計算所有因子

```go
scores := engine.CalculateAllScores("2330", quotes, agentRecs, agentWeights, factorWeights)
// 返回: map[FactorType]float64{"momentum": 75, "value": 60, "quality": 80, "agent": 70, "total": 71.5}
```

## 透明度

FactorEngine 提供詳細的計算過程：

```go
breakdown, scores := engine.CalculateAllScoresWithBreakdown("2330", quotes, ...)
// breakdown 包含每個因子的: Score, Weight, Formula, RawInputs, IsFallback
```

## 注意事項

1. **缺失報價**會觸發 fallback，分數會降低
2. **fallback 因子**會自動降低權重
3. 總分範圍: 0-100

## 測試

```bash
go test ./internal/portfolio/... -run TestFactorEngine -v
```
