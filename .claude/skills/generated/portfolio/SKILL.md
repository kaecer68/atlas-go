---
name: portfolio
description: "Skill for the Portfolio area of atlas. 146 symbols across 23 files."
---

# Portfolio

146 symbols | 23 files | Cohesion: 85%

## When to Use

- Working with code in `internal/`
- Understanding how NewEngine, TestScreenPassesWithNilCriteria, TestScreenFiltersByPE work
- Modifying portfolio-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/screener/engine_test.go` | ptrFloat64, ptrInt64, loadTestFundamentals, TestScreenPassesWithNilCriteria, TestScreenFiltersByPE (+13) |
| `internal/portfolio/factor_engine_test.go` | TestFactorEngineCalculateMomentumScore, TestFactorEngineCalculateValueScore, TestFactorEngineCalculateValueScoreWithFundamentals, TestFactorEngineCalculateQualityScore, TestFactorEngineCalculateQualityScoreWithDividendYield (+11) |
| `internal/portfolio/capital_allocator_test.go` | TestNewCapitalAllocator, TestAllocate_EmptyRecommendations, TestAllocate_PhaseLimit, TestAllocate_EqualDistribution, TestAllocate_ConvictionWeighted (+7) |
| `internal/portfolio/factor_engine.go` | NewFactorEngine, WithHistoricalPrices, WithFundamentalProvider, CalculateMomentumScore, calculateMomentumDetail (+6) |
| `internal/portfolio/sector_rotator.go` | NewSectorRotator, NewSectorRotatorWithConfig, GetRebalancingTrades, CanExecuteRotation, absFloat64 (+6) |
| `internal/portfolio/darwinian_weights.go` | rankBySharpe, constrainWeight, GetWeight, GetAllWeights, GetAgentWeightData (+5) |
| `internal/portfolio/sizing.go` | CalculateSize, calculateKellySize, adjustForVolatility, adjustForATR, applyLiquidityLimit (+5) |
| `internal/portfolio/analysis.go` | HoldingPeriod, CalculateMetrics, AttributionByAgent, AttributionBySymbol, CalculateAgentStats (+4) |
| `internal/portfolio/agent_weights.go` | UpdateWeights, calculateScore, constrainWeight, renormalizeWeights, RankAgents (+4) |
| `internal/portfolio/risk_manager.go` | NewRiskManager, SetRiskParameters, UpdatePortfolioValue, AddPosition, UpdatePosition (+1) |

## Entry Points

Start here when exploring this area:

- **`NewEngine`** (Function) — `internal/screener/screener.go:24`
- **`TestScreenPassesWithNilCriteria`** (Function) — `internal/screener/engine_test.go:37`
- **`TestScreenFiltersByPE`** (Function) — `internal/screener/engine_test.go:49`
- **`TestScreenFiltersByPB`** (Function) — `internal/screener/engine_test.go:72`
- **`TestScreenFiltersByDividendYield`** (Function) — `internal/screener/engine_test.go:95`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewEngine` | Function | `internal/screener/screener.go` | 24 |
| `TestScreenPassesWithNilCriteria` | Function | `internal/screener/engine_test.go` | 37 |
| `TestScreenFiltersByPE` | Function | `internal/screener/engine_test.go` | 49 |
| `TestScreenFiltersByPB` | Function | `internal/screener/engine_test.go` | 72 |
| `TestScreenFiltersByDividendYield` | Function | `internal/screener/engine_test.go` | 95 |
| `TestScreenFiltersByVolume` | Function | `internal/screener/engine_test.go` | 118 |
| `TestScreenFiltersByMomentum` | Function | `internal/screener/engine_test.go` | 139 |
| `TestScreenFiltersByMinTotalFactorScore` | Function | `internal/screener/engine_test.go` | 161 |
| `TestScreenUniverseFiltersList` | Function | `internal/screener/engine_test.go` | 180 |
| `TestScreenMissingFundamentalRejectsWhenFilterRequired` | Function | `internal/screener/engine_test.go` | 203 |
| `TestScreenDetailedVolumeFail` | Function | `internal/screener/engine_test.go` | 219 |
| `TestScreenDetailedPEMinFail` | Function | `internal/screener/engine_test.go` | 254 |
| `TestScreenDetailedPEMaxFail` | Function | `internal/screener/engine_test.go` | 284 |
| `TestScreenDetailedPBMissingFail` | Function | `internal/screener/engine_test.go` | 314 |
| `TestScreenDetailedMomentum20DMaxFail` | Function | `internal/screener/engine_test.go` | 344 |
| `TestScreenDetailedMinTotalFactorScoreMissing` | Function | `internal/screener/engine_test.go` | 374 |
| `NewFundamentalProvider` | Function | `internal/portfolio/fundamental_loader.go` | 38 |
| `TestFactorEngineCalculateMomentumScore` | Function | `internal/portfolio/factor_engine_test.go` | 8 |
| `TestFactorEngineCalculateValueScore` | Function | `internal/portfolio/factor_engine_test.go` | 32 |
| `TestFactorEngineCalculateValueScoreWithFundamentals` | Function | `internal/portfolio/factor_engine_test.go` | 42 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → ConstrainWeight` | cross_community | 5 |
| `NewStyleRotationStrategy → StyleAllocation` | intra_community | 4 |
| `NewIntegratedAllocator → StyleAllocation` | intra_community | 4 |
| `NewIntegratedAllocator → RegimeThresholds` | intra_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 14 calls |
| Narrative | 5 calls |
| Swarm | 3 calls |
| Risk | 2 calls |
| Config | 1 calls |

## How to Explore

1. `gitnexus_context({name: "NewEngine"})` — see callers and callees
2. `gitnexus_query({query: "portfolio"})` — find related execution flows
3. Read key files listed above for implementation details
