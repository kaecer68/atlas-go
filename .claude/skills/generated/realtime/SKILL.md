---
name: realtime
description: "Skill for the Realtime area of atlas. 26 symbols across 2 files."
---

# Realtime

26 symbols | 2 files | Cohesion: 76%

## When to Use

- Working with code in `internal/`
- Understanding how TestRealTimeAdapter, NewRegimeDetector, DefaultRealTimeConfig work
- Modifying realtime-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/realtime/regime_adapter.go` | RegisterAgent, GetAgentWeight, GetCurrentRegime, GetActiveSymbols, ApplyToRecommendation (+18) |
| `internal/realtime/realtime_test.go` | TestRealTimeAdapter, TestRealTimeAdapterLifecycle, TestRegimeDetector |

## Entry Points

Start here when exploring this area:

- **`TestRealTimeAdapter`** (Function) — `internal/realtime/realtime_test.go:10`
- **`NewRegimeDetector`** (Function) — `internal/realtime/regime_adapter.go:47`
- **`DefaultRealTimeConfig`** (Function) — `internal/realtime/regime_adapter.go:207`
- **`NewRealTimeAdapter`** (Function) — `internal/realtime/regime_adapter.go:218`
- **`TestRealTimeAdapterLifecycle`** (Function) — `internal/realtime/realtime_test.go:391`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestRealTimeAdapter` | Function | `internal/realtime/realtime_test.go` | 10 |
| `NewRegimeDetector` | Function | `internal/realtime/regime_adapter.go` | 47 |
| `DefaultRealTimeConfig` | Function | `internal/realtime/regime_adapter.go` | 207 |
| `NewRealTimeAdapter` | Function | `internal/realtime/regime_adapter.go` | 218 |
| `TestRealTimeAdapterLifecycle` | Function | `internal/realtime/realtime_test.go` | 391 |
| `TestRegimeDetector` | Function | `internal/realtime/realtime_test.go` | 272 |
| `RegisterAgent` | Method | `internal/realtime/regime_adapter.go` | 391 |
| `GetAgentWeight` | Method | `internal/realtime/regime_adapter.go` | 413 |
| `GetCurrentRegime` | Method | `internal/realtime/regime_adapter.go` | 427 |
| `GetActiveSymbols` | Method | `internal/realtime/regime_adapter.go` | 463 |
| `ApplyToRecommendation` | Method | `internal/realtime/regime_adapter.go` | 584 |
| `IngestData` | Method | `internal/realtime/regime_adapter.go` | 257 |
| `DetectRegime` | Method | `internal/realtime/regime_adapter.go` | 57 |
| `GetRegimeConfidence` | Method | `internal/realtime/regime_adapter.go` | 439 |
| `GetStatistics` | Method | `internal/realtime/regime_adapter.go` | 476 |
| `GenerateReport` | Method | `internal/realtime/regime_adapter.go` | 517 |
| `Start` | Method | `internal/realtime/regime_adapter.go` | 235 |
| `Stop` | Method | `internal/realtime/regime_adapter.go` | 252 |
| `standardDeviation` | Function | `internal/realtime/regime_adapter.go` | 163 |
| `detectVolumeSpike` | Method | `internal/realtime/regime_adapter.go` | 109 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `GenerateReport → StandardDeviation` | intra_community | 5 |

## How to Explore

1. `gitnexus_context({name: "TestRealTimeAdapter"})` — see callers and callees
2. `gitnexus_query({query: "realtime"})` — find related execution flows
3. Read key files listed above for implementation details
