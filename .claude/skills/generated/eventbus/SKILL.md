---
name: eventbus
description: "Skill for the Eventbus area of atlas. 45 symbols across 3 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Eventbus

45 symbols | 3 files | Cohesion: 67%

## When to Use

- Working with code in `internal/eventbus/`
- Understanding how events are published/subscribed, including Wave 9 observability event types
- Modifying eventbus-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/eventbus/eventbus.go` | NewChannelEventBus, Publish, Subscribe, SubscribeAll, SubscribeCritical, Stats, SetEventThrottle, PublishPositionUpdate, PublishRegimeChange, PublishRecommendation, PublishGuardOutcomes, PublishRiskGateEvent, PublishHealthAlert, PublishTradeSlippage, PublishBacktestCompleted, PublishCalibrationCompleted, PublishIndustryCalendarEvent, PublishPromotionRecorded, EnrichEvent, describeEvent, eventDescriptions (+30) |
| `internal/eventbus/eventbus_test.go` | TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestPublishGuardOutcomes, TestEventBusStats, TestSubscribeAndUnsubscribe, TestSubscribeAll (+3) |
| `internal/live/eventbus_publish_test.go` | TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestEventBusStats, TestSubscribeAndUnsubscribe, TestSubscribeAll (+1) |

## Wave 9 Event Types

| Constant | Description |
|----------|-------------|
| `EventChannelIndividualHealth` | Per-channel health status change |
| `EventRegimeChangeConfirmed` | Regime change stabilized for 30s |
| `EventFactorWeightRegression` | Factor-weight regression detected after regime change |
| `EventDriftDetected` | Portfolio concentration / turnover / target-weight drift |
| `EventIngestionLagSpike` | API Gateway p99 ingestion latency spike |

## Entry Points

Start here when exploring this area:

- **`NewChannelEventBus`** (Function) — `internal/eventbus/eventbus.go:495`
- **`PublishPositionUpdate`** (Method) — `internal/eventbus/eventbus.go:633`
- **`TestPublishPositionUpdate`** (Function) — `internal/live/eventbus_publish_test.go:54`
- **`TestPublishRegimeChange`** (Function) — `internal/eventbus/eventbus_test.go:33`
- **`EnrichEvent`** (Function) — `internal/eventbus/eventbus.go:338`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewChannelEventBus` | Function | `internal/eventbus/eventbus.go` | 495 |
| `Publish` | Method | `internal/eventbus/eventbus.go` | 512 |
| `Subscribe` | Method | `internal/eventbus/eventbus.go` | 960 |
| `SubscribeAll` | Method | `internal/eventbus/eventbus.go` | 980 |
| `SubscribeCritical` | Method | `internal/eventbus/eventbus.go` | 998 |
| `Stats` | Method | `internal/eventbus/eventbus.go` | 1135 |
| `SetEventThrottle` | Method | `internal/eventbus/eventbus.go` | 556 |
| `EnrichEvent` | Function | `internal/eventbus/eventbus.go` | 338 |
| `BusEvent` | Struct | `internal/eventbus/eventbus.go` | 325 |
| `EventBus` | Interface | `internal/eventbus/eventbus.go` | 447 |
| `ChannelEventBus` | Struct | `internal/eventbus/eventbus.go` | 463 |
| `Subscription` | Struct | `internal/eventbus/eventbus.go` | 456 |
| `EventType` | Type | `internal/eventbus/eventbus.go` | 16 |
| `EventPositionUpdate` | Const | `internal/eventbus/eventbus.go` | 29 |
| `EventRegimeChange` | Const | `internal/eventbus/eventbus.go` | 26 |
| `EventRegimeChangeConfirmed` | Const | `internal/eventbus/eventbus.go` | 93 |
| `EventFactorWeightRegression` | Const | `internal/eventbus/eventbus.go` | 94 |
| `EventDriftDetected` | Const | `internal/eventbus/eventbus.go` | 95 |
| `EventIngestionLagSpike` | Const | `internal/eventbus/eventbus.go` | 96 |
| `EventChannelIndividualHealth` | Const | `internal/eventbus/eventbus.go` | 92 |
| `EventMarketSnapshot` | Const | `internal/eventbus/eventbus.go` | 20 |
| `EventAgentRecommendation` | Const | `internal/eventbus/eventbus.go` | 33 |
| `EventGuardOutcome` | Const | `internal/eventbus/eventbus.go` | 47 |
| `EventRiskGateRejected` | Const | `internal/eventbus/eventbus.go` | 78 |
| `EventRiskGateAllowed` | Const | `internal/eventbus/eventbus.go` | 79 |
| `EventRiskGateOverridden` | Const | `internal/eventbus/eventbus.go` | 80 |
| `PublishPositionUpdate` | Method | `internal/eventbus/eventbus.go` | 633 |
| `PublishRegimeChange` | Method | `internal/eventbus/eventbus.go` | 617 |
| `PublishRecommendation` | Method | `internal/eventbus/eventbus.go` | 648 |
| `PublishGuardOutcomes` | Method | `internal/eventbus/eventbus.go` | 662 |
| `PublishRiskGateEvent` | Method | `internal/eventbus/eventbus.go` | 880 |
| `PublishHealthAlert` | Method | `internal/eventbus/eventbus.go` | 858 |
| `PublishTradeSlippage` | Method | `internal/eventbus/eventbus.go` | 938 |
| `PublishBacktestCompleted` | Method | `internal/eventbus/eventbus.go` | 912 |
| `PublishCalibrationCompleted` | Method | `internal/eventbus/eventbus.go` | 925 |
| `PublishIndustryCalendarEvent` | Method | `internal/eventbus/eventbus.go` | 899 |
| `PublishPromotionRecorded` | Method | `internal/eventbus/eventbus.go` | 949 |
| `TestPublishPositionUpdate` | Function | `internal/live/eventbus_publish_test.go` | 54 |
| `TestPublishRegimeChange` | Function | `internal/live/eventbus_publish_test.go` | 28 |
| `TestPublishRecommendation` | Function | `internal/live/eventbus_publish_test.go` | 77 |
| `TestEventBusStats` | Function | `internal/live/eventbus_publish_test.go` | 167 |
| `TestSubscribeAndUnsubscribe` | Function | `internal/live/eventbus_publish_test.go` | 99 |
| `TestSubscribeAll` | Function | `internal/live/eventbus_publish_test.go` | 133 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleIntradayCycle → Publish` | cross_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 4 calls |
| Portfolio | 1 calls |
| Orchestrator | 1 calls |

## How to Explore

1. `gitnexus_context({name: "NewChannelEventBus"})` — see callers and callees
2. `gitnexus_query({query: "eventbus"})` — find related execution flows
3. Read key files listed above for implementation details
