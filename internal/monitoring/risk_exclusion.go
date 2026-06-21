package monitoring

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// RiskExclusionResult is the per-symbol outcome of the Layer 2.5 risk filter.
type RiskExclusionResult struct {
	Symbol      string       `json:"symbol"`
	Passed      bool         `json:"passed"`
	FailReasons []string     `json:"fail_reasons"`
	HighRisk    bool         `json:"high_risk"`
	RuleResults []RuleDetail `json:"rule_results"`
}

// RuleDetail mirrors the existing risk RuleResult pattern for auditability.
type RuleDetail struct {
	RuleName     string  `json:"rule_name"`
	Passed       bool    `json:"passed"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Severity     string  `json:"severity"`
	Message      string  `json:"message"`
}

// RiskManager is a minimal, local interface that decouples this filter from
// the concrete risk module implementation.
type RiskManager interface {
	VaRContribution(symbol string) (float64, error)
	VaR95() float64
}

// QuoteProvider supplies the latest market quote for a batch of symbols.
type QuoteProvider interface {
	GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}

// HistoricalPriceProvider supplies a sorted closing-price series for a symbol.
// Satisfied by portfolio.HistoricalPrices.GetCloseSeries.
type HistoricalPriceProvider interface {
	GetCloseSeries(symbol string) []float64
}

// RiskExclusionFilter performs Layer 2.5 risk checks on a symbol universe:
// VaR contribution, volatility, drawdown flag, and liquidity re-check.
type RiskExclusionFilter struct {
	riskMgr              RiskManager
	marketDataProvider   QuoteProvider
	priceHistoryProvider HistoricalPriceProvider

	varThreshold      float64
	volMultiplier     float64
	drawdownWindow    int
	drawdownThreshold float64
	minDailyAmount    float64
}

// NewRiskExclusionFilter creates a filter with sensible defaults.
// All providers are optional; missing providers cause dependent checks to skip.
// Callers should call Configure() after construction to apply parameter overrides.
func NewRiskExclusionFilter(riskMgr RiskManager, marketDataProvider QuoteProvider, priceHistoryProvider HistoricalPriceProvider) *RiskExclusionFilter {
	return &RiskExclusionFilter{
		riskMgr:              riskMgr,
		marketDataProvider:   marketDataProvider,
		priceHistoryProvider: priceHistoryProvider,
		varThreshold:         2.0,
		volMultiplier:        2.0,
		drawdownWindow:       60,
		drawdownThreshold:    0.30,
		minDailyAmount:       5_000_000.0,
	}
}

// Configure applies SmartUniverseConfig parameter overrides. Call this
// after construction, before the first Filter() invocation. All five risk
// thresholds (VaR contribution, volatility, drawdown window + threshold,
// and liquidity) are wired from the parameters system.
func (f *RiskExclusionFilter) Configure(cfg config.SmartUniverseConfig) {
	f.varThreshold = cfg.VaRContributionMultiplier.Value
	f.volMultiplier = cfg.VolatilityMultiplier.Value
	f.drawdownWindow = cfg.DrawdownWindow.Value
	f.drawdownThreshold = cfg.DrawdownThreshold.Value
	f.minDailyAmount = cfg.MinDailyAmountTWD.Value
}

// Filter runs the four Layer 2.5 checks against every symbol and returns
// the full set of results. Callers should inspect Passed to obtain the safe universe.
func (f *RiskExclusionFilter) Filter(symbols []string) ([]RiskExclusionResult, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	quotes, err := f.fetchQuotes(symbols)
	if err != nil {
		return nil, fmt.Errorf("risk_exclusion: fetch quotes: %w", err)
	}
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteBySymbol[q.Symbol] = q
	}

	volBySymbol, medianVol := f.computeVolatilityMap(symbols)
	portfolioAvgVar := f.portfolioAverageVaR(len(symbols))

	results := make([]RiskExclusionResult, 0, len(symbols))
	for _, sym := range symbols {
		r := RiskExclusionResult{Symbol: sym, Passed: true}
		f.checkVaRContribution(sym, portfolioAvgVar, &r)
		f.checkVolatility(sym, volBySymbol, medianVol, &r)
		f.checkDrawdown(sym, &r)
		f.checkLiquidity(sym, quoteBySymbol, &r)
		results = append(results, r)
	}
	return results, nil
}

func (f *RiskExclusionFilter) fetchQuotes(symbols []string) ([]domain.Quote, error) {
	if f.marketDataProvider == nil {
		return nil, nil
	}
	return f.marketDataProvider.GetQuotes(context.Background(), time.Now(), symbols)
}

func (f *RiskExclusionFilter) portfolioAverageVaR(n int) float64 {
	if f.riskMgr == nil || n == 0 {
		return 0
	}
	return math.Abs(f.riskMgr.VaR95()) / float64(n)
}

func (f *RiskExclusionFilter) computeVolatilityMap(symbols []string) (map[string]float64, float64) {
	volBySymbol := make(map[string]float64, len(symbols))
	if f.priceHistoryProvider == nil {
		return volBySymbol, 0
	}
	vols := make([]float64, 0, len(symbols))
	for _, sym := range symbols {
		if v := f.realizedVolatility(sym, 30); v > 0 {
			volBySymbol[sym] = v
			vols = append(vols, v)
		}
	}
	return volBySymbol, median(vols)
}

func (f *RiskExclusionFilter) checkVaRContribution(symbol string, portfolioAvgVar float64, r *RiskExclusionResult) {
	if f.riskMgr == nil {
		r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "var_contribution", Passed: true, Severity: "INFO", Message: fmt.Sprintf("%s skipped: risk manager not configured", symbol)})
		return
	}
	contrib, err := f.riskMgr.VaRContribution(symbol)
	if err != nil {
		r.Passed = false
		r.FailReasons = append(r.FailReasons, "var_contribution")
		r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "var_contribution", Passed: false, Severity: "CRITICAL", Message: fmt.Sprintf("%s VaR contribution unavailable: %v", symbol, err)})
		return
	}
	threshold := f.varThreshold * portfolioAvgVar
	passed := contrib <= threshold
	if !passed {
		r.Passed = false
		r.FailReasons = append(r.FailReasons, "var_contribution")
	}
	r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "var_contribution", Passed: passed, CurrentValue: contrib, Threshold: threshold, Severity: severity(passed, contrib, threshold), Message: fmt.Sprintf("%s VaR contribution %.4f vs threshold %.4f", symbol, contrib, threshold)})
}

func (f *RiskExclusionFilter) checkVolatility(symbol string, volBySymbol map[string]float64, medianVol float64, r *RiskExclusionResult) {
	vol, ok := volBySymbol[symbol]
	if !ok || medianVol <= 0 {
		r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "volatility", Passed: true, Severity: "INFO", Message: fmt.Sprintf("%s skipped: insufficient volatility data (available=%d, median=%.4f)", symbol, len(volBySymbol), medianVol)})
		return
	}
	threshold := f.volMultiplier * medianVol
	passed := vol <= threshold
	if !passed {
		r.Passed = false
		r.FailReasons = append(r.FailReasons, "volatility")
	}
	r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "volatility", Passed: passed, CurrentValue: vol, Threshold: threshold, Severity: severity(passed, vol, threshold), Message: fmt.Sprintf("%s 30-day realized vol %.4f vs median %.4f", symbol, vol, medianVol)})
}

func (f *RiskExclusionFilter) checkDrawdown(symbol string, r *RiskExclusionResult) {
	if f.priceHistoryProvider == nil {
		r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "drawdown_warning", Passed: true, Severity: "INFO", Message: fmt.Sprintf("%s skipped: price history provider not configured", symbol)})
		return
	}
	dd := f.maxDrawdown(symbol, f.drawdownWindow)
	flagged := dd > f.drawdownThreshold
	if flagged {
		r.HighRisk = true
	}
	r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "drawdown_warning", Passed: true, CurrentValue: dd, Threshold: f.drawdownThreshold, Severity: severity(!flagged, dd, f.drawdownThreshold), Message: fmt.Sprintf("%s %d-day max drawdown %.2f%%", symbol, f.drawdownWindow, dd*100)})
}

func (f *RiskExclusionFilter) checkLiquidity(symbol string, quoteBySymbol map[string]domain.Quote, r *RiskExclusionResult) {
	q, ok := quoteBySymbol[symbol]
	if !ok {
		return
	}
	dailyAmount := q.Last * float64(q.Volume)
	passed := dailyAmount >= f.minDailyAmount
	if !passed {
		r.Passed = false
		r.FailReasons = append(r.FailReasons, "liquidity")
	}
	r.RuleResults = append(r.RuleResults, RuleDetail{RuleName: "liquidity", Passed: passed, CurrentValue: dailyAmount, Threshold: f.minDailyAmount, Severity: severity(passed, dailyAmount, f.minDailyAmount), Message: fmt.Sprintf("%s daily TWD amount %.0f vs minimum %.0f", symbol, dailyAmount, f.minDailyAmount)})
}

// realizedVolatility returns the standard deviation of daily simple returns over the last n days.
func (f *RiskExclusionFilter) realizedVolatility(symbol string, days int) float64 {
	series := f.priceHistoryProvider.GetCloseSeries(symbol)
	if len(series) < days+1 {
		return 0
	}
	returns := make([]float64, 0, days)
	start := max(0, len(series)-days-1)
	for i := start + 1; i < len(series); i++ {
		if series[i-1] == 0 {
			continue
		}
		returns = append(returns, series[i]/series[i-1]-1)
	}
	if len(returns) == 0 {
		return 0
	}
	mean := 0.0
	for _, ret := range returns {
		mean += ret
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, ret := range returns {
		d := ret - mean
		variance += d * d
	}
	variance /= float64(len(returns))
	return math.Sqrt(variance)
}

// maxDrawdown computes the largest peak-to-trough decline over the last n days.
func (f *RiskExclusionFilter) maxDrawdown(symbol string, days int) float64 {
	series := f.priceHistoryProvider.GetCloseSeries(symbol)
	if len(series) == 0 {
		return 0
	}
	if len(series) > days {
		series = series[len(series)-days:]
	}
	peak := series[0]
	maxDD := 0.0
	for _, price := range series {
		if price > peak {
			peak = price
		}
		if peak > 0 {
			if dd := (peak - price) / peak; dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

// median returns the median of a float64 slice. The input is not modified.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// severity maps a pass/fail ratio to a severity string.
func severity(passed bool, current, threshold float64) string {
	if passed {
		return "INFO"
	}
	if threshold == 0 {
		return "WARNING"
	}
	ratio := current / threshold
	switch {
	case ratio > 2.0:
		return "CRITICAL"
	case ratio > 1.5:
		return "WARNING"
	default:
		return "WARNING"
	}
}
