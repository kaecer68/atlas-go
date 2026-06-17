---
name: ledger
description: "Skill for the Ledger area of atlas. 44 symbols across 12 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Ledger

44 symbols | 12 files | Cohesion: 51%

## When to Use

- Working with code in `internal/`
- Understanding how TestBuildAgentRows, BuildAgentRows, TestRecordSessionSummaryPersistsTraceIDs work
- Modifying ledger-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/ledger/ledger.go` | NewStore, RecordSessionSummary, RecordSpawnRecord, LoadSpawnRecords, LoadSessionSummaries (+13) |
| `internal/ledger/ledger_persistence_test.go` | TestRecordAndLoadSessionOutcomes, TestRecordAndLoadSpawnRecords, TestLoadSessionSummaries, TestLoadSessionSummariesMissingReturnsNil, TestRecordAndLoadOutcomes (+5) |
| `cmd/experimental/janus-backtest/main.go` | main, loadMetrics, detectDominantRegime, printMarkdown |
| `internal/ledger/ledger_test.go` | TestRecordSessionSummaryPersistsTraceIDs, TestBuildScorecards, TestRecordAndLoadSessionScreeningRejects |
| `internal/backtest/window.go` | NewRunner, GenerateReport |
| `internal/reporting/reporting_test.go` | TestBuildAgentRows |
| `internal/reporting/agent_table.go` | BuildAgentRows |
| `internal/backtest/window_test.go` | TestRunWindow |
| `internal/monitoring/service/backtest.go` | Start |
| `internal/monitoring/dashboard_api.go` | handleAgentObservatory |

## Entry Points

Start here when exploring this area:

- **`TestBuildAgentRows`** (Function) — `internal/reporting/reporting_test.go:81`
- **`BuildAgentRows`** (Function) — `internal/reporting/agent_table.go:47`
- **`TestRecordSessionSummaryPersistsTraceIDs`** (Function) — `internal/ledger/ledger_test.go:31`
- **`TestRecordAndLoadSessionOutcomes`** (Function) — `internal/ledger/ledger_persistence_test.go:36`
- **`TestRecordAndLoadSpawnRecords`** (Function) — `internal/ledger/ledger_persistence_test.go:161`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestBuildAgentRows` | Function | `internal/reporting/reporting_test.go` | 81 |
| `BuildAgentRows` | Function | `internal/reporting/agent_table.go` | 47 |
| `TestRecordSessionSummaryPersistsTraceIDs` | Function | `internal/ledger/ledger_test.go` | 31 |
| `TestRecordAndLoadSessionOutcomes` | Function | `internal/ledger/ledger_persistence_test.go` | 36 |
| `TestRecordAndLoadSpawnRecords` | Function | `internal/ledger/ledger_persistence_test.go` | 161 |
| `TestLoadSessionSummaries` | Function | `internal/ledger/ledger_persistence_test.go` | 218 |
| `TestLoadSessionSummariesMissingReturnsNil` | Function | `internal/ledger/ledger_persistence_test.go` | 246 |
| `NewStore` | Function | `internal/ledger/ledger.go` | 18 |
| `TestRunWindow` | Function | `internal/backtest/window_test.go` | 9 |
| `NewRunner` | Function | `internal/backtest/window.go` | 24 |
| `TestBuildScorecards` | Function | `internal/ledger/ledger_test.go` | 12 |
| `BuildScorecards` | Function | `internal/ledger/ledger.go` | 333 |
| `TestRecordAndLoadOutcomes` | Function | `internal/ledger/ledger_persistence_test.go` | 11 |
| `TestLoadOutcomesMissingReturnsNil` | Function | `internal/ledger/ledger_persistence_test.go` | 55 |
| `TestRecordExperimentJSONL` | Function | `internal/ledger/ledger_persistence_test.go` | 101 |
| `TestRecordSessionExperiment` | Function | `internal/ledger/ledger_persistence_test.go` | 120 |
| `TestRecordAndLoadSessionScreeningRejects` | Function | `internal/ledger/ledger_test.go` | 110 |
| `TestLoadOutcomeFile` | Function | `internal/ledger/ledger_persistence_test.go` | 259 |
| `TestLoadOutcomeFileMissingReturnsNil` | Function | `internal/ledger/ledger_persistence_test.go` | 279 |
| `RecordSessionSummary` | Method | `internal/ledger/ledger.go` | 146 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleAgentObservatory → Mean` | cross_community | 5 |
| `Main → Close` | cross_community | 4 |
| `Main → Dataset` | cross_community | 4 |
| `Main → Value` | cross_community | 4 |
| `Main → ParseFloat` | cross_community | 4 |
| `Main → ParseInt` | cross_community | 4 |
| `Main → Policy` | cross_community | 4 |
| `HandleAgentObservatory → Store` | cross_community | 4 |
| `HandleAgentObservatory → Agg` | cross_community | 4 |
| `HandleAgentObservatory → Ratio` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Industry | 4 calls |
| Janus | 3 calls |
| Portfolio | 2 calls |
| Config | 2 calls |
| Janus-backtest | 2 calls |
| Service | 2 calls |
| Janus-status | 1 calls |
| Monitoring | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestBuildAgentRows"})` — see callers and callees
2. `gitnexus_query({query: "ledger"})` — find related execution flows
3. Read key files listed above for implementation details
