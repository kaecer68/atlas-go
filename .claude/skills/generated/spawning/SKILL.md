---
name: spawning
description: "Skill for the Spawning area of atlas. 40 symbols across 4 files."
---

# Spawning

40 symbols | 4 files | Cohesion: 80%

## When to Use

- Working with code in `internal/`
- Understanding how TestGapDetector, NewGapDetector, CalculateGapPriorityScore work
- Modifying spawning-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/spawning/agent_factory.go` | CreateAgentForGap, generateAgentID, determineLayer, generateSkill, generateName (+9) |
| `internal/spawning/gap_detector.go` | NewGapDetector, DetectGaps, detectSectorGaps, detectStyleGaps, detectMarketCapGaps (+8) |
| `internal/spawning/spawning_manager.go` | Start, Stop, runLoop, PerformSpawningCycle, prioritizeGaps (+6) |
| `internal/spawning/spawning_test.go` | TestGapDetector, TestAgentFactory |

## Entry Points

Start here when exploring this area:

- **`TestGapDetector`** (Function) — `internal/spawning/spawning_test.go:9`
- **`NewGapDetector`** (Function) — `internal/spawning/gap_detector.go:109`
- **`CalculateGapPriorityScore`** (Function) — `internal/spawning/gap_detector.go:482`
- **`TestAgentFactory`** (Function) — `internal/spawning/spawning_test.go:86`
- **`NewAgentFactory`** (Function) — `internal/spawning/agent_factory.go:19`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestGapDetector` | Function | `internal/spawning/spawning_test.go` | 9 |
| `NewGapDetector` | Function | `internal/spawning/gap_detector.go` | 109 |
| `CalculateGapPriorityScore` | Function | `internal/spawning/gap_detector.go` | 482 |
| `TestAgentFactory` | Function | `internal/spawning/spawning_test.go` | 86 |
| `NewAgentFactory` | Function | `internal/spawning/agent_factory.go` | 19 |
| `DetectGaps` | Method | `internal/spawning/gap_detector.go` | 120 |
| `GetOpenGaps` | Method | `internal/spawning/gap_detector.go` | 376 |
| `Start` | Method | `internal/spawning/spawning_manager.go` | 76 |
| `Stop` | Method | `internal/spawning/spawning_manager.go` | 90 |
| `PerformSpawningCycle` | Method | `internal/spawning/spawning_manager.go` | 118 |
| `UpdateGapStatus` | Method | `internal/spawning/gap_detector.go` | 403 |
| `CreateAgentForGap` | Method | `internal/spawning/agent_factory.go` | 27 |
| `CloneAgentWithVariation` | Method | `internal/spawning/agent_factory.go` | 361 |
| `inferSectorFromAgent` | Function | `internal/spawning/gap_detector.go` | 414 |
| `inferStyleFromAgent` | Function | `internal/spawning/gap_detector.go` | 446 |
| `average` | Function | `internal/spawning/gap_detector.go` | 470 |
| `defaultPromptTemplate` | Function | `internal/spawning/agent_factory.go` | 321 |
| `detectSectorGaps` | Method | `internal/spawning/gap_detector.go` | 161 |
| `detectStyleGaps` | Method | `internal/spawning/gap_detector.go` | 244 |
| `detectMarketCapGaps` | Method | `internal/spawning/gap_detector.go` | 290 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `PostSimulation → DetectMarketCapGaps` | cross_community | 4 |
| `PostSimulation → ScoredGap` | cross_community | 4 |
| `PostSimulation → CalculateGapPriorityScore` | cross_community | 4 |

## How to Explore

1. `gitnexus_context({name: "TestGapDetector"})` — see callers and callees
2. `gitnexus_query({query: "spawning"})` — find related execution flows
3. Read key files listed above for implementation details
