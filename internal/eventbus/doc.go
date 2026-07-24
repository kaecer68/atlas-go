// Package eventbus provides a channel-based publish/subscribe event bus
// with SSE (Server-Sent Events) broadcasting support, carrying 42 system
// event types across the atlas-go runtime.
//
// Core components:
//
//	EventBus          — Interface: Publish / Subscribe / SubscribeAll / Close
//	ChannelEventBus   — Default implementation with single dispatcher goroutine
//	SSEBridge         — SSE client broadcast bridge
//	BusEvent          — Standard envelope: ID / Type / Timestamp / Payload / Severity
//	Subscription      — Handle with Cancel function
//
// Dispatch flow:
//
//	Publisher → ChannelEventBus.Publish()
//	  → buffered channel (drop-on-full warning)
//	  → dispatcher goroutine
//	    → per-handler goroutine (panic recovery + 30s timeout)
//
// Subscribing without a type filter (SubscribeAll) receives every event; the
// handler must inspect EventType itself. Each handler runs in its own
// goroutine; if it doesn't finish within 30s it is forcibly terminated and
// the event may be only partially processed.
//
// Publish is fire-and-forget: write failures are logged internally but the
// caller has no error path to detect loss. SSEBridge broadcasts use an
// oldest-event-drop policy when a client is slow, so clients may miss events.
// Subscribers MUST call Cancel() on disconnect to avoid goroutine leaks
// from blocked senders.
//
// EnrichEvent only enriches payloads of type map[string]any; other payload
// types only get the base description in event metadata.
//
// Package independence: This package is a pure pub/sub infrastructure layer
// and is NOT related to eventdriven (capital-flow prediction) or eventquality
// (event data validation) despite the shared "event" prefix. New code should
// not import eventdriven or eventquality via eventbus; the three packages have
// disjoint responsibilities and no shared types.
//
// Maturity: stable
package eventbus
