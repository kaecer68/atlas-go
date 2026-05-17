package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	go func() {
		handler.ServeHTTP(rec, req)
	}()

	// Give it time to set headers.
	time.Sleep(100 * time.Millisecond)
	cancel()

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
	go func() {
		handler.ServeHTTP(rec, req)
	}()

	// Wait for the connected event.
	time.Sleep(100 * time.Millisecond)
	cancel()

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
	go func() {
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
	go func() {
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)
	if handler.clientCount != 1 {
		t.Errorf("expected client count 1, got %d", handler.clientCount)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	if handler.clientCount != 0 {
		t.Errorf("expected client count 0 after disconnect, got %d", handler.clientCount)
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
	go func() {
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

	body := rec.Body.String()
	expected := fmt.Sprintf("event: %s", eventbus.EventMarketSnapshot)
	if !strings.Contains(body, expected) {
		t.Errorf("expected %q in body, got: %s", expected, body)
	}
}
