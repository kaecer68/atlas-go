package reporting

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// AgentContribution represents a single agent's contribution to portfolio performance.
type AgentContribution struct {
	AgentID     string  `json:"agent_id"`
	Skill       string  `json:"skill"`
	Layer       string  `json:"layer"`
	TotalReturn float64 `json:"total_return"`
	WinRate     float64 `json:"win_rate"`
	TradeCount  int     `json:"trade_count"`
	AvgReturn   float64 `json:"avg_return"`
	SharpeLike  float64 `json:"sharpe_like"`
}

// RegimeBreakdown holds performance metrics segmented by market regime.
type RegimeBreakdown struct {
	Regimes map[string]RegimePerformance `json:"regimes"`
}

// RegimePerformance is the performance stats for a single regime.
type RegimePerformance struct {
	Regime       string  `json:"regime"`
	SessionCount int     `json:"session_count"`
	TotalReturn  float64 `json:"total_return"`
	WinRate      float64 `json:"win_rate"`
	AvgReturn    float64 `json:"avg_return"`
}

// MonthlyReturn represents a single month's return.
type MonthlyReturn struct {
	Year   int     `json:"year"`
	Month  int     `json:"month"`
	Return float64 `json:"return"`
	Label  string  `json:"label"`
}

// PerformanceReport is the structured performance report for a given period.
type PerformanceReport struct {
	Period           string              `json:"period"`
	StartDate        time.Time           `json:"start_date"`
	EndDate          time.Time           `json:"end_date"`
	TotalReturn      float64             `json:"total_return"`
	AnnualizedReturn float64             `json:"annualized_return"`
	SharpeRatio      float64             `json:"sharpe_ratio"`
	SortinoRatio     float64             `json:"sortino_ratio"`
	CalmarRatio      float64             `json:"calmar_ratio"`
	MaxDrawdown      float64             `json:"max_drawdown"`
	StartingValue    float64             `json:"starting_value"`
	EndingValue      float64             `json:"ending_value"`
	AfterTaxValue    float64             `json:"after_tax_value"`
	TotalTaxPaid     float64             `json:"total_tax_paid"`
	WinRate          float64             `json:"win_rate"`
	TotalTrades      int                 `json:"total_trades"`
	AvgWin           float64             `json:"avg_win"`
	AvgLoss          float64             `json:"avg_loss"`
	TopAgents        []AgentContribution `json:"top_agents"`
	RegimeBreakdown  RegimeBreakdown     `json:"regime_breakdown"`
	MonthlyReturns   []MonthlyReturn     `json:"monthly_returns"`
	GeneratedAt      time.Time           `json:"generated_at"`
}

// GenerateReport builds a PerformanceReport from ledger data for the given period.
// Supported periods: "30d", "90d", "1y", "all".
func GenerateReport(ledgerPath string, period string) (*PerformanceReport, error) {
	store := ledger.NewStore(ledgerPath)
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		return nil, fmt.Errorf("load session summaries: %w", err)
	}

	if len(summaries) == 0 {
		return emptyReport(period), nil
	}

	slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
		return strings.Compare(a.SessionID, b.SessionID)
	})

	cutoff := computeCutoff(period)
	filtered := filterSummariesByDate(summaries, cutoff)

	if len(filtered) == 0 {
		return emptyReport(period), nil
	}

	startDate := domain.SessionDateFromID(filtered[0].SessionID)
	endDate := domain.SessionDateFromID(filtered[len(filtered)-1].SessionID)

	equityCurve := make([]float64, len(filtered))
	portfolioValues := make([]float64, len(filtered))
	for i, s := range filtered {
		pv := s.PortfolioValue
		if pv == 0 {
			pv = s.EndingCash
		}
		equityCurve[i] = pv
		portfolioValues[i] = pv
	}

	startingValue := equityCurve[0]
	endingValue := equityCurve[len(equityCurve)-1]

	totalReturn := 0.0
	if startingValue > 0 {
		totalReturn = (endingValue - startingValue) / startingValue
	}

	dailyReturns := make([]float64, 0, max(0, len(portfolioValues)-1))
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	days := endDate.Sub(startDate).Hours() / 24
	annualizedReturn := 0.0
	if days > 0 && totalReturn > -1 {
		annualizedReturn = math.Pow(1+totalReturn, 365.0/days) - 1
	}

	sharpeRatio := CalculateSharpeRatio(dailyReturns)
	sortinoRatio := calculateSortinoRatio(dailyReturns, 0.0)
	maxDD := risk.CalculateMaxDrawdown(portfolioValues)
	calmarRatio := calculateCalmarRatio(annualizedReturn, maxDD)

	var totalTaxPaid float64
	for _, s := range filtered {
		totalTaxPaid += s.TotalTaxPaid
	}
	afterTaxValue := endingValue - totalTaxPaid

	outcomes := loadAllOutcomes(ledgerPath, filtered)
	winRate, totalTrades, avgWin, avgLoss := calculateTradeMetrics(outcomes)

	topAgents := calculateTopAgents(outcomes)
	regimeBreakdown := calculateRegimeBreakdown(filtered, outcomes)
	monthlyReturns := calculateMonthlyReturns(filtered, portfolioValues)

	return &PerformanceReport{
		Period:           period,
		StartDate:        startDate,
		EndDate:          endDate,
		TotalReturn:      totalReturn,
		AnnualizedReturn: annualizedReturn,
		SharpeRatio:      sharpeRatio,
		SortinoRatio:     sortinoRatio,
		CalmarRatio:      calmarRatio,
		MaxDrawdown:      maxDD,
		StartingValue:    startingValue,
		EndingValue:      endingValue,
		AfterTaxValue:    afterTaxValue,
		TotalTaxPaid:     totalTaxPaid,
		WinRate:          winRate,
		TotalTrades:      totalTrades,
		AvgWin:           avgWin,
		AvgLoss:          avgLoss,
		TopAgents:        topAgents,
		RegimeBreakdown:  regimeBreakdown,
		MonthlyReturns:   monthlyReturns,
		GeneratedAt:      time.Now(),
	}, nil
}

// GenerateMarkdownReport renders a PerformanceReport as Markdown.
func GenerateMarkdownReport(report *PerformanceReport) string {
	if report == nil {
		return "_No report data available._\n"
	}

	var sb strings.Builder

	sb.WriteString("# Performance Report\n\n")
	fmt.Fprintf(
		&sb, "**Period:** %s (%s to %s)\n\n",
		report.Period,
		report.StartDate.Format("2006-01-02"),
		report.EndDate.Format("2006-01-02"),
	)

	sb.WriteString("## Key Metrics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(&sb, "| Total Return | %.2f%% |\n", report.TotalReturn*100)
	fmt.Fprintf(&sb, "| Annualized Return | %.2f%% |\n", report.AnnualizedReturn*100)
	fmt.Fprintf(&sb, "| Sharpe Ratio | %.3f |\n", report.SharpeRatio)
	fmt.Fprintf(&sb, "| Sortino Ratio | %.3f |\n", report.SortinoRatio)
	fmt.Fprintf(&sb, "| Calmar Ratio | %.3f |\n", report.CalmarRatio)
	fmt.Fprintf(&sb, "| Max Drawdown | %.2f%% |\n", report.MaxDrawdown*100)
	fmt.Fprintf(&sb, "| Starting Value | %s |\n", domain.FormatNTD(report.StartingValue))
	fmt.Fprintf(&sb, "| Ending Value | %s |\n", domain.FormatNTD(report.EndingValue))
	fmt.Fprintf(&sb, "| After-Tax Value | %s |\n", domain.FormatNTD(report.AfterTaxValue))
	fmt.Fprintf(&sb, "| Total Tax Paid | %s |\n", domain.FormatNTD(report.TotalTaxPaid))
	fmt.Fprintf(&sb, "| Win Rate | %.1f%% |\n", report.WinRate*100)
	fmt.Fprintf(&sb, "| Total Trades | %d |\n", report.TotalTrades)
	fmt.Fprintf(&sb, "| Avg Win | %.2f%% |\n", report.AvgWin*100)
	fmt.Fprintf(&sb, "| Avg Loss | %.2f%% |\n", report.AvgLoss*100)
	sb.WriteString("\n")

	sb.WriteString("## Top Agent Contributions\n\n")
	if len(report.TopAgents) == 0 {
		sb.WriteString("_No agent data available._\n")
	} else {
		sb.WriteString("| Agent | Skill | Layer | Trades | Win Rate | Avg Return | Total Return |\n")
		sb.WriteString("|-------|-------|-------|--------|----------|------------|-------------|\n")
		for _, a := range report.TopAgents {
			fmt.Fprintf(
				&sb, "| %s | %s | %s | %d | %.1f%% | %.2f%% | %.2f%% |\n",
				truncate(a.AgentID, 20),
				a.Skill,
				a.Layer,
				a.TradeCount,
				a.WinRate*100,
				a.AvgReturn*100,
				a.TotalReturn*100,
			)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Regime Breakdown\n\n")
	if len(report.RegimeBreakdown.Regimes) == 0 {
		sb.WriteString("_No regime data available._\n")
	} else {
		sb.WriteString("| Regime | Sessions | Total Return | Win Rate | Avg Return |\n")
		sb.WriteString("|--------|----------|--------------|----------|------------|\n")
		for _, r := range report.RegimeBreakdown.Regimes {
			fmt.Fprintf(
				&sb, "| %s | %d | %.2f%% | %.1f%% | %.2f%% |\n",
				r.Regime,
				r.SessionCount,
				r.TotalReturn*100,
				r.WinRate*100,
				r.AvgReturn*100,
			)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Monthly Returns\n\n")
	if len(report.MonthlyReturns) == 0 {
		sb.WriteString("_No monthly data available._\n")
	} else {
		sb.WriteString("| Month | Return |\n")
		sb.WriteString("|-------|--------|\n")
		for _, m := range report.MonthlyReturns {
			fmt.Fprintf(&sb, "| %s | %.2f%% |\n", m.Label, m.Return*100)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Generated at %s*\n", report.GeneratedAt.Format(time.RFC3339))

	return sb.String()
}

func emptyReport(period string) *PerformanceReport {
	return &PerformanceReport{
		Period:          period,
		TopAgents:       []AgentContribution{},
		RegimeBreakdown: RegimeBreakdown{Regimes: map[string]RegimePerformance{}},
		MonthlyReturns:  []MonthlyReturn{},
		GeneratedAt:     time.Now(),
	}
}

func computeCutoff(period string) time.Time {
	now := time.Now()
	switch period {
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	case "1y":
		return now.AddDate(-1, 0, 0)
	case "all", "":
		return time.Time{}
	default:
		return time.Time{}
	}
}

func filterSummariesByDate(summaries []domain.SessionSummary, cutoff time.Time) []domain.SessionSummary {
	if cutoff.IsZero() {
		return summaries
	}
	var filtered []domain.SessionSummary
	for _, s := range summaries {
		d := domain.SessionDateFromID(s.SessionID)
		if !d.IsZero() && !d.Before(cutoff) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func loadAllOutcomes(ledgerPath string, summaries []domain.SessionSummary) []domain.RecommendationOutcome {
	var allOutcomes []domain.RecommendationOutcome
	sessionsDir := filepath.Join(ledgerPath, "sessions")
	for _, s := range summaries {
		path := filepath.Join(sessionsDir, s.SessionID, "recommendation_outcomes.jsonl")
		outcomes, err := loadOutcomeFile(path)
		if err != nil {
			continue
		}
		allOutcomes = append(allOutcomes, outcomes...)
	}
	return allOutcomes
}

func loadOutcomeFile(path string) ([]domain.RecommendationOutcome, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var outcomes []domain.RecommendationOutcome
	decoder := json.NewDecoder(f)
	for decoder.More() {
		var outcome domain.RecommendationOutcome
		if err := decoder.Decode(&outcome); err != nil {
			break
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// CalculateSharpeRatio computes the annualized Sharpe ratio from daily returns.
func CalculateSharpeRatio(dailyReturns []float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	mean := 0.0
	for _, r := range dailyReturns {
		mean += r
	}
	mean /= float64(len(dailyReturns))

	var variance float64
	for _, r := range dailyReturns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(dailyReturns))
	stdDev := math.Sqrt(variance)

	if stdDev == 0 {
		return 0
	}
	return (mean / stdDev) * math.Sqrt(252)
}

func calculateSortinoRatio(dailyReturns []float64, targetReturn float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	var excessSum, downsideSum float64
	for _, r := range dailyReturns {
		excess := r - targetReturn
		excessSum += excess
		if r < targetReturn {
			downsideSum += (r - targetReturn) * (r - targetReturn)
		}
	}
	meanExcess := excessSum / float64(len(dailyReturns))
	downsideDev := 0.0
	if downsideSum > 0 {
		downsideDev = math.Sqrt(downsideSum / float64(len(dailyReturns)))
	}
	if downsideDev == 0 {
		return 0
	}
	return (meanExcess / downsideDev) * math.Sqrt(252)
}

func calculateCalmarRatio(annualizedReturn, maxDrawdown float64) float64 {
	if maxDrawdown == 0 {
		return 0
	}
	return annualizedReturn / maxDrawdown
}

func calculateTradeMetrics(outcomes []domain.RecommendationOutcome) (winRate float64, totalTrades int, avgWin, avgLoss float64) {
	if len(outcomes) == 0 {
		return 0, 0, 0, 0
	}

	wins := 0
	var winSum, lossSum float64
	winCount, lossCount := 0, 0

	for _, oc := range outcomes {
		if !oc.PassedGuards {
			continue
		}
		totalTrades++
		if oc.ForwardReturn > 0 {
			wins++
			winSum += oc.ForwardReturn
			winCount++
		} else if oc.ForwardReturn < 0 {
			lossSum += oc.ForwardReturn
			lossCount++
		}
	}

	if totalTrades > 0 {
		winRate = float64(wins) / float64(totalTrades)
	}
	if winCount > 0 {
		avgWin = winSum / float64(winCount)
	}
	if lossCount > 0 {
		avgLoss = lossSum / float64(lossCount)
	}

	return winRate, totalTrades, avgWin, avgLoss
}

func calculateTopAgents(outcomes []domain.RecommendationOutcome) []AgentContribution {
	type agg struct {
		agentID string
		skill   string
		layer   string
		returns []float64
		wins    int
		trades  int
	}

	byAgent := map[string]*agg{}
	for _, oc := range outcomes {
		if !oc.PassedGuards || oc.AgentID == "" {
			continue
		}
		entry, ok := byAgent[oc.AgentID]
		if !ok {
			entry = &agg{
				agentID: oc.AgentID,
				skill:   oc.Skill,
				layer:   string(oc.Layer),
			}
			byAgent[oc.AgentID] = entry
		}
		entry.returns = append(entry.returns, oc.ForwardReturn)
		entry.trades++
		if oc.ForwardReturn > 0 {
			entry.wins++
		}
	}

	var contributions []AgentContribution
	for _, a := range byAgent {
		if a.trades == 0 {
			continue
		}
		var totalReturn float64
		for _, r := range a.returns {
			totalReturn += r
		}
		avgReturn := totalReturn / float64(a.trades)
		winRate := float64(a.wins) / float64(a.trades)
		sharpeLike := calculateSharpeLike(a.returns)

		contributions = append(contributions, AgentContribution{
			AgentID:     a.agentID,
			Skill:       a.skill,
			Layer:       a.layer,
			TotalReturn: totalReturn,
			WinRate:     winRate,
			TradeCount:  a.trades,
			AvgReturn:   avgReturn,
			SharpeLike:  sharpeLike,
		})
	}

	slices.SortFunc(contributions, func(a, b AgentContribution) int {
		if a.TotalReturn > b.TotalReturn {
			return -1
		}
		if a.TotalReturn < b.TotalReturn {
			return 1
		}
		return 0
	})

	if len(contributions) > 5 {
		contributions = contributions[:5]
	}
	return contributions
}

func calculateSharpeLike(returns []float64) float64 {
	if len(returns) < 60 {
		return 0
	}
	m := mean(returns)
	s := stdDev(returns)
	if s == 0 {
		return 0
	}
	return m / s
}

// CalculateBeta computes the CAPM beta of the portfolio relative to the benchmark.
// Uses the ratio of portfolio volatility to benchmark return magnitude,
// consistent with benchmark.go's simplified approach.
func CalculateBeta(portfolioReturns []float64, benchmarkReturn float64) float64 {
	if len(portfolioReturns) < 60 || benchmarkReturn == 0 {
		return 1.0
	}
	portVol := stdDev(portfolioReturns)
	benchVol := math.Abs(benchmarkReturn)
	if benchVol == 0 {
		return 1.0
	}
	return portVol / benchVol
}

// CalculateAlpha computes the risk-adjusted excess return.
func CalculateAlpha(portfolioReturn, beta, benchmarkReturn float64) float64 {
	return portfolioReturn - beta*benchmarkReturn
}

// CalculateTrackingError computes the standard deviation of portfolio returns
// as a measure of tracking error relative to the benchmark.
func CalculateTrackingError(portfolioReturns []float64) float64 {
	if len(portfolioReturns) < 2 {
		return 0
	}
	return stdDev(portfolioReturns)
}

// CalculateInfoRatio computes the information ratio (outperformance / tracking error).
func CalculateInfoRatio(outperformance, trackingError float64) float64 {
	if trackingError == 0 {
		return 0
	}
	return outperformance / trackingError
}

func calculateRegimeBreakdown(summaries []domain.SessionSummary, outcomes []domain.RecommendationOutcome) RegimeBreakdown {
	regimeData := map[string]*RegimePerformance{}

	for _, s := range summaries {
		r := string(s.Regime)
		if r == "" {
			r = "unknown"
		}
		if _, ok := regimeData[r]; !ok {
			regimeData[r] = &RegimePerformance{Regime: r}
		}
		regimeData[r].SessionCount++
	}

	regimeReturns := map[string][]float64{}
	for _, oc := range outcomes {
		if !oc.PassedGuards {
			continue
		}
		regime := findRegimeForWindow(summaries, oc.Window)
		if regime == "" {
			regime = "unknown"
		}
		regimeReturns[regime] = append(regimeReturns[regime], oc.ForwardReturn)
	}

	for regime, returns := range regimeReturns {
		if _, ok := regimeData[regime]; !ok {
			regimeData[regime] = &RegimePerformance{Regime: regime}
		}
		var totalReturn float64
		wins := 0
		for _, r := range returns {
			totalReturn += r
			if r > 0 {
				wins++
			}
		}
		regimeData[regime].TotalReturn = totalReturn
		regimeData[regime].AvgReturn = totalReturn / float64(len(returns))
		regimeData[regime].WinRate = float64(wins) / float64(len(returns))
	}

	result := make(map[string]RegimePerformance, len(regimeData))
	for k, v := range regimeData {
		result[k] = *v
	}

	return RegimeBreakdown{Regimes: result}
}

func findRegimeForWindow(summaries []domain.SessionSummary, window string) string {
	for _, s := range summaries {
		if s.SessionID == window {
			return string(s.Regime)
		}
	}
	return ""
}

func calculateMonthlyReturns(summaries []domain.SessionSummary, portfolioValues []float64) []MonthlyReturn {
	if len(summaries) == 0 || len(portfolioValues) == 0 {
		return nil
	}

	type monthKey struct {
		Year  int
		Month int
	}
	monthData := map[monthKey][]float64{}
	monthLabels := map[monthKey]string{}

	for i, s := range summaries {
		d := domain.SessionDateFromID(s.SessionID)
		if d.IsZero() {
			continue
		}
		key := monthKey{Year: d.Year(), Month: int(d.Month())}
		monthData[key] = append(monthData[key], portfolioValues[i])
		monthLabels[key] = d.Format("2006-01")
	}

	var monthlyReturns []MonthlyReturn
	for key, values := range monthData {
		if len(values) < 2 {
			continue
		}
		startVal := values[0]
		endVal := values[len(values)-1]
		ret := 0.0
		if startVal > 0 {
			ret = (endVal - startVal) / startVal
		}
		monthlyReturns = append(monthlyReturns, MonthlyReturn{
			Year:   key.Year,
			Month:  key.Month,
			Return: ret,
			Label:  monthLabels[key],
		})
	}

	slices.SortFunc(monthlyReturns, func(a, b MonthlyReturn) int {
		if a.Year != b.Year {
			return a.Year - b.Year
		}
		return a.Month - b.Month
	})

	return monthlyReturns
}

// mean computes the arithmetic mean of a slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// stdDev computes the sample standard deviation of a slice.
func stdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	sumSq := 0.0
	for _, v := range values {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)-1))
}
