package live

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RiskGateConfig holds configuration parameters for the risk gate.
type RiskGateConfig struct {
	// MaxDailyLossPct is the maximum allowed daily loss as a fraction of the
	// reference value (e.g. 0.03 = 3%).
	MaxDailyLossPct float64

	// VaRCriticalThreshold is the VaR value above which trading should be
	// restricted.
	VaRCriticalThreshold float64
}

// RiskGate validates orders against current risk state before execution.
// It is the live trading subsystem's final pre-execution risk check.
type RiskGate struct {
	mu  sync.RWMutex
	cfg RiskGateConfig

	dailyLoss   float64
	varValue    float64
	haltTrading bool

	// today tracks the calendar date for midnight reset detection.
	today string
}

// NewRiskGate creates a new RiskGate with the given configuration.
func NewRiskGate(cfg RiskGateConfig) *RiskGate {
	return &RiskGate{
		cfg:   cfg,
		today: currentDateString(),
	}
}

// Check verifies whether an order can proceed given current risk state.
// Returns nil if the order is allowed, or an error describing why it was blocked.
func (g *RiskGate) Check(ctx context.Context, order domain.Order) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Auto-reset daily loss at midnight
	g.maybeResetDaily()

	// Check for hard halt
	if g.haltTrading {
		return fmt.Errorf("risk gate: trading halted; order for %s (%s %d) blocked", order.Symbol, order.Side, order.Quantity)
	}

	// Check daily loss limit
	if g.cfg.MaxDailyLossPct > 0 && g.dailyLoss > g.cfg.MaxDailyLossPct {
		return fmt.Errorf("risk gate: daily loss %.4f exceeds limit %.4f; order for %s (%s %d) blocked",
			g.dailyLoss, g.cfg.MaxDailyLossPct, order.Symbol, order.Side, order.Quantity)
	}

	// Check VaR critical threshold
	if g.cfg.VaRCriticalThreshold > 0 && g.varValue > g.cfg.VaRCriticalThreshold {
		return fmt.Errorf("risk gate: VaR %.4f exceeds critical threshold %.4f; order for %s (%s %d) blocked",
			g.varValue, g.cfg.VaRCriticalThreshold, order.Symbol, order.Side, order.Quantity)
	}

	return nil
}

// SetHaltTrading enables or disables a trading halt. When true, all orders
// are rejected until the halt is cleared.
func (g *RiskGate) SetHaltTrading(halt bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltTrading = halt
}

// UpdateVaR updates the current Value-at-Risk estimate.
func (g *RiskGate) UpdateVaR(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.varValue = value
}

// UpdateDailyLoss updates the accumulated daily loss as a fraction.
func (g *RiskGate) UpdateDailyLoss(loss float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dailyLoss = loss
	g.maybeResetDaily()
}

// ResetDaily resets the daily loss accumulator to zero.
func (g *RiskGate) ResetDaily() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dailyLoss = 0
	g.today = currentDateString()
}

// Status returns a snapshot of the current risk gate state for monitoring.
func (g *RiskGate) Status() map[string]any {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return map[string]any{
		"halt_trading":           g.haltTrading,
		"daily_loss":             g.dailyLoss,
		"max_daily_loss_pct":     g.cfg.MaxDailyLossPct,
		"var_value":              g.varValue,
		"var_critical_threshold": g.cfg.VaRCriticalThreshold,
		"date":                   g.today,
	}
}

// maybeResetDaily resets the daily loss if the date has changed.
// Must be called with the write lock held.
func (g *RiskGate) maybeResetDaily() {
	today := currentDateString()
	if g.today != today {
		g.dailyLoss = 0
		g.today = today
	}
}

// currentDateString returns the current date in "2006-01-02" format.
func currentDateString() string {
	return time.Now().Format("2006-01-02")
}
