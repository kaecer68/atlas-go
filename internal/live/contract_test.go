package live

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// Test 1: C3 — const alias identity
func TestContract_LiveAlias_EventRegimeChange(t *testing.T) {
	if EventRegimeChange != eventbus.EventRegimeChange {
		t.Errorf("live.EventRegimeChange != eventbus.EventRegimeChange: drift detected")
	}
}

// Test 2: C5 — const alias identity
func TestContract_LiveAlias_EventPositionUpdate(t *testing.T) {
	if EventPositionUpdate != eventbus.EventPositionUpdate {
		t.Errorf("live.EventPositionUpdate != eventbus.EventPositionUpdate: drift detected")
	}
}

// Test 3: C6 — const alias identity
func TestContract_LiveAlias_EventPortfolioPnL(t *testing.T) {
	if EventPortfolioPnL != eventbus.EventPortfolioPnL {
		t.Errorf("live.EventPortfolioPnL != eventbus.EventPortfolioPnL: drift detected")
	}
}

// Test 4: C6 — const alias identity
func TestContract_LiveAlias_EventMarketSnapshot(t *testing.T) {
	if EventMarketSnapshot != eventbus.EventMarketSnapshot {
		t.Errorf("live.EventMarketSnapshot != eventbus.EventMarketSnapshot: drift detected")
	}
}

// Test 5: C3 — type alias identity (compile-time check via assignment)
func TestContract_LiveAlias_RegimeEventPayload_Type(t *testing.T) {
	var _ RegimeEventPayload = eventbus.RegimeEventPayload{}
}

// Test 6: C5 — type alias identity
func TestContract_LiveAlias_PositionEventPayload_Type(t *testing.T) {
	var _ PositionEventPayload = eventbus.PositionEventPayload{}
}
