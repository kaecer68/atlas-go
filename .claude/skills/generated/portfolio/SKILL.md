---
name: portfolio
description: "Skill for the Portfolio area of atlas. 260 symbols across 50 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Portfolio

260 symbols | 50 files | Cohesion: 84%

## When to Use

- Working with code in `internal/portfolio/`
- Understanding how the multi-factor engine (`FactorEngine`) is split across `factor_engine_*.go` files
- Modifying portfolio-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/portfolio/darwinian_weights.go` | NewDarwinianWeightManager, InitializeFromRegistry, RecordOutcome, updateRollingMetrics, calculateSharpe (+17) |
| `internal/screener/engine_test.go` | ptrFloat64, ptrInt64, loadTestFundamentals, TestScreenPassesWithNilCriteria, TestScreenFiltersByPE (+13) |
| `internal/portfolio/factor_engine_test.go` | TestFactorEngineCalculateMomentumScore, TestFactorEngineCalculateValueScore, TestFactorEngineCalculateValueScoreWithFundamentals, TestFactorEngineCalculateQualityScore, TestFactorEngineCalculateQualityScoreWithDividendYield (+25) |
| `internal/portfolio/factor_engine_constructors.go` | NewFactorEngine, WithHistoricalPrices, WithFundamentalProvider, WithParameters, WithNarrativeProvider, WithIndustryCycleProvider, WithLinkageProvider, WithTSMCProvider, WithCorporateActionProvider, SetCycleTracker, SetCycleCardBuilder, WithPreciousMetalsProvider, WithETFAnalyzer, GetETFAnalyzer |
| `internal/portfolio/factor_engine_types.go` | FactorEngine, PreciousMetalsContext, PMContextProvider, NarrativeProviderFunc, IndustryCycleProviderFunc, LinkageProviderFunc, TSMCProviderFunc, QuoteProvider, CorporateActionProvider |
| `internal/portfolio/factor_engine_helpers.go` | isFinite, isPreciousMetal, IsPreciousMetal, ensureAdjusted, classifyIndustry |
| `internal/portfolio/factor_engine_aggregate.go` | CalculateAllScores, CalculateAllScoresWithBreakdown |
| `internal/portfolio/factor_engine_momentum.go` | CalculateMomentumScore, calculateMomentumDetail |
| `internal/portfolio/factor_engine_value.go` | CalculateValueScore, calculateValueDetail |
| `internal/portfolio/agent_health_test.go` | TestRecordOutcomeUpdatesStreaks, TestMuteAfterConsecutiveLosses, TestMuteAfterNegativeSharpe, TestAutoUnmuteAfterConsecutiveWins, TestAutoUnmuteAfterTimeBasedRecovery (+7) |

## Factor Engine Split

The monolithic `factor_engine.go` has been split into focused files:

| File | Symbols |
|------|---------|
| `internal/portfolio/factor_engine.go` | package documentation stub |
| `internal/portfolio/factor_engine_constructors.go` | NewFactorEngine and all `With*` / `Set*` builder methods |
| `internal/portfolio/factor_engine_types.go` | FactorEngine struct and provider type definitions |
| `internal/portfolio/factor_engine_helpers.go` | isFinite, isPreciousMetal, ensureAdjusted, classifyIndustry |
| `internal/portfolio/factor_engine_aggregate.go` | CalculateAllScores, CalculateAllScoresWithBreakdown |
| `internal/portfolio/factor_engine_momentum.go` | CalculateMomentumScore, calculateMomentumDetail |
| `internal/portfolio/factor_engine_value.go` | CalculateValueScore, calculateValueDetail |
| `internal/portfolio/factor_engine_quality.go` | CalculateQualityScore, calculateQualityDetail |
| `internal/portfolio/factor_engine_institutional.go` | CalculateInstitutionalSentimentScore |
| `internal/portfolio/factor_engine_liquidity.go` | CalculateLiquidityScore |
| `internal/portfolio/factor_engine_precious_metals.go` | CalculatePreciousMetalsScore and PM sub-factor helpers |
| `internal/portfolio/factor_engine_etf.go` | RefreshETFNAV, CalculateETFScore |
| `internal/portfolio/factor_engine_test.go` | Factor-engine unit and integration tests |

## Entry Points

Start here when exploring this area:

- **`NewEngine`** (Function) — `internal/screener/screener.go:24`
- **`NewFactorEngine`** (Function) — `internal/portfolio/factor_engine_constructors.go:12`
- **`TestScreenPassesWithNilCriteria`** (Function) — `internal/screener/engine_test.go:37`
- **`TestScreenFiltersByPE`** (Function) — `internal/screener/engine_test.go:49`
- **`TestFactorEngineCalculateMomentumScore`** (Function) — `internal/portfolio/factor_engine_test.go:8`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewEngine` | Function | `internal/screener/screener.go` | 24 |
| `NewFactorEngine` | Function | `internal/portfolio/factor_engine_constructors.go` | 12 |
| `FactorEngine` | Struct | `internal/portfolio/factor_engine_types.go` | 32 |
| `TestScreenPassesWithNilCriteria` | Function | `internal/screener/engine_test.go` | 37 |
| `TestScreenFiltersByPE` | Function | `internal/screener/engine_test.go` | 49 |
| `TestScreenFiltersByPB` | Function | `internal/screener/engine_test.go` | 72 |
| `TestScreenFiltersByDividendYield` | Function | `internal/screener/engine_test.go` | 95 |
| `TestFactorEngineCalculateMomentumScore` | Function | `internal/portfolio/factor_engine_test.go` | 8 |
| `TestFactorEngineCalculateValueScore` | Function | `internal/portfolio/factor_engine_test.go` | 32 |
| `TestFactorEngineCalculateQualityScore` | Function | `internal/portfolio/factor_engine_test.go` | 50 |
| `CalculateAllScores` | Method | `internal/portfolio/factor_engine_aggregate.go` | 25 |
| `CalculateAllScoresWithBreakdown` | Method | `internal/portfolio/factor_engine_aggregate.go` | 108 |
| `CalculateMomentumScore` | Method | `internal/portfolio/factor_engine_momentum.go` | 10 |
| `CalculateValueScore` | Method | `internal/portfolio/factor_engine_value.go` | 10 |
| `CalculateQualityScore` | Method | `internal/portfolio/factor_engine_quality.go` | 10 |
| `CalculateInstitutionalSentimentScore` | Method | `internal/portfolio/factor_engine_institutional.go` | 10 |
| `CalculateLiquidityScore` | Method | `internal/portfolio/factor_engine_liquidity.go` | 10 |
| `CalculatePreciousMetalsScore` | Method | `internal/portfolio/factor_engine_precious_metals.go` | 10 |
| `CalculateETFScore` | Method | `internal/portfolio/factor_engine_etf.go` | 19 |
| `RefreshETFNAV` | Method | `internal/portfolio/factor_engine_etf.go` | 10 |
| `WithHistoricalPrices` | Method | `internal/portfolio/factor_engine_constructors.go` | 19 |
| `WithFundamentalProvider` | Method | `internal/portfolio/factor_engine_constructors.go` | 27 |
| `WithCorporateActionProvider` | Method | `internal/portfolio/factor_engine_constructors.go` | 77 |

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

1. `gitnexus_context({name: "NewFactorEngine"})` — see callers and callees
2. `gitnexus_query({query: "portfolio"})` — find related execution flows
3. Read key files listed above for implementation details
