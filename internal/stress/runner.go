package stress

import (
	"fmt"
	"math"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// ScenarioResult holds the simulation outcome for a single scenario.
type ScenarioResult struct {
	ScenarioID       string
	ScenarioName     string
	TotalReturn      float64
	MaxDrawdown      float64
	SharpeRatio      float64
	VaR95            float64
	TradeCount       int
	FinalRegime      domain.Regime
	MomentumDisabled bool
}

// Report aggregates results across all scenarios.
type Report struct {
	ScenarioResults []ScenarioResult
	BaselineResult  *ScenarioResult
	WorstDrawdown   float64
	WorstVaR        float64
	AvgReturn       float64
}

// Runner executes stress test scenarios.
type Runner struct {
	registry domain.AgentRegistry
	policy   domain.ExecutionPolicy
	plugins  *orchestrator.PluginRegistry
}

// NewRunner creates a stress test runner.
func NewRunner(registry domain.AgentRegistry, policy domain.ExecutionPolicy) *Runner {
	return &Runner{
		registry: registry,
		policy:   policy,
		plugins:  orchestrator.NewPluginRegistry(),
	}
}

// WithPlugins attaches a custom plugin registry.
func (r *Runner) WithPlugins(plugins *orchestrator.PluginRegistry) *Runner {
	r.plugins = plugins
	return r
}

// RunScenario executes a single scenario and returns the result.
func (r *Runner) RunScenario(scenario Scenario, stockQuotes []domain.Quote, recs []domain.Recommendation) ScenarioResult {
	quotes := scenario.MergeQuotes(stockQuotes)
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	// Use scenario regime if available
	regime := scenario.Regime
	if regime == "" {
		regime = domain.RegimeNeutral
	}

	// Determine if momentum crash protection was triggered
	momentumDisabled := false
	for _, q := range scenario.Quotes {
		if (q.Symbol == "VIX" || q.Symbol == "^VIX") && q.Last > 30 {
			momentumDisabled = true
			break
		}
	}

	constraints := domain.SimulationConstraints{
		StartingCash:                10000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            10,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          1,
		SlippageBPS:                 5,
		ReserveCashFraction:         0.1,
	}

	engine := sim.NewEngine(constraints).WithSlippageModel(sim.DefaultSlippageModel())

	result := engine.Run(regime, quotes, recs)

	// Calculate metrics from simulation state if available
	// For single-day simulation, use basic metrics
	portfolioValue := result.PortfolioValue
	startingCash := constraints.StartingCash
	totalReturn := 0.0
	if startingCash > 0 {
		totalReturn = (portfolioValue - startingCash) / startingCash
	}

	return ScenarioResult{
		ScenarioID:       scenario.ID,
		ScenarioName:     scenario.Name,
		TotalReturn:      totalReturn,
		MaxDrawdown:      0, // Single day, no drawdown
		SharpeRatio:      0, // Single day, no Sharpe
		VaR95:            0, // Single day, no VaR
		TradeCount:       len(result.Orders),
		FinalRegime:      regime,
		MomentumDisabled: momentumDisabled,
	}
}

// RunAll executes all built-in scenarios with the given stock quotes and recommendations.
func (r *Runner) RunAll(stockQuotes []domain.Quote, recs []domain.Recommendation) Report {
	scenarios := AllScenarios()
	results := make([]ScenarioResult, 0, len(scenarios))

	var worstDrawdown, worstVaR, totalReturn float64
	var baselineResult *ScenarioResult

	for _, scenario := range scenarios {
		result := r.RunScenario(scenario, stockQuotes, recs)
		results = append(results, result)

		if math.Abs(result.MaxDrawdown) > math.Abs(worstDrawdown) {
			worstDrawdown = result.MaxDrawdown
		}
		if result.VaR95 < worstVaR {
			worstVaR = result.VaR95
		}
		totalReturn += result.TotalReturn

		if scenario.ID == ScenarioNormalMarket.ID {
			cp := result
			baselineResult = &cp
		}
	}

	avgReturn := 0.0
	if len(results) > 0 {
		avgReturn = totalReturn / float64(len(results))
	}

	return Report{
		ScenarioResults: results,
		BaselineResult:  baselineResult,
		WorstDrawdown:   worstDrawdown,
		WorstVaR:        worstVaR,
		AvgReturn:       avgReturn,
	}
}

// FormatReport returns a human-readable summary of the stress test report.
func FormatReport(report Report) string {
	var output strings.Builder
	output.WriteString("=== Stress Test Report ===\n\n")
	for _, r := range report.ScenarioResults {
		fmt.Fprintf(&output, "Scenario: %s\n", r.ScenarioName)
		fmt.Fprintf(&output, "  Return:     %.2f%%\n", r.TotalReturn*100)
		fmt.Fprintf(&output, "  Drawdown:   %.2f%%\n", r.MaxDrawdown*100)
		fmt.Fprintf(&output, "  VaR95:      %.2f%%\n", r.VaR95*100)
		fmt.Fprintf(&output, "  Trades:     %d\n", r.TradeCount)
		fmt.Fprintf(&output, "  Regime:     %s\n", r.FinalRegime)
		if r.MomentumDisabled {
			output.WriteString("  Momentum:   DISABLED (VIX > 30)\n")
		}
		output.WriteString("\n")
	}
	fmt.Fprintf(&output, "Worst Drawdown: %.2f%%\n", report.WorstDrawdown*100)
	fmt.Fprintf(&output, "Worst VaR95:    %.2f%%\n", report.WorstVaR*100)
	fmt.Fprintf(&output, "Avg Return:     %.2f%%\n", report.AvgReturn*100)
	return output.String()
}
