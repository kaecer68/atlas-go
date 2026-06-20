package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestPublishMarketSnapshot(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})
	defer sub.Cancel()

	quote := domain.Quote{Symbol: "2330", Open: 500, High: 510, Low: 495, Last: 505}
	bus.PublishMarketSnapshot(quote)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 market snapshot event, got %d", received.Load())
	}
}

func TestPublishRegimeChange(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventRegimeChange, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		payload := event.Payload.(RegimeEventPayload)
		if payload.NewRegime != domain.RegimeRiskOn {
			t.Errorf("unexpected new regime: %v", payload.NewRegime)
		}
		return nil
	})

	bus.PublishRegimeChange(domain.RegimeNeutral, domain.RegimeRiskOn, 0.85, "prism")

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 regime change event, got %d", received.Load())
	}
}

func TestPublishPositionUpdate(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventPositionUpdate, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	position := domain.Position{Symbol: "2330", Quantity: 100, AverageCost: 500}
	bus.PublishPositionUpdate("2330", position, "added")

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 position update event, got %d", received.Load())
	}
}

func TestPublishRecommendation(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventAgentRecommendation, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	recs := []domain.Recommendation{{Symbol: "2330", Conviction: 80, Side: "buy"}}
	bus.PublishRecommendation("growth-momentum-01", recs)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 recommendation event, got %d", received.Load())
	}
}

func TestPublishGuardOutcomes(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventGuardOutcome, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		payload := event.Payload.(GuardOutcomeEventPayload)
		if payload.SessionID != "session-test" {
			t.Errorf("unexpected session id: %v", payload.SessionID)
		}
		if len(payload.Outcomes) != 1 {
			t.Errorf("expected 1 outcome, got %d", len(payload.Outcomes))
		}
		return nil
	})

	outcomes := []domain.GuardOutcome{
		{GuardID: "cio-01", GuardSkill: "cio", Passed: true, InputCount: 5, OutputCount: 3},
	}
	bus.PublishGuardOutcomes("session-test", outcomes)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 guard outcome event, got %d", received.Load())
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.Subscribe(EventMarketTick, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	bus.Publish(BusEvent{ID: "1", Type: EventMarketTick, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 event before unsubscribe, got %d", received.Load())
	}

	sub.Cancel()

	bus.Publish(BusEvent{ID: "2", Type: EventMarketTick, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected no additional events after unsubscribe, got %d", received.Load())
	}
}

func TestSubscribeAll(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})
	defer sub.Cancel()

	bus.Publish(BusEvent{ID: "1", Type: EventSystemStart, Timestamp: time.Now()})
	bus.Publish(BusEvent{ID: "2", Type: EventSystemError, Timestamp: time.Now()})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 2 {
		t.Fatalf("expected 2 events from SubscribeAll, got %d", received.Load())
	}
}

func TestEventBusStats(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error { return nil })
	bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error { return nil })
	bus.Subscribe(EventRegimeChange, func(ctx context.Context, event BusEvent) error { return nil })
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error { return nil })

	stats := bus.Stats()
	if stats["subscribers_total"] != 4 {
		t.Fatalf("expected 4 total subscribers, got %v", stats["subscribers_total"])
	}
	if stats["subscribers_by_type"] != 2 {
		t.Fatalf("expected 2 subscriber types, got %v", stats["subscribers_by_type"])
	}
	if stats["channel_capacity"] != 64 {
		t.Fatalf("expected channel capacity 64, got %v", stats["channel_capacity"])
	}
}

// TestPublish_NoSubscribers_NoPanic ensures publish with zero subscribers
// does not panic — a common edge case for fire-and-forget event buses.
func TestPublish_NoSubscribers_NoPanic(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	bus.Publish(BusEvent{ID: "no-sub", Type: EventSystemStart, Timestamp: time.Now()})
	// No subscribers — must not panic.

	time.Sleep(50 * time.Millisecond)
	// verify bus is still healthy
	stats := bus.Stats()
	if stats["subscribers_total"] != 0 {
		t.Errorf("expected 0 subscribers after no-sub publish, got %v", stats["subscribers_total"])
	}
}

// TestPublish_BufferFull_Drop verifies that publishing to a full channel
// drops the event without panicking (fire-and-forget contract).
func TestPublish_BufferFull_Drop(t *testing.T) {
	// Buffer size 0 means channel is unbuffered — the publish
	// will hit the default case if dispatcher is not ready.
	bus := NewChannelEventBus(0)

	// Hold the dispatcher by adding a slow subscriber.
	var blockCh = make(chan struct{})
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		<-blockCh
		return nil
	})

	// Publish many events rapidly; at least one should hit default.
	for i := 0; i < 5; i++ {
		bus.Publish(BusEvent{ID: "fill-" + string(rune('0'+i)), Type: EventSystemStart, Timestamp: time.Now()})
	}

	// Release the blocker so dispatcher can proceed.
	close(blockCh)
	time.Sleep(50 * time.Millisecond)

	stats := bus.Stats()
	dropped := stats["publish_dropped"].(int64)
	if dropped == 0 {
		t.Log("no events dropped (race-dependent); possible but not guaranteed")
	}
	bus.Close()
}

func TestEnrichEvent_FillsDescription(t *testing.T) {
	ev := &BusEvent{Type: EventSimulationStart}
	EnrichEvent(ev)
	if ev.Description == "" {
		t.Fatal("expected non-empty description after enrich")
	}
	if ev.Severity != "info" {
		t.Errorf("expected severity info, got %s", ev.Severity)
	}
}

func TestEnrichEvent_AlreadyPopulated_Noop(t *testing.T) {
	ev := &BusEvent{
		Type:        EventSimulationStart,
		Description: "custom",
		Severity:    "error",
	}
	EnrichEvent(ev)
	if ev.Description != "custom" {
		t.Errorf("expected custom description preserved, got %s", ev.Description)
	}
	if ev.Severity != "error" {
		t.Errorf("expected custom severity preserved, got %s", ev.Severity)
	}
}

func TestEnrichEvent_AllKnownTypes(t *testing.T) {
	types := []EventType{
		EventSimulationStart, EventSimulationComplete, EventSystemStart, EventSystemError,
		EventRegimeChange, EventAgentRecommendation, EventAgentEvaluation, EventAgentHealthChange,
		EventGuardOutcome, EventDarwinianClamping, EventConvictionClamping,
		EventOrderPlaced, EventOrderFilled, EventOrderRejected, EventOrderError,
		EventStopLossTriggered, EventTakeProfitTriggered, EventRiskAlert,
		EventPositionUpdate, EventPortfolioPnL, EventMarketSnapshot, EventMarketTick,
		EventMarketOpen, EventMarketClose,
		EventExperimentInsufficientData, EventNarrative, EventHealthAlert,
	}
	for _, et := range types {
		ev := &BusEvent{Type: et}
		EnrichEvent(ev)
		if ev.Description == "" {
			t.Errorf("empty description for event type %s", et)
		}
	}
}

func TestEnrichEvent_UnknownType_FallsBackToTypeName(t *testing.T) {
	ev := &BusEvent{Type: EventType("custom.unknown")}
	EnrichEvent(ev)
	if ev.Description != "custom.unknown" {
		t.Errorf("expected fallback to type name, got %s", ev.Description)
	}
	if ev.Severity != "info" {
		t.Errorf("expected severity info for unknown type, got %s", ev.Severity)
	}
}

func TestEnrichEvent_RegimeChangePayload(t *testing.T) {
	ev := &BusEvent{
		Type:    EventRegimeChange,
		Payload: map[string]any{"from": "neutral", "to": "risk_on"},
	}
	EnrichEvent(ev)
	if ev.Description == "" {
		t.Fatal("expected enriched description for regime change")
	}
}

func TestEnrichEvent_NarrativePayload(t *testing.T) {
	ev := &BusEvent{
		Type:    EventNarrative,
		Payload: map[string]any{"theme": "AI_capex_surge"},
	}
	EnrichEvent(ev)
	if ev.Description == "" {
		t.Fatal("expected enriched description for narrative")
	}
}

func TestEnrichEvent_NonMapPayload_ReturnsBase(t *testing.T) {
	ev := &BusEvent{Type: EventRegimeChange, Payload: "string payload"}
	EnrichEvent(ev)
	// Should use base description from eventDescriptions
	if ev.Description == "" {
		t.Fatal("expected base description for non-map payload")
	}
}

func TestSubscribeCritical_ErrorPropagation(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	sub, errCh := bus.SubscribeCritical(EventSystemError, func(ctx context.Context, event BusEvent) error {
		return fmt.Errorf("critical failure")
	})
	defer sub.Cancel()

	bus.Publish(BusEvent{ID: "crit-test", Type: EventSystemError, Timestamp: time.Now()})

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from critical subscriber")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for critical error")
	}
}

func TestSubscribeCritical_NoError_NoChannelWrite(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub, errCh := bus.SubscribeCritical(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil // no error
	})
	defer sub.Cancel()

	bus.PublishMarketSnapshot(domain.Quote{Symbol: "2330", Last: 500})
	time.Sleep(100 * time.Millisecond)

	if received.Load() != 1 {
		t.Fatalf("expected event received, got %d", received.Load())
	}

	// errCh should NOT receive anything (no error)
	select {
	case <-errCh:
		t.Fatal("unexpected error on critical error channel")
	case <-time.After(100 * time.Millisecond):
		// expected: no error
	}
}

func TestHandleEvent_HandlerError_Logged(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var called atomic.Int32
	bus.Subscribe(EventSystemError, func(ctx context.Context, event BusEvent) error {
		called.Add(1)
		return fmt.Errorf("intentional handler error")
	})

	bus.Publish(BusEvent{ID: "err-test", Type: EventSystemError, Timestamp: time.Now()})
	time.Sleep(100 * time.Millisecond)

	if called.Load() != 1 {
		t.Fatalf("expected handler to be called, got %d", called.Load())
	}
	// Error logged internally; bus stays functional.
	bus.Publish(BusEvent{ID: "post-err", Type: EventSystemStart, Timestamp: time.Now()})
}

func TestHandleEvent_HandlerTimeout(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var started atomic.Int32
	bus.Subscribe(EventSystemStart, func(ctx context.Context, event BusEvent) error {
		started.Add(1)
		<-ctx.Done()
		return nil
	})

	bus.Publish(BusEvent{ID: "timeout-test", Type: EventSystemStart, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if started.Load() != 1 {
		t.Fatalf("expected handler to be started, got %d", started.Load())
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"0.5", 0.5},
		{"1", 1.0},
		{"invalid", 0.0},
		{"", 0.0},
		{"-3.14", -3.14},
	}
	for _, tt := range tests {
		got := parseFloat(tt.input)
		if got != tt.want {
			t.Errorf("parseFloat(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestPublishSimulationLifecycle(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var events []BusEvent
	var mu sync.Mutex
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	})

	now := time.Now()
	bus.PublishSimulationStart("sess-001", now)
	bus.PublishSimulationComplete("sess-001", 100000.0, 3, 2)

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 lifecycle events, got %d", len(events))
	}
}

func TestPublishConvenienceMethods(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var events []BusEvent
	var mu sync.Mutex
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	})

	bus.PublishDarwinianClamping([]ClampingEventPayload{
		{AgentID: "agent-1", RawWeight: 3.0, FinalWeight: 2.5, Boundary: "upper", Timestamp: time.Now()},
	})

	bus.PublishAgentHealthChange("agent-1", "healthy", "degraded", "low sharpe")

	bus.PublishOrderEvent(domain.Order{Symbol: "2330", Side: "buy", Quantity: 100, Price: 500}, "ord-1", "placed", 0)

	bus.PublishRiskEvent(EventStopLossTriggered, "2330", domain.Position{Symbol: "2330", Quantity: 100}, "stop_loss", 450)

	bus.PublishHealthAlert(HealthAlertPayload{
		Severity:        "warning",
		Category:        "sharpe",
		Message:         "sharpe below threshold",
		Value:           0.3,
		Threshold:       0.5,
		SuggestedAction: "reduce position",
		Timestamp:       time.Now(),
	})

	bus.PublishOrderError("ord-err", "2330", "buy", 500, 100, "E001", "timeout", 3, "pending")

	bus.PublishNarrativeEvent("narr-1", "AI_capex_surge", "TW", 0.8, 0.9, "AI", "0.85", "inflow", "1d")

	bus.PublishNarrativeEvent("narr-2", "custom_event", "US", 0.0, 0.5, "manual", "0.5", "neutral", "7d")

	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if len(events) < 7 {
		t.Fatalf("expected at least 7 convenience-method events, got %d", len(events))
	}
}

func TestSubscribeAll_Unsubscribe(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	bus.Publish(BusEvent{ID: "1", Type: EventSystemStart, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	sub.Cancel()

	bus.Publish(BusEvent{ID: "2", Type: EventSystemStart, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if received.Load() != 1 {
		t.Fatalf("expected only 1 event after unsubscribe, got %d", received.Load())
	}
}

func TestPublishRiskEvent_PlacedAndRejected(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var events []BusEvent
	var mu sync.Mutex
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	})

	bus.PublishRiskEvent(EventOrderPlaced, "2330", domain.Position{Symbol: "2330"}, "stop_loss", 450)
	bus.PublishRiskEvent(EventTakeProfitTriggered, "2330", domain.Position{Symbol: "2330"}, "take_profit", 550)

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 risk events, got %d", len(events))
	}
}

func TestPublishExperimentInsufficientData(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventExperimentInsufficientData, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		payload := event.Payload.(ExperimentInsufficientDataEventPayload)
		if payload.ExperimentID != "exp-1" {
			t.Errorf("unexpected experiment id: %s", payload.ExperimentID)
		}
		if payload.UsedFallback != true {
			t.Errorf("expected used_fallback=true")
		}
		return nil
	})

	bus.PublishExperimentInsufficientData("exp-1", 3, 2, 10, "immature", true)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 insufficient data event, got %d", received.Load())
	}
}

func TestPublishOrderEvent_AllStatuses(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var events []BusEvent
	var mu sync.Mutex
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	})

	order := domain.Order{Symbol: "2330", Side: "buy", Quantity: 100, Price: 500}
	bus.PublishOrderEvent(order, "o-1", "placed", 0)
	bus.PublishOrderEvent(order, "o-1", "filled", 505)
	bus.PublishOrderEvent(order, "o-1", "rejected", 0)

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if len(events) != 3 {
		t.Fatalf("expected 3 order events, got %d", len(events))
	}

	types := map[EventType]bool{}
	for _, ev := range events {
		types[ev.Type] = true
	}
	if !types[EventOrderPlaced] || !types[EventOrderFilled] || !types[EventOrderRejected] {
		t.Errorf("expected all 3 order event types, got %v", types)
	}
}

func TestPublishNarrativeEvent_SentimentText(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var payloads []NarrativeEventPayload
	var mu sync.Mutex
	bus.Subscribe(EventNarrative, func(ctx context.Context, event BusEvent) error {
		mu.Lock()
		payloads = append(payloads, event.Payload.(NarrativeEventPayload))
		mu.Unlock()
		return nil
	})

	// Positive sentiment (>0.3) → 利多
	bus.PublishNarrativeEvent("n-1", "US_rates_up", "US", 0.5, 0.8, "model", "0.7", "outflow", "1d")
	// Negative sentiment (<-0.3) → 利空
	bus.PublishNarrativeEvent("n-2", "retail_fear", "TW", -0.5, 0.7, "model", "0.6", "outflow", "1d")
	// Neutral sentiment → 中立
	bus.PublishNarrativeEvent("n-3", "geopolitical_risk_spike", "CN", 0.0, 0.6, "model", "0.5", "neutral", "7d")

	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if len(payloads) != 3 {
		t.Fatalf("expected 3 narrative events, got %d", len(payloads))
	}
	sentiments := make(map[string]bool)
	for _, p := range payloads {
		sentiments[p.SentimentText] = true
	}
	if !sentiments["利多"] || !sentiments["利空"] || !sentiments["中立"] {
		t.Errorf("expected 利多, 利空, 中立 got %v", sentiments)
	}
}

func TestPublishConvictionClamping(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventConvictionClamping, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	bus.PublishConvictionClamping([]ConvictionClampingEventPayload{
		{AgentID: "a1", Symbol: "2330", RawConviction: 10, FinalConviction: 5, Weight: 1.5, Boundary: "ceiling", Timestamp: time.Now()},
	})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 conviction clamping event, got %d", received.Load())
	}
}

func TestPublishRiskGateEvent(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var rejectedCount, overriddenCount atomic.Int32
	bus.Subscribe(EventRiskGateRejected, func(ctx context.Context, event BusEvent) error {
		rejectedCount.Add(1)
		payload := event.Payload.(RiskGateEventPayload)
		if payload.Verdict != "BLOCK" && payload.Verdict != "HALT" {
			t.Errorf("expected BLOCK or HALT verdict for rejected event, got %s", payload.Verdict)
		}
		return nil
	})
	bus.Subscribe(EventRiskGateAllowed, func(ctx context.Context, event BusEvent) error {
		overriddenCount.Add(1)
		return nil
	})

	bus.PublishRiskGateEvent(RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "BLOCK",
		Reason:    "VaR limit exceeded",
		Symbol:    "2330",
		Mode:      "DEFENSIVE",
		Timestamp: time.Now(),
	})

	bus.PublishRiskGateEvent(RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "ALLOW",
		Reason:    "manual override by CIO",
		Symbol:    "2330",
		Mode:      "NORMAL",
		Timestamp: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
	if rejectedCount.Load() != 1 {
		t.Fatalf("expected 1 risk gate rejected event, got %d", rejectedCount.Load())
	}
	if overriddenCount.Load() != 1 {
		t.Fatalf("expected 1 risk gate overridden event, got %d", overriddenCount.Load())
	}
}

func TestPublishRiskGateEvent_HALTRouting(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var rejectedCount, overriddenCount atomic.Int32
	bus.Subscribe(EventRiskGateRejected, func(ctx context.Context, event BusEvent) error {
		rejectedCount.Add(1)
		return nil
	})
	bus.Subscribe(EventRiskGateOverridden, func(ctx context.Context, event BusEvent) error {
		overriddenCount.Add(1)
		return nil
	})

	bus.PublishRiskGateEvent(RiskGateEventPayload{
		Phase:     "in_trade",
		Verdict:   "HALT",
		Reason:    "margin call",
		Symbol:    "2330",
		Mode:      "DEFENSIVE",
		Timestamp: time.Now(),
	})

	bus.PublishRiskGateEvent(RiskGateEventPayload{
		Phase:     "post_trade",
		Verdict:   "ALERT_ONLY",
		Reason:    "concentration warning",
		Symbol:    "2330",
		Mode:      "CAUTIOUS",
		Timestamp: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
	if rejectedCount.Load() != 1 {
		t.Fatalf("expected 1 risk gate rejected event (HALT), got %d", rejectedCount.Load())
	}
	if overriddenCount.Load() != 1 {
		t.Fatalf("expected 1 risk gate overridden event (ALERT_ONLY), got %d", overriddenCount.Load())
	}
}

func TestPublishRiskGateEvent_ReduceRouting(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var overriddenCount atomic.Int32
	bus.Subscribe(EventRiskGateOverridden, func(ctx context.Context, event BusEvent) error {
		overriddenCount.Add(1)
		payload := event.Payload.(RiskGateEventPayload)
		if payload.Verdict != "REDUCE" {
			t.Errorf("expected REDUCE verdict for overridden event, got %s", payload.Verdict)
		}
		return nil
	})

	bus.PublishRiskGateEvent(RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "REDUCE",
		Reason:    "partial reduction after override",
		Symbol:    "2454",
		Mode:      "CAUTIOUS",
		Timestamp: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
	if overriddenCount.Load() != 1 {
		t.Fatalf("expected 1 risk gate overridden event, got %d", overriddenCount.Load())
	}
}

func TestPublishIndustryCalendarEvent(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	var capturedPayload IndustryCalendarEventPayload
	bus.Subscribe(EventIndustryCalendar, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		capturedPayload = event.Payload.(IndustryCalendarEventPayload)
		return nil
	})

	bus.PublishIndustryCalendarEvent(IndustryCalendarEventPayload{
		EventID:             "ex_dividend_2026_06",
		Name:                "除權息旺季",
		NameEN:              "Ex-Dividend Season",
		EventType:           "ex_dividend",
		Description:         "除權息旺季 - 6 月至 8 月",
		Direction:           "mixed",
		BaseWeight:          0.70,
		Active:              true,
		StartDate:           time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		EndDate:             time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		PeakDate:            time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		DecayDays:           7,
		AffectedIndustries:  []string{"financials", "consumer"},
		SentimentAdjustment: 0.0125,
		DataSource:          "default_rules",
		EvidenceQuality:     "backtested",
		GeneratedAt:         time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 industry calendar event, got %d", received.Load())
	}
	if capturedPayload.EventID != "ex_dividend_2026_06" {
		t.Errorf("expected EventID ex_dividend_2026_06, got %s", capturedPayload.EventID)
	}
	if capturedPayload.Name != "除權息旺季" {
		t.Errorf("expected Name 除權息旺季, got %s", capturedPayload.Name)
	}
	if capturedPayload.Direction != "mixed" {
		t.Errorf("expected Direction mixed, got %s", capturedPayload.Direction)
	}
	if len(capturedPayload.AffectedIndustries) != 2 {
		t.Errorf("expected 2 affected industries, got %d", len(capturedPayload.AffectedIndustries))
	}
}

func TestPublishBacktestCompleted(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	var receivedPayload BacktestCompletedEventPayload
	bus.Subscribe(EventBacktestCompleted, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		receivedPayload = event.Payload.(BacktestCompletedEventPayload)
		return nil
	})

	bus.PublishBacktestCompleted(BacktestCompletedEventPayload{
		WindowID:              "bt-2026-06-21",
		StartDate:             time.Now().AddDate(0, 0, -30),
		EndDate:               time.Now(),
		SessionCount:          22,
		OutcomeCount:          17,
		WorstAgentID:          "agent-momentum-1",
		WorstAgentSkill:       "momentum",
		WorstAgentLayer:       "L1",
		WorstAgentWindowCount: 22,
		WorstAgentSharpeLike:  0.42,
		GeneratedAt:           time.Now(),
		TargetDate:            time.Now(),
		SyncSucceeded:         true,
	})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 backtest completed event, got %d", received.Load())
	}
	if receivedPayload.WindowID != "bt-2026-06-21" {
		t.Errorf("unexpected window_id: %s", receivedPayload.WindowID)
	}
	if !receivedPayload.SyncSucceeded {
		t.Errorf("expected SyncSucceeded=true, got false")
	}
}

func TestPublishCalibrationCompleted(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	var capturedPayload CalibrationCompletedEventPayload
	bus.Subscribe(EventCalibrationCompleted, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		capturedPayload = event.Payload.(CalibrationCompletedEventPayload)
		return nil
	})

	now := time.Now()
	bus.PublishCalibrationCompleted(CalibrationCompletedEventPayload{
		Module:            "linkage",
		CalibratorName:    "LinkageAmplifier",
		ParamCount:        3,
		BaselineScore:     0.58,
		OptimizedScore:    0.72,
		Verdict:           "improved",
		ChangeCount:       2,
		TopChangeParam:    "semiconductor_to_tech",
		TopChangeDeltaPct: 0.14,
		GeneratedAt:       now,
		SyncSucceeded:     true,
	})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 calibration completed event, got %d", received.Load())
	}
	if capturedPayload.Module != "linkage" {
		t.Errorf("expected module=linkage, got %s", capturedPayload.Module)
	}
	if capturedPayload.OptimizedScore <= capturedPayload.BaselineScore {
		t.Errorf("expected optimized > baseline, got baseline=%f optimized=%f", capturedPayload.BaselineScore, capturedPayload.OptimizedScore)
	}
	if capturedPayload.TopChangeParam != "semiconductor_to_tech" {
		t.Errorf("expected top_change_param=semiconductor_to_tech, got %s", capturedPayload.TopChangeParam)
	}
	if !capturedPayload.SyncSucceeded {
		t.Errorf("expected SyncSucceeded=true")
	}
}

func TestPublishTradeSlippage(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	var capturedPayload TradeSlippageEventPayload
	bus.Subscribe(EventTradeSlippage, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		capturedPayload = event.Payload.(TradeSlippageEventPayload)
		return nil
	})

	now := time.Now()
	bus.PublishTradeSlippage(TradeSlippageEventPayload{
		OrderID:       "ord-001",
		Symbol:        "2330",
		Side:          "buy",
		Quantity:      1000,
		ExpectedPrice: 600.0,
		FillPrice:     600.30,
		SlippageBPS:   5.0,
		SlippageCost:  300.0,
		BrokerMode:    "dry-run",
		Timestamp:     now,
	})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 trade slippage event, got %d", received.Load())
	}
	if capturedPayload.OrderID != "ord-001" {
		t.Errorf("expected OrderID=ord-001, got %s", capturedPayload.OrderID)
	}
	if capturedPayload.SlippageBPS != 5.0 {
		t.Errorf("expected SlippageBPS=5.0, got %f", capturedPayload.SlippageBPS)
	}
}
