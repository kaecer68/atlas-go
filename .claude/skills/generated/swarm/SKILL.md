---
name: swarm
description: "Skill for the Swarm area of atlas. 22 symbols across 12 files."
---

# Swarm

22 symbols | 12 files | Cohesion: 47%

## When to Use

- Working with code in `internal/`
- Understanding how TestMarketState, TestMarketScenario, TestFishPerformance work
- Modifying swarm-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/swarm/swarm_test.go` | TestMarketState, TestMarketScenario, TestFishPerformance, TestSwarmConfig, TestMiroFish (+4) |
| `internal/ledger/ledger.go` | RecordWindowSummary, RecordMutationBrief |
| `internal/swarm/mirofish_swarm.go` | min, max |
| `internal/metalearning/metalearning_test.go` | TestLearningStrategy |
| `internal/ledger/ledger_persistence_test.go` | TestRecordMutationBrief |
| `internal/globalmarket/globalmarket_test.go` | TestGlobalAgent |
| `internal/config/config_test.go` | TestLoad_FugleAPIKeyPriority |
| `internal/backtest/window.go` | Run |
| `cmd/revert-baseline/main_test.go` | TestTruncate |
| `internal/prism/prism_manager.go` | GetCompletedResults |

## Entry Points

Start here when exploring this area:

- **`TestMarketState`** (Function) — `internal/swarm/swarm_test.go:116`
- **`TestMarketScenario`** (Function) — `internal/swarm/swarm_test.go:133`
- **`TestFishPerformance`** (Function) — `internal/swarm/swarm_test.go:149`
- **`TestSwarmConfig`** (Function) — `internal/swarm/swarm_test.go:169`
- **`TestMiroFish`** (Function) — `internal/swarm/swarm_test.go:187`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestMarketState` | Function | `internal/swarm/swarm_test.go` | 116 |
| `TestMarketScenario` | Function | `internal/swarm/swarm_test.go` | 133 |
| `TestFishPerformance` | Function | `internal/swarm/swarm_test.go` | 149 |
| `TestSwarmConfig` | Function | `internal/swarm/swarm_test.go` | 169 |
| `TestMiroFish` | Function | `internal/swarm/swarm_test.go` | 187 |
| `TestPrediction` | Function | `internal/swarm/swarm_test.go` | 215 |
| `TestConsensusPrediction` | Function | `internal/swarm/swarm_test.go` | 235 |
| `TestSimulationResult` | Function | `internal/swarm/swarm_test.go` | 256 |
| `TestAnomaly` | Function | `internal/swarm/swarm_test.go` | 276 |
| `TestLearningStrategy` | Function | `internal/metalearning/metalearning_test.go` | 167 |
| `TestRecordMutationBrief` | Function | `internal/ledger/ledger_persistence_test.go` | 140 |
| `TestGlobalAgent` | Function | `internal/globalmarket/globalmarket_test.go` | 319 |
| `TestLoad_FugleAPIKeyPriority` | Function | `internal/config/config_test.go` | 205 |
| `TestTruncate` | Function | `cmd/revert-baseline/main_test.go` | 4 |
| `RecordWindowSummary` | Method | `internal/ledger/ledger.go` | 181 |
| `RecordMutationBrief` | Method | `internal/ledger/ledger.go` | 194 |
| `Run` | Method | `internal/backtest/window.go` | 28 |
| `GetCompletedResults` | Method | `internal/prism/prism_manager.go` | 642 |
| `ProcessRecommendations` | Method | `internal/orchestrator/plugin_adapters.go` | 20 |
| `ApplyPRISMWeights` | Method | `internal/orchestrator/phase3_controller.go` | 131 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → Dataset` | cross_community | 5 |
| `Main → Value` | cross_community | 5 |
| `Main → ParseFloat` | cross_community | 5 |
| `Main → ParseInt` | cross_community | 5 |
| `Main → Policy` | cross_community | 5 |
| `Main → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `Main → Dataset` | cross_community | 4 |
| `Main → Value` | cross_community | 4 |
| `Main → ParseFloat` | cross_community | 4 |
| `Main → ParseInt` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 8 calls |
| Monitoring | 5 calls |
| Evolution | 2 calls |
| Janus | 1 calls |
| Baseline | 1 calls |
| Sim | 1 calls |
| Replay | 1 calls |
| Ledger | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestMarketState"})` — see callers and callees
2. `gitnexus_query({query: "swarm"})` — find related execution flows
3. Read key files listed above for implementation details
