package live

import (
	"context"
	"sync"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRiskGate_Check_AllowsOrder(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500,
	}

	if err := gate.Check(context.Background(), order); err != nil {
		t.Fatalf("expected order to be allowed, got: %v", err)
	}
}

func TestRiskGate_Check_HaltsOnDrawdown(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.SetHaltTrading(true)

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1000, Price: 500}

	if err := gate.Check(context.Background(), order); err == nil {
		t.Fatal("expected order to be blocked due to trading halt")
	} else if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestRiskGate_Check_RejectsOnDailyLoss(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.UpdateDailyLoss(0.05) // exceeds 3% limit

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1000, Price: 500}

	if err := gate.Check(context.Background(), order); err == nil {
		t.Fatal("expected order to be blocked due to daily loss")
	}
}

func TestRiskGate_Check_RejectsOnVaR(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.UpdateVaR(0.07) // exceeds 5% threshold

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1000, Price: 500}

	if err := gate.Check(context.Background(), order); err == nil {
		t.Fatal("expected order to be blocked due to VaR critical")
	}
}

func TestRiskGate_UpdateDailyLoss(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	gate.UpdateDailyLoss(0.01)
	gate.UpdateDailyLoss(0.02)

	status := gate.Status()
	loss, ok := status["daily_loss"].(float64)
	if !ok {
		t.Fatal("daily_loss not found in status")
	}
	if loss != 0.02 {
		t.Fatalf("expected daily_loss 0.02, got %.4f", loss)
	}
}

func TestRiskGate_ResetDaily(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	gate.UpdateDailyLoss(0.02)
	gate.ResetDaily()

	status := gate.Status()
	loss, ok := status["daily_loss"].(float64)
	if !ok {
		t.Fatal("daily_loss not found in status")
	}
	if loss != 0 {
		t.Fatalf("expected daily_loss 0 after reset, got %.4f", loss)
	}
}

func TestRiskGate_SetHaltTrading(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	gate.SetHaltTrading(true)
	status := gate.Status()
	if !status["halt_trading"].(bool) {
		t.Fatal("expected halt_trading to be true")
	}

	gate.SetHaltTrading(false)
	status = gate.Status()
	if status["halt_trading"].(bool) {
		t.Fatal("expected halt_trading to be false")
	}
}

func TestRiskGate_Status(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	status := gate.Status()

	keys := []string{"halt_trading", "daily_loss", "max_daily_loss_pct", "var_value", "var_critical_threshold", "date"}
	for _, k := range keys {
		if _, ok := status[k]; !ok {
			t.Fatalf("expected key %q in status", k)
		}
	}
}

func TestRiskGate_ConcurrentRecordFill_NoRace(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.50,
		VaRCriticalThreshold: 0.50,
	})

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	for range goroutines {
		go func() {
			defer wg.Done()
			gate.RecordFill(domain.SideSell, 100.0, 90.0, 10, 1000.0)
		}()
		go func() {
			defer wg.Done()
			gate.UpdateVaR(0.05)
		}()
		go func() {
			defer wg.Done()
			gate.SetHaltTrading(true)
			gate.SetHaltTrading(false)
		}()
		go func() {
			defer wg.Done()
			_ = gate.Check(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100})
		}()
	}

	wg.Wait()

	status := gate.Status()
	if _, ok := status["daily_loss"]; !ok {
		t.Fatal("status missing daily_loss after concurrent calls")
	}
}

func TestRiskGate_MidnightReset_DoesNotAccumulate(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.10,
		VaRCriticalThreshold: 0.10,
	})

	gate.mu.Lock()
	gate.today = "2020-01-01"
	gate.dailyLoss = 0.05
	gate.mu.Unlock()

	gate.UpdateDailyLoss(0.10)

	status := gate.Status()
	loss, _ := status["daily_loss"].(float64)
	if loss != 0.10 {
		t.Fatalf("expected daily_loss 0.10 after midnight reset+update, got %.4f", loss)
	}
	if gate.today == "2020-01-01" {
		t.Fatal("expected today field to be updated to current date")
	}
}

func TestRiskGate_Check_AllowsLossExactlyAtThreshold(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.05,
		VaRCriticalThreshold: 0.10,
	})

	gate.UpdateDailyLoss(0.05)

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	if err := gate.Check(context.Background(), order); err != nil {
		t.Fatalf("expected order allowed when loss equals threshold, got: %v", err)
	}
}

func TestRiskGate_SyncHaltFromCircuitState(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.05,
		VaRCriticalThreshold: 0.10,
	})

	gate.SyncHaltFromCircuitState(CircuitHalted)
	if !gate.Status()["halt_trading"].(bool) {
		t.Fatal("expected halt_trading true after Halted sync")
	}

	gate.SyncHaltFromCircuitState(CircuitNormal)
	if gate.Status()["halt_trading"].(bool) {
		t.Fatal("expected halt_trading false after Normal sync")
	}

	gate.SyncHaltFromCircuitState(CircuitPaused)
	if !gate.Status()["halt_trading"].(bool) {
		t.Fatal("expected halt_trading true after Paused sync (close-only)")
	}
}

func TestRiskGate_UpdateVaR_BlocksOrderAboveThreshold(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0,
		VaRCriticalThreshold: 0.10,
	})

	gate.UpdateVaR(0.15)

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	if err := gate.Check(context.Background(), order); err == nil {
		t.Fatal("expected order blocked when VaR exceeds threshold")
	}
}

func TestRiskGate_RecordFill_UpdatesDailyLossOnLoss(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.05,
		VaRCriticalThreshold: 0.10,
	})

	gate.RecordFill(domain.SideSell, 100.0, 90.0, 10, 1000.0)

	status := gate.Status()
	loss, _ := status["daily_loss"].(float64)
	if loss != 0.10 {
		t.Fatalf("expected daily_loss 0.10 after losing fill, got %.4f", loss)
	}
}

func TestRiskGate_RecordFill_DoesNotReduceOnGain(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.05,
		VaRCriticalThreshold: 0.10,
	})

	gate.RecordFill(domain.SideSell, 100.0, 90.0, 10, 1000.0)
	gate.RecordFill(domain.SideSell, 100.0, 110.0, 5, 1000.0)

	status := gate.Status()
	loss, _ := status["daily_loss"].(float64)
	if loss != 0.10 {
		t.Fatalf("expected daily_loss 0.10 (gain should not reduce), got %.4f", loss)
	}
}

func TestRiskGate_RecordFill_BuyDirection(t *testing.T) {
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0.05,
		VaRCriticalThreshold: 0.10,
	})

	gate.RecordFill(domain.SideBuy, 100.0, 110.0, 10, 1000.0)

	status := gate.Status()
	loss, _ := status["daily_loss"].(float64)
	if loss != 0.10 {
		t.Fatalf("expected daily_loss 0.10 after buy-losing fill, got %.4f", loss)
	}
}

func TestRiskGate_Check_ZerodConfig(t *testing.T) {
	// With zero MaxDailyLossPct and VaRCriticalThreshold, the gate should
	// not block orders based on those limits (only halt matters).
	gate := NewRiskGate(RiskGateConfig{
		MaxDailyLossPct:      0,
		VaRCriticalThreshold: 0,
	})
	gate.UpdateDailyLoss(100)
	gate.UpdateVaR(100)

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1000, Price: 500}

	if err := gate.Check(context.Background(), order); err != nil {
		t.Fatalf("expected order to be allowed with zero config, got: %v", err)
	}
}
