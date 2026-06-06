package risk

import (
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// PostTradeGate evaluates portfolio-level metrics after a trading session and
// recommends risk mode transitions (CAUTIOUS, DEFENSIVE, SUSPENDED) based on
// drawdown, rolling Sharpe, consecutive losses, and other portfolio health indicators.
type PostTradeGate struct {
	maxDrawdownHaltPct      float64
	maxDrawdownDefensivePct float64
	minRollingSharpe        float64
	consecutiveLossDays     int
}

// NewPostTradeGate creates a PostTradeGate using centralized parameter config.
func NewPostTradeGate() *PostTradeGate {
	cfg := config.GetParametersConfig()
	return &PostTradeGate{
		maxDrawdownHaltPct:      cfg.RiskGate.PostTrade.MaxDrawdownHaltPct.Value,
		maxDrawdownDefensivePct: cfg.RiskGate.PostTrade.MaxDrawdownDefensivePct.Value,
		minRollingSharpe:        cfg.RiskGate.PostTrade.MinRollingSharpe.Value,
		consecutiveLossDays:     cfg.RiskGate.PostTrade.ConsecutiveLossDays.Value,
	}
}

// PostTradeInput contains all data needed by the PostTradeGate for evaluation.
type PostTradeInput struct {
	CurrentDrawdownPct float64
	RollingSharpe      float64
	ConsecutiveLosses  int
}

// Evaluate runs all post-trade rules and returns the recommended mode transition
// along with detailed rule results.
func (g *PostTradeGate) Evaluate(input PostTradeInput, currentMode string) (*RiskDecision, error) {
	decision := &RiskDecision{
		Phase:    PhasePostTrade,
		Verdict:  VerdictAlertOnly,
		Mode:     currentMode,
		Recorded: time.Now(),
	}

	var details []RuleResult

	r := g.checkDrawdown(input.CurrentDrawdownPct)
	details = append(details, r)
	if !r.Passed && r.Severity == "CRITICAL" {
		decision.Verdict = VerdictHalt
		decision.Reason = r.Message
		decision.Mode = string(ModeSuspended)
		decision.Action = RiskAction{
			Type:        ActionFreeze,
			Description: fmt.Sprintf("critical drawdown %.1f%% - switching to SUSPENDED", input.CurrentDrawdownPct*100),
		}
	} else if !r.Passed {
		if decision.Verdict.Level() < LevelAlertOnly {
			decision.Verdict = VerdictAlertOnly
		}
		decision.Mode = string(ModeDefensive)
		decision.Reason = r.Message
		decision.Action = RiskAction{
			Type:        ActionReduce,
			TargetPct:   0.5,
			Description: fmt.Sprintf("defensive drawdown %.1f%% - halving positions", input.CurrentDrawdownPct*100),
		}
	}

	r = g.checkRollingSharpe(input.RollingSharpe)
	details = append(details, r)
	if !r.Passed && decision.Verdict.Level() < LevelAlertOnly {
		decision.Verdict = VerdictAlertOnly
		decision.Mode = string(ModeCautious)
		decision.Reason = r.Message
		decision.Action = RiskAction{
			Type:        ActionReduce,
			TargetPct:   0.8,
			Description: fmt.Sprintf("rolling Sharpe %.2f below threshold - switching to CAUTIOUS", input.RollingSharpe),
		}
	}

	r = g.checkConsecutiveLosses(input.ConsecutiveLosses)
	details = append(details, r)
	if !r.Passed && decision.Verdict.Level() < LevelAlertOnly {
		decision.Verdict = VerdictAlertOnly
		decision.Mode = string(ModeCautious)
		decision.Reason = r.Message
	}

	decision.Details = details
	return decision, nil
}

func (g *PostTradeGate) checkDrawdown(drawdownPct float64) RuleResult {
	switch {
	case drawdownPct >= g.maxDrawdownHaltPct:
		return RuleResult{
			RuleName:     "max_drawdown_halt",
			Passed:       false,
			CurrentValue: drawdownPct,
			Threshold:    g.maxDrawdownHaltPct,
			Severity:     "CRITICAL",
			Message:      fmt.Sprintf("drawdown %.1f%% exceeds halt threshold %.0f%%", drawdownPct*100, g.maxDrawdownHaltPct*100),
		}
	case drawdownPct >= g.maxDrawdownDefensivePct:
		return RuleResult{
			RuleName:     "max_drawdown_defensive",
			Passed:       false,
			CurrentValue: drawdownPct,
			Threshold:    g.maxDrawdownDefensivePct,
			Severity:     "WARNING",
			Message:      fmt.Sprintf("drawdown %.1f%% exceeds defensive threshold %.0f%%", drawdownPct*100, g.maxDrawdownDefensivePct*100),
		}
	default:
		return RuleResult{
			RuleName:     "max_drawdown",
			Passed:       true,
			CurrentValue: drawdownPct,
			Threshold:    g.maxDrawdownDefensivePct,
			Severity:     "INFO",
			Message:      fmt.Sprintf("drawdown %.1f%% within limits", drawdownPct*100),
		}
	}
}

func (g *PostTradeGate) checkRollingSharpe(sharpe float64) RuleResult {
	passed := sharpe >= g.minRollingSharpe
	sev := "INFO"
	if !passed {
		sev = "WARNING"
	}
	return RuleResult{
		RuleName:     "rolling_sharpe",
		Passed:       passed,
		CurrentValue: sharpe,
		Threshold:    g.minRollingSharpe,
		Severity:     sev,
		Message:      fmt.Sprintf("rolling Sharpe %.2f (minimum %.2f)", sharpe, g.minRollingSharpe),
	}
}

func (g *PostTradeGate) checkConsecutiveLosses(losses int) RuleResult {
	passed := losses < g.consecutiveLossDays
	sev := "INFO"
	if !passed {
		sev = "WARNING"
	}
	return RuleResult{
		RuleName:     "consecutive_losses",
		Passed:       passed,
		CurrentValue: float64(losses),
		Threshold:    float64(g.consecutiveLossDays),
		Severity:     sev,
		Message:      fmt.Sprintf("%d consecutive losses (limit %d)", losses, g.consecutiveLossDays),
	}
}

func CheckModeTransition(input PostTradeInput, currentMode string) (string, string) {
	switch {
	case math.Abs(input.CurrentDrawdownPct) >= 0.20:
		return string(ModeSuspended), fmt.Sprintf("drawdown %.0f%% exceeds 20%% limit", input.CurrentDrawdownPct*100)
	case math.Abs(input.CurrentDrawdownPct) >= 0.10:
		return string(ModeDefensive), fmt.Sprintf("drawdown %.0f%% exceeds 10%% threshold", input.CurrentDrawdownPct*100)
	case input.RollingSharpe < 0 && currentMode != string(ModeDefensive):
		return string(ModeCautious), fmt.Sprintf("rolling Sharpe %.2f is negative", input.RollingSharpe)
	case input.ConsecutiveLosses >= 5:
		return string(ModeCautious), fmt.Sprintf("%d consecutive losses", input.ConsecutiveLosses)
	default:
		return currentMode, ""
	}
}
