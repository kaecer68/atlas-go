package risk

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// PreTradeGate evaluates proposed orders against position limits, sector caps,
// VaR constraints, cash buffers, and correlation thresholds before execution.
type PreTradeGate struct {
	maxPositionPct float64
	maxSectorPct   float64
	varLimitPct    float64
	minCashBuffer  float64
	maxCorrelation float64
}

// NewPreTradeGate creates a PreTradeGate using values from the centralized parameter config.
func NewPreTradeGate() *PreTradeGate {
	cfg := config.GetParametersConfig()
	return &PreTradeGate{
		maxPositionPct: cfg.RiskGate.PreTrade.MaxPositionPct.Value,
		maxSectorPct:   cfg.RiskGate.PreTrade.MaxSectorExposurePct.Value,
		varLimitPct:    cfg.RiskGate.PreTrade.VarLimitPct.Value,
		minCashBuffer:  cfg.RiskGate.PreTrade.MinCashBufferPct.Value,
		maxCorrelation: cfg.RiskGate.PreTrade.MaxCorrelation.Value,
	}
}

func (g *PreTradeGate) MaxPositionPct() float64 { return g.maxPositionPct }
func (g *PreTradeGate) MaxSectorPct() float64   { return g.maxSectorPct }
func (g *PreTradeGate) VarLimitPct() float64    { return g.varLimitPct }
func (g *PreTradeGate) MinCashBuffer() float64  { return g.minCashBuffer }
func (g *PreTradeGate) MaxCorrelation() float64 { return g.maxCorrelation }

// Check evaluates a proposed order against all pre-trade risk rules and returns
// a unified RiskDecision. It stops at the first BLOCK/HALT rule; subsequent
// rules are still appended for diagnostics.
func (g *PreTradeGate) Check(_ context.Context, order OrderIntent, pf PortfolioState, mode string) (*RiskDecision, error) {
	if pf.TotalValue <= 0 {
		return nil, fmt.Errorf("pre_trade gate: portfolio total value is zero or negative")
	}

	decision := &RiskDecision{
		Phase:    PhasePreTrade,
		Verdict:  VerdictAllow,
		Mode:     mode,
		Symbol:   order.Symbol,
		Recorded: time.Now(),
	}

	var details []RuleResult

	r := g.ruleMaxPosition(order, pf)
	details = append(details, r)
	if !r.Passed && decision.Verdict < VerdictBlock {
		decision.Verdict = VerdictBlock
		decision.Reason = r.Message
	}

	r = g.ruleSectorExposure(order, pf)
	details = append(details, r)
	if !r.Passed && decision.Verdict < VerdictBlock {
		decision.Verdict = VerdictBlock
		decision.Reason = r.Message
	}

	r = g.ruleVaRLimit(order, pf)
	details = append(details, r)
	if !r.Passed && decision.Verdict < VerdictBlock {
		decision.Verdict = VerdictBlock
		decision.Reason = r.Message
	}

	r = g.ruleCashBuffer(order, pf)
	details = append(details, r)
	if !r.Passed && decision.Verdict < VerdictBlock {
		decision.Verdict = VerdictBlock
		decision.Reason = r.Message
	}

	if decision.Verdict == VerdictBlock {
		decision.Action = RiskAction{
			Type:        ActionFreeze,
			Description: decision.Reason,
		}
	}

	decision.Details = details
	return decision, nil
}

func (g *PreTradeGate) ruleMaxPosition(order OrderIntent, pf PortfolioState) RuleResult {
	newNotional := pf.Positions[order.Symbol] + order.Notional
	pct := newNotional / pf.TotalValue
	return RuleResult{
		RuleName:     "max_position_pct",
		Passed:       pct <= g.maxPositionPct,
		CurrentValue: pct,
		Threshold:    g.maxPositionPct,
		Severity:     verdictSeverity(pct, g.maxPositionPct),
		Message:      fmt.Sprintf("%s position %s would be %.1f%% of portfolio (limit %.0f%%)", order.Side, order.Symbol, pct*100, g.maxPositionPct*100),
	}
}

func (g *PreTradeGate) ruleSectorExposure(order OrderIntent, pf PortfolioState) RuleResult {
	if order.Sector == "" {
		return RuleResult{RuleName: "sector_exposure", Passed: true, Severity: "INFO"}
	}
	current := pf.SectorExposure[order.Sector]
	pct := (current + order.Notional) / pf.TotalValue
	return RuleResult{
		RuleName:     "sector_exposure",
		Passed:       pct <= g.maxSectorPct,
		CurrentValue: pct,
		Threshold:    g.maxSectorPct,
		Severity:     verdictSeverity(pct, g.maxSectorPct),
		Message:      fmt.Sprintf("%s sector exposure would be %.1f%% (limit %.0f%%)", order.Sector, pct*100, g.maxSectorPct*100),
	}
}

func (g *PreTradeGate) ruleVaRLimit(_ OrderIntent, pf PortfolioState) RuleResult {
	absVaR := math.Abs(pf.Var95)
	pct := absVaR / pf.TotalValue
	return RuleResult{
		RuleName:     "var_limit",
		Passed:       pct <= g.varLimitPct,
		CurrentValue: pct,
		Threshold:    g.varLimitPct,
		Severity:     verdictSeverity(pct, g.varLimitPct),
		Message:      fmt.Sprintf("portfolio VaR95 is %.2f%% of total value (limit %.0f%%)", pct*100, g.varLimitPct*100),
	}
}

func (g *PreTradeGate) ruleCashBuffer(order OrderIntent, pf PortfolioState) RuleResult {
	remaining := pf.Cash - order.Notional
	pct := remaining / pf.TotalValue
	return RuleResult{
		RuleName:     "cash_buffer",
		Passed:       pct >= g.minCashBuffer,
		CurrentValue: pct,
		Threshold:    g.minCashBuffer,
		Severity:     verdictSeverity(g.minCashBuffer-pct, g.minCashBuffer),
		Message:      fmt.Sprintf("post-trade cash would be %.1f%% of portfolio (minimum %.0f%%)", pct*100, g.minCashBuffer*100),
	}
}

func verdictSeverity(current, threshold float64) string {
	ratio := current / threshold
	switch {
	case ratio > 2.0:
		return "CRITICAL"
	case ratio > 1.5:
		return "WARNING"
	default:
		return "INFO"
	}
}

var timeNow = func() time.Time { return time.Now() }
