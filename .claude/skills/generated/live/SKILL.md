---
name: live
description: "Skill for the Live area of atlas. 177 symbols across 31 files."
---

# Live

177 symbols | 31 files | Cohesion: 75%

## When to Use

- Working with code in `internal/`
- Understanding how TestOrderManagerRetriesThenPublishesFilled, TestOrderManagerPublishRejectedWithReason, TestOrderManagerPublishSystemErrorAfterRetryExhausted work
- Modifying live-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/live/orchestrator.go` | NewOrchestrator, SetBroker, executeOrder, checkRiskTriggers, Start (+20) |
| `internal/live/store.go` | NewStateStore, Save, GetPosition, UpdatePosition, writeFileAtomic (+15) |
| `internal/live/eventbus.go` | NewChannelEventBus, Publish, PublishMarketSnapshot, PublishRegimeChange, PublishPositionUpdate (+11) |
| `internal/live/http_adapter_test.go` | TestHTTPBrokerAdapterSubmitOrderSuccess, TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion, expectedHMACSignature, TestHTTPBrokerAdapterNoRetryOnBadRequest, TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault (+11) |
| `internal/live/http_adapter.go` | Sign, Error, NewHTTPBrokerAdapter, SubmitOrder, validateSignerConfig (+11) |
| `internal/live/circuit_breaker.go` | DefaultCircuitBreakerRules, NewCircuitBreaker, SetRules, State, ResetDayState (+7) |
| `internal/live/nonce_store_test.go` | TestRedisNonceReplayStoreRejectsReplayAcrossInstances, TestRedisNonceReplayStoreAllowsReuseAfterTTL, TestBuildNonceReplayStoreDefaultsToMemory, TestBuildNonceReplayStoreFileRequiresPath, TestBuildNonceReplayStoreRedisRequiresURLWhenNoClient (+4) |
| `internal/live/orchestrator_mode_test.go` | TestNewOrchestratorAppliesLiveGuardedMode, TestNewOrchestratorAppliesLiveHTTPFileNonceStoreConfig, TestResolveBrokerModeLiveUsesGuardedBroker, TestResolveBrokerModeUnknownFallsBackToDryRun, TestResolveBrokerModeLiveUsesMockAdapter (+3) |
| `internal/live/eventbus_publish_test.go` | TestPublishMarketSnapshot, TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestSubscribeAndUnsubscribe (+2) |
| `internal/live/nonce_store.go` | NewRedisNonceReplayStore, NewFileNonceReplayStore, BuildNonceReplayStore, BuildNonceReplayStoreWithOptions, NewInMemoryNonceReplayStore |

## Entry Points

Start here when exploring this area:

- **`TestOrderManagerRetriesThenPublishesFilled`** (Function) — `internal/live/order_manager_test.go:34`
- **`TestOrderManagerPublishRejectedWithReason`** (Function) — `internal/live/order_manager_test.go:77`
- **`TestOrderManagerPublishSystemErrorAfterRetryExhausted`** (Function) — `internal/live/order_manager_test.go:123`
- **`TestOrderManagerPublishesSignerErrorClassification`** (Function) — `internal/live/order_manager_test.go:162`
- **`NewOrderManager`** (Function) — `internal/live/order_manager.go:18`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestOrderManagerRetriesThenPublishesFilled` | Function | `internal/live/order_manager_test.go` | 34 |
| `TestOrderManagerPublishRejectedWithReason` | Function | `internal/live/order_manager_test.go` | 77 |
| `TestOrderManagerPublishSystemErrorAfterRetryExhausted` | Function | `internal/live/order_manager_test.go` | 123 |
| `TestOrderManagerPublishesSignerErrorClassification` | Function | `internal/live/order_manager_test.go` | 162 |
| `NewOrderManager` | Function | `internal/live/order_manager.go` | 18 |
| `TestNewOrchestratorAppliesLiveGuardedMode` | Function | `internal/live/orchestrator_mode_test.go` | 132 |
| `TestNewOrchestratorAppliesLiveHTTPFileNonceStoreConfig` | Function | `internal/live/orchestrator_mode_test.go` | 198 |
| `NewOrchestrator` | Function | `internal/live/orchestrator.go` | 118 |
| `TestRedisNonceReplayStoreRejectsReplayAcrossInstances` | Function | `internal/live/nonce_store_test.go` | 96 |
| `TestRedisNonceReplayStoreAllowsReuseAfterTTL` | Function | `internal/live/nonce_store_test.go` | 124 |
| `NewRedisNonceReplayStore` | Function | `internal/live/nonce_store.go` | 116 |
| `TestGuardedToHTTPFlowIntegration` | Function | `internal/live/http_flow_integration_test.go` | 16 |
| `TestHTTPFlowIntegrationRejectsClockSkew` | Function | `internal/live/http_flow_integration_test.go` | 90 |
| `TestPublishMarketSnapshot` | Function | `internal/live/eventbus_publish_test.go` | 11 |
| `TestPublishRegimeChange` | Function | `internal/live/eventbus_publish_test.go` | 33 |
| `TestPublishPositionUpdate` | Function | `internal/live/eventbus_publish_test.go` | 57 |
| `TestPublishRecommendation` | Function | `internal/live/eventbus_publish_test.go` | 78 |
| `TestSubscribeAndUnsubscribe` | Function | `internal/live/eventbus_publish_test.go` | 99 |
| `TestSubscribeAll` | Function | `internal/live/eventbus_publish_test.go` | 128 |
| `TestEventBusStats` | Function | `internal/live/eventbus_publish_test.go` | 152 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleIntradayCycle → Publish` | cross_community | 5 |
| `HandleIntradayCycle → BusEvent` | cross_community | 5 |
| `HandleIntradayCycle → MarketEventPayload` | cross_community | 5 |
| `Main → Close` | cross_community | 5 |
| `Main → Close` | cross_community | 5 |
| `Main → Close` | cross_community | 5 |
| `Main → Close` | cross_community | 4 |
| `Main → SeedRegistry` | cross_community | 4 |
| `Main → PRISMManager` | cross_community | 4 |
| `Main → PRISMConfig` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 12 calls |
| Orchestrator | 11 calls |
| Config | 1 calls |
| Marketdata | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestOrderManagerRetriesThenPublishesFilled"})` — see callers and callees
2. `gitnexus_query({query: "live"})` — find related execution flows
3. Read key files listed above for implementation details
