---
name: orchestrator
description: "Skill for the Orchestrator area of atlas. 319 symbols across 67 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Orchestrator

319 symbols | 67 files | Cohesion: 76%

## When to Use

- Working with code in `internal/`
- Understanding how TestPhase3Integration, TestMiroFishSwarm, TestMiroFishSwarmLifecycle work
- Modifying orchestrator-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/spawning/spawning_manager.go` | DefaultSpawningConfig, NewSpawningManager, Start, Stop, runLoop (+15) |
| `internal/swarm/mirofish_swarm.go` | DefaultSwarmConfig, NewMiroFishSwarm, UpdateScenario, InitializeScenarios, Start (+14) |
| `internal/orchestrator/system.go` | NewSystem, newSession, WithCapitalManagement, checkCapitalPhase, updateCapitalMetrics (+13) |
| `internal/orchestrator/executors.go` | filterMutedAgents, ExecuteRegistryResearchDetailed, collectRecommendations, symbolIterator, executeRegistryResearchDetailedWithPolicyAndGuards (+9) |
| `internal/orchestrator/forward_return_fallback_test.go` | TestGenerateForwardReturn_ValidQuotePositiveIntraday, TestGenerateForwardReturn_FlatDayUsesDistributionFallback, TestGenerateForwardReturn_RiskOnVsRiskOffDiffers, TestGenerateForwardReturn_QuoteWithNoOpenFallsBack, TestGenerateForwardReturn_ClipToMin (+9) |
| `internal/reflexivity/reflexivity_engine.go` | NewReflexivityEngine, RegisterBias, validateBias, mergeBiases, UpdateReality (+8) |
| `internal/orchestrator/phase3_controller.go` | NewPhase3Controller, StartBackgroundSwarm, StopBackgroundSwarm, UpdateSwarmState, swarmUpdateLoop (+8) |
| `internal/prism/prism_manager.go` | DefaultPRISMConfig, NewPRISMManager, Start, Stop, ScheduleTraining (+7) |
| `internal/orchestrator/strategy_evolver.go` | String, NewStrategyEvolver, NewStrategyEvolverWithConfig, Evaluate, determineState (+6) |
| `internal/orchestrator/system_plugins.go` | WithJANUS, WithPRISM, WithSwarm, WithSpawning, WithPhase3Controller (+5) |

## Entry Points

Start here when exploring this area:

- **`TestPhase3Integration`** (Function) — `integration_test.go:122`
- **`TestMiroFishSwarm`** (Function) — `internal/swarm/swarm_test.go:7`
- **`TestMiroFishSwarmLifecycle`** (Function) — `internal/swarm/swarm_runtime_test.go:7`
- **`TestMiroFishSwarmUpdateScenario`** (Function) — `internal/swarm/swarm_runtime_test.go:38`
- **`TestMiroFishSwarmResults`** (Function) — `internal/swarm/swarm_runtime_test.go:65`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestPhase3Integration` | Function | `integration_test.go` | 122 |
| `TestMiroFishSwarm` | Function | `internal/swarm/swarm_test.go` | 7 |
| `TestMiroFishSwarmLifecycle` | Function | `internal/swarm/swarm_runtime_test.go` | 7 |
| `TestMiroFishSwarmUpdateScenario` | Function | `internal/swarm/swarm_runtime_test.go` | 38 |
| `TestMiroFishSwarmResults` | Function | `internal/swarm/swarm_runtime_test.go` | 65 |
| `TestMiroFishSwarmComputeConsensus` | Function | `internal/swarm/swarm_runtime_test.go` | 102 |
| `TestMiroFishSwarmCollectPerformance` | Function | `internal/swarm/swarm_runtime_test.go` | 136 |
| `DefaultSwarmConfig` | Function | `internal/swarm/mirofish_swarm.go` | 124 |
| `NewMiroFishSwarm` | Function | `internal/swarm/mirofish_swarm.go` | 135 |
| `TestSpawningManager` | Function | `internal/spawning/spawning_test.go` | 151 |
| `DefaultSpawningConfig` | Function | `internal/spawning/spawning_manager.go` | 46 |
| `NewSpawningManager` | Function | `internal/spawning/spawning_manager.go` | 58 |
| `CalculateGapPriorityScore` | Function | `internal/spawning/gap_detector.go` | 482 |
| `TestFeedbackLoop` | Function | `internal/reflexivity/reflexivity_test.go` | 251 |
| `NewReflexivityEngine` | Function | `internal/reflexivity/reflexivity_engine.go` | 100 |
| `TestPRISMManager` | Function | `internal/prism/prism_test.go` | 9 |
| `DefaultPRISMConfig` | Function | `internal/prism/prism_manager.go` | 261 |
| `NewPRISMManager` | Function | `internal/prism/prism_manager.go` | 271 |
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

1. `gitnexus_context({name: "TestPhase3Integration"})` — see callers and callees
2. `gitnexus_query({query: "orchestrator"})` — find related execution flows
3. Read key files listed above for implementation details
