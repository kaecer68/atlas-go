package stress

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ScenarioResult holds the simulation outcome for a single scenario.
type ScenarioResult struct {
	ScenarioID             string
	ScenarioName           string
	TotalReturn            float64
	MaxDrawdown            float64
	SharpeRatio            float64
	SortinoRatio           float64
	VaR95                  float64
	TradeCount             int
	FinalRegime            domain.Regime
	MomentumDisabled       bool
	RecoveryDays           int       // days from MDD trough back to prior peak (−1 if not recovered)
	MaxConsecutiveLossDays int       // longest streak of negative daily returns
	DailyValues            []float64 // V(t) path, normalized to V(0)=1.0
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
	registry    domain.AgentRegistry
	policy      domain.ExecutionPolicy
	covMatrix   [][]float64
	covSymbols  []string
	portWeights map[string]float64
}

func (r *Runner) SetCovariance(matrix [][]float64, symbols []string) {
	r.covMatrix, r.covSymbols = matrix, symbols
}
func (r *Runner) SetPortfolioWeights(weights map[string]float64) { r.portWeights = weights }

// NewRunner creates a stress test runner.
func NewRunner(registry domain.AgentRegistry, policy domain.ExecutionPolicy) *Runner {
	return &Runner{registry: registry, policy: policy}
}

// RunScenario simulates a multi-day stress scenario using shock-decay macro conditions.
//
// For each day t=0..W−1:
//  1. Compute decayed macro shock from scenario.Quotes
//  2. Generate daily portfolio return from shock × symbol sensitivity
//  3. Track cumulative value V(t)
//
// After the path is built, compute real metrics from V(t).
func (r *Runner) RunScenario(scenario Scenario, stockQuotes []domain.Quote, recs []domain.Recommendation) ScenarioResult {
	W := scenario.WindowDays
	if W < 1 {
		W = 20
	}

	// Build per-symbol return signs from recommendations.
	symbolSign := make(map[string]float64)
	for _, rec := range recs {
		symbolSign[rec.Symbol] += float64(rec.Conviction) / 100.0
	}
	for sym, v := range symbolSign {
		symbolSign[sym] = 1.0
		if v < 0 {
			symbolSign[sym] = -1.0
		}
	}
	for _, q := range stockQuotes {
		if _, ok := symbolSign[q.Symbol]; !ok {
			symbolSign[q.Symbol] = -1.0
		}
	}

	vix := scenario.VIXLevel()
	volScale := vix / 20.0
	if volScale < 0.5 {
		volScale = 0.5
	}

	if r.covMatrix != nil && len(r.covSymbols) > 0 && r.portWeights != nil {
		return r.runScenarioCov(scenario, vix, volScale)
	}

	goldSyms := map[string]bool{"GLD": true, "IAU": true, "00635U": true, "SLV": true}

	n := len(stockQuotes)
	values := make([]float64, W+1)
	values[0] = 1.0
	rng := rand.New(rand.NewPCG(42, 0))

	for t := 0; t < W; t++ {
		decay := decayFactor(t, W)
		dailyVol := volScale * decay * 0.06
		baseDrift := -dailyVol * 0.5
		if vix < 20 {
			baseDrift = dailyVol * 0.1
		}

		portRet := baseDrift
		for i := range n {
			sym := stockQuotes[i].Symbol
			sign := symbolSign[sym]
			if goldSyms[sym] {
				sign = 1.0
				if vix < 25 {
					sign = -0.5
				}
			}
			noise := boxMullerStress(rng) * dailyVol
			portRet += (sign*dailyVol*0.02 + noise) / float64(n)
		}

		values[t+1] = values[t] * (1 + portRet)
	}

	mdd, troughDay := maxDrawdown(values)
	sharpe := sharpeRatio(values)
	sortino := sortinoRatio(values)
	vaR95 := historicalVaR(values, 0.95)
	recovery := recoveryDays(values, troughDay)
	consecutive := maxConsecutiveLossDays(values)

	totalRet := values[W] - 1.0

	return ScenarioResult{
		ScenarioID:             scenario.ID,
		ScenarioName:           scenario.Name,
		TotalReturn:            totalRet,
		MaxDrawdown:            mdd,
		SharpeRatio:            sharpe,
		SortinoRatio:           sortino,
		VaR95:                  vaR95,
		TradeCount:             len(recs),
		FinalRegime:            scenario.Regime,
		MomentumDisabled:       scenario.VIXLevel() > 30,
		RecoveryDays:           recovery,
		MaxConsecutiveLossDays: consecutive,
		DailyValues:            values,
	}
}

func (r *Runner) runScenarioCov(scenario Scenario, vix, volScale float64) ScenarioResult {
	W := max(scenario.WindowDays, 1)
	N := len(r.covSymbols)
	weightSlice := make([]float64, N)
	var wSum float64
	for i, sym := range r.covSymbols {
		weightSlice[i] = r.portWeights[sym]
		wSum += weightSlice[i]
	}
	if wSum > 0 {
		for i := range weightSlice {
			weightSlice[i] /= wSum
		}
	}
	L := choleskyDecompose(r.covMatrix)
	rng := rand.New(rand.NewPCG(42, 0))
	values := make([]float64, W+1)
	values[0] = 1.0
	baseDrift := -volScale * 0.015
	if vix < 20 {
		baseDrift = volScale * 0.005
	}
	for t := range W {
		decay := decayFactor(t, W)
		z := make([]float64, N)
		for i := range N {
			z[i] = boxMullerStress(rng)
		}
		var portRet float64
		for i := range N {
			var ri float64
			for j := 0; j <= i; j++ {
				ri += L[i][j] * z[j]
			}
			portRet += weightSlice[i] * ri * volScale * 0.02 * decay
		}
		portRet += baseDrift
		values[t+1] = values[t] * (1 + portRet)
	}
	mdd, troughDay := maxDrawdown(values)
	sharpe := sharpeRatio(values)
	sortino := sortinoRatio(values)
	vaR95 := historicalVaR(values, 0.95)
	recovery := recoveryDays(values, troughDay)
	consecutive := maxConsecutiveLossDays(values)
	totalRet := values[W] - 1.0
	return ScenarioResult{
		ScenarioID: scenario.ID, ScenarioName: scenario.Name, TotalReturn: totalRet,
		MaxDrawdown: mdd, SharpeRatio: sharpe, SortinoRatio: sortino, VaR95: vaR95, TradeCount: 0,
		FinalRegime: scenario.Regime, MomentumDisabled: vix > 30,
		RecoveryDays: recovery, MaxConsecutiveLossDays: consecutive, DailyValues: values,
	}
}

func choleskyDecompose(a [][]float64) [][]float64 {
	n := len(a)
	if n == 0 {
		return nil
	}
	L := make([][]float64, n)
	for i := range L {
		L[i] = make([]float64, n)
	}
	for i := range n {
		for j := 0; j <= i; j++ {
			sum := 0.0
			for k := 0; k < j; k++ {
				sum += L[i][k] * L[j][k]
			}
			if i == j {
				val := a[i][i] - sum
				if val <= 0 {
					val = 1e-10
				}
				L[i][j] = math.Sqrt(val)
			} else {
				L[i][j] = (a[i][j] - sum) / L[j][j]
			}
		}
	}
	return L
}

// VIXLevel extracts the VIX value from scenario macro quotes.
func (s Scenario) VIXLevel() float64 {
	for _, q := range s.Quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			return q.Last
		}
	}
	return 20
}

// decayFactor computes exponential shock decay: e^(−λ·t).
// λ = 0.15/day → shock at 20% after 10 days, 5% after 20 days.
func decayFactor(t, W int) float64 {
	const lambda = 0.15
	if W <= 0 {
		return 1.0
	}
	return math.Exp(-lambda * float64(t))
}

// maxDrawdown computes peak-to-trough drawdown from a value path.
// Returns MDD and the day index of the trough.
func maxDrawdown(values []float64) (float64, int) {
	peak := values[0]
	mdd := 0.0
	troughDay := 0
	for i, v := range values {
		if v > peak {
			peak = v
		}
		dd := (peak - v) / peak
		if dd > mdd {
			mdd = dd
			troughDay = i
		}
	}
	return mdd, troughDay
}

// sharpeRatio computes annualized Sharpe from a price series.
func sharpeRatio(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	returns := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		returns[i-1] = (values[i] - values[i-1]) / values[i-1]
	}
	return portfolio.ComputeSharpe(returns, portfolio.SharpeConfig{
		Frequency:  portfolio.FrequencyPerDay,
		MinSamples: 2,
	})
}

func sortinoRatio(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	returns := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		returns[i-1] = (values[i] - values[i-1]) / values[i-1]
	}
	return shared.ComputeSortino(returns, shared.SortinoConfig{
		Frequency:  shared.FrequencyPerDay,
		MinSamples: 2,
	})
}

// historicalVaR computes Value-at-Risk from the return distribution.
func historicalVaR(values []float64, confidence float64) float64 {
	if len(values) < 2 {
		return 0
	}
	returns := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		returns[i-1] = (values[i] - values[i-1]) / values[i-1]
	}
	sort.Float64s(returns)
	idx := int((1 - confidence) * float64(len(returns)))
	if idx < 0 {
		idx = 0
	}
	return -returns[idx]
}

// recoveryDays counts days from the trough until the portfolio recovers to the prior peak.
// Returns −1 if it never recovers within the window.
func recoveryDays(values []float64, troughDay int) int {
	if troughDay < 0 || troughDay >= len(values) {
		return -1
	}
	peakBefore := values[0]
	for i := 0; i <= troughDay; i++ {
		if values[i] > peakBefore {
			peakBefore = values[i]
		}
	}
	for d := troughDay + 1; d < len(values); d++ {
		if values[d] >= peakBefore {
			return d - troughDay
		}
	}
	return -1
}

// maxConsecutiveLossDays returns the longest streak of negative daily returns.
func maxConsecutiveLossDays(values []float64) int {
	maxStreak, streak := 0, 0
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			streak++
			if streak > maxStreak {
				maxStreak = streak
			}
		} else {
			streak = 0
		}
	}
	return maxStreak
}

// boxMullerStress generates a standard normal variate.
func boxMullerStress(rng *rand.Rand) float64 {
	u1 := rng.Float64()
	u2 := rng.Float64()
	return math.Sqrt(-2*math.Log(max(u1, 1e-10))) * math.Cos(2*math.Pi*u2)
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

		if result.MaxDrawdown > worstDrawdown {
			worstDrawdown = result.MaxDrawdown
		}
		if result.VaR95 > worstVaR {
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
		fmt.Fprintf(&output, "Scenario: %s (%d days)\n", r.ScenarioName, len(r.DailyValues)-1)
		fmt.Fprintf(&output, "  Return:     %.2f%%\n", r.TotalReturn*100)
		fmt.Fprintf(&output, "  Max DD:     %.2f%%\n", r.MaxDrawdown*100)
		fmt.Fprintf(&output, "  Sharpe:     %.2f\n", r.SharpeRatio)
		fmt.Fprintf(&output, "  VaR95:      %.2f%%\n", r.VaR95*100)
		fmt.Fprintf(&output, "  Recov Days: %d\n", r.RecoveryDays)
		fmt.Fprintf(&output, "  Consec Loss: %d days\n", r.MaxConsecutiveLossDays)
		fmt.Fprintf(&output, "  TradeCount: %d\n", r.TradeCount)
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
