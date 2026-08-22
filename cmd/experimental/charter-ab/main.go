// Command charter-ab — Phase C3 stepwise incremental A/B of the five charter
// switches (PeriodOnly → +StrategyFilter → +MacroFlow → +CashReserve →
// +ConvictionFloor).
//
// For each step the harness replays the same window twice:
//
//	baseline arm — previous step's feature set (step 1: Phase A, no charter)
//	feature arm  — current step's cumulative switch set
//
// Both arms share the replay CSV, agent registry, parameters and sim rules;
// only the charter switches differ (isolated variable). Each arm runs in its
// own LedgerDir (MkdirTemp) and produces a ComparisonReport with equity
// curve, Sharpe, MaxDrawdown, win rate, order/position counts, ending cash,
// regime/period distribution, per-agent hit rate, and raw-vs-final
// recommendation attribution.
//
// Statistics (internal/charter/stats.go):
//   - paired t-test on daily return differences (α = 0.05)
//   - BCa bootstrap 10,000 resamples, 95% CI for Sharpe/MaxDrawdown diffs
//
// Output: /tmp/charter-ab/step-<n>.json per step + terminal summary.
//
// Phase C4 adds -writeback: each step report is converted into a charter
// delta (internal/charter/delta.go) and recorded into the baseline policy
// (internal/baseline writeback.go) through baseline.NewManager. Only
// significant_enable deltas write runtime constraints; directional_watch /
// inert / degenerate deltas are recorded as evidence-only promotions (the
// runtime behavior is never overridden). Re-running the writeback is
// idempotent.
//
// Usage:
//
//	go run ./cmd/experimental/charter-ab -start 2026-01-01 -end 2026-08-21
//	go run ./cmd/experimental/charter-ab -writeback -policy data/state/baseline_policy.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ─── report structures ────────────────────────────────────────────────────

// StepReport is one step's ComparisonReport (baseline vs +feature).
type StepReport struct {
	Step               int                     `json:"step"`
	Arm                string                  `json:"arm"`
	FeatureSwitches    []string                `json:"feature_switches"`
	Window             WindowInfo              `json:"window"`
	Baseline           RunMetrics              `json:"baseline"`
	Feature            RunMetrics              `json:"feature"`
	Stats              Stats                   `json:"stats"`
	Attribution        Attribution             `json:"attribution"`
	BaselinePeriodDist map[string]int          `json:"baseline_period_distribution"`
	FeaturePeriodDist  map[string]int          `json:"feature_period_distribution"`
	PerAgent           map[string]AgentCompare `json:"per_agent"`
	HasSignificant     bool                    `json:"has_significant_finding"`
	DurationSeconds    float64                 `json:"duration_seconds"`
}

// WindowInfo describes the replay window actually processed.
type WindowInfo struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Days  int    `json:"days"`
}

// RunMetrics is one arm's summary metrics.
type RunMetrics struct {
	Arm                string         `json:"arm"`
	Sessions           int            `json:"sessions"`
	Outcomes           int            `json:"outcomes"`
	Hits               int            `json:"hits"`
	WinRate            float64        `json:"win_rate"`
	TotalOrders        int            `json:"total_orders"`
	TotalPositions     int            `json:"total_positions"`
	EndingCash         float64        `json:"ending_cash"`
	PortfolioValue     float64        `json:"portfolio_value"`
	TotalReturn        float64        `json:"total_return"`
	AnnualizedReturn   float64        `json:"annualized_return"`
	Sharpe             float64        `json:"sharpe"`
	MaxDrawdown        float64        `json:"max_drawdown"`
	EquityCurve        []float64      `json:"equity_curve"`
	DailyReturns       []float64      `json:"daily_returns"`
	RegimeDistribution map[string]int `json:"regime_distribution"`
}

// Stats holds the statistical tests for the step.
type Stats struct {
	PairedT     charter.TTestResult     `json:"paired_t_test_daily_return_diff"`
	Sharpe      charter.BootstrapResult `json:"sharpe_diff_bca_95"`
	MaxDrawdown charter.BootstrapResult `json:"max_drawdown_diff_bca_95"`
}

// Attribution reports raw vs final recommendation totals (pipeline shrinkage)
// and per-agent raw recommendation deltas that isolate each switch's gate.
type Attribution struct {
	RawRecsBaseline   int            `json:"raw_recs_baseline"`
	RawRecsFeature    int            `json:"raw_recs_feature"`
	FinalRecsBaseline int            `json:"final_recs_baseline"`
	FinalRecsFeature  int            `json:"final_recs_feature"`
	RawByAgent        map[string]int `json:"raw_by_agent_delta_feature_minus_baseline"`
}

// AgentCompare is a per-agent outcome comparison between the two arms.
type AgentCompare struct {
	BaselineOutcomes int     `json:"baseline_outcomes"`
	BaselineHits     int     `json:"baseline_hits"`
	BaselineHitRate  float64 `json:"baseline_hit_rate"`
	FeatureOutcomes  int     `json:"feature_outcomes"`
	FeatureHits      int     `json:"feature_hits"`
	FeatureHitRate   float64 `json:"feature_hit_rate"`
}

// armResult bundles one arm run's metrics, outcomes and recommendation trace.
type armResult struct {
	Summary domain.BacktestWindowSummary
	Metrics RunMetrics
	Trace   *charter.RecommendationTrace
	// outcomesByAgent aggregates recommendation outcomes per agent (needed for
	// the report after the arm's LedgerDir is removed).
	outcomesByAgent map[string]agentOutcomeAgg
}

type agentOutcomeAgg struct {
	outcomes int
	hits     int
}

func main() {
	start := flag.String("start", "2026-01-01", "window start (YYYY-MM-DD)")
	end := flag.String("end", "2026-08-21", "window end (YYYY-MM-DD)")
	outDir := flag.String("out", "/tmp/charter-ab", "output directory for per-step JSON")
	steps := flag.Int("steps", 5, "number of stepwise arms to run (1..5)")
	writeback := flag.Bool("writeback", false, "write charter A/B deltas back to the baseline policy instead of running the A/B")
	policyPath := flag.String("policy", "", "baseline policy path for -writeback (default: <workdir>/data/state/baseline_policy.json)")
	flag.Parse()

	if *writeback {
		cfg := config.Load()
		if err := runWriteback(*outDir, *steps, *policyPath, cfg); err != nil {
			log.Fatalf("charter-ab -writeback: %v", err)
		}
		return
	}

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		log.Fatalf("parse start date: %v", err)
	}
	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		log.Fatalf("parse end date: %v", err)
	}
	if *steps < 1 || *steps > 5 {
		log.Fatalf("steps must be in [1,5], got %d", *steps)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	cfg := config.Load()
	cfg.CharterMode = false // the harness controls each arm via WithCharterMode
	// Force the JSONL store: each arm replays into an isolated temp LedgerDir,
	// so a user-level ATLAS_STORE_BACKEND=postgres must not route the harness's
	// per-arm stores to a shared database.
	cfg.StoreBackend = "jsonl"
	cfg.ReplayDataPath = filepath.Join(cfg.WorkDir, constants.ReplayCSVPath)
	if _, err := os.Stat(cfg.ReplayDataPath); err != nil {
		log.Fatalf("replay CSV %s not found — data/ is gitignored; copy it from the main workspace: %v", cfg.ReplayDataPath, err)
	}

	arms := charter.StepwiseArms()
	armNames := charter.ArmNames()
	prevOpts := charter.Options{} // step 1 baseline = Phase A (no charter)

	for step := 1; step <= *steps; step++ {
		featureOpts := arms[step-1]
		stepStart := time.Now()

		baselineRes, err := runArm(cfg, startDate, endDate, prevOpts, "baseline")
		if err != nil {
			log.Fatalf("step %d baseline run: %v", step, err)
		}
		featureRes, err := runArm(cfg, startDate, endDate, featureOpts, "feature")
		if err != nil {
			log.Fatalf("step %d feature run: %v", step, err)
		}

		report := buildReport(step, armNames[step-1], featureOpts, startDate, endDate, baselineRes, featureRes)
		report.DurationSeconds = time.Since(stepStart).Seconds()

		path := filepath.Join(*outDir, fmt.Sprintf("step-%d.json", step))
		if err := writeJSON(path, report); err != nil {
			log.Fatalf("step %d write report: %v", step, err)
		}
		printStepSummary(report)
		prevOpts = featureOpts
	}
	fmt.Println()
	fmt.Printf("✅ charter-ab done: %d steps → %s/step-{1..%d}.json\n", *steps, *outDir, *steps)
}

// runArm replays the window once with the given charter options in an
// isolated LedgerDir. Zero options → Phase A (no charter).
func runArm(cfg config.Config, start, end time.Time, opts charter.Options, label string) (*armResult, error) {
	dir, err := os.MkdirTemp("", "charter-ab-"+label+"-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	armCfg := cfg
	armCfg.LedgerDir = dir

	tr := &charter.RecommendationTrace{}
	runner := backtest.NewRunner(armCfg, ledger.NewStore(dir))
	if opts.Enabled() {
		runner.WithCharterMode(opts)
	}
	runner.WithRecommendationTrace(tr)

	summary, err := runner.Run(start, end)
	if err != nil {
		return nil, err
	}
	metrics, byAgent, err := collectMetrics(dir, label)
	if err != nil {
		return nil, fmt.Errorf("collect metrics: %w", err)
	}
	return &armResult{Summary: summary, Metrics: metrics, Trace: tr, outcomesByAgent: byAgent}, nil
}

// collectMetrics reads session summaries + outcomes from an arm's LedgerDir
// and derives the equity curve, daily returns, Sharpe, MaxDrawdown, win rate,
// order/position counts and regime distribution.
func collectMetrics(ledgerDir, label string) (RunMetrics, map[string]agentOutcomeAgg, error) {
	store := ledger.NewStore(ledgerDir)
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		return RunMetrics{}, nil, err
	}
	// Sort by session date (session-YYYYMMDD-*) — directory order is not
	// guaranteed.
	sort.Slice(summaries, func(i, j int) bool {
		return domain.SessionDateFromID(summaries[i].SessionID).Before(domain.SessionDateFromID(summaries[j].SessionID))
	})

	m := RunMetrics{Arm: label, RegimeDistribution: make(map[string]int)}
	equity := make([]float64, 0, len(summaries))
	for _, s := range summaries {
		m.Sessions++
		m.TotalOrders += s.OrderCount
		m.TotalPositions += s.PositionCount
		m.EndingCash = s.EndingCash
		pv := s.PortfolioValue
		if pv == 0 {
			pv = s.EndingCash
		}
		equity = append(equity, pv)
		m.RegimeDistribution[string(s.Regime)]++
	}
	m.EquityCurve = equity
	if len(equity) > 0 {
		m.PortfolioValue = equity[len(equity)-1]
		if startVal := equity[0]; startVal > 0 {
			m.TotalReturn = (equity[len(equity)-1] - startVal) / startVal
		}
	}
	m.DailyReturns = dailyReturns(equity)
	m.Sharpe = armSharpe(m.DailyReturns)
	m.MaxDrawdown = charter.MaxDrawdown(equity)
	if len(equity) > 1 {
		days := float64(len(equity) - 1)
		if m.TotalReturn > -1 {
			m.AnnualizedReturn = math.Pow(1+m.TotalReturn, 252/days) - 1
		} else {
			m.AnnualizedReturn = -1
		}
	}

	// Win rate + per-agent hits from outcomes.
	_, outcomes, err := store.LoadAllSessionScorecards()
	if err != nil {
		return RunMetrics{}, nil, err
	}
	m.Outcomes = len(outcomes)
	byAgent := make(map[string]agentOutcomeAgg)
	for _, o := range outcomes {
		if o.Hit {
			m.Hits++
		}
		agg := byAgent[o.AgentID]
		agg.outcomes++
		if o.Hit {
			agg.hits++
		}
		byAgent[o.AgentID] = agg
	}
	if m.Outcomes > 0 {
		m.WinRate = float64(m.Hits) / float64(m.Outcomes)
	}
	return m, byAgent, nil
}

func dailyReturns(equity []float64) []float64 {
	if len(equity) < 2 {
		return nil
	}
	rets := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		if equity[i-1] > 0 {
			rets = append(rets, (equity[i]-equity[i-1])/equity[i-1])
		}
	}
	return rets
}

func armSharpe(daily []float64) float64 {
	return portfolio.ComputeSharpe(daily, portfolio.SharpeConfig{Frequency: portfolio.FrequencyPerDay})
}

func buildReport(step int, armName string, opts charter.Options, start, end time.Time, baseline, feature *armResult) StepReport {
	bMetrics, fMetrics := baseline.Metrics, feature.Metrics

	stats := Stats{
		PairedT:     charter.PairedTTest(bMetrics.DailyReturns, fMetrics.DailyReturns),
		Sharpe:      charter.BCaBootstrap(bMetrics.DailyReturns, fMetrics.DailyReturns, charter.SharpeDiff, 10000, 0.05),
		MaxDrawdown: charter.BCaBootstrap(bMetrics.EquityCurve, fMetrics.EquityCurve, charter.MaxDrawdownDiff, 10000, 0.05),
	}

	return StepReport{
		Step:               step,
		Arm:                armName,
		FeatureSwitches:    opts.Names(),
		Window:             WindowInfo{Start: start.Format("2006-01-02"), End: end.Format("2006-01-02"), Days: bMetrics.Sessions},
		Baseline:           bMetrics,
		Feature:            fMetrics,
		Stats:              stats,
		Attribution:        buildAttribution(baseline.Trace, feature.Trace),
		BaselinePeriodDist: baseline.Trace.PeriodDistribution(),
		FeaturePeriodDist:  feature.Trace.PeriodDistribution(),
		PerAgent:           perAgentComparison(baseline, feature),
		HasSignificant:     stats.PairedT.Significant || stats.Sharpe.Significant || stats.MaxDrawdown.Significant,
	}
}

func buildAttribution(baseline, feature *charter.RecommendationTrace) Attribution {
	bRaw, bFinal, bRawByAgent, _ := baseline.Totals()
	fRaw, fFinal, fRawByAgent, _ := feature.Totals()

	allAgents := make(map[string]bool)
	for a := range bRawByAgent {
		allAgents[a] = true
	}
	for a := range fRawByAgent {
		allAgents[a] = true
	}
	delta := make(map[string]int, len(allAgents))
	for a := range allAgents {
		delta[a] = fRawByAgent[a] - bRawByAgent[a]
	}
	return Attribution{
		RawRecsBaseline:   bRaw,
		RawRecsFeature:    fRaw,
		FinalRecsBaseline: bFinal,
		FinalRecsFeature:  fFinal,
		RawByAgent:        delta,
	}
}

// perAgentComparison merges both arms' per-agent outcome aggregates.
func perAgentComparison(baseline, feature *armResult) map[string]AgentCompare {
	agents := make(map[string]bool)
	for a := range baseline.outcomesByAgent {
		agents[a] = true
	}
	for a := range feature.outcomesByAgent {
		agents[a] = true
	}
	result := make(map[string]AgentCompare, len(agents))
	for a := range agents {
		b := baseline.outcomesByAgent[a]
		f := feature.outcomesByAgent[a]
		bc := AgentCompare{
			BaselineOutcomes: b.outcomes,
			BaselineHits:     b.hits,
			FeatureOutcomes:  f.outcomes,
			FeatureHits:      f.hits,
		}
		if b.outcomes > 0 {
			bc.BaselineHitRate = float64(b.hits) / float64(b.outcomes)
		}
		if f.outcomes > 0 {
			bc.FeatureHitRate = float64(f.hits) / float64(f.outcomes)
		}
		result[a] = bc
	}
	return result
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// printStepSummary prints the step's main metrics to the terminal.
func printStepSummary(r StepReport) {
	fmt.Printf("\n=== Step %d: %s (%s) ===\n", r.Step, r.Arm, joinSwitches(r.FeatureSwitches))
	fmt.Printf("window: %s → %s (%d sessions)\n", r.Window.Start, r.Window.End, r.Window.Days)
	fmt.Printf("%-28s %14s %14s\n", "metric", "baseline", "feature")
	row := func(name string, b, f float64) {
		fmt.Printf("%-28s %14.4f %14.4f\n", name, b, f)
	}
	row("total_return", r.Baseline.TotalReturn, r.Feature.TotalReturn)
	row("annualized_return", r.Baseline.AnnualizedReturn, r.Feature.AnnualizedReturn)
	row("sharpe", r.Baseline.Sharpe, r.Feature.Sharpe)
	row("max_drawdown", r.Baseline.MaxDrawdown, r.Feature.MaxDrawdown)
	row("win_rate", r.Baseline.WinRate, r.Feature.WinRate)
	row("ending_cash", r.Baseline.EndingCash, r.Feature.EndingCash)
	fmt.Printf("%-28s %14d %14d\n", "orders", r.Baseline.TotalOrders, r.Feature.TotalOrders)
	fmt.Printf("%-28s %14d %14d\n", "positions", r.Baseline.TotalPositions, r.Feature.TotalPositions)
	fmt.Printf("%-28s %14d %14d\n", "outcomes", r.Baseline.Outcomes, r.Feature.Outcomes)
	fmt.Printf("%-28s %14d %14d\n", "raw_recs", r.Attribution.RawRecsBaseline, r.Attribution.RawRecsFeature)
	fmt.Printf("%-28s %14d %14d\n", "final_recs", r.Attribution.FinalRecsBaseline, r.Attribution.FinalRecsFeature)

	fmt.Println()
	fmt.Printf("paired t-test (daily return diff): t=%.3f p=%.4g df=%d mean_diff=%.6f %s\n",
		r.Stats.PairedT.T, r.Stats.PairedT.P, r.Stats.PairedT.DF, r.Stats.PairedT.MeanDiff, sigMark(r.Stats.PairedT.Significant))
	fmt.Printf("sharpe diff BCa 95%% CI: [%.4f, %.4f] (obs %.4f) %s\n",
		r.Stats.Sharpe.CI95Low, r.Stats.Sharpe.CI95High, r.Stats.Sharpe.Observed, sigMark(r.Stats.Sharpe.Significant))
	fmt.Printf("max_drawdown diff BCa 95%% CI: [%.4f, %.4f] (obs %.4f) %s\n",
		r.Stats.MaxDrawdown.CI95Low, r.Stats.MaxDrawdown.CI95High, r.Stats.MaxDrawdown.Observed, sigMark(r.Stats.MaxDrawdown.Significant))
	fmt.Println()
}

func joinSwitches(sw []string) string {
	if len(sw) == 0 {
		return "none (Phase A)"
	}
	s := ""
	for i, n := range sw {
		if i > 0 {
			s += "+"
		}
		s += n
	}
	return s
}

func sigMark(sig bool) string {
	if sig {
		return "✅ significant (p<0.05 / CI excludes 0)"
	}
	return "➖ not significant"
}

// ─── writeback mode (Phase C4) ────────────────────────────────────────────

// runWriteback loads the C3 step reports from outDir, converts each into a
// charter delta (internal/charter/delta.go) and writes them back to the
// baseline policy at policyPath through baseline.NewManager. It prints a
// per-record summary, then reloads the policy to confirm the closed loop
// ("write back → load → verify"). Directional watch / inert / degenerate
// deltas never override runtime values — the writeback only records evidence
// (and bumps Version per record, keeping version == len(promotions)+1).
func runWriteback(outDir string, steps int, policyPath string, cfg config.Config) error {
	deltas, err := charter.LoadDeltasFromReports(outDir, steps)
	if err != nil {
		return fmt.Errorf("load charter deltas: %w", err)
	}
	if policyPath == "" {
		policyPath = filepath.Join(cfg.WorkDir, "data", "state", "baseline_policy.json")
	}

	before, err := baseline.LoadStrict(policyPath)
	if err != nil {
		return fmt.Errorf("load baseline policy %s: %w", policyPath, err)
	}

	manager := baseline.NewManager(policyPath)
	next, err := manager.WritebackCharter(deltas, outDir)
	if err != nil {
		return fmt.Errorf("charter writeback: %w", err)
	}

	fmt.Printf("\n=== charter-ab -writeback ===\n")
	fmt.Printf("policy: %s\n", policyPath)
	fmt.Printf("version: %d → %d\n", before.Version, next.Version)
	fmt.Printf("promotions: %d → %d\n", len(before.Promotions), len(next.Promotions))
	for _, d := range deltas {
		wasRecorded := false
		for _, p := range before.Promotions {
			if p.ExperimentID == d.ExperimentID() {
				wasRecorded = true
				break
			}
		}
		action := "skipped (already recorded)"
		for _, p := range next.Promotions {
			if p.ExperimentID == d.ExperimentID() && !wasRecorded {
				action = fmt.Sprintf("appended (status=%s, mutation=%s, version_after=%d)",
					p.Status, p.MutationType, p.VersionAfter)
				break
			}
		}
		fmt.Printf("  step %d  %-18s  %-20s  → %s\n", d.Step, d.Switch, d.Verdict, action)
		fmt.Printf("      evidence: %s\n", d.Evidence)
	}

	// Closed-loop reload: the policy on disk must load and carry the charter
	// records (mechanism integrity — runtime behavior is unchanged).
	reloaded, err := baseline.LoadStrict(policyPath)
	if err != nil {
		return fmt.Errorf("reload policy after writeback: %w", err)
	}
	charterRecs := 0
	for _, p := range reloaded.Promotions {
		if strings.HasPrefix(p.ExperimentID, "charter-ab-") {
			charterRecs++
		}
	}
	fmt.Printf("reload OK: version=%d promotions=%d (charter records=%d)\n",
		reloaded.Version, len(reloaded.Promotions), charterRecs)
	return nil
}
