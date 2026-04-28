---
name: monitoring
description: "Skill for the Monitoring area of atlas. 200 symbols across 36 files."
---

# Monitoring

200 symbols | 36 files | Cohesion: 77%

## When to Use

- Working with code in `internal/`
- Understanding how TestMetricsCollector_RecordCounter_Accumulates, TestMetricsCollector_RecordGauge_Overwrites, TestMetricsCollector_RecordHistogram work
- Modifying monitoring-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/monitoring/dashboard_api.go` | NewDashboardAPI, RegisterRoutes, RegisterIndustryRoutes, RegisterNarrativeRoutes, RegisterControlRoutes (+20) |
| `internal/monitoring/metrics.go` | NewMetricsCollector, RecordCounter, RecordGauge, RecordHistogram, GetMetric (+12) |
| `internal/monitoring/monitoring_test.go` | TestMetricsCollector_RecordCounter_Accumulates, TestMetricsCollector_RecordGauge_Overwrites, TestMetricsCollector_RecordHistogram, TestMetricsCollector_GetAllMetrics, TestTradingMetrics_RecordOrder (+9) |
| `internal/monitoring/alert_api_test.go` | newTestAlertAPI, seedAlerts, TestAlertAPI_ListAlerts, TestAlertAPI_ListAlerts_Empty, TestAlertAPI_ListAlerts_MethodNotAllowed (+9) |
| `internal/monitoring/monitor.go` | Info, Warning, Error, Critical, SetAlertStore (+8) |
| `internal/monitoring/notifier_test.go` | TestWebhookNotifier_IsConfigured, TestWebhookNotifier_Name, TestWebhookNotifier_Notify_NotConfigured, TestWebhookNotifier_Notify_Success, TestWebhookNotifier_Notify_CustomHeaders (+8) |
| `internal/monitoring/alert_store_test.go` | newTestStore, makeAlert, TestAlertStore_SaveAndLoadAll, TestAlertStore_LoadAll_EmptyFile, TestAlertStore_LoadAll_NonExistentFile (+7) |
| `internal/monitoring/notifier.go` | NewWebhookNotifier, Name, IsConfigured, Notify, NewTelegramNotifier (+3) |
| `internal/monitoring/dashboard_helpers.go` | sessionDateFromID, LoadSessionSummary, writeJSON, writeJSONError, isStockPickingLayer (+3) |
| `internal/monitoring/alert_store.go` | Save, LoadAll, LoadUnacknowledged, Acknowledge, loadFromFile (+2) |

## Entry Points

Start here when exploring this area:

- **`TestMetricsCollector_RecordCounter_Accumulates`** (Function) — `internal/monitoring/monitoring_test.go:124`
- **`TestMetricsCollector_RecordGauge_Overwrites`** (Function) — `internal/monitoring/monitoring_test.go:144`
- **`TestMetricsCollector_RecordHistogram`** (Function) — `internal/monitoring/monitoring_test.go:159`
- **`TestMetricsCollector_GetAllMetrics`** (Function) — `internal/monitoring/monitoring_test.go:172`
- **`TestTradingMetrics_RecordOrder`** (Function) — `internal/monitoring/monitoring_test.go:360`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestMetricsCollector_RecordCounter_Accumulates` | Function | `internal/monitoring/monitoring_test.go` | 124 |
| `TestMetricsCollector_RecordGauge_Overwrites` | Function | `internal/monitoring/monitoring_test.go` | 144 |
| `TestMetricsCollector_RecordHistogram` | Function | `internal/monitoring/monitoring_test.go` | 159 |
| `TestMetricsCollector_GetAllMetrics` | Function | `internal/monitoring/monitoring_test.go` | 172 |
| `TestTradingMetrics_RecordOrder` | Function | `internal/monitoring/monitoring_test.go` | 360 |
| `TestMetricsCollector_Screening` | Function | `internal/monitoring/metrics_test.go` | 6 |
| `TestMetricsCollector_Alerts` | Function | `internal/monitoring/metrics_test.go` | 22 |
| `TestMetricsCollector_Snapshot` | Function | `internal/monitoring/metrics_test.go` | 46 |
| `TestCheckThresholds` | Function | `internal/monitoring/metrics_test.go` | 66 |
| `NewMetricsCollector` | Function | `internal/monitoring/metrics.go` | 47 |
| `NewTradingMetrics` | Function | `internal/monitoring/metrics.go` | 208 |
| `DefaultAlertThreshold` | Function | `internal/monitoring/metrics.go` | 310 |
| `TestAlertAPI_ListAlerts` | Function | `internal/monitoring/alert_api_test.go` | 54 |
| `TestAlertAPI_ListAlerts_Empty` | Function | `internal/monitoring/alert_api_test.go` | 78 |
| `TestAlertAPI_ListAlerts_MethodNotAllowed` | Function | `internal/monitoring/alert_api_test.go` | 104 |
| `TestAlertAPI_Unacknowledged` | Function | `internal/monitoring/alert_api_test.go` | 116 |
| `TestAlertAPI_Unacknowledged_MethodNotAllowed` | Function | `internal/monitoring/alert_api_test.go` | 145 |
| `TestAlertAPI_Acknowledge` | Function | `internal/monitoring/alert_api_test.go` | 157 |
| `TestAlertAPI_Acknowledge_NotFound` | Function | `internal/monitoring/alert_api_test.go` | 193 |
| `TestAlertAPI_Acknowledge_MissingAlertID` | Function | `internal/monitoring/alert_api_test.go` | 206 |

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

1. `gitnexus_context({name: "TestMetricsCollector_RecordCounter_Accumulates"})` — see callers and callees
2. `gitnexus_query({query: "monitoring"})` — find related execution flows
3. Read key files listed above for implementation details
