---
name: service
description: "Skill for the Service area of atlas. 45 symbols across 16 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Service

45 symbols | 16 files | Cohesion: 58%

## When to Use

- Working with service-layer code under `internal/monitoring/service/` and orchestrator registry helpers
- **Note:** there is no top-level `internal/service/` package; the "service" area spans `internal/monitoring/service/*` plus `internal/orchestrator/registry*.go`
- Understanding how `TestSeedRegistryIsValid`, `ValidateRegistry`, `LoadRegistry`, `LoadUniverseOverlap`, and Wave 9 service detectors work
- Modifying service-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/monitoring/service/pipeline.go` | LoadUniverseOverlap, isStockPickingLayer, isStockPickingLayerByID, LoadMacroRadar, LoadForecastVsReality (+5) |
| `internal/monitoring/service/regime_debouncer.go` | RegimeDebouncer, NewRegimeDebouncer |
| `internal/monitoring/service/factor_weight_regression.go` | FactorWeightRegressionDetector, NewFactorWeightRegressionDetector |
| `internal/monitoring/service/drift_detector.go` | DriftDetector, NewDriftDetector, NewDriftDetectorWithTargets |
| `internal/monitoring/service/channel_health_synthesizer.go` | ChannelHealthSynthesizer, ChannelHealthProvider, NewChannelHealthSynthesizer |
| `internal/monitoring/service/ingestion_lag_monitor.go` | IngestionLagMonitor, IngestionLagProvider, NewIngestionLagMonitor |
| `internal/monitoring/service/ingestion_lag_provider.go` | ChannelHealthIngestionLagProvider, NewChannelHealthIngestionLagProvider |
| `internal/monitoring/service/weight_provider.go` | WeightProvider, FactorWeightEngineWeightProvider, NewFactorWeightEngineWeightProvider |
| `internal/monitoring/dashboard_api.go` | handleUniverseOverlap, handleSystemHealth, buildChannelInfo, handleMacroRadar, handleForecastVsReality (+2) |
| `internal/monitoring/service/report.go` | LoadDailySummary, loadNarrativeEventsForDate, loadRiskSnapshot, loadRecommendationsForDate |
| `internal/monitoring/service/system.go` | LoadSystemHealth, statusText, buildChannelInfo |
| `internal/orchestrator/registry.go` | ValidateRegistry, LoadRegistry |
| `internal/orchestrator/registry_test.go` | TestSeedRegistryIsValid |
| `internal/monitoring/service/control.go` | GetAgentHealth |
| `internal/ledger/transaction.go` | Error |
| `internal/ledger/ledger.go` | LoadSessionOutcomes |

## Entry Points

Start here when exploring this area:

- **`TestSeedRegistryIsValid`** (Function) — `internal/orchestrator/registry_test.go:10`
- **`ValidateRegistry`** (Function) — `internal/orchestrator/registry.go:183`
- **`LoadRegistry`** (Function) — `internal/orchestrator/registry.go:208`
- **`LoadSessionSummary`** (Function) — `internal/monitoring/service/pipeline.go:639`
- **`LoadUniverseOverlap`** (Method) — `internal/monitoring/service/pipeline.go:433`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestSeedRegistryIsValid` | Function | `internal/orchestrator/registry_test.go` | 10 |
| `ValidateRegistry` | Function | `internal/orchestrator/registry.go` | 183 |
| `LoadRegistry` | Function | `internal/orchestrator/registry.go` | 208 |
| `LoadSessionSummary` | Function | `internal/monitoring/service/pipeline.go` | 639 |
| `LoadUniverseOverlap` | Method | `internal/monitoring/service/pipeline.go` | 433 |
| `GetAgentHealth` | Method | `internal/monitoring/service/control.go` | 72 |
| `Error` | Method | `internal/ledger/transaction.go` | 20 |
| `LoadSessionOutcomes` | Method | `internal/ledger/ledger.go` | 70 |
| `LoadSystemHealth` | Method | `internal/monitoring/service/system.go` | 62 |
| `LoadMacroRadar` | Method | `internal/monitoring/service/pipeline.go` | 33 |
| `LoadForecastVsReality` | Method | `internal/monitoring/service/pipeline.go` | 99 |
| `LoadRecommendationPipeline` | Method | `internal/monitoring/service/pipeline.go` | 197 |
| `LoadDailySummary` | Method | `internal/monitoring/service/report.go` | 131 |
| `NewRegimeDebouncer` | Function | `internal/monitoring/service/regime_debouncer.go` | 38 |
| `NewFactorWeightRegressionDetector` | Function | `internal/monitoring/service/factor_weight_regression.go` | 33 |
| `NewDriftDetector` | Function | `internal/monitoring/service/drift_detector.go` | 31 |
| `NewChannelHealthSynthesizer` | Function | `internal/monitoring/service/channel_health_synthesizer.go` | 38 |
| `NewIngestionLagMonitor` | Function | `internal/monitoring/service/ingestion_lag_monitor.go` | 42 |
| `NewChannelHealthIngestionLagProvider` | Function | `internal/monitoring/service/ingestion_lag_provider.go` | 23 |
| `NewFactorWeightEngineWeightProvider` | Function | `internal/monitoring/service/weight_provider.go` | 18 |
| `isStockPickingLayer` | Function | `internal/monitoring/service/pipeline.go` | 534 |
| `isStockPickingLayerByID` | Function | `internal/monitoring/service/pipeline.go` | 538 |
| `statusText` | Function | `internal/monitoring/service/system.go` | 155 |
| `buildChannelInfo` | Function | `internal/monitoring/service/system.go` | 170 |
| `fallbackPriceTargets` | Function | `internal/monitoring/service/session.go` | 235 |
| `computePipelineTags` | Function | `internal/monitoring/service/session.go` | 173 |
| `handleUniverseOverlap` | Method | `internal/monitoring/dashboard_api.go` | 897 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → ValidateRegistry` | cross_community | 5 |
| `HandleRecommendationPipeline → Close` | cross_community | 5 |
| `RunSimulation → SeedRegistry` | cross_community | 5 |
| `RunSimulation → ValidateRegistry` | cross_community | 5 |
| `LoadDailySummary → CalculateVaR` | cross_community | 5 |
| `HandleForecastVsReality → ForecastVsRealityItem` | cross_community | 4 |
| `HandleSystemHealth → Policy` | cross_community | 4 |
| `HandleSystemHealth → ExecutionPolicyFromConstraints` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Ledger | 7 calls |
| Orchestrator | 6 calls |
| Replay | 6 calls |
| Monitoring | 3 calls |
| Baseline | 2 calls |
| Daily-replay-sync | 2 calls |
| Narrative | 1 calls |
| Risk | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestSeedRegistryIsValid"})` — see callers and callees
2. `gitnexus_query({query: "service"})` — find related execution flows
3. Read key files listed above for implementation details
