package reporting

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	configpkg "github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// AgentContribution represents a single agent's contribution to portfolio performance.
type AgentContribution struct {
	AgentID                string   `json:"agent_id"`
	DisplayName            string   `json:"display_name"`
	Skill                  string   `json:"skill"`
	Layer                  string   `json:"layer"`
	AggregateForwardReturn float64  `json:"aggregate_forward_return"`
	WinRate                float64  `json:"win_rate"`
	RealTradeCount         int      `json:"real_trade_count"`
	SyntheticTradeCount    int      `json:"synthetic_trade_count"`
	TradeCount             int      `json:"trade_count"`
	AvgReturn              float64  `json:"avg_return"`
	ProfitFactor           float64  `json:"profit_factor"`
	SharpeLike             *float64 `json:"sharpe_like"`
}

// RegimeBreakdown holds performance metrics segmented by market regime.
type RegimeBreakdown struct {
	Regimes map[string]RegimePerformance `json:"regimes"`
}

// RegimePerformance is the performance stats for a single regime.
type RegimePerformance struct {
	Regime                 string  `json:"regime"`
	SessionCount           int     `json:"session_count"`
	AggregateForwardReturn float64 `json:"aggregate_forward_return"`
	WinRate                float64 `json:"win_rate"`
	AvgReturn              float64 `json:"avg_return"`
}

// MonthlyReturn represents a single month's return.
type MonthlyReturn struct {
	Year       int      `json:"year"`
	Month      int      `json:"month"`
	Return     float64  `json:"return"`
	Cumulative *float64 `json:"cumulative,omitempty"`
	Label      string   `json:"label"`
}

// PerformanceReport is the structured performance report for a given period.
type PerformanceReport struct {
	Period              string              `json:"period"`
	StartDate           time.Time           `json:"start_date"`
	EndDate             time.Time           `json:"end_date"`
	TotalReturn         float64             `json:"total_return"`
	AnnualizedReturn    float64             `json:"annualized_return"`
	SharpeRatio         *float64            `json:"sharpe_ratio"`
	SortinoRatio        float64             `json:"sortino_ratio"`
	CalmarRatio         float64             `json:"calmar_ratio"`
	MaxDrawdown         float64             `json:"max_drawdown"`
	StartingValue       float64             `json:"starting_value"`
	EndingValue         float64             `json:"ending_value"`
	AfterTaxValue       float64             `json:"after_tax_value"`
	TotalTaxPaid        float64             `json:"total_tax_paid"`
	WinRate             float64             `json:"win_rate"`
	TotalTrades         int                 `json:"total_trades"`
	RealTradeCount      int                 `json:"real_trade_count"`
	SyntheticTradeCount int                 `json:"synthetic_trade_count"`
	ProfitFactor        float64             `json:"profit_factor"`
	AvgWin              float64             `json:"avg_win"`
	AvgLoss             float64             `json:"avg_loss"`
	TopAgents           []AgentContribution `json:"top_agents"`
	RegimeBreakdown     RegimeBreakdown     `json:"regime_breakdown"`
	MonthlyReturns      []MonthlyReturn     `json:"monthly_returns"`
	GeneratedAt         time.Time           `json:"generated_at"`
}

// GenerateReport builds a PerformanceReport from ledger data for the given period.
// Supported periods: "30d", "90d", "1y", "all".
func GenerateReport(store ledger.OutcomeStore, ledgerPath string, period string) (*PerformanceReport, error) {
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

	// Exclude sessions with no equity data (PortfolioValue and EndingCash both
	// zero). Legacy SQLite rows written before the summary_json backfill
	// (perf-report-zero BL-01 follow-up) carry only the 5-column projection and
	// would otherwise become a 0-valued starting point, collapsing total_return
	// to 0 and max_drawdown to 100%.
	valid := filtered[:0]
	for _, s := range filtered {
		if s.PortfolioValue == 0 && s.EndingCash == 0 {
			continue
		}
		valid = append(valid, s)
	}
	filtered = valid
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

	sortinoRatio := shared.ComputeSortino(dailyReturns, shared.SortinoConfig{
		Frequency:  shared.FrequencyPerDay,
		MinSamples: 2,
	})
	maxDD := risk.CalculateMaxDrawdown(portfolioValues)
	calmarRatio := calculateCalmarRatio(annualizedReturn, maxDD)

	var totalTaxPaid float64
	for _, s := range filtered {
		totalTaxPaid += s.TotalTaxPaid
	}
	afterTaxValue := endingValue - totalTaxPaid

	agentNames := loadAgentDisplayNames()

	outcomes := loadAllOutcomes(store, filtered)
	winRate, totalTrades, realTrades, syntheticTrades, avgWin, avgLoss, profitFactor := calculateTradeMetrics(outcomes)

	topAgents := calculateTopAgents(outcomes, agentNames)
	regimeBreakdown := calculateRegimeBreakdown(filtered, outcomes)
	monthlyReturns := calculateMonthlyReturns(filtered, portfolioValues)

	return &PerformanceReport{
		Period:              period,
		StartDate:           startDate,
		EndDate:             endDate,
		TotalReturn:         totalReturn,
		AnnualizedReturn:    annualizedReturn,
		SharpeRatio:         sharpeRatio,
		SortinoRatio:        sortinoRatio,
		CalmarRatio:         calmarRatio,
		MaxDrawdown:         maxDD,
		StartingValue:       startingValue,
		EndingValue:         endingValue,
		AfterTaxValue:       afterTaxValue,
		TotalTaxPaid:        totalTaxPaid,
		WinRate:             winRate,
		TotalTrades:         totalTrades,
		RealTradeCount:      realTrades,
		SyntheticTradeCount: syntheticTrades,
		ProfitFactor:        profitFactor,
		AvgWin:              avgWin,
		AvgLoss:             avgLoss,
		TopAgents:           topAgents,
		RegimeBreakdown:     regimeBreakdown,
		MonthlyReturns:      monthlyReturns,
		GeneratedAt:         time.Now(),
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
	if report.SharpeRatio == nil {
		sb.WriteString("| Sharpe Ratio | N/A |\n")
	} else {
		fmt.Fprintf(&sb, "| Sharpe Ratio | %.3f |\n", *report.SharpeRatio)
	}
	fmt.Fprintf(&sb, "| Sortino Ratio | %.3f |\n", report.SortinoRatio)
	fmt.Fprintf(&sb, "| Calmar Ratio | %.3f |\n", report.CalmarRatio)
	fmt.Fprintf(&sb, "| Max Drawdown | %.2f%% |\n", report.MaxDrawdown*100)
	fmt.Fprintf(&sb, "| Starting Value | %s |\n", domain.FormatNTD(report.StartingValue))
	fmt.Fprintf(&sb, "| Ending Value | %s |\n", domain.FormatNTD(report.EndingValue))
	fmt.Fprintf(&sb, "| After-Tax Value | %s |\n", domain.FormatNTD(report.AfterTaxValue))
	fmt.Fprintf(&sb, "| Total Tax Paid | %s |\n", domain.FormatNTD(report.TotalTaxPaid))
	fmt.Fprintf(&sb, "| Win Rate | %.1f%% |\n", report.WinRate*100)
	fmt.Fprintf(&sb, "| Total Trades | %d |\n", report.TotalTrades)
	fmt.Fprintf(&sb, "| Real Trades | %d |\n", report.RealTradeCount)
	fmt.Fprintf(&sb, "| Synthetic Trades | %d |\n", report.SyntheticTradeCount)
	fmt.Fprintf(&sb, "| Profit Factor | %.2f |\n", report.ProfitFactor)
	fmt.Fprintf(&sb, "| Avg Win | %.2f%% |\n", report.AvgWin*100)
	fmt.Fprintf(&sb, "| Avg Loss | %.2f%% |\n", report.AvgLoss*100)
	sb.WriteString("\n")

	sb.WriteString("## Top Agent Contributions\n\n")
	if len(report.TopAgents) == 0 {
		sb.WriteString("_No agent data available._\n")
	} else {
		sb.WriteString("| Agent | Skill | Layer | Real | Synthetic | Win Rate | Avg Return | Total Return | Sharpe | Prof. Factor |\n")
		sb.WriteString("|-------|-------|-------|------|-----------|----------|------------|-------------|--------|-------------|\n")
		for _, a := range report.TopAgents {
			name := a.DisplayName
			if name == "" {
				name = a.AgentID
			}
			fmt.Fprintf(
				&sb, "| %s | %s | %s | %d | %d | %.1f%% | %.2f%% | %.2f%% | %s | %.2f |\n",
				truncate(name, 20),
				a.Skill,
				a.Layer,
				a.RealTradeCount,
				a.SyntheticTradeCount,
				a.WinRate*100,
				a.AvgReturn*100,
				a.AggregateForwardReturn*100,
				formatSharpeLike(a.SharpeLike),
				a.ProfitFactor,
			)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Regime Breakdown\n\n")
	if len(report.RegimeBreakdown.Regimes) == 0 {
		sb.WriteString("_No regime data available._\n")
	} else {
		sb.WriteString("| Regime | Sessions | Aggregate Forward Return | Win Rate | Avg Return |\n")
		sb.WriteString("|--------|----------|--------------------------|----------|------------|\n")
		for _, r := range report.RegimeBreakdown.Regimes {
			fmt.Fprintf(
				&sb, "| %s | %d | %.2f%% | %.1f%% | %.2f%% |\n",
				r.Regime,
				r.SessionCount,
				r.AggregateForwardReturn*100,
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

// filterSummariesByDate returns the subset of summaries whose SessionID
// parses to a date on or after cutoff. When cutoff is the zero time (the
// "all" period), no date bound is applied — but summaries with an
// unparseable SessionID are still dropped, because they would otherwise
// surface as the first entry (empty < "session-…") and pollute start_date /
// starting_value with a Go zero time.
func filterSummariesByDate(summaries []domain.SessionSummary, cutoff time.Time) []domain.SessionSummary {
	var filtered []domain.SessionSummary
	for _, s := range summaries {
		d := domain.SessionDateFromID(s.SessionID)
		if d.IsZero() {
			continue
		}
		if !cutoff.IsZero() && d.Before(cutoff) {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

func loadAllOutcomes(store ledger.OutcomeStore, summaries []domain.SessionSummary) []domain.RecommendationOutcome {
	var allOutcomes []domain.RecommendationOutcome
	for _, s := range summaries {
		outcomes, err := store.LoadSessionOutcomes(s.SessionID)
		if err != nil {
			continue
		}
		allOutcomes = append(allOutcomes, outcomes...)
	}
	return allOutcomes
}

// loadAgentDisplayNames loads the agent display name map from the registry.
// Returns nil on error (graceful fallback).
func loadAgentDisplayNames() map[string]string {
	data, err := os.ReadFile(constants.AgentsConfigPath)
	if err != nil {
		return nil
	}
	var reg domain.AgentRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil
	}
	names := make(map[string]string, len(reg.Agents))
	for _, a := range reg.Agents {
		names[a.ID] = a.Name
	}
	return names
}

// CalculateSharpeRatio computes the annualized Sharpe ratio from daily returns.
// Returns nil when there are fewer than 2 daily returns.
func CalculateSharpeRatio(dailyReturns []float64) *float64 {
	if len(dailyReturns) < 2 {
		return nil
	}
	v := portfolio.ComputeSharpe(dailyReturns, portfolio.SharpeConfig{
		Frequency:  portfolio.FrequencyPerDay,
		MinSamples: 2,
	})
	return &v
}

func calculateCalmarRatio(annualizedReturn, maxDrawdown float64) float64 {
	if maxDrawdown == 0 {
		return 0
	}
	return annualizedReturn / maxDrawdown
}

func calculateTradeMetrics(outcomes []domain.RecommendationOutcome) (winRate float64, totalTrades int, realTrades int, syntheticTrades int, avgWin, avgLoss, profitFactor float64) {
	wins := 0
	var winSum, lossSum float64
	winCount, lossCount := 0, 0

	for _, oc := range outcomes {
		if !oc.PassedGuards {
			continue
		}
		totalTrades++
		if oc.IsSynthetic {
			syntheticTrades++
			continue
		}
		realTrades++
		threshold := winRateThreshold()
		if oc.ForwardReturn > threshold {
			wins++
			winSum += oc.ForwardReturn
			winCount++
		} else if oc.ForwardReturn < threshold {
			lossSum += oc.ForwardReturn
			lossCount++
		}
	}

	if realTrades > 0 {
		winRate = float64(wins) / float64(realTrades)
	}
	if winCount > 0 {
		avgWin = winSum / float64(winCount)
	}
	if lossCount > 0 {
		avgLoss = lossSum / float64(lossCount)
	}
	if lossSum != 0 {
		profitFactor = winSum / math.Abs(lossSum)
	}

	return
}

func calculateTopAgents(outcomes []domain.RecommendationOutcome, agentNames map[string]string) []AgentContribution {
	type agg struct {
		agentID        string
		skill          string
		layer          string
		returns        []float64
		wins           int
		trades         int
		syntheticCount int
		grossWins      float64
		grossLosses    float64
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
		entry.trades++
		if oc.IsSynthetic {
			entry.syntheticCount++
			continue
		}
		entry.returns = append(entry.returns, oc.ForwardReturn)
		threshold := winRateThreshold()
		if oc.ForwardReturn > threshold {
			entry.wins++
			entry.grossWins += oc.ForwardReturn
		} else if oc.ForwardReturn < threshold {
			entry.grossLosses += math.Abs(oc.ForwardReturn)
		}
	}

	var contributions []AgentContribution
	for _, a := range byAgent {
		if a.trades == 0 {
			continue
		}
		realTrades := a.trades - a.syntheticCount
		// Skip agents whose real outcomes all carry a zero forward return.
		// This is the signature of a SQLite fallback read (the SQLite outcomes
		// table drops evaluation fields), not a genuine flat return — showing
		// such an agent as 0.00% misleads. Genuine forward returns are nonzero
		// in practice (they track price moves), so all-zero here reliably means
		// missing data rather than a real flat performance.
		if realTrades > 0 && len(a.returns) == realTrades && allZero(a.returns) {
			continue
		}
		var aggregateForwardReturn float64
		for _, r := range a.returns {
			aggregateForwardReturn += r
		}
		avgReturn := 0.0
		if realTrades > 0 {
			avgReturn = aggregateForwardReturn / float64(realTrades)
		}
		winRate := 0.0
		if realTrades > 0 {
			winRate = float64(a.wins) / float64(realTrades)
		}
		sharpeLike := portfolio.ComputeSharpe(a.returns, portfolio.SharpeConfig{
			Frequency:  portfolio.FrequencyPerOutcome,
			MinSamples: 5,
		})
		var sharpeLikePtr *float64
		if len(a.returns) >= 5 && stdDev(a.returns) > 0 {
			v := sharpeLike
			sharpeLikePtr = &v
		}
		pf := 0.0
		if a.grossLosses > 0 {
			pf = a.grossWins / a.grossLosses
		}
		displayName := a.agentID
		if name, ok := agentNames[a.agentID]; ok && name != "" {
			displayName = name
		}

		contributions = append(contributions, AgentContribution{
			AgentID:                a.agentID,
			DisplayName:            displayName,
			Skill:                  a.skill,
			Layer:                  a.layer,
			AggregateForwardReturn: aggregateForwardReturn,
			WinRate:                winRate,
			RealTradeCount:         realTrades,
			SyntheticTradeCount:    a.syntheticCount,
			TradeCount:             a.trades,
			AvgReturn:              avgReturn,
			ProfitFactor:           pf,
			SharpeLike:             sharpeLikePtr,
		})
	}

	slices.SortFunc(contributions, func(a, b AgentContribution) int {
		if a.AggregateForwardReturn > b.AggregateForwardReturn {
			return -1
		}
		if a.AggregateForwardReturn < b.AggregateForwardReturn {
			return 1
		}
		return 0
	})

	if len(contributions) > 5 {
		contributions = contributions[:5]
	}
	return contributions
}

// CalculateBeta computes the CAPM beta of the portfolio relative to the benchmark.
// Uses the ratio of portfolio volatility to benchmark return magnitude,
// consistent with benchmark.go's simplified approach.
// Returns nil when there are insufficient samples or the benchmark return is unavailable.
func CalculateBeta(portfolioReturns []float64, benchmarkReturn *float64) *float64 {
	if len(portfolioReturns) < 60 || benchmarkReturn == nil || *benchmarkReturn == 0 {
		return nil
	}
	portVol := stdDev(portfolioReturns)
	benchVol := math.Abs(*benchmarkReturn)
	if benchVol == 0 {
		return nil
	}
	v := portVol / benchVol
	return &v
}

// CalculateAlpha computes the risk-adjusted excess return.
// Returns nil when either beta or benchmark return is unavailable.
func CalculateAlpha(portfolioReturn float64, beta *float64, benchmarkReturn *float64) *float64 {
	if beta == nil || benchmarkReturn == nil {
		return nil
	}
	v := portfolioReturn - *beta**benchmarkReturn
	return &v
}

// CalculateTrackingError computes the standard deviation of portfolio returns
// as a measure of tracking error relative to the benchmark.
// Returns nil when there are fewer than 2 portfolio returns.
func CalculateTrackingError(portfolioReturns []float64) *float64 {
	if len(portfolioReturns) < 2 {
		return nil
	}
	v := stdDev(portfolioReturns)
	return &v
}

// CalculateInfoRatio computes the information ratio (outperformance / tracking error).
// Returns nil when either input is nil or tracking error is zero.
func CalculateInfoRatio(outperformance *float64, trackingError *float64) *float64 {
	if outperformance == nil || trackingError == nil || *trackingError == 0 {
		return nil
	}
	v := *outperformance / *trackingError
	return &v
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
		if oc.IsSynthetic {
			continue
		}
		// Prefer the regime recorded on the outcome itself (rich JSONL source
		// carries it). Fall back to the summary lookup only when the outcome
		// has no regime field.
		regime := oc.Regime
		if regime == "" {
			regime = findRegimeForWindow(summaries, oc.Window)
		}
		// Outcomes with no resolvable regime (e.g. SQLite fallback rows written
		// before BL-06 carried evaluation fields) have no genuine regime data —
		// skip rather than bucket them under "unknown", which would mislead as
		// a real market state.
		if regime == "" {
			continue
		}
		regimeReturns[regime] = append(regimeReturns[regime], oc.ForwardReturn)
	}

	for regime, returns := range regimeReturns {
		if _, ok := regimeData[regime]; !ok {
			regimeData[regime] = &RegimePerformance{Regime: regime}
		}
		var aggregateForwardReturn float64
		wins := 0
		threshold := winRateThreshold()
		for _, r := range returns {
			aggregateForwardReturn += r
			if r > threshold {
				wins++
			}
		}
		regimeData[regime].AggregateForwardReturn = aggregateForwardReturn
		regimeData[regime].AvgReturn = aggregateForwardReturn / float64(len(returns))
		regimeData[regime].WinRate = float64(wins) / float64(len(returns))
	}

	result := make(map[string]RegimePerformance, len(regimeData))
	for k, v := range regimeData {
		result[k] = *v
	}

	return RegimeBreakdown{Regimes: result}
}

func findRegimeForWindow(summaries []domain.SessionSummary, window string) string {
	if window == "" {
		return ""
	}
	// RecommendationOutcome.Window is stored in ISO format ("2026-01-01"),
	// not the "session-YYYYMMDD-..." session ID that SessionDateFromID parses.
	// Resolve the window date from either format.
	windowDate := domain.SessionDateFromID(window)
	if windowDate.IsZero() {
		if d, err := time.Parse("2006-01-02", window); err == nil {
			windowDate = d
		}
	}
	for _, s := range summaries {
		sessionDate := domain.SessionDateFromID(s.SessionID)
		if !windowDate.IsZero() && !sessionDate.IsZero() && windowDate.Equal(sessionDate) {
			return string(s.Regime)
		}
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

	// Compute cumulative return from the sorted monthly returns.
	cumulative := 1.0
	for i := range monthlyReturns {
		cumulative *= 1 + monthlyReturns[i].Return
		c := cumulative - 1
		monthlyReturns[i].Cumulative = &c
	}

	return monthlyReturns
}

// allZero reports whether every value in the slice is exactly zero.
// Used to detect agents whose outcomes were read from a SQLite fallback
// (which drops ForwardReturn), rather than a genuine flat performance.
func allZero(values []float64) bool {
	for _, v := range values {
		if v != 0 {
			return false
		}
	}
	return true
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

// winRateThreshold returns the cost-adjusted ForwardReturn cutoff for win
// classification. ForwardReturn values strictly greater than this threshold
// are counted as wins; values below it are losses (covering transaction cost
// + slippage). Falls back to the package default if config is unavailable
// (e.g. during tests that don't initialize the singleton).
func winRateThreshold() float64 {
	defaultWinRateThreshold := 0.002
	cfg := configpkg.GetParametersConfig()
	if cfg == nil || cfg.Reporting.WinRateThreshold.Value == 0 {
		return defaultWinRateThreshold
	}
	return cfg.Reporting.WinRateThreshold.Value
}

// formatSharpeLike renders a *float64 SharpeLike for markdown tables.
// Returns "N/A" when nil (insufficient samples), "%.2f" otherwise.
func formatSharpeLike(s *float64) string {
	if s == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", *s)
}
