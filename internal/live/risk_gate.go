package live

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// MetricRiskGateHaltTrading is the /metrics gauge that exposes the risk gate's
// halt flag (FU-20261016-32 re-certification condition 3: "observability for the
// halt flag plus an alert path").
//
// 1 = the gate rejects every order (the hard-halt branch of Check); 0 = it does
// not. It is recorded once per intraday cycle by Orchestrator.onIntradayCycle,
// and monitoring/rules/atlas_live_risk_gate_alerts.yml alerts when it stays at 1.
//
// Why the constant lives here and not in internal/monitoring (where every other
// Metric* constant lives): internal/monitoring imports internal/live/store, so
// the live engine cannot import it back. The name is therefore defined next to
// its producer, and a test in this package asserts the rule file still names it
// (see risk_gate_metrics_rule_test.go).
//
// ⚠️ Producer scope: this series only exists while the **research-only** live
// engine runs (`atlas -live`). Production runs `atlas -api` (Dockerfile CMD, and
// the -live flag was removed from the production compose in #2072 / E34), so the
// series is absent there and the alert cannot fire in production. That is stated
// in the rule file's header too; do not read the rule as production protection.
const MetricRiskGateHaltTrading = "atlas_live_risk_gate_halt_trading"

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
// NOT A PRODUCTION WRITER (FU-20261016-32, re-verified 2026-10-16).
//
// The halt flag is written on the production path by
// SyncRiskGateFromCircuitBreaker, which Orchestrator.onIntradayCycle calls after
// every circuit evaluation, so the "trading halted" branch of Check is live. This
// setter is the operator-facing variant and still has no caller: there is no
// manual-halt control surface yet, and no decision on what would clear a manual
// halt. It is kept so that surface has a named entry point, and it stays wired
// == false in internal/risk/exported_risk_levers_wiring_test.go so a first caller
// has to think about that control path.
func (g *RiskGate) SetHaltTrading(halt bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltTrading = halt
}

// SyncHaltFromCircuitState mirrors the circuit breaker state into the risk
// gate's halt flag. Halted and Paused both block new orders; Normal clears
// the halt. This is the canonical wire point between CircuitBreaker and
// RiskGate, and it is live: SyncRiskGateFromCircuitBreaker calls it, and
// Orchestrator.onIntradayCycle calls that after every circuit evaluation.
//
// It is listed as wired == false in
// internal/risk/exported_risk_levers_wiring_test.go only because its single
// caller sits in this same file, which that guard excludes from its
// external-reference scan; the row carries wiredVia to say so.
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
// WIRED (FU-20261016-32, 2026-10-16): Orchestrator.onIntradayCycle calls this
// with a nil provider right after every circuit evaluation, so the gate's halt
// follows CircuitHalted / CircuitPaused and clears when the breaker returns to
// CircuitNormal. The provider is nil on purpose: the live VaR estimate is not
// calibrated yet, and mirroring it would put an unvalidated number on the
// order-reject path. Callers that do pass a provider must accept that risk.
//
// cb is the interface the Orchestrator holds, not the concrete breaker, so the
// wire point does not have to type-assert.
func SyncRiskGateFromCircuitBreaker(rg *RiskGate, cb CircuitBreakerOps, varProvider func() float64) {
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

// halted reports the halt flag under the read lock.
//
// It is the typed accessor for the value Status() already exposes, used by
// Orchestrator.recordRiskGateHalt to publish MetricRiskGateHaltTrading. It stays
// unexported on purpose: the external surface of the flag is the metric and the
// Status() map, and an exported accessor would be one more symbol to keep
// truthful (see internal/risk/exported_risk_levers_wiring_test.go).
func (g *RiskGate) halted() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.haltTrading
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
