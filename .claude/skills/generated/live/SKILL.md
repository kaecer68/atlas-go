---
name: live
description: "Skill for the Live area of atlas. 175 symbols across 37 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Live

175 symbols | 37 files | Cohesion: 73%

## When to Use

- Working with code in `internal/live/`
- Understanding how live orchestration publishes position updates via `PublishPositionUpdate`
- Modifying live-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/live/orchestrator.go` | executeOrder, checkRiskTriggers, SetBroker, DefaultOrchestratorConfig, SetTradingMetrics, publishRiskEvent, ExecuteOrder (+20) |
| `internal/live/store.go` | NewStateStore, Save, UpdatePosition, writeFileAtomic, Load (+13) |
| `internal/live/http_adapter_test.go` | TestHTTPBrokerAdapterSubmitOrderSuccess, TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion, expectedHMACSignature, TestHTTPBrokerAdapterNoRetryOnBadRequest, TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault (+11) |
| `internal/live/http_adapter.go` | Sign, NewHTTPBrokerAdapter, SubmitOrder, validateSignerConfig, validateClockSkew (+10) |
| `internal/live/circuit_breaker.go` | DefaultCircuitBreakerRules, NewCircuitBreaker, SetRules, State, ResetDayState (+7) |
| `internal/monitoring/api/live/handlers.go` | writeJSON, writeJSONError, getSymbolSector, computeSectorFactorExposure, HandlePnLAttribution (+5) |
| `internal/live/orchestrator_mode_test.go` | TestResolveBrokerModeLiveUsesGuardedBroker, TestResolveBrokerModeUnknownFallsBackToDryRun, TestResolveBrokerModeLiveUsesMockAdapter, TestResolveBrokerModeLiveHTTPMissingConfigFallsBackToGuarded, TestResolveBrokerModeLiveHTTPConfiguredUsesLiveHTTP (+3) |
| `internal/live/nonce_store_test.go` | TestBuildNonceReplayStoreDefaultsToMemory, TestBuildNonceReplayStoreFileRequiresPath, TestBuildNonceReplayStoreRedisRequiresURLWhenNoClient, TestFileNonceReplayStorePersistsAcrossInstances, TestFileNonceReplayStoreAllowsReuseAfterTTL (+2) |
| `internal/live/circuit_breaker_test.go` | TestCircuitBreakerDailyLossHalt, TestCircuitBreakerDrawdownPause, TestCircuitBreakerConsecutiveStopLossCooldown, TestCircuitBreakerAutoRecoverAfterCooldown, TestCircuitBreakerResetDayState |
| `internal/live/broker.go` | NewDryRunBroker, validateOrder, NewGuardedLiveBroker, SubmitOrder |
| `internal/live/eventbus_publish_test.go` | TestPublishMarketSnapshot, TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestSubscribeAndUnsubscribe, TestSubscribeAll, TestEventBusStats |

## Entry Points

Start here when exploring this area:

- **`NewStateStore`** (Function) — `internal/live/store.go:70`
- **`TestCheckRiskTriggers`** (Function) — `internal/live/orchestrator_test.go:10`
- **`TestPublishPositionUpdate`** (Function) — `internal/live/eventbus_publish_test.go:54`
- **`NewOrchestrator`** (Function) — `internal/live/orchestrator.go:40`
- **`DefaultOrchestratorConfig`** (Function) — `internal/live/orchestrator.go:30`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewStateStore` | Function | `internal/live/store.go` | 70 |
| `TestCheckRiskTriggers` | Function | `internal/live/orchestrator_test.go` | 10 |
| `TestExecuteOrderBlockedByCircuitBreaker` | Function | `internal/live/orchestrator_test.go` | 170 |
| `TestPublishPositionUpdate` | Function | `internal/live/eventbus_publish_test.go` | 54 |
| `TestPublishMarketSnapshot` | Function | `internal/live/eventbus_publish_test.go` | 12 |
| `TestPublishRegimeChange` | Function | `internal/live/eventbus_publish_test.go` | 28 |
| `TestPublishRecommendation` | Function | `internal/live/eventbus_publish_test.go` | 77 |
| `TestSubscribeAndUnsubscribe` | Function | `internal/live/eventbus_publish_test.go` | 99 |
| `TestSubscribeAll` | Function | `internal/live/eventbus_publish_test.go` | 133 |
| `TestEventBusStats` | Function | `internal/live/eventbus_publish_test.go` | 167 |
| `DefaultCircuitBreakerRules` | Function | `internal/live/circuit_breaker.go` | 34 |
| `NewCircuitBreaker` | Function | `internal/live/circuit_breaker.go` | 74 |
| `TestExecuteOrderPublishesFilledEventInDryRunMode` | Function | `internal/live/broker_test.go` | 11 |
| `TestExecuteOrderPublishesSystemErrorWhenOrderInvalid` | Function | `internal/live/broker_test.go` | 70 |
| `NewDryRunBroker` | Function | `internal/live/broker.go` | 32 |
| `TestHTTPBrokerAdapterSubmitOrderSuccess` | Function | `internal/live/http_adapter_test.go` | 19 |
| `TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion` | Function | `internal/live/http_adapter_test.go` | 87 |
| `TestHTTPBrokerAdapterNoRetryOnBadRequest` | Function | `internal/live/http_adapter_test.go` | 135 |
| `TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault` | Function | `internal/live/http_adapter_test.go` | 161 |
| `NewOrchestrator` | Function | `internal/live/orchestrator.go` | 40 |
| `ExecuteOrder` | Method | `internal/live/orchestrator.go` | 240 |

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

1. `gitnexus_context({name: "NewOrchestrator"})` — see callers and callees
2. `gitnexus_query({query: "live"})` — find related execution flows
3. Read key files listed above for implementation details
