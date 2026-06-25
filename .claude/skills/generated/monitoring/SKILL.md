---
name: monitoring
description: "Skill for the Monitoring area of atlas. 250 symbols across 46 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Monitoring

250 symbols | 46 files | Cohesion: 77%

## When to Use

- Working with code in `internal/monitoring/`
- Understanding how Wave 9 observability (`Wave9Observability`, `NewWave9Observability`) and the five detectors work
- Modifying monitoring-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/monitoring/dashboard_api.go` | NewDashboardAPI, RegisterRoutes, RegisterIndustryRoutes, RegisterNarrativeRoutes, RegisterControlRoutes (+20) |
| `internal/monitoring/metrics.go` | NewMetricsCollector, RecordCounter, RecordGauge, RecordHistogram, GetMetric (+12) |
| `internal/monitoring/wave9_runtime.go` | Wave9Observability, NewWave9Observability, Start, Stop, Close, NewChannelHealthProviderFromStore (+15) |
| `internal/monitoring/service/pipeline.go` | LoadUniverseOverlap, isStockPickingLayer, isStockPickingLayerByID, LoadMacroRadar, LoadForecastVsReality (+5) |
| `internal/monitoring/monitoring_test.go` | TestMetricsCollector_RecordCounter_Accumulates, TestMetricsCollector_RecordGauge_Overwrites, TestMetricsCollector_RecordHistogram, TestMetricsCollector_GetAllMetrics, TestTradingMetrics_RecordOrder (+9) |
| `internal/monitoring/alert_api_test.go` | newTestAlertAPI, seedAlerts, TestAlertAPI_ListAlerts, TestAlertAPI_ListAlerts_Empty, TestAlertAPI_ListAlerts_MethodNotAllowed (+9) |
| `internal/monitoring/monitor.go` | Info, Warning, Error, Critical, SetAlertStore (+8) |
| `internal/monitoring/notifier_test.go` | TestWebhookNotifier_IsConfigured, TestWebhookNotifier_Name, TestWebhookNotifier_Notify_NotConfigured, TestWebhookNotifier_Notify_Success, TestWebhookNotifier_Notify_CustomHeaders (+8) |
| `internal/monitoring/alert_store_test.go` | newTestStore, makeAlert, TestAlertStore_SaveAndLoadAll, TestAlertStore_LoadAll_EmptyFile, TestAlertStore_LoadAll_NonExistentFile (+7) |
| `internal/monitoring/notifier.go` | NewWebhookNotifier, Name, IsConfigured, Notify, NewTelegramNotifier (+3) |

## Wave 9 / Service Files

| File | Symbols |
|------|---------|
| `internal/monitoring/service/regime_debouncer.go` | RegimeDebouncer, NewRegimeDebouncer |
| `internal/monitoring/service/factor_weight_regression.go` | FactorWeightRegressionDetector, NewFactorWeightRegressionDetector |
| `internal/monitoring/service/drift_detector.go` | DriftDetector, NewDriftDetector, NewDriftDetectorWithTargets |
| `internal/monitoring/service/channel_health_synthesizer.go` | ChannelHealthSynthesizer, ChannelHealthProvider, NewChannelHealthSynthesizer |
| `internal/monitoring/service/ingestion_lag_monitor.go` | IngestionLagMonitor, IngestionLagProvider, NewIngestionLagMonitor |
| `internal/monitoring/service/ingestion_lag_provider.go` | ChannelHealthIngestionLagProvider, NewChannelHealthIngestionLagProvider |
| `internal/monitoring/service/weight_provider.go` | WeightProvider, FactorWeightEngineWeightProvider, NewFactorWeightEngineWeightProvider |
| `internal/monitoring/wave9_runtime_test.go` | TestWave9Observability_StartStop, TestWave9Observability_RequiresProviders |
| `internal/monitoring/wave9_runtime_integration_test.go` | TestWave9Integration_RegimeDebouncerFlow, TestWave9Integration_FactorWeightRegressionFlow, TestWave9Integration_DriftDetectorFlow, TestWave9Integration_ChannelHealthSynthesizerFlow, TestWave9Integration_IngestionLagMonitorFlow, TestWave9Integration_EndToEndEventFlow |
| `internal/monitoring/service/wave9_integration_test.go` | TestWave9Integration_RegimeDebouncerFlow, TestWave9Integration_FactorWeightRegressionFlow, TestWave9Integration_DriftDetectorFlow, TestWave9Integration_ChannelHealthSynthesizerFlow, TestWave9Integration_IngestionLagMonitorFlow, TestWave9Integration_EndToEndEventFlow |

## Entry Points

Start here when exploring this area:

- **`TestMetricsCollector_RecordCounter_Accumulates`** (Function) — `internal/monitoring/monitoring_test.go:124`
- **`TestMetricsCollector_RecordGauge_Overwrites`** (Function) — `internal/monitoring/monitoring_test.go:144`
- **`TestMetricsCollector_RecordHistogram`** (Function) — `internal/monitoring/monitoring_test.go:159`
- **`TestMetricsCollector_GetAllMetrics`** (Function) — `internal/monitoring/monitoring_test.go:172`
- **`NewWave9Observability`** (Function) — `internal/monitoring/wave9_runtime.go:105`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestMetricsCollector_RecordCounter_Accumulates` | Function | `internal/monitoring/monitoring_test.go` | 124 |
| `TestMetricsCollector_RecordGauge_Overwrites` | Function | `internal/monitoring/monitoring_test.go` | 144 |
| `TestMetricsCollector_RecordHistogram` | Function | `internal/monitoring/monitoring_test.go` | 159 |
| `TestMetricsCollector_GetAllMetrics` | Function | `internal/monitoring/monitoring_test.go` | 172 |
| `TestTradingMetrics_RecordOrder` | Function | `internal/monitoring/monitoring_test.go` | 360 |
| `NewWave9Observability` | Function | `internal/monitoring/wave9_runtime.go` | 105 |
| `Wave9Observability` | Struct | `internal/monitoring/wave9_runtime.go` | 18 |
| `NewRegimeDebouncer` | Function | `internal/monitoring/service/regime_debouncer.go` | 38 |
| `NewFactorWeightRegressionDetector` | Function | `internal/monitoring/service/factor_weight_regression.go` | 33 |
| `NewDriftDetector` | Function | `internal/monitoring/service/drift_detector.go` | 31 |
| `NewDriftDetectorWithTargets` | Function | `internal/monitoring/service/drift_detector.go` | 39 |
| `NewChannelHealthSynthesizer` | Function | `internal/monitoring/service/channel_health_synthesizer.go` | 38 |
| `NewIngestionLagMonitor` | Function | `internal/monitoring/service/ingestion_lag_monitor.go` | 42 |
| `NewChannelHealthIngestionLagProvider` | Function | `internal/monitoring/service/ingestion_lag_provider.go` | 23 |
| `NewFactorWeightEngineWeightProvider` | Function | `internal/monitoring/service/weight_provider.go` | 18 |
| `TestMetricsCollector_Screening` | Function | `internal/monitoring/metrics_test.go` | 6 |
| `TestMetricsCollector_Alerts` | Function | `internal/monitoring/metrics_test.go` | 22 |
| `TestMetricsCollector_Snapshot` | Function | `internal/monitoring/metrics_test.go` | 46 |
| `TestCheckThresholds` | Function | `internal/monitoring/metrics_test.go` | 66 |
| `NewMetricsCollector` | Function | `internal/monitoring/metrics.go` | 47 |
| `NewTradingMetrics` | Function | `internal/monitoring/metrics.go` | 208 |
| `DefaultAlertThreshold` | Function | `internal/monitoring/metrics.go` | 310 |
| `TestAlertAPI_ListAlerts` | Function | `internal/monitoring/alert_api_test.go` | 54 |
| `TestAlertAPI_ListAlerts_Empty` | Function | `internal/monitoring/alert_api_test.go` | 78 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `RunLiveTrading → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → ValidateRegistry` | cross_community | 5 |
| `RunLiveTrading → Policy` | cross_community | 5 |
| `RunLiveTrading → ExecutionPolicyFromConstraints` | cross_community | 5 |
| `HandleDailySummary → CalculateVaR` | cross_community | 5 |
| `Main → CompositeMacroProvider` | cross_community | 5 |
| `Main → YahooFinanceMacroProvider` | cross_community | 5 |
| `RunLiveTrading → PRISMManager` | cross_community | 4 |
| `RunLiveTrading → PRISMConfig` | cross_community | 4 |
| `RunLiveTrading → MiroFishSwarm` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Narrative | 12 calls |
| Live | 8 calls |
| Industry | 6 calls |
| Marketdata | 5 calls |
| Atlas | 5 calls |
| Orchestrator | 3 calls |
| Ledger | 2 calls |
| Portfolio | 1 calls |

## How to Explore

1. `gitnexus_context({name: "NewWave9Observability"})` — see callers and callees
2. `gitnexus_query({query: "monitoring"})` — find related execution flows
3. Read key files listed above for implementation details
