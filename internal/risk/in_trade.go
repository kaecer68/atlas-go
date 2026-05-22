package risk

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// InTradeGate monitors open positions and triggers stop-loss, take-profit,
// trailing-stop, volatility-spike, and circuit-breaker actions during a trading session.
type InTradeGate struct {
	stopLossPct           float64
	takeProfitPct         float64
	trailingStopATRMult   float64
	volSpikeMult          float64
	circuitBreakerLossPct float64
}

// NewInTradeGate creates an InTradeGate using centralized parameter config.
func NewInTradeGate() *InTradeGate {
	cfg := config.GetParametersConfig()
	return &InTradeGate{
		stopLossPct:           cfg.RiskGate.InTrade.StopLossPct.Value,
		takeProfitPct:         cfg.RiskGate.InTrade.TakeProfitPct.Value,
		trailingStopATRMult:   cfg.RiskGate.InTrade.TrailingStopATRMult.Value,
		volSpikeMult:          cfg.RiskGate.InTrade.VolatilitySpikeMult.Value,
		circuitBreakerLossPct: cfg.RiskGate.InTrade.CircuitBreakerDailyLossPct.Value,
	}
}

// InTradePosition holds the real-time state of a single position for in-trade evaluation.
type InTradePosition struct {
	Symbol            string  `json:"symbol"`
	EntryPrice        float64 `json:"entry_price"`
	CurrentPrice      float64 `json:"current_price"`
	Quantity          int     `json:"quantity"`
	UnrealizedPnLPct  float64 `json:"unrealized_pnl_pct"`
	ATR               float64 `json:"atr"`
	HighestPrice      float64 `json:"highest_price"`
}

// Evaluate checks all open positions for stop-loss, take-profit, trailing-stop,
// and volatility-spike conditions. Returns a combined RiskDecision with all rule results.
func (g *InTradeGate) Evaluate(_ context.Context, positions []InTradePosition, histVol, currentVol float64, dailyLossPct float64, mode string) (*RiskDecision, error) {
	decision := &RiskDecision{
		Phase:    PhaseInTrade,
		Verdict:  VerdictAllow,
		Mode:     mode,
		Recorded: time.Now(),
	}

	var allDetails []RuleResult

	for _, pos := range positions {
		r := g.checkStopLoss(pos)
		allDetails = append(allDetails, r)
		if !r.Passed && decision.Verdict < VerdictBlock {
			decision.Verdict = VerdictBlock
			decision.Reason = r.Message
			decision.Action = RiskAction{
				Type:        ActionSell,
				Symbols:     []string{pos.Symbol},
				Description: fmt.Sprintf("stop-loss triggered for %s at %.1f%% loss", pos.Symbol, pos.UnrealizedPnLPct*100),
			}
		}

		r = g.checkTakeProfit(pos)
		allDetails = append(allDetails, r)
		if !r.Passed && decision.Verdict < VerdictReduce {
			decision.Verdict = VerdictReduce
			decision.Reason = r.Message
			decision.Action = RiskAction{
				Type:        ActionReduce,
				Symbols:     []string{pos.Symbol},
				TargetPct:   0.5,
				Description: fmt.Sprintf("take-profit triggered for %s at %.1f%% gain", pos.Symbol, pos.UnrealizedPnLPct*100),
			}
		}

		r = g.checkTrailingStop(pos)
		allDetails = append(allDetails, r)
		if !r.Passed && decision.Verdict < VerdictBlock {
			decision.Verdict = VerdictBlock
			decision.Reason = r.Message
			decision.Action = RiskAction{
				Type:        ActionSell,
				Symbols:     []string{pos.Symbol},
				Description: fmt.Sprintf("trailing-stop triggered for %s", pos.Symbol),
			}
		}
	}

	r := g.checkVolatilitySpike(histVol, currentVol)
	allDetails = append(allDetails, r)
	if !r.Passed && decision.Verdict < VerdictReduce {
		decision.Verdict = VerdictReduce
		if decision.Action.Type == ActionSell {
			decision.Action.TargetPct = 0.5
		} else {
			decision.Action = RiskAction{
				Type:      ActionReduce,
				TargetPct: 0.5,
				Description: r.Message,
			}
		}
	}

	r = g.checkCircuitBreaker(dailyLossPct)
	allDetails = append(allDetails, r)
	if !r.Passed {
		decision.Verdict = VerdictHalt
		decision.Reason = r.Message
		decision.Action = RiskAction{
			Type:        ActionFreeze,
			Description: r.Message,
		}
	}

	decision.Details = allDetails
	return decision, nil
}

func (g *InTradeGate) checkStopLoss(pos InTradePosition) RuleResult {
	passed := pos.UnrealizedPnLPct >= g.stopLossPct
	return RuleResult{
		RuleName:     "stop_loss",
		Passed:       passed,
		CurrentValue: pos.UnrealizedPnLPct,
		Threshold:    g.stopLossPct,
		Severity:     severityForDiff(pos.UnrealizedPnLPct, g.stopLossPct),
		Message:      fmt.Sprintf("%s PnL %.1f%% (stop-loss at %.1f%%)", pos.Symbol, pos.UnrealizedPnLPct*100, g.stopLossPct*100),
	}
}

func (g *InTradeGate) checkTakeProfit(pos InTradePosition) RuleResult {
	passed := pos.UnrealizedPnLPct <= g.takeProfitPct
	return RuleResult{
		RuleName:     "take_profit",
		Passed:       passed,
		CurrentValue: pos.UnrealizedPnLPct,
		Threshold:    g.takeProfitPct,
		Severity:     "INFO",
		Message:      fmt.Sprintf("%s PnL %.1f%% (take-profit at %.1f%%)", pos.Symbol, pos.UnrealizedPnLPct*100, g.takeProfitPct*100),
	}
}

func (g *InTradeGate) checkTrailingStop(pos InTradePosition) RuleResult {
	if pos.HighestPrice <= 0 || pos.ATR <= 0 || pos.EntryPrice <= 0 {
		return RuleResult{RuleName: "trailing_stop", Passed: true, Severity: "INFO", Message: "insufficient data for trailing stop"}
	}
	if pos.CurrentPrice <= 0 {
		return RuleResult{RuleName: "trailing_stop", Passed: true, Severity: "INFO", Message: "no current price"}
	}
	maxRunup := (pos.HighestPrice - pos.EntryPrice) / pos.EntryPrice
	if maxRunup < 0.05 {
		return RuleResult{RuleName: "trailing_stop", Passed: true, Severity: "INFO", Message: "insufficient runup for trailing stop"}
	}
	trailDistance := pos.ATR * g.trailingStopATRMult / pos.EntryPrice
	trailLevel := 1.0 - trailDistance
	currentRatio := pos.CurrentPrice / pos.HighestPrice
	passed := currentRatio >= trailLevel
	return RuleResult{
		RuleName:     "trailing_stop",
		Passed:       passed,
		CurrentValue: currentRatio,
		Threshold:    trailLevel,
		Severity:     "WARNING",
		Message:      fmt.Sprintf("%s trailing stop: price ratio %.3f (threshold %.3f)", pos.Symbol, currentRatio, trailLevel),
	}
}

func (g *InTradeGate) checkVolatilitySpike(histVol, currentVol float64) RuleResult {
	if histVol <= 0 || g.volSpikeMult <= 0 {
		return RuleResult{RuleName: "volatility_spike", Passed: true, Severity: "INFO", Message: "insufficient volatility data"}
	}
	ratio := currentVol / histVol
	passed := ratio <= g.volSpikeMult
	return RuleResult{
		RuleName:     "volatility_spike",
		Passed:       passed,
		CurrentValue: ratio,
		Threshold:    g.volSpikeMult,
		Severity:     severityForDiff(ratio, g.volSpikeMult),
		Message:      fmt.Sprintf("volatility ratio %.1fx (threshold %.1fx)", ratio, g.volSpikeMult),
	}
}

func (g *InTradeGate) checkCircuitBreaker(dailyLossPct float64) RuleResult {
	passed := dailyLossPct >= g.circuitBreakerLossPct
	return RuleResult{
		RuleName:     "circuit_breaker",
		Passed:       passed,
		CurrentValue: dailyLossPct,
		Threshold:    g.circuitBreakerLossPct,
		Severity:     "CRITICAL",
		Message:      fmt.Sprintf("daily portfolio loss %.1f%% exceeds circuit breaker (%.1f%%)", dailyLossPct*100, g.circuitBreakerLossPct*100),
	}
}

func severityForDiff(current, threshold float64) string {
	diff := math.Abs(current - threshold)
	if current < threshold {
		if diff > threshold*0.5 {
			return "CRITICAL"
		}
		return "WARNING"
	}
	return "INFO"
}
