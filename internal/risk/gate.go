package risk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
	mu              sync.RWMutex
	mode            RiskGateMode
	preTrade        *PreTradeGate
	inTrade         *InTradeGate
	postTrade       *PostTradeGate
	subs            []func(RiskDecision)
	lastDecision    RiskDecision
	lastCalibration *CalibrationReport
}

// NewRiskGate creates a RiskGate with the given phase-specific gates.
func NewRiskGate(preTrade *PreTradeGate, inTrade *InTradeGate, postTrade *PostTradeGate) *RiskGate {
	return &RiskGate{
		mode:      ModeNormal,
		preTrade:  preTrade,
		inTrade:   inTrade,
		postTrade: postTrade,
	}
}

// PreTradeCheck evaluates a proposed order against all pre-trade risk rules.
func (g *RiskGate) PreTradeCheck(ctx context.Context, order OrderIntent, pf PortfolioState) (*RiskDecision, error) {
	g.mu.RLock()
	mode := g.mode
	g.mu.RUnlock()

	if g.preTrade == nil {
		return nil, fmt.Errorf("pre_trade gate not initialized")
	}

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
		g.publish(ctx, *dec)
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

	g.publish(ctx, *decision)
	return decision, nil
}

// InTradeCheck evaluates all open positions for in-trade risk conditions.
func (g *RiskGate) InTradeCheck(ctx context.Context, positions []InTradePosition, histVol, currentVol float64, dailyLossPct float64) (*RiskDecision, error) {
	g.mu.RLock()
	mode := g.mode
	g.mu.RUnlock()

	if g.inTrade == nil {
		return nil, fmt.Errorf("in_trade gate not initialized")
	}

	decision, err := g.inTrade.Evaluate(ctx, positions, histVol, currentVol, dailyLossPct, string(mode))
	if err != nil {
		return nil, fmt.Errorf("in_trade check: %w", err)
	}

	if decision.Verdict == VerdictHalt {
		g.SetMode(ModeSuspended)
	}

	g.publish(ctx, *decision)
	return decision, nil
}

// PostTradeCheck evaluates portfolio-level metrics and recommends mode changes.
func (g *RiskGate) PostTradeCheck(ctx context.Context, input PostTradeInput) (*RiskDecision, error) {
	g.mu.RLock()
	mode := g.mode
	g.mu.RUnlock()

	if g.postTrade == nil {
		return nil, fmt.Errorf("post_trade gate not initialized")
	}

	decision, err := g.postTrade.Evaluate(input, string(mode))
	if err != nil {
		return nil, fmt.Errorf("post_trade check: %w", err)
	}

	if decision.Mode != string(mode) {
		g.SetMode(RiskGateMode(decision.Mode))
	}

	g.publish(ctx, *decision)
	return decision, nil
}

// Mode returns the current risk gate mode.
func (g *RiskGate) Mode() RiskGateMode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.mode
}

// SetMode transitions the risk gate to a new mode and publishes a decision event.
// The mutex is released before publish() to avoid holding the write lock during
// EnrichDecision (a blocking LLM hook). This matches the pattern used by
// PreTradeCheck, InTradeCheck, and PostTradeCheck.
func (g *RiskGate) SetMode(mode RiskGateMode) {
	g.mu.Lock()
	prev := g.mode
	g.mode = mode
	g.mu.Unlock()

	// Bail early: no-op if mode unchanged.
	if prev == mode {
		return
	}

	// Mode change is a system-internal action without a request context.
	// Using context.Background() is the documented exception for this path.
	g.publish(context.Background(), RiskDecision{
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

func (g *RiskGate) publish(ctx context.Context, dec RiskDecision) {
	dec.ConfidenceCommentary = EnrichDecision(ctx, dec)
	g.mu.Lock()
	g.lastDecision = dec
	g.mu.Unlock()
	for _, sub := range g.subs {
		sub(dec)
	}
}

// RecordDecision records an externally-produced decision into the gate
// (#1785-D). The simulation engine's pre-trade checks flow through here so
// LastDecision (風控長評語 surface) reflects simulation activity — previously
// only the live/paper order path published decisions, leaving the panel
// permanently empty in dry-run deployments. Quiet variant: updates
// lastDecision without fanning out to subscribers (the daily sim produces
// hundreds of decisions per session; SSE spam is undesirable).
func (g *RiskGate) RecordDecision(dec RiskDecision) {
	g.mu.Lock()
	g.lastDecision = dec
	g.mu.Unlock()
}

// LastDecision returns the most recent risk decision, or a zero-value RiskDecision
// if no decision has been recorded yet.
func (g *RiskGate) LastDecision() RiskDecision {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastDecision
}

// Subscribe registers a callback for risk decision events.
func (g *RiskGate) Subscribe(fn func(RiskDecision)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.subs = append(g.subs, fn)
}

// SetPreTradeRSITwScore pushes the RSI-tw retail sentiment score to the
// underlying PreTradeGate. Scores near ±0.7 or beyond trigger the
// ruleRetailSentiment check.
func (g *RiskGate) SetPreTradeRSITwScore(score float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.preTrade.SetRSITwScore(score)
}

// WithMaturityTracker injects a maturity tracker into the pre-trade gate
// for burn-in / calibrating phase gating of VaR checks.
func (g *RiskGate) WithMaturityTracker(mt *domain.MaturityTracker) *RiskGate {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.preTrade != nil {
		g.preTrade.WithMaturityTracker(mt)
	}
	return g
}

// SetLastCalibration stores the most recent calibration run result.
func (g *RiskGate) SetLastCalibration(report *CalibrationReport) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastCalibration = report
}

// LastCalibrationReport returns the most recent calibration result, or nil.
func (g *RiskGate) LastCalibrationReport() *CalibrationReport {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.lastCalibration == nil {
		return nil
	}
	cp := *g.lastCalibration
	return &cp
}
