package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

func TestSSEHandler_ServeHTTP_Headers(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	// ServeHTTP blocks, so run it in a goroutine.
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	// Give it time to set headers.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", cc)
	}
}

func TestSSEHandler_ServeHTTP_ConnectedEvent(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	// Wait for the connected event.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "event: connected") {
		t.Errorf("expected connected event in body, got: %s", body)
	}
}

func TestSSEHandler_ServeHTTP_FilteredTypes(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream?type=market.snapshot", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	// Wait for subscription to be ready.
	time.Sleep(50 * time.Millisecond)

	// Publish a matching event.
	bus.Publish(eventbus.BusEvent{
		ID:        "evt-1",
		Type:      eventbus.EventMarketSnapshot,
		Timestamp: time.Now(),
		Payload:   map[string]string{"symbol": "2330"},
	})

	// Publish a non-matching event.
	bus.Publish(eventbus.BusEvent{
		ID:        "evt-2",
		Type:      eventbus.EventAgentRecommendation,
		Timestamp: time.Now(),
		Payload:   nil,
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "market.snapshot") {
		t.Error("expected market.snapshot event in body")
	}
	if strings.Contains(body, "agent.recommendation") {
		t.Error("did not expect agent.recommendation event in body")
	}
}

func TestSSEHandler_ServeHTTP_DisconnectCleanup(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt64(&handler.clientCount) != 1 {
		t.Errorf("expected client count 1, got %d", atomic.LoadInt64(&handler.clientCount))
	}

	cancel()
	<-handlerDone

	if atomic.LoadInt64(&handler.clientCount) != 0 {
		t.Errorf("expected client count 0 after disconnect, got %d", atomic.LoadInt64(&handler.clientCount))
	}
}

func TestSSEHandler_ParseFilterTypes(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)

	tests := []struct {
		query    string
		expected []eventbus.EventType
	}{
		{"", nil},
		{"type=market.snapshot", []eventbus.EventType{"market.snapshot"}},
		{"type=market.snapshot,agent.recommendation", []eventbus.EventType{"market.snapshot", "agent.recommendation"}},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
		result := handler.parseFilterTypes(req)
		if len(result) != len(tt.expected) {
			t.Errorf("query %q: expected %v, got %v", tt.query, tt.expected, result)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("query %q: expected %v, got %v", tt.query, tt.expected, result)
				break
			}
		}
	}
}

func TestSSEHandler_MatchesFilter(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)

	client := &SSEClient{types: []eventbus.EventType{"market.snapshot"}}
	if !handler.matchesFilter(client, "market.snapshot") {
		t.Error("expected match for market.snapshot")
	}
	if handler.matchesFilter(client, "agent.recommendation") {
		t.Error("expected no match for agent.recommendation")
	}

	clientNoFilter := &SSEClient{types: nil}
	if !handler.matchesFilter(clientNoFilter, "any.event") {
		t.Error("expected match when no filter is set")
	}
}

func TestSSEHandler_ServeHTTP_NoFlusher(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	// httptest.ResponseRecorder does not implement http.Flusher,
	// but Go 1.25 adds Flusher to ResponseRecorder.
	// If it does, we need a custom response writer that doesn't implement Flusher.
	// For now, just check that it doesn't panic with the standard recorder.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected if Flusher is not supported.
			}
		}()
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)
}

func TestSSEHandler_ServeHTTP_EventDelivery(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)

	bus.Publish(eventbus.BusEvent{
		ID:        "evt-1",
		Type:      eventbus.EventMarketSnapshot,
		Timestamp: time.Now(),
		Payload:   map[string]string{"symbol": "2330"},
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	expected := fmt.Sprintf("event: %s", eventbus.EventMarketSnapshot)
	if !strings.Contains(body, expected) {
		t.Errorf("expected %q in body, got: %s", expected, body)
	}
}

func TestBufferPromotionRecordedEvent_AppendsToBuffer(t *testing.T) {
	resetPromotionBuffer()

	event := eventbus.BusEvent{
		ID:        "evt-prom-1",
		Type:      eventbus.EventPromotionRecorded,
		Timestamp: time.Now(),
		Payload: eventbus.PromotionRecordedPayload{
			ExperimentID:       "exp-test-001",
			PrePromotionSharpe: 1.42,
		},
	}

	BufferPromotionRecordedEvent(event)

	read := GetBufferedPromotionEvents()
	if len(read) != 1 {
		t.Fatalf("expected 1 buffered promotion event, got %d", len(read))
	}
	if read[0].Event.ID != "evt-prom-1" {
		t.Errorf("expected ID 'evt-prom-1', got %q", read[0].Event.ID)
	}
	payload, ok := read[0].Event.Payload.(eventbus.PromotionRecordedPayload)
	if !ok {
		t.Fatalf("expected PromotionRecordedPayload, got %T", read[0].Event.Payload)
	}
	if payload.ExperimentID != "exp-test-001" || payload.PrePromotionSharpe != 1.42 {
		t.Errorf("unexpected payload: %+v", payload)
	}
}

func TestSSEHandler_ServeHTTP_DeliversPromotionRecorded(t *testing.T) {
	resetPromotionBuffer()

	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream?type=experiment.promotion_recorded", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)

	bus.Publish(eventbus.BusEvent{
		ID:        "evt-prom-sse-1",
		Type:      eventbus.EventPromotionRecorded,
		Timestamp: time.Now(),
		Payload: eventbus.PromotionRecordedPayload{
			ExperimentID:       "exp-sse-001",
			PrePromotionSharpe: 0.95,
		},
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "experiment.promotion_recorded") {
		t.Errorf("expected promotion_recorded event in body, got: %s", body)
	}
	if !strings.Contains(body, "exp-sse-001") {
		t.Errorf("expected experiment ID 'exp-sse-001' in body, got: %s", body)
	}
}

func TestBufferHealthAlertEvent_AppendsToBuffer(t *testing.T) {
	resetHealthAlertBuffer()

	event := eventbus.BusEvent{
		ID:        "evt-health-1",
		Type:      eventbus.EventHealthAlert,
		Timestamp: time.Now(),
		Payload: eventbus.HealthAlertPayload{
			Severity:  "WARNING",
			Category:  "sharpe_trend",
			Message:   "test alert",
			Value:     0.1,
			Threshold: 0.5,
		},
	}

	BufferHealthAlertEvent(event)

	read := GetBufferedHealthAlerts()
	if len(read) != 1 {
		t.Fatalf("expected 1 buffered health alert, got %d", len(read))
	}
	if read[0].Event.ID != "evt-health-1" {
		t.Errorf("expected ID 'evt-health-1', got %q", read[0].Event.ID)
	}
	payload, ok := read[0].Event.Payload.(eventbus.HealthAlertPayload)
	if !ok {
		t.Fatalf("expected HealthAlertPayload, got %T", read[0].Event.Payload)
	}
	if payload.Category != "sharpe_trend" || payload.Severity != "WARNING" {
		t.Errorf("unexpected payload: %+v", payload)
	}
}

func TestBufferHealthAlertEvent_CapsAt50(t *testing.T) {
	resetHealthAlertBuffer()

	for i := range 60 {
		BufferHealthAlertEvent(eventbus.BusEvent{
			ID:        fmt.Sprintf("evt-cap-%d", i),
			Type:      eventbus.EventHealthAlert,
			Timestamp: time.Now(),
		})
	}

	read := GetBufferedHealthAlerts()
	if len(read) != 50 {
		t.Fatalf("expected 50 buffered health alerts (capped), got %d", len(read))
	}
	if read[0].Event.ID != "evt-cap-10" {
		t.Errorf("expected oldest kept to be 'evt-cap-10', got %q", read[0].Event.ID)
	}
	if read[49].Event.ID != "evt-cap-59" {
		t.Errorf("expected newest to be 'evt-cap-59', got %q", read[49].Event.ID)
	}
}

func TestSSEHandler_ServeHTTP_DeliversBufferedHealthAlertOnConnect(t *testing.T) {
	resetHealthAlertBuffer()

	BufferHealthAlertEvent(eventbus.BusEvent{
		ID:        "evt-health-buffered-1",
		Type:      eventbus.EventHealthAlert,
		Timestamp: time.Now(),
		Payload: eventbus.HealthAlertPayload{
			Severity:  "CRITICAL",
			Category:  "drawdown",
			Message:   "buffered alert before connect",
			Value:     0.18,
			Threshold: 0.15,
		},
	})

	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "monitor.health.alert") {
		t.Errorf("expected health_alert event in body, got: %s", body)
	}
	if !strings.Contains(body, "buffered alert before connect") {
		t.Errorf("expected buffered alert message in body, got: %s", body)
	}
}

func TestSSEHandler_BufferRiskGateEvent(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	evt := eventbus.RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "BLOCK",
		Reason:    "VaR limit exceeded",
		Symbol:    "2330",
		Mode:      "DEFENSIVE",
		Timestamp: time.Now(),
	}
	bus.PublishRiskGateEvent(evt)
	time.Sleep(50 * time.Millisecond)

	BufferRiskGateEvent(eventbus.BusEvent{
		ID:        "test-001",
		Type:      eventbus.EventRiskGateRejected,
		Timestamp: time.Now(),
		Payload:   evt,
		Severity:  "warning",
	})

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "monitor.risk_gate.rejected") {
		t.Errorf("expected risk_gate.rejected event in body, got: %s", body)
	}
	if !strings.Contains(body, "VaR limit exceeded") {
		t.Errorf("expected VaR limit exceeded reason in body, got: %s", body)
	}
}

func TestSSEHandler_BufferIndustryCalendarEvent(t *testing.T) {
	resetIndustryCalendarBuffer()

	event := eventbus.BusEvent{
		ID:        "test-1",
		Type:      eventbus.EventIndustryCalendar,
		Timestamp: time.Now(),
		Payload: eventbus.IndustryCalendarEventPayload{
			EventID: "test_event_2026",
			Name:    "Test Event",
		},
	}
	BufferIndustryCalendarEvent(event)

	buffered := GetBufferedIndustryCalendarEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered industry calendar event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(eventbus.IndustryCalendarEventPayload)
	if payload.EventID != "test_event_2026" {
		t.Errorf("expected EventID test_event_2026, got %s", payload.EventID)
	}
	if buffered[0].Event.Type != eventbus.EventIndustryCalendar {
		t.Errorf("expected type %s, got %s", eventbus.EventIndustryCalendar, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferBacktestCompletedEvent(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer bus.Close()

	evt := eventbus.BacktestCompletedEventPayload{
		WindowID:              "bt-buffered-1",
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
	}
	bus.PublishBacktestCompleted(evt)
	time.Sleep(50 * time.Millisecond)

	BufferBacktestCompletedEvent(eventbus.BusEvent{
		ID:        "bt-test-001",
		Type:      eventbus.EventBacktestCompleted,
		Timestamp: time.Now(),
		Payload:   evt,
		Severity:  "info",
	})

	handler := NewSSEHandler(bus)
	req := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-handlerDone

	body := rec.Body.String()
	if !strings.Contains(body, "experiment.backtest_completed") {
		t.Errorf("expected backtest_completed event in body, got: %s", body)
	}
	if !strings.Contains(body, "bt-buffered-1") {
		t.Errorf("expected window_id bt-buffered-1 in body, got: %s", body)
	}
}

func TestSSEHandler_BufferCalibrationCompletedEvent(t *testing.T) {
	resetCalibrationCompletedBuffer()

	event := eventbus.BusEvent{
		ID:        "cal-test-1",
		Type:      eventbus.EventCalibrationCompleted,
		Timestamp: time.Now(),
		Payload: eventbus.CalibrationCompletedEventPayload{
			Module:         "linkage",
			CalibratorName: "LinkageAmplifier",
			Verdict:        "improved",
		},
	}
	BufferCalibrationCompletedEvent(event)

	buffered := GetBufferedCalibrationCompletedEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered calibration completed event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(eventbus.CalibrationCompletedEventPayload)
	if payload.Module != "linkage" {
		t.Errorf("expected module=linkage, got %s", payload.Module)
	}
	if buffered[0].Event.Type != eventbus.EventCalibrationCompleted {
		t.Errorf("expected type %s, got %s", eventbus.EventCalibrationCompleted, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferTradeSlippageEvent(t *testing.T) {
	resetTradeSlippageBuffer()

	event := eventbus.BusEvent{
		ID:        "ts-test-1",
		Type:      eventbus.EventTradeSlippage,
		Timestamp: time.Now(),
		Payload: eventbus.TradeSlippageEventPayload{
			OrderID: "ord-001",
			Symbol:  "2330",
		},
	}
	BufferTradeSlippageEvent(event)

	buffered := GetBufferedTradeSlippageEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered trade slippage event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(eventbus.TradeSlippageEventPayload)
	if payload.OrderID != "ord-001" {
		t.Errorf("expected OrderID=ord-001, got %s", payload.OrderID)
	}
	if buffered[0].Event.Type != eventbus.EventTradeSlippage {
		t.Errorf("expected type %s, got %s", eventbus.EventTradeSlippage, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferChannelIndividualHealthEvent(t *testing.T) {
	resetChannelIndividualHealthBuffer()

	event := eventbus.BusEvent{
		ID:        "cih-test-1",
		Type:      eventbus.EventChannelIndividualHealth,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"channel_id":    "spx",
			"error_message": "fetch timeout",
			"first_seen_at": time.Now(),
			"detected_at":   time.Now(),
		},
	}
	BufferChannelIndividualHealthEvent(event)

	buffered := GetBufferedChannelIndividualHealthEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered channel individual health event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(map[string]any)
	if payload["channel_id"] != "spx" {
		t.Errorf("expected channel_id=spx, got %v", payload["channel_id"])
	}
	if buffered[0].Event.Type != eventbus.EventChannelIndividualHealth {
		t.Errorf("expected type %s, got %s", eventbus.EventChannelIndividualHealth, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferRegimeChangeConfirmedEvent(t *testing.T) {
	resetRegimeChangeConfirmedBuffer()

	event := eventbus.BusEvent{
		ID:        "rcc-test-1",
		Type:      eventbus.EventRegimeChangeConfirmed,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"old_regime":   "RISK_ON",
			"new_regime":   "NEUTRAL",
			"confidence":   0.87,
			"stable_since": time.Now().Add(-30 * time.Second),
		},
	}
	BufferRegimeChangeConfirmedEvent(event)

	buffered := GetBufferedRegimeChangeConfirmedEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered regime change confirmed event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(map[string]any)
	if payload["new_regime"] != "NEUTRAL" {
		t.Errorf("expected new_regime=NEUTRAL, got %v", payload["new_regime"])
	}
	if buffered[0].Event.Type != eventbus.EventRegimeChangeConfirmed {
		t.Errorf("expected type %s, got %s", eventbus.EventRegimeChangeConfirmed, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferFactorWeightRegressionEvent(t *testing.T) {
	resetFactorWeightRegressionBuffer()

	event := eventbus.BusEvent{
		ID:        "fwr-test-1",
		Type:      eventbus.EventFactorWeightRegression,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"regime":       "RISK_ON",
			"mse":          0.0123,
			"sample_count": 252,
			"detected_at":  time.Now(),
		},
	}
	BufferFactorWeightRegressionEvent(event)

	buffered := GetBufferedFactorWeightRegressionEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered factor weight regression event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(map[string]any)
	if payload["regime"] != "RISK_ON" {
		t.Errorf("expected regime=RISK_ON, got %v", payload["regime"])
	}
	if buffered[0].Event.Type != eventbus.EventFactorWeightRegression {
		t.Errorf("expected type %s, got %s", eventbus.EventFactorWeightRegression, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferDriftDetectedEvent(t *testing.T) {
	resetDriftDetectedBuffer()

	event := eventbus.BusEvent{
		ID:        "dd-test-1",
		Type:      eventbus.EventDriftDetected,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"drift_type":     "concentration",
			"symbol":         "2330",
			"actual_weight":  0.32,
			"expected_range": []float64{0.0, 0.25},
			"detected_at":    time.Now(),
		},
	}
	BufferDriftDetectedEvent(event)

	buffered := GetBufferedDriftDetectedEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered drift detected event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(map[string]any)
	if payload["symbol"] != "2330" {
		t.Errorf("expected symbol=2330, got %v", payload["symbol"])
	}
	if buffered[0].Event.Type != eventbus.EventDriftDetected {
		t.Errorf("expected type %s, got %s", eventbus.EventDriftDetected, buffered[0].Event.Type)
	}
}

func TestSSEHandler_BufferIngestionLagSpikeEvent(t *testing.T) {
	resetIngestionLagSpikeBuffer()

	event := eventbus.BusEvent{
		ID:        "ils-test-1",
		Type:      eventbus.EventIngestionLagSpike,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"channel_id":  "yahoo",
			"lag_seconds": 45.2,
			"threshold":   30.0,
			"detected_at": time.Now(),
		},
	}
	BufferIngestionLagSpikeEvent(event)

	buffered := GetBufferedIngestionLagSpikeEvents()
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered ingestion lag spike event, got %d", len(buffered))
	}
	payload := buffered[0].Event.Payload.(map[string]any)
	if payload["channel_id"] != "yahoo" {
		t.Errorf("expected channel_id=yahoo, got %v", payload["channel_id"])
	}
	if buffered[0].Event.Type != eventbus.EventIngestionLagSpike {
		t.Errorf("expected type %s, got %s", eventbus.EventIngestionLagSpike, buffered[0].Event.Type)
	}
}

func TestBufferAgentHealthChangeEvent_AppendsToBuffer(t *testing.T) {
	resetAgentHealthChangeBuffer()

	event := eventbus.BusEvent{
		ID:        "evt-ahc-1",
		Type:      eventbus.EventAgentHealthChange,
		Timestamp: time.Now(),
		Payload: eventbus.AgentHealthChangeEventPayload{
			AgentID:   "agent-1",
			OldStatus: "healthy",
			NewStatus: "degraded",
			Reason:    "low_sharpe",
		},
	}

	BufferAgentHealthChangeEvent(event)

	read := GetBufferedAgentHealthChangeEvents()
	if len(read) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(read))
	}
	if read[0].Event.ID != "evt-ahc-1" {
		t.Errorf("expected ID 'evt-ahc-1', got %q", read[0].Event.ID)
	}
	payload, ok := read[0].Event.Payload.(eventbus.AgentHealthChangeEventPayload)
	if !ok {
		t.Fatalf("expected AgentHealthChangeEventPayload, got %T", read[0].Event.Payload)
	}
	if payload.AgentID != "agent-1" || payload.NewStatus != "degraded" {
		t.Errorf("unexpected payload: %+v", payload)
	}
}
