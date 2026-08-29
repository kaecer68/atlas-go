package eventbus

import (
	"testing"
	"time"
)

func TestSSEBridge_AddClient(t *testing.T) {
	bridge := NewSSEBridge()
	client := bridge.AddClient("client-1", []EventType{EventMarketSnapshot})

	if client.ID != "client-1" {
		t.Errorf("expected client ID client-1, got %s", client.ID)
	}
	if len(client.Types) != 1 || client.Types[0] != EventMarketSnapshot {
		t.Errorf("expected types [EventMarketSnapshot], got %v", client.Types)
	}
	if cap(client.Events) != 128 {
		t.Errorf("expected channel capacity 128, got %d", cap(client.Events))
	}
	if bridge.ClientCount() != 1 {
		t.Errorf("expected client count 1, got %d", bridge.ClientCount())
	}
}

func TestSSEBridge_RemoveClient(t *testing.T) {
	bridge := NewSSEBridge()
	bridge.AddClient("client-1", nil)
	bridge.RemoveClient("client-1")

	if bridge.ClientCount() != 0 {
		t.Errorf("expected client count 0, got %d", bridge.ClientCount())
	}
}

func TestSSEBridge_Broadcast(t *testing.T) {
	bridge := NewSSEBridge()
	client := bridge.AddClient("client-1", nil)

	event := BusEvent{
		ID:        "evt-1",
		Type:      EventMarketSnapshot,
		Timestamp: time.Now(),
		Payload:   map[string]string{"symbol": "2330"},
	}
	bridge.Broadcast(event)

	select {
	case received := <-client.Events:
		if received.ID != "evt-1" {
			t.Errorf("expected event ID evt-1, got %s", received.ID)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestSSEBridge_Broadcast_Filtered(t *testing.T) {
	bridge := NewSSEBridge()
	client := bridge.AddClient("client-1", []EventType{EventMarketSnapshot})

	event := BusEvent{
		ID:        "evt-1",
		Type:      EventAgentRecommendation,
		Timestamp: time.Now(),
		Payload:   nil,
	}
	bridge.Broadcast(event)

	select {
	case <-client.Events:
		t.Error("expected no event for filtered type")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event received
	}
}

func TestSSEBridge_Broadcast_SlowClient(t *testing.T) {
	bridge := NewSSEBridge()
	client := bridge.AddClient("client-1", nil)

	// Fill the channel to capacity.
	for range 128 {
		select {
		case client.Events <- BusEvent{ID: "fill", Type: EventMarketSnapshot}:
		default:
			t.Fatal("failed to fill channel")
		}
	}

	// This broadcast should drop the oldest event.
	event := BusEvent{ID: "new-evt", Type: EventMarketSnapshot}
	bridge.Broadcast(event)

	// Drain the channel and verify the new event is present.
	found := false
	drainCount := 0
drainLoop:
	for {
		select {
		case evt := <-client.Events:
			drainCount++
			if evt.ID == "new-evt" {
				found = true
			}
		default:
			break drainLoop
		}
	}

	if drainCount != 128 {
		t.Errorf("expected 128 events in channel, got %d", drainCount)
	}
	if !found {
		t.Error("expected new event to be present after dropping oldest")
	}
}

func TestSSEBridge_ClientCount_Concurrent(t *testing.T) {
	bridge := NewSSEBridge()
	for i := range 10 {
		bridge.AddClient(string(rune('a'+i)), nil)
	}
	if bridge.ClientCount() != 10 {
		t.Errorf("expected client count 10, got %d", bridge.ClientCount())
	}
}
