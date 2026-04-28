---
name: live
description: "Skill for the Live area of atlas. 170 symbols across 36 files."
---

# Live

170 symbols | 36 files | Cohesion: 73%

## When to Use

- Working with code in `internal/`
- Understanding how TestTaiwanRSSGeopoliticalProvider_FetchScore, Warn, Err work
- Modifying live-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/live/orchestrator.go` | executeOrder, checkRiskTriggers, SetBroker, DefaultOrchestratorConfig, SetTradingMetrics (+19) |
| `internal/live/store.go` | NewStateStore, Save, UpdatePosition, writeFileAtomic, Load (+13) |
| `internal/live/http_adapter_test.go` | TestHTTPBrokerAdapterSubmitOrderSuccess, TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion, expectedHMACSignature, TestHTTPBrokerAdapterNoRetryOnBadRequest, TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault (+11) |
| `internal/live/http_adapter.go` | Sign, NewHTTPBrokerAdapter, SubmitOrder, validateSignerConfig, validateClockSkew (+10) |
| `internal/live/circuit_breaker.go` | DefaultCircuitBreakerRules, NewCircuitBreaker, SetRules, State, ResetDayState (+7) |
| `internal/monitoring/api/live/handlers.go` | writeJSON, writeJSONError, getSymbolSector, computeSectorFactorExposure, HandlePnLAttribution (+5) |
| `internal/live/orchestrator_mode_test.go` | TestResolveBrokerModeLiveUsesGuardedBroker, TestResolveBrokerModeUnknownFallsBackToDryRun, TestResolveBrokerModeLiveUsesMockAdapter, TestResolveBrokerModeLiveHTTPMissingConfigFallsBackToGuarded, TestResolveBrokerModeLiveHTTPConfiguredUsesLiveHTTP (+3) |
| `internal/live/nonce_store_test.go` | TestBuildNonceReplayStoreDefaultsToMemory, TestBuildNonceReplayStoreFileRequiresPath, TestBuildNonceReplayStoreRedisRequiresURLWhenNoClient, TestFileNonceReplayStorePersistsAcrossInstances, TestFileNonceReplayStoreAllowsReuseAfterTTL (+2) |
| `internal/live/circuit_breaker_test.go` | TestCircuitBreakerDailyLossHalt, TestCircuitBreakerDrawdownPause, TestCircuitBreakerConsecutiveStopLossCooldown, TestCircuitBreakerAutoRecoverAfterCooldown, TestCircuitBreakerResetDayState |
| `internal/live/broker.go` | NewDryRunBroker, validateOrder, NewGuardedLiveBroker, SubmitOrder |

## Entry Points

Start here when exploring this area:

- **`TestTaiwanRSSGeopoliticalProvider_FetchScore`** (Function) — `internal/narrative/taiwan_geopolitical_provider_test.go:79`
- **`Warn`** (Function) — `internal/logging/logger.go:51`
- **`Err`** (Function) — `internal/logging/logger.go:81`
- **`NewStateStore`** (Function) — `internal/live/store.go:70`
- **`TestCheckRiskTriggers`** (Function) — `internal/live/orchestrator_test.go:10`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestTaiwanRSSGeopoliticalProvider_FetchScore` | Function | `internal/narrative/taiwan_geopolitical_provider_test.go` | 79 |
| `Warn` | Function | `internal/logging/logger.go` | 51 |
| `Err` | Function | `internal/logging/logger.go` | 81 |
| `NewStateStore` | Function | `internal/live/store.go` | 70 |
| `TestCheckRiskTriggers` | Function | `internal/live/orchestrator_test.go` | 10 |
| `TestExecuteOrderBlockedByCircuitBreaker` | Function | `internal/live/orchestrator_test.go` | 170 |
| `TestCircuitBreakerDailyLossHalt` | Function | `internal/live/circuit_breaker_test.go` | 9 |
| `TestCircuitBreakerDrawdownPause` | Function | `internal/live/circuit_breaker_test.go` | 25 |
| `TestCircuitBreakerConsecutiveStopLossCooldown` | Function | `internal/live/circuit_breaker_test.go` | 44 |
| `TestCircuitBreakerAutoRecoverAfterCooldown` | Function | `internal/live/circuit_breaker_test.go` | 59 |
| `TestCircuitBreakerResetDayState` | Function | `internal/live/circuit_breaker_test.go` | 80 |
| `DefaultCircuitBreakerRules` | Function | `internal/live/circuit_breaker.go` | 34 |
| `NewCircuitBreaker` | Function | `internal/live/circuit_breaker.go` | 74 |
| `TestExecuteOrderPublishesFilledEventInDryRunMode` | Function | `internal/live/broker_test.go` | 11 |
| `TestExecuteOrderPublishesSystemErrorWhenOrderInvalid` | Function | `internal/live/broker_test.go` | 70 |
| `NewDryRunBroker` | Function | `internal/live/broker.go` | 32 |
| `TestHTTPBrokerAdapterSubmitOrderSuccess` | Function | `internal/live/http_adapter_test.go` | 19 |
| `TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion` | Function | `internal/live/http_adapter_test.go` | 87 |
| `TestHTTPBrokerAdapterNoRetryOnBadRequest` | Function | `internal/live/http_adapter_test.go` | 135 |
| `TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault` | Function | `internal/live/http_adapter_test.go` | 161 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → SeedRegistry` | cross_community | 5 |
| `HandleIntradayCycle → Publish` | cross_community | 5 |
| `HandleIntradayCycle → BusEvent` | cross_community | 5 |
| `HandleIntradayCycle → MarketEventPayload` | cross_community | 5 |
| `Start → Warn` | cross_community | 5 |
| `RunEnhancedExperiment → Warn` | cross_community | 5 |
| `Main → Close` | cross_community | 4 |
| `Main → PRISMManager` | cross_community | 4 |
| `Main → PRISMConfig` | cross_community | 4 |
| `Main → MiroFishSwarm` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 12 calls |
| Orchestrator | 9 calls |
| Eventbus | 7 calls |
| Marketdata | 5 calls |
| Industry | 3 calls |
| Narrative | 1 calls |
| Config | 1 calls |
| Prism | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestTaiwanRSSGeopoliticalProvider_FetchScore"})` — see callers and callees
2. `gitnexus_query({query: "live"})` — find related execution flows
3. Read key files listed above for implementation details
