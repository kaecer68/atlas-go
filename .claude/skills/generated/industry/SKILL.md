---
name: industry
description: "Skill for the Industry area of atlas. 185 symbols across 22 files."
---

# Industry

185 symbols | 22 files | Cohesion: 71%

## When to Use

- Working with code in `internal/`
- Understanding how TestMarketState, TestMarketScenario, TestFishPerformance work
- Modifying industry-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/industry/risk_test.go` | TestRiskLevelConstants, TestRiskMonitor_CustomerConcentrationAction, TestRiskMonitor_AsymmetricAction, TestCustomerConcentration_Validation, TestAddAndGetCustomerConcentration (+21) |
| `internal/industry/linkage.go` | AddNode, GetNode, GetDownstreamChain, NewCorrelationMatrix, UpdateCorrelation (+15) |
| `internal/industry/types.go` | NewClassificationTree, AddSegment, GetSegment, GetChildren, GetLevel1 (+13) |
| `internal/industry/cycle.go` | IsFavorablePhase, GetPhaseScore, GetTypicalTransitions, NewCycleTracker, initializeDefaultPositions (+10) |
| `internal/industry/risk.go` | getCustomerConcentrationAction, getAsymmetricAction, NewRiskMonitor, AddCustomerConcentration, GetCustomerConcentration (+8) |
| `internal/industry/cycle_test.go` | TestCyclePositionIsFavorable, TestGetPhaseScore, TestGetTypicalTransitions, TestNewCycleTracker, TestDetectBusinessCycle (+8) |
| `internal/industry/linkage_test.go` | TestDefaultSupplyChainGraph, TestGetDownstreamChain, TestCorrelationMatrix, TestUpdateCorrelation, TestShockPropagation (+7) |
| `internal/industry/seasonality.go` | NewSeasonalEngine, DefaultSeasonalPatterns, isDateInRange, GetPatternByID, GetHistoricalAccuracy (+7) |
| `internal/monitoring/api/industry/handlers.go` | writeJSON, writeJSONError, HandleIndustryClassification, HandleIndustrySeasonality, HandleIndustrySeasonalityCalendar (+6) |
| `internal/monitoring/service/industry.go` | GetRiskInfo, GetLinkageInfo, abs, GetIndustryGraph, PropagateShock (+5) |

## Entry Points

Start here when exploring this area:

- **`TestMarketState`** (Function) — `internal/swarm/swarm_test.go:116`
- **`TestMarketScenario`** (Function) — `internal/swarm/swarm_test.go:133`
- **`TestFishPerformance`** (Function) — `internal/swarm/swarm_test.go:149`
- **`TestPrediction`** (Function) — `internal/swarm/swarm_test.go:215`
- **`TestConsensusPrediction`** (Function) — `internal/swarm/swarm_test.go:235`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestMarketState` | Function | `internal/swarm/swarm_test.go` | 116 |
| `TestMarketScenario` | Function | `internal/swarm/swarm_test.go` | 133 |
| `TestFishPerformance` | Function | `internal/swarm/swarm_test.go` | 149 |
| `TestPrediction` | Function | `internal/swarm/swarm_test.go` | 215 |
| `TestConsensusPrediction` | Function | `internal/swarm/swarm_test.go` | 235 |
| `TestSimulationResult` | Function | `internal/swarm/swarm_test.go` | 256 |
| `TestAnomaly` | Function | `internal/swarm/swarm_test.go` | 276 |
| `TestLearningStrategy` | Function | `internal/metalearning/metalearning_test.go` | 167 |
| `TestRecordMutationBrief` | Function | `internal/ledger/ledger_persistence_test.go` | 140 |
| `TestLoad_FugleAPIKeyPriority` | Function | `internal/config/config_test.go` | 205 |
| `TestRiskLevelConstants` | Function | `internal/industry/risk_test.go` | 8 |
| `TestRiskMonitor_CustomerConcentrationAction` | Function | `internal/industry/risk_test.go` | 560 |
| `TestRiskMonitor_AsymmetricAction` | Function | `internal/industry/risk_test.go` | 583 |
| `TestCustomerConcentration_Validation` | Function | `internal/industry/risk_test.go` | 646 |
| `TestCyclePositionIsFavorable` | Function | `internal/industry/cycle_test.go` | 170 |
| `TestGetPhaseScore` | Function | `internal/industry/cycle_test.go` | 191 |
| `TestTruncate` | Function | `cmd/revert-baseline/main_test.go` | 4 |
| `TestDefaultClassification` | Function | `internal/industry/types_test.go` | 6 |
| `TestClassificationTreeValidate` | Function | `internal/industry/types_test.go` | 55 |
| `TestIndustrySegmentAttributes` | Function | `internal/industry/types_test.go` | 88 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → Dataset` | cross_community | 4 |
| `Main → Value` | cross_community | 4 |
| `Main → ParseFloat` | cross_community | 4 |
| `Main → ParseInt` | cross_community | 4 |
| `Main → Policy` | cross_community | 4 |
| `Run → Dataset` | cross_community | 4 |
| `Run → Value` | cross_community | 4 |
| `Run → ParseFloat` | cross_community | 4 |
| `Run → ParseInt` | cross_community | 4 |
| `Run → Policy` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Orchestrator | 4 calls |
| Ledger | 4 calls |
| Replay | 2 calls |
| Baseline | 2 calls |
| Evolution | 2 calls |
| Sim | 1 calls |
| Portfolio | 1 calls |
| Service | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestMarketState"})` — see callers and callees
2. `gitnexus_query({query: "industry"})` — find related execution flows
3. Read key files listed above for implementation details
