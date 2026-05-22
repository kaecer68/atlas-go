package risk

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RiskGateMode represents the current operational mode of the risk gate.
type RiskGateMode string

const (
	ModeNormal    RiskGateMode = "NORMAL"
	ModeCautious  RiskGateMode = "CAUTIOUS"
	ModeDefensive RiskGateMode = "DEFENSIVE"
	ModeSuspended RiskGateMode = "SUSPENDED"
)

// RiskGate is the unified entry point for all pre-, in-, and post-trade risk checks.
// It delegates to phase-specific gates and maintains the system-wide risk mode.
type RiskGate struct {
	mu       sync.RWMutex
	mode     RiskGateMode
	preTrade *PreTradeGate
	subs     []func(RiskDecision)
}

// NewRiskGate creates a RiskGate with the given phase-specific gates.
func NewRiskGate(preTrade *PreTradeGate) *RiskGate {
	return &RiskGate{
		mode:     ModeNormal,
		preTrade: preTrade,
	}
}

// PreTradeCheck evaluates a proposed order against all pre-trade risk rules.
func (g *RiskGate) PreTradeCheck(ctx context.Context, order OrderIntent, pf PortfolioState) (*RiskDecision, error) {
	g.mu.RLock()
	mode := g.mode
	g.mu.RUnlock()

	if mode == ModeSuspended {
		dec := &RiskDecision{
			Phase:   PhasePreTrade,
			Verdict: VerdictBlock,
			Reason:  "risk gate suspended - all trading halted",
			Action: RiskAction{
				Type:        ActionFreeze,
				Description: "系統已暫停所有交易，請聯繫風控官",
			},
			Mode:     string(mode),
			Symbol:   order.Symbol,
			Recorded: time.Now(),
		}
		g.publish(*dec)
		return dec, nil
	}

	decision, err := g.preTrade.Check(ctx, order, pf, string(mode))
	if err != nil {
		return nil, fmt.Errorf("pre_trade check: %w", err)
	}

	if mode == ModeDefensive && decision.Action.TargetPct > 0.5 {
		decision.Action.TargetPct = 0.5
		decision.Details = append(decision.Details, RuleResult{
			RuleName: "defensive_mode_cap",
			Passed:   false,
			Severity: "WARNING",
			Message:  fmt.Sprintf("DEFENSIVE mode capped target position at 50%%"),
		})
	}

	g.publish(*decision)
	return decision, nil
}

// Mode returns the current risk gate mode.
func (g *RiskGate) Mode() RiskGateMode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.mode
}

// SetMode transitions the risk gate to a new mode and publishes a decision event.
func (g *RiskGate) SetMode(mode RiskGateMode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	prev := g.mode
	g.mode = mode
	if prev != mode {
		g.publish(RiskDecision{
			Phase:   PhasePostTrade,
			Verdict: VerdictAlertOnly,
			Mode:    string(mode),
			Reason:  fmt.Sprintf("risk gate mode changed from %s to %s", prev, mode),
			Action: RiskAction{
				Type:        ActionNotify,
				Description: fmt.Sprintf("風控模式切換：%s → %s", prev, mode),
			},
			Recorded: time.Now(),
		})
	}
}

func (g *RiskGate) publish(dec RiskDecision) {
	for _, sub := range g.subs {
		sub(dec)
	}
}

// Subscribe registers a callback for risk decision events.
func (g *RiskGate) Subscribe(fn func(RiskDecision)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.subs = append(g.subs, fn)
}
