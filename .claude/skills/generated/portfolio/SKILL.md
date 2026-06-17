---
name: portfolio
description: "Skill for the Portfolio area of atlas. 234 symbols across 39 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Portfolio

234 symbols | 39 files | Cohesion: 84%

## When to Use

- Working with code in `internal/`
- Understanding how NewEngine, TestScreenPassesWithNilCriteria, TestScreenFiltersByPE work
- Modifying portfolio-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/portfolio/darwinian_weights.go` | NewDarwinianWeightManager, InitializeFromRegistry, RecordOutcome, updateRollingMetrics, calculateSharpe (+17) |
| `internal/screener/engine_test.go` | ptrFloat64, ptrInt64, loadTestFundamentals, TestScreenPassesWithNilCriteria, TestScreenFiltersByPE (+13) |
| `internal/portfolio/factor_engine_test.go` | TestFactorEngineCalculateMomentumScore, TestFactorEngineCalculateValueScore, TestFactorEngineCalculateValueScoreWithFundamentals, TestFactorEngineCalculateQualityScore, TestFactorEngineCalculateQualityScoreWithDividendYield (+11) |
| `internal/portfolio/agent_health_test.go` | TestRecordOutcomeUpdatesStreaks, TestMuteAfterConsecutiveLosses, TestMuteAfterNegativeSharpe, TestAutoUnmuteAfterConsecutiveWins, TestAutoUnmuteAfterTimeBasedRecovery (+7) |
| `internal/portfolio/capital_allocator_test.go` | TestNewCapitalAllocator, TestAllocate_EmptyRecommendations, TestAllocate_PhaseLimit, TestAllocate_EqualDistribution, TestAllocate_ConvictionWeighted (+7) |
| `internal/portfolio/factor_engine.go` | NewFactorEngine, WithHistoricalPrices, WithFundamentalProvider, CalculateMomentumScore, calculateMomentumDetail (+6) |
| `internal/portfolio/sector_rotator.go` | NewSectorRotator, NewSectorRotatorWithConfig, GetRebalancingTrades, CanExecuteRotation, absFloat64 (+6) |
| `internal/portfolio/agent_health.go` | DefaultAgentHealthConfig, NewAgentHealthManager, NewAgentHealthManagerWithConfig, NewAgentHealthManagerWithStore, GetHealth (+5) |
| `internal/portfolio/sizing.go` | CalculateSize, calculateKellySize, adjustForVolatility, adjustForATR, applyLiquidityLimit (+5) |
| `internal/portfolio/analysis.go` | HoldingPeriod, CalculateMetrics, AttributionByAgent, AttributionBySymbol, CalculateAgentStats (+4) |

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
| `RunDailySimulation → StaticLoader` | cross_community | 5 |
| `RunDailySimulation → LoadRegimeExecutors` | cross_community | 5 |
| `RunDailySimulation → LoadAgentExecutors` | cross_community | 5 |
| `RunDailySimulation → LoadControlExecutors` | cross_community | 5 |
| `RunDailySimulation → IsAgentHealthy` | cross_community | 5 |
| `RunDailySimulation → Info` | cross_community | 5 |
| `RunDailySimulation → AgentID` | cross_community | 5 |
| `Main → DarwinianWeightManager` | cross_community | 5 |
| `Main → DarwinianAgentWeight` | cross_community | 5 |
| `RunEnhancedExperiment → CalculateSharpe` | cross_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 39 calls |
| Risk | 11 calls |
| Industry | 8 calls |
| Ledger | 5 calls |
| Eventbus | 5 calls |
| Experiment | 4 calls |
| Sim | 3 calls |
| Narrative | 3 calls |

## How to Explore

1. `gitnexus_context({name: "NewEngine"})` — see callers and callees
2. `gitnexus_query({query: "portfolio"})` — find related execution flows
3. Read key files listed above for implementation details
