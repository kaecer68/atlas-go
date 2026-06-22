package eventbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Test 1: C3 — event constant value
func TestContract_EventRegimeChange_Constant(t *testing.T) {
	if string(EventRegimeChange) != "market.regime.change" {
		t.Errorf("EventRegimeChange = %q, want %q", EventRegimeChange, "market.regime.change")
	}
}

// Test 2: C5 — event constant value
func TestContract_EventPositionUpdate_Constant(t *testing.T) {
	if string(EventPositionUpdate) != "portfolio.position.update" {
		t.Errorf("EventPositionUpdate = %q, want %q", EventPositionUpdate, "portfolio.position.update")
	}
}

// Test 3: C6 — event constant value
func TestContract_EventPortfolioPnL_Constant(t *testing.T) {
	if string(EventPortfolioPnL) != "portfolio.pnl.update" {
		t.Errorf("EventPortfolioPnL = %q, want %q", EventPortfolioPnL, "portfolio.pnl.update")
	}
}

// Test 4: C6 — event constant value
func TestContract_EventMarketSnapshot_Constant(t *testing.T) {
	if string(EventMarketSnapshot) != "market.snapshot" {
		t.Errorf("EventMarketSnapshot = %q, want %q", EventMarketSnapshot, "market.snapshot")
	}
}

// Test 5: C3 — compile-time publisher signature (method expression assignment)
// If PublishRegimeChange signature changes, this line fails to compile → go build fails.
// Method expression (*ChannelEventBus).PublishRegimeChange has the receiver as its first parameter.
func TestContract_PublishRegimeChange_Signature(t *testing.T) {
	var _ func(*ChannelEventBus, domain.Regime, domain.Regime, float64, string) = (*ChannelEventBus).PublishRegimeChange
}

// Test 6: C5 — compile-time publisher signature
func TestContract_PublishPositionUpdate_Signature(t *testing.T) {
	var _ func(*ChannelEventBus, string, domain.Position, string) = (*ChannelEventBus).PublishPositionUpdate
}

// Test 7: C6 — compile-time publisher signature
func TestContract_PublishMarketSnapshot_Signature(t *testing.T) {
	var _ func(*ChannelEventBus, domain.Quote) = (*ChannelEventBus).PublishMarketSnapshot
}

// Test 8: C3 — runtime publish/subscribe payload assertion
func TestContract_PublishRegimeChange_Payload(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var got RegimeEventPayload
	done := make(chan struct{})
	bus.Subscribe(EventRegimeChange, func(ctx context.Context, event BusEvent) error {
		got = event.Payload.(RegimeEventPayload)
		select {
		case <-done:
		default:
			close(done)
		}
		return nil
	})

	bus.PublishRegimeChange(domain.RegimeNeutral, domain.RegimeRiskOn, 0.85, "contract_test")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for regime change event")
	}
	if got.NewRegime != domain.RegimeRiskOn {
		t.Errorf("NewRegime = %v, want %v", got.NewRegime, domain.RegimeRiskOn)
	}
	if got.Confidence != 0.85 {
		t.Errorf("Confidence = %v, want 0.85", got.Confidence)
	}
	if got.DeterminedBy != "contract_test" {
		t.Errorf("DeterminedBy = %v, want contract_test", got.DeterminedBy)
	}
}

// Test 9: C5 — runtime publish/subscribe payload assertion
func TestContract_PublishPositionUpdate_Payload(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	pos := domain.Position{Quantity: 1000, AverageCost: 500.0}
	var got PositionEventPayload
	done := make(chan struct{})
	bus.Subscribe(EventPositionUpdate, func(ctx context.Context, event BusEvent) error {
		got = event.Payload.(PositionEventPayload)
		select {
		case <-done:
		default:
			close(done)
		}
		return nil
	})

	bus.PublishPositionUpdate("2330", pos, "added")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for position update event")
	}
	if got.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", got.Symbol)
	}
	if got.ChangeType != "added" {
		t.Errorf("ChangeType = %q, want added", got.ChangeType)
	}
}

// Test 10: C6 — runtime publish/subscribe payload assertion
func TestContract_PublishMarketSnapshot_Payload(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	quote := domain.Quote{Symbol: "2330", Last: 500.0}
	var got MarketEventPayload
	done := make(chan struct{})
	bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		got = event.Payload.(MarketEventPayload)
		select {
		case <-done:
		default:
			close(done)
		}
		return nil
	})

	bus.PublishMarketSnapshot(quote)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for market snapshot event")
	}
	if got.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", got.Symbol)
	}
}

// Test 11: C3 — JSON tag verification
func TestContract_RegimeEventPayload_JSONTags(t *testing.T) {
	payload := RegimeEventPayload{
		OldRegime:    domain.RegimeNeutral,
		NewRegime:    domain.RegimeRiskOn,
		Confidence:   0.85,
		DeterminedBy: "test",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	requiredKeys := []string{"old_regime", "new_regime", "confidence", "determined_by"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in RegimeEventPayload", key)
		}
	}
}

// Test 12: C5 — JSON tag verification
func TestContract_PositionEventPayload_JSONTags(t *testing.T) {
	payload := PositionEventPayload{
		Symbol:     "2330",
		Position:   domain.Position{Quantity: 1000, AverageCost: 500.0},
		ChangeType: "added",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	requiredKeys := []string{"symbol", "position", "change_type"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in PositionEventPayload", key)
		}
	}
}

// Test 13: C6 — MarketEventPayload JSON tag verification
func TestContract_MarketEventPayload_JSONTags(t *testing.T) {
	payload := MarketEventPayload{
		Symbol:    "2330",
		Quote:     domain.Quote{Symbol: "2330", Last: 500.0},
		Timestamp: time.Now(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	requiredKeys := []string{"symbol", "quote", "timestamp"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in MarketEventPayload", key)
		}
	}
}
