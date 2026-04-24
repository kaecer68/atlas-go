---
name: atlas
description: "Skill for the Atlas area of atlas. 35 symbols across 4 files."
---

# Atlas

35 symbols | 4 files | Cohesion: 73%

## When to Use

- Working with code in `cmd/`
- Understanding how NewSystemMetrics, TestRunAPIModeStartsServerAndRegistersRoutes, TestRunAPIModeReturnsListenError work
- Modifying atlas-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `cmd/atlas/main_test.go` | TestRunAPIModeStartsServerAndRegistersRoutes, TestRunAPIModeReturnsListenError, TestRunRejectsLiveBrokerWithoutExplicitAllow, TestRunAllowsLiveBrokerWhenExplicitlyEnabled, TestRunRejectsUnsupportedBrokerAdapter (+15) |
| `cmd/atlas/main.go` | defaultAppDeps, main, run, parseStatusCodeCSV, runAutoBackfillOnStartup (+7) |
| `internal/monitoring/metrics.go` | NewSystemMetrics, Start |
| `internal/monitoring/dashboard_api.go` | SetPool |

## Entry Points

Start here when exploring this area:

- **`NewSystemMetrics`** (Function) — `internal/monitoring/metrics.go:276`
- **`TestRunAPIModeStartsServerAndRegistersRoutes`** (Function) — `cmd/atlas/main_test.go:16`
- **`TestRunAPIModeReturnsListenError`** (Function) — `cmd/atlas/main_test.go:64`
- **`TestRunRejectsLiveBrokerWithoutExplicitAllow`** (Function) — `cmd/atlas/main_test.go:89`
- **`TestRunAllowsLiveBrokerWhenExplicitlyEnabled`** (Function) — `cmd/atlas/main_test.go:111`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewSystemMetrics` | Function | `internal/monitoring/metrics.go` | 276 |
| `TestRunAPIModeStartsServerAndRegistersRoutes` | Function | `cmd/atlas/main_test.go` | 16 |
| `TestRunAPIModeReturnsListenError` | Function | `cmd/atlas/main_test.go` | 64 |
| `TestRunRejectsLiveBrokerWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 89 |
| `TestRunAllowsLiveBrokerWhenExplicitlyEnabled` | Function | `cmd/atlas/main_test.go` | 111 |
| `TestRunRejectsUnsupportedBrokerAdapter` | Function | `cmd/atlas/main_test.go` | 158 |
| `TestRunRejectsHTTPBrokerAdapterWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 180 |
| `TestRunRejectsRealSignerWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 228 |
| `TestRunRejectsRealSignerWithoutKeyID` | Function | `cmd/atlas/main_test.go` | 276 |
| `TestParseStatusCodeCSV` | Function | `cmd/atlas/main_test.go` | 298 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeRetries` | Function | `cmd/atlas/main_test.go` | 147 |
| `TestValidateBrokerRuntimeConfigRejectsInvalidRetryStatusCode` | Function | `cmd/atlas/main_test.go` | 311 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeClockSkew` | Function | `cmd/atlas/main_test.go` | 322 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeNonceTTL` | Function | `cmd/atlas/main_test.go` | 333 |
| `TestValidateBrokerRuntimeConfigRejectsUnsupportedNonceStore` | Function | `cmd/atlas/main_test.go` | 344 |
| `TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathFromLedgerDir` | Function | `cmd/atlas/main_test.go` | 355 |
| `TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathWithEmptyLedgerDir` | Function | `cmd/atlas/main_test.go` | 367 |
| `TestValidateBrokerRuntimeConfigNormalizesRelativeFileNonceStorePath` | Function | `cmd/atlas/main_test.go` | 378 |
| `TestValidateBrokerRuntimeConfigKeepsAbsoluteFileNonceStorePath` | Function | `cmd/atlas/main_test.go` | 400 |
| `TestValidateBrokerRuntimeConfigRejectsRedisNonceStoreWithoutURL` | Function | `cmd/atlas/main_test.go` | 421 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → CompositeMacroProvider` | cross_community | 5 |
| `Main → YahooFinanceMacroProvider` | cross_community | 5 |
| `Main → TWSECapitalFlowProvider` | cross_community | 5 |
| `Main → NormalizeBrokerStrings` | cross_community | 4 |
| `Main → ValidateBrokerEnums` | cross_community | 4 |
| `Main → ValidateBrokerLiveMode` | cross_community | 4 |
| `Main → ValidateBrokerRetryConfig` | cross_community | 4 |
| `Main → MetricsCollector` | cross_community | 4 |
| `Main → Close` | cross_community | 4 |
| `Main → RunMigrations` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 22 calls |
| Live | 2 calls |
| Db | 1 calls |
| Config | 1 calls |
| Narrative | 1 calls |
| Marketdata | 1 calls |
| Orchestrator | 1 calls |

## How to Explore

1. `gitnexus_context({name: "NewSystemMetrics"})` — see callers and callees
2. `gitnexus_query({query: "atlas"})` — find related execution flows
3. Read key files listed above for implementation details
