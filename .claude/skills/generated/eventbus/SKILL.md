---
name: eventbus
description: "Skill for the Eventbus area of atlas. 25 symbols across 3 files."
---

# Eventbus

25 symbols | 3 files | Cohesion: 67%

## When to Use

- Working with code in `internal/`
- Understanding how TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation work
- Modifying eventbus-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/eventbus/eventbus.go` | PublishRegimeChange, PublishPositionUpdate, PublishRecommendation, Subscribe, unsubscribe (+7) |
| `internal/eventbus/eventbus_test.go` | TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestPublishGuardOutcomes, TestEventBusStats (+2) |
| `internal/live/eventbus_publish_test.go` | TestPublishRegimeChange, TestPublishPositionUpdate, TestPublishRecommendation, TestEventBusStats, TestSubscribeAndUnsubscribe (+1) |

## Entry Points

Start here when exploring this area:

- **`TestPublishRegimeChange`** (Function) — `internal/live/eventbus_publish_test.go:33`
- **`TestPublishPositionUpdate`** (Function) — `internal/live/eventbus_publish_test.go:57`
- **`TestPublishRecommendation`** (Function) — `internal/live/eventbus_publish_test.go:78`
- **`TestEventBusStats`** (Function) — `internal/live/eventbus_publish_test.go:152`
- **`TestPublishRegimeChange`** (Function) — `internal/eventbus/eventbus_test.go:33`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestPublishRegimeChange` | Function | `internal/live/eventbus_publish_test.go` | 33 |
| `TestPublishPositionUpdate` | Function | `internal/live/eventbus_publish_test.go` | 57 |
| `TestPublishRecommendation` | Function | `internal/live/eventbus_publish_test.go` | 78 |
| `TestEventBusStats` | Function | `internal/live/eventbus_publish_test.go` | 152 |
| `TestPublishRegimeChange` | Function | `internal/eventbus/eventbus_test.go` | 33 |
| `TestPublishPositionUpdate` | Function | `internal/eventbus/eventbus_test.go` | 57 |
| `TestPublishRecommendation` | Function | `internal/eventbus/eventbus_test.go` | 78 |
| `TestPublishGuardOutcomes` | Function | `internal/eventbus/eventbus_test.go` | 99 |
| `TestEventBusStats` | Function | `internal/eventbus/eventbus_test.go` | 182 |
| `TestSubscribeAndUnsubscribe` | Function | `internal/live/eventbus_publish_test.go` | 99 |
| `TestSubscribeAll` | Function | `internal/live/eventbus_publish_test.go` | 128 |
| `TestSubscribeAndUnsubscribe` | Function | `internal/eventbus/eventbus_test.go` | 129 |
| `TestSubscribeAll` | Function | `internal/eventbus/eventbus_test.go` | 158 |
| `NewChannelEventBus` | Function | `internal/eventbus/eventbus.go` | 144 |
| `PublishRegimeChange` | Method | `internal/eventbus/eventbus.go` | 187 |
| `PublishPositionUpdate` | Method | `internal/eventbus/eventbus.go` | 202 |
| `PublishRecommendation` | Method | `internal/eventbus/eventbus.go` | 216 |
| `Subscribe` | Method | `internal/eventbus/eventbus.go` | 284 |
| `Stats` | Method | `internal/eventbus/eventbus.go` | 403 |
| `Publish` | Method | `internal/eventbus/eventbus.go` | 161 |

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

1. `gitnexus_context({name: "TestPublishRegimeChange"})` — see callers and callees
2. `gitnexus_query({query: "eventbus"})` — find related execution flows
3. Read key files listed above for implementation details
