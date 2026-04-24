---
name: orchestrator
description: "Skill for the Orchestrator area of atlas. 313 symbols across 59 files."
---

# Orchestrator

313 symbols | 59 files | Cohesion: 75%

## When to Use

- Working with code in `internal/`
- Understanding how TestPhase3Integration, TestMiroFishSwarm, TestMiroFishSwarmLifecycle work
- Modifying orchestrator-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/orchestrator/system.go` | RunDailySimulation, runReplaySimulation, detectNarrativeEvents, applyNarrativeContextWithEvents, applyAlphaDiscovery (+15) |
| `internal/orchestrator/executors.go` | executeRegistryResearchDetailedWithPolicyAndGuards, executeRegistryResearchDetailedWithPolicyAndGuardsAndDarwinian, executeRegistryResearchWithDarwinianWeights, inferRegime, RegistrySymbols (+12) |
| `internal/prism/prism_manager.go` | DefaultPRISMConfig, NewPRISMManager, Start, Stop, ScheduleTraining (+10) |
| `internal/swarm/mirofish_swarm.go` | DefaultSwarmConfig, NewMiroFishSwarm, UpdateScenario, InitializeScenarios, Start (+8) |
| `internal/reflexivity/reflexivity_engine.go` | NewReflexivityEngine, RegisterBias, validateBias, mergeBiases, UpdateReality (+8) |
| `internal/orchestrator/phase3_controller.go` | NewPhase3Controller, StartBackgroundSwarm, StopBackgroundSwarm, UpdateSwarmState, swarmUpdateLoop (+8) |
| `internal/portfolio/darwinian_weights.go` | NewDarwinianWeightManager, InitializeFromRegistry, RecordOutcome, updateRollingMetrics, calculateSharpe (+7) |
| `internal/orchestrator/executors_test.go` | TestRegistrySymbolsIncludesNewSectorUniverses, containsSymbol, TestExecuteRegistryResearchWithDarwinianWeightsAppliesWeightMarker, TestGrowthMomentumOverrideChangesRecommendations, TestTechnicalBreakoutOverrideRejectsLowVolumeSetups (+7) |
| `internal/orchestrator/strategy_evolver.go` | NewStrategyEvolver, NewStrategyEvolverWithConfig, GetStrategyConfig, ShouldEnterPosition, GetPositionSizeLimit (+6) |
| `internal/orchestrator/phase3_controller_test.go` | TestPhase3ControllerApplyPRISMWeights, TestPhase3ControllerAutoPromote, TestPhase3ControllerBackgroundSwarmLifecycle, TestPhase3ControllerRunParallelOptimization, TestSystemWithPRISMAutoWiresRealExecutor (+5) |

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
| `DefaultSwarmConfig` | Function | `internal/swarm/mirofish_swarm.go` | 123 |
| `NewMiroFishSwarm` | Function | `internal/swarm/mirofish_swarm.go` | 134 |
| `TestSpawningManager` | Function | `internal/spawning/spawning_test.go` | 151 |
| `DefaultSpawningConfig` | Function | `internal/spawning/spawning_manager.go` | 46 |
| `NewSpawningManager` | Function | `internal/spawning/spawning_manager.go` | 58 |
| `TestFeedbackLoop` | Function | `internal/reflexivity/reflexivity_test.go` | 251 |
| `NewReflexivityEngine` | Function | `internal/reflexivity/reflexivity_engine.go` | 100 |
| `TestPRISMManager` | Function | `internal/prism/prism_test.go` | 9 |
| `DefaultPRISMConfig` | Function | `internal/prism/prism_manager.go` | 261 |
| `NewPRISMManager` | Function | `internal/prism/prism_manager.go` | 271 |
| `TestSeedRegistryIsValid` | Function | `internal/orchestrator/registry_test.go` | 10 |
| `SeedRegistry` | Function | `internal/orchestrator/registry.go` | 10 |
| `ValidateRegistry` | Function | `internal/orchestrator/registry.go` | 183 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `RunLiveTrading → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → ValidateRegistry` | cross_community | 5 |
| `RunLiveTrading → Policy` | cross_community | 5 |
| `RunSimulation → SeedRegistry` | cross_community | 5 |
| `RunSimulation → ValidateRegistry` | cross_community | 5 |
| `RunSimulation → Policy` | cross_community | 5 |
| `RunSimulation → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `NewProductionSystem → Policy` | cross_community | 5 |
| `RunDailySimulation → ResolvePrompt` | intra_community | 5 |
| `RunDailySimulation → RegimeScore` | intra_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Portfolio | 33 calls |
| Monitoring | 15 calls |
| Risk | 13 calls |
| Swarm | 11 calls |
| Spawning | 9 calls |
| Prism | 7 calls |
| Sim | 5 calls |
| Stress | 5 calls |

## How to Explore

1. `gitnexus_context({name: "TestPhase3Integration"})` — see callers and callees
2. `gitnexus_query({query: "orchestrator"})` — find related execution flows
3. Read key files listed above for implementation details
