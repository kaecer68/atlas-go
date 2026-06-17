---
name: atlas
description: "Skill for the Atlas area of atlas. 38 symbols across 5 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Atlas

38 symbols | 5 files | Cohesion: 71%

## When to Use

- Working with code in `cmd/`
- Understanding how NewSystemMetrics, TestDashboardSwaggerRoutes, TestRunAPIModeStartsServerAndRegistersRoutes work
- Modifying atlas-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `cmd/atlas/main_test.go` | TestRunAPIModeStartsServerAndRegistersRoutes, TestRunAPIModeReturnsListenError, TestRunRejectsLiveBrokerWithoutExplicitAllow, TestRunAllowsLiveBrokerWhenExplicitlyEnabled, TestRunRejectsUnsupportedBrokerAdapter (+16) |
| `cmd/atlas/main.go` | run, parseStatusCodeCSV, runAutoBackfillOnStartup, getLatestReplayDate, validateBrokerRuntimeConfig (+5) |
| `internal/monitoring/dashboard_api.go` | RegisterSwaggerRoutes, SetPool, SetHealthManager, SetJanusEngine |
| `internal/monitoring/metrics.go` | NewSystemMetrics, Start |
| `internal/monitoring/dashboard_api_test.go` | TestDashboardSwaggerRoutes |

## Entry Points

Start here when exploring this area:

- **`NewSystemMetrics`** (Function) — `internal/monitoring/metrics.go:276`
- **`TestDashboardSwaggerRoutes`** (Function) — `internal/monitoring/dashboard_api_test.go:389`
- **`TestRunAPIModeStartsServerAndRegistersRoutes`** (Function) — `cmd/atlas/main_test.go:16`
- **`TestRunAPIModeReturnsListenError`** (Function) — `cmd/atlas/main_test.go:69`
- **`TestRunRejectsLiveBrokerWithoutExplicitAllow`** (Function) — `cmd/atlas/main_test.go:96`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewSystemMetrics` | Function | `internal/monitoring/metrics.go` | 276 |
| `TestDashboardSwaggerRoutes` | Function | `internal/monitoring/dashboard_api_test.go` | 389 |
| `TestRunAPIModeStartsServerAndRegistersRoutes` | Function | `cmd/atlas/main_test.go` | 16 |
| `TestRunAPIModeReturnsListenError` | Function | `cmd/atlas/main_test.go` | 69 |
| `TestRunRejectsLiveBrokerWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 96 |
| `TestRunAllowsLiveBrokerWhenExplicitlyEnabled` | Function | `cmd/atlas/main_test.go` | 120 |
| `TestRunRejectsUnsupportedBrokerAdapter` | Function | `cmd/atlas/main_test.go` | 172 |
| `TestRunRejectsHTTPBrokerAdapterWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 196 |
| `TestRunAllowsHTTPBrokerAdapterWithExplicitAllow` | Function | `cmd/atlas/main_test.go` | 220 |
| `TestRunRejectsRealSignerWithoutExplicitAllow` | Function | `cmd/atlas/main_test.go` | 248 |
| `TestRunRejectsRealSignerWithoutKeyID` | Function | `cmd/atlas/main_test.go` | 300 |
| `TestParseStatusCodeCSV` | Function | `cmd/atlas/main_test.go` | 324 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeRetries` | Function | `cmd/atlas/main_test.go` | 161 |
| `TestValidateBrokerRuntimeConfigRejectsInvalidRetryStatusCode` | Function | `cmd/atlas/main_test.go` | 337 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeClockSkew` | Function | `cmd/atlas/main_test.go` | 348 |
| `TestValidateBrokerRuntimeConfigRejectsNegativeNonceTTL` | Function | `cmd/atlas/main_test.go` | 359 |
| `TestValidateBrokerRuntimeConfigRejectsUnsupportedNonceStore` | Function | `cmd/atlas/main_test.go` | 370 |
| `TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathFromLedgerDir` | Function | `cmd/atlas/main_test.go` | 381 |
| `TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathWithEmptyLedgerDir` | Function | `cmd/atlas/main_test.go` | 393 |
| `TestValidateBrokerRuntimeConfigNormalizesRelativeFileNonceStorePath` | Function | `cmd/atlas/main_test.go` | 404 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → NormalizeBrokerStrings` | cross_community | 4 |
| `Main → ValidateBrokerEnums` | cross_community | 4 |
| `Main → ValidateBrokerLiveMode` | cross_community | 4 |
| `Main → ValidateBrokerRetryConfig` | cross_community | 4 |
| `Main → MetricsCollector` | cross_community | 4 |
| `Main → Ping` | cross_community | 4 |
| `Main → Close` | cross_community | 4 |
| `Main → RunMigrations` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 26 calls |
| Portfolio | 3 calls |
| Db | 1 calls |
| Janus | 1 calls |
| Industry | 1 calls |
| Config | 1 calls |
| Orchestrator | 1 calls |

## How to Explore

1. `gitnexus_context({name: "NewSystemMetrics"})` — see callers and callees
2. `gitnexus_query({query: "atlas"})` — find related execution flows
3. Read key files listed above for implementation details
