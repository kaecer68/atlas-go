---
name: monitoring
description: "Skill for the Monitoring area of atlas. 268 symbols across 43 files."
---

# Monitoring

268 symbols | 43 files | Cohesion: 78%

## When to Use

- Working with code in `internal/`
- Understanding how TestBuildAgentRows, BuildAgentRows, LoadTWSEOpenDataCSV work
- Modifying monitoring-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/monitoring/dashboard_api.go` | handleExperimentPromote, handleExperimentRevert, promotionHistoryToAPI, handleExperimentHistory, handlePhase3Status (+65) |
| `internal/monitoring/metrics.go` | NewMetricsCollector, RecordCounter, RecordGauge, RecordHistogram, GetMetric (+11) |
| `internal/monitoring/monitoring_test.go` | TestMetricsCollector_RecordCounter_Accumulates, TestMetricsCollector_RecordGauge_Overwrites, TestMetricsCollector_RecordHistogram, TestMetricsCollector_GetAllMetrics, TestTradingMetrics_RecordOrder (+9) |
| `internal/monitoring/alert_api_test.go` | newTestAlertAPI, seedAlerts, TestAlertAPI_ListAlerts, TestAlertAPI_ListAlerts_Empty, TestAlertAPI_ListAlerts_MethodNotAllowed (+9) |
| `internal/monitoring/monitor.go` | Info, Warning, Error, Critical, SetAlertStore (+8) |
| `internal/monitoring/notifier_test.go` | TestWebhookNotifier_IsConfigured, TestWebhookNotifier_Name, TestWebhookNotifier_Notify_NotConfigured, TestWebhookNotifier_Notify_Success, TestWebhookNotifier_Notify_CustomHeaders (+8) |
| `internal/monitoring/alert_store_test.go` | newTestStore, makeAlert, TestAlertStore_SaveAndLoadAll, TestAlertStore_LoadAll_EmptyFile, TestAlertStore_LoadAll_NonExistentFile (+7) |
| `internal/ledger/ledger.go` | NewStore, LoadOutcomes, RecordExperiment, RecordSessionExperiment, RecordSessionSummary (+5) |
| `internal/ledger/ledger_persistence_test.go` | TestRecordAndLoadOutcomes, TestLoadOutcomesMissingReturnsNil, TestRecordExperimentJSONL, TestRecordSessionExperiment, TestRecordAndLoadSpawnRecords (+3) |
| `internal/monitoring/channel_health.go` | NewChannelHealthStore, NewChannelHealthStoreWithPool, Record, Get, Alerts (+2) |

## Entry Points

Start here when exploring this area:

- **`TestBuildAgentRows`** (Function) — `internal/reporting/reporting_test.go:81`
- **`BuildAgentRows`** (Function) — `internal/reporting/agent_table.go:47`
- **`LoadTWSEOpenDataCSV`** (Function) — `internal/replay/twse_csv.go:20`
- **`NewGeopoliticalStore`** (Function) — `internal/narrative/geopolitical_store.go:15`
- **`NewChannelHealthStore`** (Function) — `internal/monitoring/channel_health.go:31`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestBuildAgentRows` | Function | `internal/reporting/reporting_test.go` | 81 |
| `BuildAgentRows` | Function | `internal/reporting/agent_table.go` | 47 |
| `LoadTWSEOpenDataCSV` | Function | `internal/replay/twse_csv.go` | 20 |
| `NewGeopoliticalStore` | Function | `internal/narrative/geopolitical_store.go` | 15 |
| `NewChannelHealthStore` | Function | `internal/monitoring/channel_health.go` | 31 |
| `NewChannelHealthStoreWithPool` | Function | `internal/monitoring/channel_health.go` | 36 |
| `RecordChannelFetch` | Function | `internal/monitoring/channel_health.go` | 256 |
| `RecordChannelFetchWithPool` | Function | `internal/monitoring/channel_health.go` | 261 |
| `NewTWSERetailSentimentProvider` | Function | `internal/marketdata/retail_sentiment_provider.go` | 20 |
| `TestApplyHumanOverridesFiltersPausedAgentsAndBannedSectors` | Function | `internal/orchestrator/human_override_test.go` | 9 |
| `TestApplyHumanOverridesResumesAgentAndUnbansSector` | Function | `internal/orchestrator/human_override_test.go` | 56 |
| `TestRecordSessionSummaryPersistsTraceIDs` | Function | `internal/ledger/ledger_test.go` | 31 |
| `TestRecordAndLoadOutcomes` | Function | `internal/ledger/ledger_persistence_test.go` | 11 |
| `TestLoadOutcomesMissingReturnsNil` | Function | `internal/ledger/ledger_persistence_test.go` | 55 |
| `TestRecordExperimentJSONL` | Function | `internal/ledger/ledger_persistence_test.go` | 101 |
| `TestRecordSessionExperiment` | Function | `internal/ledger/ledger_persistence_test.go` | 120 |
| `TestRecordAndLoadSpawnRecords` | Function | `internal/ledger/ledger_persistence_test.go` | 161 |
| `TestRecordAndLoadHumanInterventions` | Function | `internal/ledger/ledger_persistence_test.go` | 189 |
| `TestLoadSessionSummaries` | Function | `internal/ledger/ledger_persistence_test.go` | 218 |
| `TestLoadSessionSummariesMissingReturnsNil` | Function | `internal/ledger/ledger_persistence_test.go` | 246 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → Dataset` | cross_community | 6 |
| `Main → Value` | cross_community | 6 |
| `RunLiveTrading → SeedRegistry` | cross_community | 5 |
| `RunLiveTrading → ValidateRegistry` | cross_community | 5 |
| `RunLiveTrading → Policy` | cross_community | 5 |
| `HandleChannelsIngest → NarrativeEvent` | cross_community | 5 |
| `HandleChannelsIngest → HitRateForTheme` | cross_community | 5 |
| `HandleChannelsIngest → BoolToFloat` | cross_community | 5 |
| `HandleDailySummary → CalculateVaR` | cross_community | 5 |
| `HandleMacroIngest → NarrativeEvent` | cross_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 16 calls |
| Narrative | 14 calls |
| Orchestrator | 8 calls |
| Ledger | 7 calls |
| Marketdata | 6 calls |
| Baseline | 6 calls |
| Experiment | 4 calls |
| Atlas | 4 calls |

## How to Explore

1. `gitnexus_context({name: "TestBuildAgentRows"})` — see callers and callees
2. `gitnexus_query({query: "monitoring"})` — find related execution flows
3. Read key files listed above for implementation details
