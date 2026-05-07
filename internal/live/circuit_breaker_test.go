package live

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestCircuitBreakerDailyLossHalt(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.ResetDayState(1000000)

	cb.Evaluate(store.PortfolioState{Cash: 1000000, DayPnL: -30000}, nil, nil)
	if cb.State() != CircuitHalted {
		t.Fatalf("expected halted, got %s", cb.State())
	}
	if cb.CanPlaceOrder(domain.SideBuy) {
		t.Fatal("halted should block buy")
	}
	if cb.CanPlaceOrder(domain.SideSell) {
		t.Fatal("halted should block sell")
	}
}

func TestCircuitBreakerDrawdownPause(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.ResetDayState(1000000)

	// Push peak up first
	cb.Evaluate(store.PortfolioState{Cash: 1100000, UnrealizedPnL: 0}, nil, nil)
	// Then drop 5%
	cb.Evaluate(store.PortfolioState{Cash: 1045000, UnrealizedPnL: 0}, nil, nil)
	if cb.State() != CircuitPaused {
		t.Fatalf("expected paused, got %s", cb.State())
	}
	if cb.CanPlaceOrder(domain.SideBuy) {
		t.Fatal("paused should block buy")
	}
	if !cb.CanPlaceOrder(domain.SideSell) {
		t.Fatal("paused should allow sell")
	}
}

func TestCircuitBreakerConsecutiveStopLossCooldown(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.ResetDayState(1000000)

	cb.RecordStopLoss()
	cb.RecordStopLoss()
	if cb.State() != CircuitNormal {
		t.Fatalf("expected normal after 2 SL, got %s", cb.State())
	}
	cb.RecordStopLoss()
	if cb.State() != CircuitPaused {
		t.Fatalf("expected paused after 3 SL, got %s", cb.State())
	}
}

func TestCircuitBreakerAutoRecoverAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.SetRules([]CircuitBreakerRule{
		{Name: "test", Enabled: true, ConsecutiveSL: 2, CooldownMinutes: 0},
	})
	cb.ResetDayState(1000000)

	cb.RecordStopLoss()
	cb.RecordStopLoss()
	if cb.State() != CircuitPaused {
		t.Fatalf("expected paused, got %s", cb.State())
	}

	// Immediate evaluate should recover because cooldown=0
	time.Sleep(10 * time.Millisecond)
	cb.Evaluate(store.PortfolioState{Cash: 1000000}, nil, nil)
	if cb.State() != CircuitNormal {
		t.Fatalf("expected normal after cooldown expired, got %s", cb.State())
	}
}

func TestCircuitBreakerResetDayState(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.ResetDayState(1000000)
	cb.RecordStopLoss()
	cb.RecordStopLoss()
	cb.RecordStopLoss()
	if cb.State() != CircuitPaused {
		t.Fatal("expected paused")
	}
	cb.ResetDayState(1000000)
	if cb.State() != CircuitNormal {
		t.Fatal("expected normal after reset")
	}
	if cb.consecutiveSL != 0 {
		t.Fatalf("expected consecutiveSL=0, got %d", cb.consecutiveSL)
	}
}
