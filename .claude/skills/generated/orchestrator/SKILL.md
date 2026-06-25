---
name: orchestrator
description: "Skill for the Orchestrator area of atlas. 350 symbols across 78 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Orchestrator

350 symbols | 78 files | Cohesion: 76%

## When to Use

- Working with code in `internal/orchestrator/`
- Understanding how executor logic is split across `executor_*.go` files
- Modifying orchestrator-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/spawning/spawning_manager.go` | DefaultSpawningConfig, NewSpawningManager, Start, Stop, runLoop (+15) |
| `internal/swarm/mirofish_swarm.go` | DefaultSwarmConfig, NewMiroFishSwarm, UpdateScenario, InitializeScenarios, Start (+14) |
| `internal/orchestrator/system.go` | NewSystem, newSession, WithCapitalManagement, checkCapitalPhase, updateCapitalMetrics (+13) |
| `internal/orchestrator/executor_pipeline.go` | ExecuteWithContext, ExecuteRegistryResearch, ExecuteRegistryResearchDetailed, ExecuteRegistryResearchDetailedWithPolicy, ExecuteRegistryResearchDetailedWithPolicyAndGuards, ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins |
| `internal/orchestrator/executor_strategies.go` | RegimeInferenceStrategy, DefaultRegimeInferenceStrategy, RecommendationCollectionStrategy, DefaultRecommendationCollectionStrategy, MomentumCrashProtectionStrategy, DefaultMomentumCrashProtectionStrategy, ControlLayerStrategy, DefaultControlLayerStrategy, WeightApplicationStrategy, DefaultWeightApplicationStrategy, MutedAgentFilterStrategy, DefaultMutedAgentFilterStrategy |
| `internal/orchestrator/executor_types.go` | LayerRouter, DefaultLayerRouter, ExecutionContext, ResearchResult, FilterAgentsByLayer |
| `internal/orchestrator/forward_return_fallback_test.go` | TestGenerateForwardReturn_ValidQuotePositiveIntraday, TestGenerateForwardReturn_FlatDayUsesDistributionFallback, TestGenerateForwardReturn_RiskOnVsRiskOffDiffers, TestGenerateForwardReturn_QuoteWithNoOpenFallsBack, TestGenerateForwardReturn_ClipToMin (+9) |
| `internal/reflexivity/reflexivity_engine.go` | NewReflexivityEngine, RegisterBias, validateBias, mergeBiases, UpdateReality (+8) |
| `internal/orchestrator/phase3_controller.go` | NewPhase3Controller, StartBackgroundSwarm, StopBackgroundSwarm, UpdateSwarmState, swarmUpdateLoop (+8) |
| `internal/prism/prism_manager.go` | DefaultPRISMConfig, NewPRISMManager, Start, Stop, ScheduleTraining (+7) |

## Executor Split

The monolithic `executors.go` has been split into focused executor files:

| File | Symbols |
|------|---------|
| `internal/orchestrator/executors.go` | package documentation stub |
| `internal/orchestrator/executor_muted_filter.go` | filterMutedAgents, loadRecOverrides |
| `internal/orchestrator/executor_collection.go` | collectRecommendations, avgConvictionScore |
| `internal/orchestrator/executor_control.go` | applyControlLayerWithOutcomes, applyCrowdingPenalty, applyAntiCorrelationLayer, severityForControlAgent, passRatio |
| `internal/orchestrator/executor_darwinian.go` | ExecuteRegistryResearchWithDarwinianWeights, executeRegistryResearchWithDarwinianWeights |
| `internal/orchestrator/executor_momentum_crash.go` | applyMomentumCrashProtection |
| `internal/orchestrator/executor_pipeline.go` | ExecuteWithContext, ExecuteRegistryResearch, ExecuteRegistryResearchDetailed, ExecuteRegistryResearchDetailedWithPolicy, ExecuteRegistryResearchDetailedWithPolicyAndGuards, ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins |
| `internal/orchestrator/executor_policy.go` | DefaultExecutionPolicy |
| `internal/orchestrator/executor_regime.go` | inferRegime |
| `internal/orchestrator/executor_strategies.go` | Strategy interfaces and default implementations |
| `internal/orchestrator/executor_symbols.go` | DefaultSymbols, loadSymbolsFromCSV, ExpandUniverse, RegistrySymbols, SymbolsForSkill, symbolIterator |
| `internal/orchestrator/executor_types.go` | LayerRouter, DefaultLayerRouter, ExecutionContext, ResearchResult |
| `internal/orchestrator/executors_test.go` | Executor unit tests |

## Entry Points

Start here when exploring this area:

- **`TestPhase3Integration`** (Function) — `integration_test.go:122`
- **`ExecuteWithContext`** (Function) — `internal/orchestrator/executor_pipeline.go:10`
- **`ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins`** (Function) — `internal/orchestrator/executor_pipeline.go:48`
- **`TestMiroFishSwarm`** (Function) — `internal/swarm/swarm_test.go:7`
- **`TestMiroFishSwarmLifecycle`** (Function) — `internal/swarm/swarm_runtime_test.go:7`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestPhase3Integration` | Function | `integration_test.go` | 122 |
| `ExecuteWithContext` | Function | `internal/orchestrator/executor_pipeline.go` | 10 |
| `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins` | Function | `internal/orchestrator/executor_pipeline.go` | 48 |
| `ExecuteRegistryResearch` | Function | `internal/orchestrator/executor_pipeline.go` | 18 |
| `ExecuteRegistryResearchDetailed` | Function | `internal/orchestrator/executor_pipeline.go` | 27 |
| `ExecuteRegistryResearchDetailedWithPolicy` | Function | `internal/orchestrator/executor_pipeline.go` | 36 |
| `ExecuteRegistryResearchDetailedWithPolicyAndGuards` | Function | `internal/orchestrator/executor_pipeline.go` | 45 |
| `DefaultExecutionPolicy` | Function | `internal/orchestrator/executor_policy.go` | 10 |
| `filterMutedAgents` | Function | `internal/orchestrator/executor_muted_filter.go` | 14 |
| `collectRecommendations` | Function | `internal/orchestrator/executor_collection.go` | 10 |
| `applyControlLayerWithOutcomes` | Function | `internal/orchestrator/executor_control.go` | 10 |
| `applyMomentumCrashProtection` | Function | `internal/orchestrator/executor_momentum_crash.go` | 10 |
| `inferRegime` | Function | `internal/orchestrator/executor_regime.go` | 10 |
| `ExecuteRegistryResearchWithDarwinianWeights` | Function | `internal/orchestrator/executor_darwinian.go` | 10 |
| `DefaultSymbols` | Function | `internal/orchestrator/executor_symbols.go` | 10 |
| `ExpandUniverse` | Function | `internal/orchestrator/executor_symbols.go` | 30 |
| `LayerRouter` | Interface | `internal/orchestrator/executor_types.go` | 10 |
| `ExecutionContext` | Struct | `internal/orchestrator/executor_types.go` | 60 |
| `ResearchResult` | Struct | `internal/orchestrator/executor_types.go` | 75 |
| `TestMiroFishSwarm` | Function | `internal/swarm/swarm_test.go` | 7 |
| `TestMiroFishSwarmLifecycle` | Function | `internal/swarm/swarm_runtime_test.go` | 7 |
| `TestMiroFishSwarmUpdateScenario` | Function | `internal/swarm/swarm_runtime_test.go` | 38 |
| `TestMiroFishSwarmResults` | Function | `internal/swarm/swarm_runtime_test.go` | 65 |
| `SeedRegistry` | Function | `internal/orchestrator/registry.go` | 10 |
| `LoadPhase3Metrics` | Function | `internal/orchestrator/phase3_metrics.go` | 106 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Start → InferSectorFromAgent` | cross_community | 6 |
| `Start → KnowledgeGap` | cross_community | 6 |
| `Start → Average` | cross_community | 6 |
| `Start → InferStyleFromAgent` | cross_community | 6 |
| `Main → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → ValidateRegistry` | cross_community | 5 |
| `RunLiveTrading → Policy` | cross_community | 5 |
| `RunLiveTrading → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `RunSimulation → SeedRegistry` | cross_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Portfolio | 32 calls |
| Risk | 31 calls |
| Industry | 14 calls |
| Ledger | 13 calls |
| Prism | 12 calls |
| Live | 9 calls |
| Swarm | 8 calls |
| Replay | 8 calls |

## How to Explore

1. `gitnexus_context({name: "ExecuteWithContext"})` — see callers and callees
2. `gitnexus_query({query: "orchestrator"})` — find related execution flows
3. Read key files listed above for implementation details
