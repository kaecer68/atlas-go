# FactorEngine 操作手冊

## 概述

FactorEngine 提供多因子評分計算，支援動能、價值、品質、法人情緒、流動性、敘事、產業週期、連動、台積電、貴金屬、ETF 等因子。

自 PR #691 起，`internal/portfolio/factor_engine.go` 已拆分為 12 個職責分離檔案。`factor_engine.go` 本身只剩 12 行 entry stub，所有實作邏輯位於 `factor_engine_*.go`。

## 檔案結構

| 檔案 | 職責 |
|------|------|
| `factor_engine.go` | 12 行 entry stub，保留 grep 錨點與 API snapshot 相容性 |
| `factor_engine_types.go` | `FactorEngine` struct、provider 型別、`PreciousMetalsContext` |
| `factor_engine_constructors.go` | `NewFactorEngine`、各種 `With*Provider` builder、`SetCycleTracker` |
| `factor_engine_helpers.go` | `isPreciousMetal`、`ensureAdjusted` 等通用輔助函式 |
| `factor_engine_momentum.go` | `CalculateMomentumScore` / `calculateMomentumDetail` |
| `factor_engine_value.go` | `CalculateValueScore` / `calculateValueDetail` |
| `factor_engine_quality.go` | `CalculateQualityScore` / `calculateQualityDetail` |
| `factor_engine_institutional.go` | `CalculateInstitutionalSentimentScore` |
| `factor_engine_liquidity.go` | `CalculateLiquidityScore` |
| `factor_engine_aggregate.go` | `CalculateAllScores` / `CalculateAllScoresWithBreakdown` |
| `factor_engine_etf.go` | `CalculateETFScore` / `RefreshETFNAV` |
| `factor_engine_precious_metals.go` | `CalculatePreciousMetalsScore` |

## 因子類型與預設權重

| 因子 | 基礎權重 | 計算依據 | 實作檔案 |
|------|----------|----------|----------|
| Momentum | 0.25 | 20 日漲跌幅 / 標準差 | `factor_engine_momentum.go` |
| Value | 0.20 | P/E、P/B、P/S（產業相對） | `factor_engine_value.go` |
| Quality | 0.20 | 股息率、價格穩定性 | `factor_engine_quality.go` |
| Agent | 0.15 | 推薦信念加權 | `factor_engine_aggregate.go` |
| InstitutionalSentiment | 0.10 | 外資/法人/融資/散戶情緒 | `factor_engine_institutional.go` |
| Liquidity | 0.05 | Amihud ILLIQ proxy | `factor_engine_liquidity.go` |
| Narrative | 0.05 | 宏觀敘事事件 | `factor_engine_aggregate.go` |
| TSMC | 0.05 | 台積電相關性 | `factor_engine_aggregate.go` |
| IndustryCycle | 0.00（條件式） | 產業週期位置 | `factor_engine_aggregate.go` |
| PreciousMetals | 0.00（條件式） | 黃金/白銀綜合模型 | `factor_engine_precious_metals.go` |
| ETF | 0.00（條件式） | ETF NAV / 資金流 | `factor_engine_etf.go` |
| Linkage | 0.00（條件式） | 跨市場連動 | `factor_engine_aggregate.go` |

權重由 `FactorWeightEngine`（`factor_weight_engine.go`）根據 narrative 事件動態調整；條件式因子預設權重為 0，僅在對應 provider 注入或事件觸發時生效。

## 使用方法

### 初始化

```go
engine := portfolio.NewFactorEngine()
engine.WithHistoricalPrices(historicalPrices)
engine.WithFundamentalProvider(fundamentalProvider)
engine.WithETFAnalyzer(etfAnalyzer)
engine.WithNarrativeProvider(narrativeProv)
engine.WithIndustryCycleProvider(cycleProv)
engine.WithLinkageProvider(linkageProv)
engine.WithTSMCProvider(tsmcProv)
engine.WithPreciousMetalsProvider(pmCtxProv)
engine.WithCorporateActionProvider(corpActionProvider)
```

### 計算單一因子

```go
momentumScore := engine.CalculateMomentumScore("2330", quotes)
valueScore := engine.CalculateValueScore("2330", quotes)
qualityScore := engine.CalculateQualityScore("2330", quotes)
instScore := engine.CalculateInstitutionalSentimentScore(bridgeInput)
liqScore := engine.CalculateLiquidityScore("2330", quotes)
```

### 計算所有因子

```go
scores := engine.CalculateAllScores(
    "2330",
    quotes,
    agentRecs,
    agentWeights,
    factorWeights,
    bridgeInput,
)
// 返回 map[FactorType]float64，若提供 factorWeights 會額外包含 "total" key
```

## 透明度

FactorEngine 提供詳細的計算過程：

```go
breakdown, scores := engine.CalculateAllScoresWithBreakdown(
    "2330", quotes, agentRecs, agentWeights, factorWeights, bridgeInput,
)
// breakdown 包含每個因子的 Score、Weight、Formula、RawInputs、IsFallback
```

`CalculateAllScoresWithBreakdown` 位於 `factor_engine_aggregate.go`。

## 注意事項

1. **缺失報價**會觸發 fallback，分數會降低。
2. **fallback 因子**會自動降低權重。
3. 總分範圍：0-100（`CalculateAllScores` 的 `total` 為加權平均後轉換）。
4. 修改或新增 `FactorType` 時，必須同步 8 個位置（詳見 `internal/portfolio/AGENTS.md` §12）。
5. 動態權重與事件調整請參考 `factor_weight_engine.go` 與 `.claude/skills/atlas-event-driven-weights/SKILL.md`。

## 測試

```bash
go test ./internal/portfolio/... -run TestFactorEngine -v
```

完整因子完整性驗證：

```bash
bash scripts/ci/verify_factor_integrity.sh
```
