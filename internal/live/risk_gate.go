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
//
// DECLARED ONLY / NOT ENFORCED (FU-20261016-32, verified 2026-10-16).
//
// The gate itself is live — OrderManager.checkRiskGate calls RiskGate.Check
// (order_manager.go:128) on every attempt — but nothing in production writes the
// halt flag, so the "trading halted" branch in Check is unreachable today. All
// three writers are test-only: SetHaltTrading, SyncHaltFromCircuitState and
// SyncRiskGateFromCircuitBreaker. The live CircuitBreaker can reach CircuitHalted
// / CircuitPaused, and SyncRiskGateFromCircuitBreaker exists precisely to mirror
// that state, but no orchestrator loop calls it.
//
// The halt therefore rejects no order in production today: it is a flag with no
// writer, not a halt that sometimes fires. Wiring a writer is a production
// risk-behavior change, so it is proposed (FU-20261016-32) and not applied
// here. Re-certification conditions (all of them):
//  1. An owner decision on which writer is authoritative (circuit breaker state
//     vs. an operator action) and on what clears the halt.
//  2. An end-to-end test: CircuitBreaker in CircuitHalted ⇒ a submitted order is
//     rejected with the "trading halted" error, and clearing the breaker lets it
//     through.
//  3. Observability for the halt flag (RiskGate.Status already exposes
//     halt_trading) plus an alert path, so a stuck halt is visible.
//  4. Removal of this entry from
//     internal/risk/exported_risk_levers_wiring_test.go and closing
//     FU-20261016-32 in the same commit.
func (g *RiskGate) SetHaltTrading(halt bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltTrading = halt
}

// SyncHaltFromCircuitState mirrors the circuit breaker state into the risk
// gate's halt flag. Halted and Paused both block new orders; Normal clears
// the halt. This is the canonical wire point between CircuitBreaker and
// RiskGate.
func (g *RiskGate) SyncHaltFromCircuitState(state CircuitState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltTrading = state == CircuitHalted || state == CircuitPaused
}

// SyncRiskGateFromCircuitBreaker is the canonical wire point between the
// circuit breaker and the risk gate. It mirrors the circuit state into the
// risk gate's halt flag and, if a non-nil provider is given, refreshes the
// VaR estimate.
//
// NOT WIRED (FU-20261016-32, verified 2026-10-16): this function has no caller
// at all, production or test. The orchestrator does not call it on a schedule —
// the only circuit-breaker integration in the live path is
// Orchestrator.circuitBreaker, whose state is never mirrored into RiskGate. The
// function is kept so the intended wire point is not lost, and it is listed in
// internal/risk/exported_risk_levers_wiring_test.go so that a first caller fails
// the guard until the re-certification conditions on SetHaltTrading are met.
func SyncRiskGateFromCircuitBreaker(rg *RiskGate, cb *CircuitBreaker, varProvider func() float64) {
	if rg == nil || cb == nil {
		return
	}
	rg.SyncHaltFromCircuitState(cb.State())
	if varProvider != nil {
		rg.UpdateVaR(varProvider())
	}
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
	g.maybeResetDaily()
	g.dailyLoss = loss
}

// RecordFill ingests a fill event and updates the daily loss accumulator
// when the fill resulted in a loss. Gains do not reduce the accumulator.
//
// For SELL fills: PnL = (fillPrice - orderPrice) * qty
// For BUY fills:  PnL = (orderPrice - fillPrice) * qty
//
// portfolioValue is the pre-fill portfolio notional used to express loss
// as a fraction. If non-positive, the call is a no-op.
func (g *RiskGate) RecordFill(side domain.Side, orderPrice, fillPrice, qty, portfolioValue float64) {
	if portfolioValue <= 0 || qty == 0 {
		return
	}
	var pnl float64
	if side == domain.SideSell {
		pnl = (fillPrice - orderPrice) * qty
	} else {
		pnl = (orderPrice - fillPrice) * qty
	}
	if pnl >= 0 {
		return
	}
	lossFraction := -pnl / portfolioValue

	g.mu.Lock()
	defer g.mu.Unlock()
	g.maybeResetDaily()
	if lossFraction > g.dailyLoss {
		g.dailyLoss = lossFraction
	}
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
