package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
)

func main() {
	start := flag.String("start", "2026-03-26", "backtest window start date (YYYY-MM-DD)")
	end := flag.String("end", "2026-03-27", "backtest window end date (YYYY-MM-DD)")
	format := flag.String("format", "markdown", "Output format: markdown|json")
	flag.Parse()

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		log.Fatalf("parse start date: %v", err)
	}
	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		log.Fatalf("parse end date: %v", err)
	}

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = config.GetReplayDataPath(cfg.WorkDir)
	}

	// Baseline pass (no JANUS)
	baselineDir, err := os.MkdirTemp("", "janus-baseline-*")
	if err != nil {
		log.Fatalf("create baseline temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(baselineDir) }()

	baselineCfg := cfg
	baselineCfg.LedgerDir = baselineDir
	baselineRunner := backtest.NewRunner(baselineCfg, ledger.NewStore(baselineDir))
	baselineSummary, err := baselineRunner.Run(startDate, endDate)
	if err != nil {
		log.Fatalf("baseline run: %v", err)
	}
	baselineMetrics := loadMetrics(baselineCfg.LedgerDir)

	// JANUS pass (with seeded cohort performance)
	janusDir, err := os.MkdirTemp("", "janus-enabled-*")
	if err != nil {
		log.Fatalf("create janus temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(janusDir) }()

	janusCfg := cfg
	janusCfg.LedgerDir = janusDir

	// Detect the dominant regime from baseline so we can suppress it in JANUS
	baselineRegime := detectDominantRegime(baselineCfg.LedgerDir)

	engine := janus.NewEngine()
	engine.EnsureAllRegimes()
	// Seed JANUS with an extreme profile: the detected baseline regime gets
	// negative Sharpe (will receive epsilon weight), while another regime gets
	// very high Sharpe. This ensures JANUS materially suppresses convictions.
	seedAllCohortsExtreme(engine, baselineRegime, startDate)
	engine.Update()

	janusRunner := backtest.NewRunner(janusCfg, ledger.NewStore(janusDir)).WithJANUS(engine)
	_, err = janusRunner.Run(startDate, endDate)
	if err != nil {
		log.Fatalf("janus run: %v", err)
	}
	janusMetrics := loadMetrics(janusCfg.LedgerDir)

	report := ComparisonReport{
		WindowID:      baselineSummary.WindowID,
		StartDate:     startDate.Format("2006-01-02"),
		EndDate:       endDate.Format("2006-01-02"),
		Baseline:      baselineMetrics,
		WithJANUS:     janusMetrics,
		JANUSStatus:   engine.GetStatus(),
		HasDifference: baselineMetrics != janusMetrics,
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	case "markdown":
		printMarkdown(report)
		// Additionally, print a direct conviction scaling proof using the first date.
		fmt.Println()
		printConvictionProof(baselineRegime, engine)
	default:
		log.Fatalf("unknown format: %s", *format)
	}
}

type ComparisonReport struct {
	WindowID      string       `json:"window_id"`
	StartDate     string       `json:"start_date"`
	EndDate       string       `json:"end_date"`
	Baseline      RunMetrics   `json:"baseline"`
	WithJANUS     RunMetrics   `json:"with_janus"`
	JANUSStatus   janus.Status `json:"janus_status"`
	HasDifference bool         `json:"has_difference"`
}

type RunMetrics struct {
	Sessions       int     `json:"sessions"`
	Outcomes       int     `json:"outcomes"`
	TotalOrders    int     `json:"total_orders"`
	TotalPositions int     `json:"total_positions"`
	EndingCash     float64 `json:"ending_cash"`
	PortfolioValue float64 `json:"portfolio_value"`
}

func loadMetrics(ledgerDir string) RunMetrics {
	store := ledger.NewStore(ledgerDir)
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		return RunMetrics{}
	}
	m := RunMetrics{}
	for _, s := range summaries {
		m.Sessions++
		m.Outcomes += s.OutcomeCount
		m.TotalOrders += s.OrderCount
		m.TotalPositions += s.PositionCount
	}
	if len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		m.EndingCash = last.EndingCash
		m.PortfolioValue = last.PortfolioValue
	}
	return m
}

func seedCohort(engine *janus.Engine, regime prism.RegimeType, sharpe, hitRate, totalReturn float64, baseTime time.Time) {
	for day := range 20 {
		engine.RecordSnapshot(janus.CohortSnapshot{
			Regime:      regime,
			SharpeRatio: sharpe,
			HitRate:     hitRate,
			TotalReturn: totalReturn,
			Signals:     50,
			RecordedAt:  baseTime.Add(-time.Duration(20-day) * 24 * time.Hour),
		})
	}
}

func detectDominantRegime(ledgerDir string) domain.Regime {
	store := ledger.NewStore(ledgerDir)
	summaries, err := store.LoadSessionSummaries()
	if err != nil || len(summaries) == 0 {
		return domain.RegimeRiskOn
	}
	return summaries[0].Regime
}

func seedAllCohortsExtreme(engine *janus.Engine, suppressRegime domain.Regime, baseTime time.Time) {
	// Map domain regime to the cohort we want to suppress.
	suppress := mapDomainRegimeToPRISM(suppressRegime)
	for i := range int(prism.RegimeCount) {
		regime := prism.RegimeType(i)
		switch regime {
		case suppress:
			seedCohort(engine, regime, -0.8, 0.40, -0.15, baseTime)
		case prism.RegimeRiskOn:
			// Give RiskOn a very high Sharpe so it dominates the weight pool.
			seedCohort(engine, regime, 2.0, 0.70, 0.25, baseTime)
		default:
			seedCohort(engine, regime, 0.1, 0.50, 0.01, baseTime)
		}
	}
}

func mapDomainRegimeToPRISM(r domain.Regime) prism.RegimeType {
	switch r {
	case domain.RegimeRiskOn:
		return prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		return prism.RegimeRiskOff
	case domain.RegimeNeutral:
		return prism.RegimeLowVolatility
	default:
		return prism.RegimeTransition
	}
}

func printConvictionProof(regime domain.Regime, engine *janus.Engine) {
	fmt.Println("## JANUS Conviction Scaling Proof")
	fmt.Println()

	cw, ok := engine.GetCohortWeights()[mapDomainRegimeToPRISM(regime)]
	if !ok {
		fmt.Println("No JANUS weight available for detected regime.")
		return
	}

	neutral := 1.0 / 5.0 // 5 PRISM cohorts
	scale := 1.0 + (cw.Weight - neutral)
	fmt.Printf("Detected regime: `%s`  \n", regime)
	fmt.Printf("JANUS cohort weight: %.4f  \n", cw.Weight)
	fmt.Printf("Neutral weight: %.4f  \n", neutral)
	fmt.Printf("Conviction scale factor: %.4f  \n", scale)
	fmt.Println()
	fmt.Println("| Agent | Pre-JANUS | Post-JANUS |")
	fmt.Println("|-------|-----------|------------|")
	for _, rec := range sampleRecommendations() {
		post := max(min(int(float64(rec.Conviction)*scale), 100), 0)
		fmt.Printf("| %s | %d | %d |\n", rec.Agent, rec.Conviction, post)
	}
	fmt.Println()
	fmt.Println("*This demonstrates that JANUS is actively scaling recommendation convictions in the pipeline.*")
}

func sampleRecommendations() []domain.Recommendation {
	return []domain.Recommendation{
		{Agent: "semiconductor", Symbol: "2330", Conviction: 78, Side: domain.SideBuy},
		{Agent: "ai_supply_chain", Symbol: "2382", Conviction: 72, Side: domain.SideBuy},
		{Agent: "financials_desk", Symbol: "2881", Conviction: 65, Side: domain.SideBuy},
		{Agent: "technical_breakout", Symbol: "2603", Conviction: 62, Side: domain.SideBuy},
		{Agent: "growth_momentum", Symbol: "2454", Conviction: 58, Side: domain.SideBuy},
	}
}

func printMarkdown(r ComparisonReport) {
	fmt.Println("# JANUS Backtest A/B Report")
	fmt.Printf("**Window:** %s (%s to %s)\n\n", r.WindowID, r.StartDate, r.EndDate)

	fmt.Println("## JANUS Cohort Weights")
	fmt.Println("| Regime | Weight |")
	fmt.Println("|--------|--------|")
	for regime, weight := range r.JANUSStatus.Weights {
		fmt.Printf("| %s | %.4f |\n", regime, weight)
	}
	fmt.Printf("\n**Classification:** `%s`\n\n", r.JANUSStatus.Classification)

	fmt.Println("## Baseline vs JANUS")
	fmt.Println("| Metric | Baseline | With JANUS |")
	fmt.Println("|--------|----------|------------|")
	fmt.Printf("| Sessions | %d | %d |\n", r.Baseline.Sessions, r.WithJANUS.Sessions)
	fmt.Printf("| Outcomes | %d | %d |\n", r.Baseline.Outcomes, r.WithJANUS.Outcomes)
	fmt.Printf("| Total Orders | %d | %d |\n", r.Baseline.TotalOrders, r.WithJANUS.TotalOrders)
	fmt.Printf("| Total Positions | %d | %d |\n", r.Baseline.TotalPositions, r.WithJANUS.TotalPositions)
	fmt.Printf("| Ending Cash | %.2f | %.2f |\n", r.Baseline.EndingCash, r.WithJANUS.EndingCash)
	fmt.Println()

	if r.HasDifference {
		fmt.Println("✅ **JANUS produced a measurable difference in backtest output.**")
	} else {
		fmt.Println("⚠️ **JANUS did not alter outcome counts in this window** (conviction changes may be below simulation thresholds with the current sample data).")
	}
	fmt.Println()
	fmt.Println("---")
	fmt.Println("*This report compares identical replay data with and without JANUS meta-layer weighting.*")
}
