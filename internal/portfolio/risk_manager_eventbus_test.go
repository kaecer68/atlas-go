package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestRiskManager_WithEventBus verifies WithEventBus attaches the bus to the
// RiskManager and returns the same instance (chainable, mirroring the
// AgentHealthManager.WithEventBus pattern).
func TestRiskManager_WithEventBus(t *testing.T) {
	rm := NewRiskManager()
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	result := rm.WithEventBus(bus)
	if result != rm {
		t.Fatalf("WithEventBus must return the same *RiskManager (chainable), got %v", result)
	}
	if rm.eventBus != bus {
		t.Errorf("expected eventBus to be attached, got %v / want %v", rm.eventBus, bus)
	}
}

// TestRiskManager_PublishesDrawdownBreachOnBreach is a financial-engineering-
// grade regression test: when portfolio drawdown exceeds maxDrawdownPct, the
// RiskManager must publish EventDrawdownBreach so monitoring/dashboard layers
// can surface the alert.
func TestRiskManager_PublishesDrawdownBreachOnBreach(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	rm.WithEventBus(bus)

	// Build peak at 100000
	rm.UpdatePortfolioValue(100000)
	// Drop to 91000 — 9% drawdown, exceeds 8% limit → should publish EventDrawdownBreach
	rm.UpdatePortfolioValue(91000)

	select {
	case e := <-received:
		if e.Type != eventbus.EventDrawdownBreach {
			t.Errorf("expected EventDrawdownBreach event, got type=%s", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventDrawdownBreach event after drawdown breach")
	}
}

// TestRiskManager_DoesNotPublishDrawdownBelowThreshold verifies that
// EventDrawdownBreach is NOT published when drawdown is below the threshold.
// Avoids alert fatigue from benign fluctuations.
func TestRiskManager_DoesNotPublishDrawdownBelowThreshold(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	rm.WithEventBus(bus)

	// Build peak at 100000
	rm.UpdatePortfolioValue(100000)
	// Drop to 95000 — 5% drawdown, below 8% limit → should NOT publish
	rm.UpdatePortfolioValue(95000)

	select {
	case e := <-received:
		t.Fatalf("unexpected event published: type=%s", e.Type)
	case <-time.After(500 * time.Millisecond):
		// Expected: no event received within timeout.
	}
}

// TestRiskManager_NilBusDrawdownIsSafe verifies the no-bus branch does not
// panic when a drawdown breach would otherwise fire. With eventBus == nil,
// UpdatePortfolioValue must complete without panicking and still return the
// RiskAlert slice (backward compatibility for existing callers).
func TestRiskManager_NilBusDrawdownIsSafe(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// No bus attached — should silently skip publishing.
	alerts := rm.UpdatePortfolioValue(100000)
	alerts = append(alerts, rm.UpdatePortfolioValue(91000)...) // 9% drawdown

	// Verify backward-compatible behavior: RiskAlert slice still returned.
	foundDrawdownAlert := false
	for _, alert := range alerts {
		if alert.Type == AlertDrawdown {
			foundDrawdownAlert = true
		}
	}
	if !foundDrawdownAlert {
		t.Error("expected RiskAlert of type AlertDrawdown when exceeding limit (backward compat)")
	}
	if rm.eventBus != nil {
		t.Errorf("expected eventBus to remain nil, got %v", rm.eventBus)
	}
}
