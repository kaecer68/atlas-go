package risk

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// PreTradeGate evaluates proposed orders against position limits, sector caps,
// VaR constraints, cash buffers, correlation thresholds, and RSI-tw retail
// sentiment extremes before execution.
type PreTradeGate struct {
	maxPositionPct   float64
	maxSectorPct     float64
	varLimitPct      float64
	minCashBuffer    float64
	maxCorrelation   float64
	maxOpenPositions int
	maturityTracker  *domain.MaturityTracker

	rsiTwScore float64 // RSI-tw extreme reading score; set via SetRSITwScore
}

// NewPreTradeGate creates a PreTradeGate using values from the centralized parameter config.
func NewPreTradeGate() *PreTradeGate {
	cfg := config.GetParametersConfig()
	return &PreTradeGate{
		maxPositionPct:   cfg.RiskGate.PreTrade.MaxPositionPct.Value,
		maxSectorPct:     cfg.RiskGate.PreTrade.MaxSectorExposurePct.Value,
		varLimitPct:      cfg.RiskGate.PreTrade.VarLimitPct.Value,
		minCashBuffer:    cfg.RiskGate.PreTrade.MinCashBufferPct.Value,
		maxCorrelation:   cfg.RiskGate.PreTrade.MaxCorrelation.Value,
		maxOpenPositions: cfg.RiskGate.PreTrade.MaxOpenPositions.Value,
	}
}

func (g *PreTradeGate) MaxPositionPct() float64 { return g.maxPositionPct }
func (g *PreTradeGate) MaxSectorPct() float64   { return g.maxSectorPct }
func (g *PreTradeGate) VarLimitPct() float64    { return g.varLimitPct }
func (g *PreTradeGate) MinCashBuffer() float64  { return g.minCashBuffer }
func (g *PreTradeGate) MaxCorrelation() float64 { return g.maxCorrelation }

// SetRSITwScore updates the RSI-tw extreme reading score used by the
// ruleRetailSentiment check.
func (g *PreTradeGate) MaxOpenPositions() int { return g.maxOpenPositions }

func (g *PreTradeGate) SetRSITwScore(score float64) {
	g.rsiTwScore = score
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (g *PreTradeGate) WithMaturityTracker(mt *domain.MaturityTracker) *PreTradeGate {
	g.maturityTracker = mt
	return g
}

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

	r = g.ruleMaxOpenPositions(order, pf)
	details = append(details, r)
	if !r.Passed && decision.Verdict < VerdictBlock {
		decision.Verdict = VerdictBlock
		decision.Reason = r.Message
	}

	r = g.ruleRetailSentiment(order, pf)
	details = append(details, r)
	if !r.Passed {
		switch {
		case r.Severity == "REDUCE" && decision.Verdict < VerdictReduce:
			decision.Verdict = VerdictReduce
			decision.Reason = r.Message
		case decision.Verdict < VerdictBlock:
			decision.Verdict = VerdictBlock
			decision.Reason = r.Message
		}
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

	// Dynamic position limit: higher conviction → higher limit.
	// Base 15%, scales linearly from conviction 35→100 up to 22%.
	// This rewards high-conviction picks with more allocation room.
	limit := g.maxPositionPct
	if order.Conviction > 50 {
		bonus := float64(order.Conviction-50) / 100.0 * 0.07
		limit = g.maxPositionPct + bonus
		if limit > 0.22 {
			limit = 0.22
		}
	}

	return RuleResult{
		RuleName:     "max_position_pct",
		Passed:       pct <= limit,
		CurrentValue: pct,
		Threshold:    limit,
		Severity:     verdictSeverity(pct, limit),
		Message:      fmt.Sprintf("%s position %s would be %.1f%% of portfolio (limit %.0f%%, conviction %d)", order.Side, order.Symbol, pct*100, limit*100, order.Conviction),
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
	// Burn-in / calibrating gate: VaR requires 252 days of history.
	// Before FULL_AUTO, pass the check but log a warning so operators
	// know the portfolio is running with static risk thresholds.
	if g.maturityTracker != nil {
		m := g.maturityTracker.Current()
		if m == domain.MaturityBurnIn || m == domain.MaturityCalibrating {
			daysUntilFull := g.maturityTracker.DaysUntil(domain.MaturityFullAuto)
			logging.Info("risk_gate", "var_warming",
				"maturity", string(m),
				"days_until_full_auto", daysUntilFull,
				"action", "var_check_passed_static_mode")
			return RuleResult{
				RuleName:     "var_limit",
				Passed:       true,
				CurrentValue: 0,
				Threshold:    g.varLimitPct,
				Severity:     "INFO",
				Message:      fmt.Sprintf("VaR warming: %d days until full_auto; using static threshold", daysUntilFull),
			}
		}
	}

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

func (g *PreTradeGate) ruleRetailSentiment(_ OrderIntent, _ PortfolioState) RuleResult {
	score := g.rsiTwScore
	passed := score > -0.5 && score < 0.5

	var severity, message string
	switch {
	case score >= 0.7:
		severity = "CRITICAL"
		message = fmt.Sprintf("extreme retail frenzy detected (RSI-tw=%.2f ≥0.7)", score)
	case score >= 0.5:
		severity = "REDUCE"
		message = fmt.Sprintf("retail frenzy detected (RSI-tw=%.2f ≥0.5) — position reduction recommended", score)
	case score <= -0.7:
		severity = "CRITICAL"
		message = fmt.Sprintf("extreme retail fear detected (RSI-tw=%.2f ≤-0.7)", score)
	case score <= -0.5:
		severity = "REDUCE"
		message = fmt.Sprintf("retail fear detected (RSI-tw=%.2f ≤-0.5) — position reduction recommended", score)
	default:
		severity = "INFO"
		message = fmt.Sprintf("RSI-tw score %.2f within normal range", score)
	}

	return RuleResult{
		RuleName:     "retail_sentiment",
		Passed:       passed,
		CurrentValue: score,
		Threshold:    0.5,
		Severity:     severity,
		Message:      message,
	}
}

func (g *PreTradeGate) ruleMaxOpenPositions(order OrderIntent, pf PortfolioState) RuleResult {
	currentCount := 0
	for _, notional := range pf.Positions {
		if notional > 0 {
			currentCount++
		}
	}
	newPosition := strings.EqualFold(order.Side, "buy") && pf.Positions[order.Symbol] <= 0
	projectedCount := currentCount
	if newPosition {
		projectedCount++
	}

	passed := projectedCount <= g.maxOpenPositions
	var message string
	if passed {
		message = fmt.Sprintf("projected positions %d/%d (current=%d)", projectedCount, g.maxOpenPositions, currentCount)
	} else {
		message = fmt.Sprintf("max open positions exceeded: projected %d > limit %d (current=%d)", projectedCount, g.maxOpenPositions, currentCount)
	}

	severity := "INFO"
	if !passed {
		severity = "WARNING"
	}

	return RuleResult{
		RuleName:     "max_open_positions",
		Passed:       passed,
		CurrentValue: float64(projectedCount),
		Threshold:    float64(g.maxOpenPositions),
		Severity:     severity,
		Message:      message,
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
