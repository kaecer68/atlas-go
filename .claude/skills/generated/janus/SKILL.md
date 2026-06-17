---
name: janus
description: "Skill for the Janus area of atlas. 34 symbols across 6 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Janus

34 symbols | 6 files | Cohesion: 71%

## When to Use

- Working with code in `internal/`
- Understanding how TestEngine_EndToEnd, TestEngine_ApplyAdjustment, TestEngine_ApplyAdjustment_NoWeights work
- Modifying janus-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/janus/janus_test.go` | TestEngine_EndToEnd, TestEngine_ApplyAdjustment, TestEngine_ApplyAdjustment_NoWeights, recsForJanus, TestRegimeDetector_NovélRegime (+6) |
| `internal/janus/engine.go` | NewEngine, Update, GetRegimeClassification, ApplyAdjustment, mapDomainRegimeToPRISM (+6) |
| `internal/janus/tracker.go` | GetPerformance, aggregate, windowFromDays, NewCohortPerformanceTracker, RecordSnapshot (+1) |
| `internal/janus/calculator.go` | CalculateWindowWeights, NewCohortWeightCalculator, CalculateWeights |
| `internal/janus/detector.go` | NewRegimeDetector, Detect |
| `internal/janus/types.go` | DefaultJANUSConfig |

## Entry Points

Start here when exploring this area:

- **`TestEngine_EndToEnd`** (Function) — `internal/janus/janus_test.go:179`
- **`TestEngine_ApplyAdjustment`** (Function) — `internal/janus/janus_test.go:223`
- **`TestEngine_ApplyAdjustment_NoWeights`** (Function) — `internal/janus/janus_test.go:271`
- **`NewEngine`** (Function) — `internal/janus/engine.go:25`
- **`DefaultJANUSConfig`** (Function) — `internal/janus/types.go:81`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestEngine_EndToEnd` | Function | `internal/janus/janus_test.go` | 179 |
| `TestEngine_ApplyAdjustment` | Function | `internal/janus/janus_test.go` | 223 |
| `TestEngine_ApplyAdjustment_NoWeights` | Function | `internal/janus/janus_test.go` | 271 |
| `NewEngine` | Function | `internal/janus/engine.go` | 25 |
| `DefaultJANUSConfig` | Function | `internal/janus/types.go` | 81 |
| `TestRegimeDetector_NovélRegime` | Function | `internal/janus/janus_test.go` | 111 |
| `TestRegimeDetector_HistoricalRegime` | Function | `internal/janus/janus_test.go` | 139 |
| `TestRegimeDetector_Mixed` | Function | `internal/janus/janus_test.go` | 160 |
| `NewRegimeDetector` | Function | `internal/janus/detector.go` | 15 |
| `NewCohortPerformanceTracker` | Function | `internal/janus/tracker.go` | 19 |
| `TestCohortPerformanceTracker_RecordAndRetrieve` | Function | `internal/janus/janus_test.go` | 11 |
| `TestCohortPerformanceTracker_RollingWindow` | Function | `internal/janus/janus_test.go` | 30 |
| `TestCohortWeightCalculator_AllNegative` | Function | `internal/janus/janus_test.go` | 52 |
| `TestCohortWeightCalculator_MixedScores` | Function | `internal/janus/janus_test.go` | 78 |
| `NewEngineWithConfig` | Function | `internal/janus/engine.go` | 31 |
| `NewCohortWeightCalculator` | Function | `internal/janus/calculator.go` | 14 |
| `Update` | Method | `internal/janus/engine.go` | 61 |
| `GetRegimeClassification` | Method | `internal/janus/engine.go` | 98 |
| `ApplyAdjustment` | Method | `internal/janus/engine.go` | 136 |
| `CalculateWindowWeights` | Method | `internal/janus/calculator.go` | 86 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → CohortPerformanceTracker` | cross_community | 5 |
| `Main → CohortWeightCalculator` | cross_community | 5 |
| `Main → RegimeDetector` | cross_community | 5 |
| `Main → JANUSConfig` | cross_community | 4 |
| `Main → Engine` | cross_community | 4 |
| `Main → CohortPerformance` | cross_community | 4 |
| `Main → CalculateWeights` | cross_community | 3 |
| `Main → CalculateWindowWeights` | cross_community | 3 |
| `Main → Detect` | cross_community | 3 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Janus-status | 2 calls |
| Janus-backtest | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestEngine_EndToEnd"})` — see callers and callees
2. `gitnexus_query({query: "janus"})` — find related execution flows
3. Read key files listed above for implementation details
