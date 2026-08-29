package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type sessionData struct {
	Summary  domain.SessionSummary
	Outcomes []recommendation.RecommendationOutcome
}

type ruleStat struct {
	Total     int
	Blocked   int
	Passed    int
	ValueSum  float64
	ThreshSum float64
}

type sessionResult struct {
	SessionID     string
	TotalOrders   int
	BlockedOrders int
	AllowedOrders int
	RuleStats     map[string]*ruleStat
}

const (
	defaultConviction = 50
	defaultPortfolio  = 3_000_000.0
	defaultCashRatio  = 0.3
	estVarPct         = 0.02
	maxOrderPct       = 0.15
)

var sectorMap = map[string]string{
	"2330": "semiconductor", "2303": "semiconductor", "2308": "semiconductor",
	"2317": "electronics", "2382": "electronics", "2395": "electronics",
	"2454": "semiconductor", "2408": "semiconductor", "2449": "semiconductor",
	"3034": "semiconductor", "3105": "semiconductor", "3443": "semiconductor",
	"2412": "telecom", "3045": "telecom", "4904": "telecom",
	"1101": "cement", "1102": "cement", "1216": "food", "1210": "food",
	"1301": "petrochemical", "1303": "petrochemical", "1326": "petrochemical",
	"1402": "textile", "1409": "textile", "1476": "textile", "2912": "textile",
	"2002": "steel", "2031": "steel",
	"2201": "automotive", "2207": "automotive", "2227": "automotive",
	"2603": "shipping", "2609": "shipping", "2610": "shipping", "2615": "shipping",
	"2618": "shipping", "2637": "shipping",
	"2801": "financials", "2812": "financials", "2880": "financials",
	"2881": "financials", "2882": "financials", "2883": "financials",
	"2884": "financials", "2885": "financials", "2886": "financials",
	"2887": "financials", "2888": "financials", "2889": "financials", "2890": "financials",
	"2891": "financials", "2892": "financials",
	"2915": "trading", "9910": "trading",
	"3008": "electronics", "3017": "electronics", "3037": "electronics",
	"3044": "electronics", "3231": "electronics", "3533": "electronics",
	"3596": "electronics", "3661": "electronics", "3679": "electronics",
	"3701": "trading", "3702": "trading", "3711": "electronics",
	"4958": "semiconductor", "5269": "semiconductor", "5347": "semiconductor",
	"6005": "financials", "8028": "electronics",
	"8046": "semiconductor", "8150": "semiconductor",
	"8454": "food", "9904": "trading", "9907": "trading",
	"2301": "electronics", "2302": "electronics",
	"2323": "electronics", "2324": "electronics", "2347": "electronics",
	"2352": "electronics", "2356": "electronics", "2357": "electronics",
	"2360": "electronics", "2376": "electronics", "2385": "electronics",
	"2392": "electronics", "2401": "electronics", "2439": "electronics",
	"2474": "semiconductor", "2498": "electronics",
}

func stripTWSuffix(symbol string) string {
	if len(symbol) > 3 && symbol[len(symbol)-3:] == ".TW" {
		return symbol[:len(symbol)-3]
	}
	return symbol
}

func lookupSector(symbol string) string {
	s := stripTWSuffix(symbol)
	if v, ok := sectorMap[s]; ok {
		return v
	}
	return "unknown"
}

func loadSessions(stateDir string) []sessionData {
	glob := filepath.Join(stateDir, "sessions", "session-*")
	dirs, err := filepath.Glob(glob)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error globbing sessions: %v\n", err)
		return nil
	}

	var sessions []sessionData
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			continue
		}
		sd := sessionData{}

		summaryData, err := os.ReadFile(filepath.Join(d, "summary.json"))
		if err == nil {
			_ = json.Unmarshal(summaryData, &sd.Summary)
		}

		outcomesData, err := os.ReadFile(filepath.Join(d, "recommendation_outcomes.jsonl"))
		if err == nil {
			sd.Outcomes = parseOutcomesJSONL(outcomesData)
		}

		sessions = append(sessions, sd)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Summary.SessionID < sessions[j].Summary.SessionID
	})
	return sessions
}

func parseOutcomesJSONL(data []byte) []recommendation.RecommendationOutcome {
	var outcomes []recommendation.RecommendationOutcome
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i > start {
			var o recommendation.RecommendationOutcome
			if json.Unmarshal(data[start:i], &o) == nil {
				outcomes = append(outcomes, o)
			}
		}
		start = i + 1
	}
	if start < len(data) {
		var o recommendation.RecommendationOutcome
		if json.Unmarshal(data[start:], &o) == nil {
			outcomes = append(outcomes, o)
		}
	}
	return outcomes
}

func deduplicateBySymbol(outcomes []recommendation.RecommendationOutcome) []recommendation.RecommendationOutcome {
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].Conviction > outcomes[j].Conviction
	})
	seen := make(map[string]bool)
	var uniq []recommendation.RecommendationOutcome
	for _, o := range outcomes {
		if !seen[o.Symbol] {
			seen[o.Symbol] = true
			uniq = append(uniq, o)
		}
	}
	return uniq
}

func filterCandidates(outcomes []recommendation.RecommendationOutcome) []recommendation.RecommendationOutcome {
	var buys []recommendation.RecommendationOutcome
	for _, o := range outcomes {
		side := string(o.Side)
		if side == "buy" || side == "BUY" {
			buys = append(buys, o)
		}
	}
	if len(buys) == 0 {
		for _, o := range outcomes {
			if o.Symbol != "" {
				buys = append(buys, o)
			}
		}
	}
	return deduplicateBySymbol(buys)
}

func buildPortfolioState(session sessionData) risk.PortfolioState {
	total := session.Summary.PortfolioValue
	cash := session.Summary.EndingCash
	if total <= 0 {
		total = defaultPortfolio
		cash = total * defaultCashRatio
	}
	return risk.PortfolioState{
		TotalValue:     total,
		Cash:           cash,
		Var95:          total * estVarPct,
		SectorExposure: make(map[string]float64),
		Positions:      make(map[string]float64),
	}
}

func buildOrderIntent(o recommendation.RecommendationOutcome, pf risk.PortfolioState, totalConviction int) risk.OrderIntent {
	c := o.Conviction
	if c <= 0 {
		c = defaultConviction
	}
	ratio := float64(c) / float64(totalConviction)
	notional := pf.TotalValue * ratio * maxOrderPct

	side := string(o.Side)
	if side == "" {
		side = "buy"
	}

	qty := 1
	if o.Price > 0 {
		computed := int(notional / o.Price)
		if computed > 1 {
			qty = computed
		}
	}

	return risk.OrderIntent{
		Symbol:     o.Symbol,
		Side:       side,
		Quantity:   qty,
		Price:      o.Price,
		Notional:   notional,
		AgentID:    o.AgentID,
		Sector:     lookupSector(o.Symbol),
		Conviction: c,
	}
}

func applyOrder(pf risk.PortfolioState, order risk.OrderIntent) risk.PortfolioState {
	pf.Cash -= order.Notional
	pf.Positions[order.Symbol] += order.Notional
	pf.SectorExposure[order.Sector] += order.Notional
	return pf
}

func validateSessions(sessions []sessionData) []sessionResult {
	preTrade := risk.NewPreTradeGate()
	var results []sessionResult

	for _, session := range sessions {
		if len(session.Outcomes) == 0 {
			continue
		}

		pf := buildPortfolioState(session)
		result := sessionResult{
			SessionID: session.Summary.SessionID,
			RuleStats: make(map[string]*ruleStat),
		}

		candidates := filterCandidates(session.Outcomes)
		if len(candidates) == 0 {
			continue
		}

		totalConviction := 0
		for _, o := range candidates {
			totalConviction += o.Conviction
		}
		if totalConviction == 0 {
			totalConviction = len(candidates) * defaultConviction
		}

		for _, o := range candidates {
			order := buildOrderIntent(o, pf, totalConviction)
			result.TotalOrders++

			decision, err := preTrade.Check(context.TODO(), order, pf, "NORMAL")
			if err != nil {
				continue
			}

			isBlocked := decision.Verdict == risk.VerdictBlock || decision.Verdict == risk.VerdictHalt
			if isBlocked {
				result.BlockedOrders++
			} else {
				result.AllowedOrders++
				pf = applyOrder(pf, order)
			}

			for _, rule := range decision.Details {
				rs, ok := result.RuleStats[rule.RuleName]
				if !ok {
					rs = &ruleStat{}
					result.RuleStats[rule.RuleName] = rs
				}
				rs.Total++
				if rule.Passed {
					rs.Passed++
				} else {
					rs.Blocked++
				}
				rs.ValueSum += rule.CurrentValue
				rs.ThreshSum += rule.Threshold
			}
		}
		results = append(results, result)
	}
	return results
}

func printReport(results []sessionResult) {
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  Risk Gate — Pre-Trade Effectiveness Report")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println()

	totalOrders := 0
	totalBlocked := 0
	totalAllowed := 0
	aggRules := make(map[string]*ruleStat)

	for _, r := range results {
		totalOrders += r.TotalOrders
		totalBlocked += r.BlockedOrders
		totalAllowed += r.AllowedOrders
		for name, rs := range r.RuleStats {
			ars, ok := aggRules[name]
			if !ok {
				ars = &ruleStat{}
				aggRules[name] = ars
			}
			ars.Total += rs.Total
			ars.Blocked += rs.Blocked
			ars.Passed += rs.Passed
			ars.ValueSum += rs.ValueSum
			ars.ThreshSum += rs.ThreshSum
		}
	}

	if totalOrders == 0 {
		fmt.Println("  No orders validated across any session.")
		fmt.Println()
		fmt.Println("  TIP: Run a daily simulation first to generate session data:")
		fmt.Println("    go run ./cmd/atlas -api")
		fmt.Println()
		return
	}

	interceptRate := float64(totalBlocked) / float64(totalOrders) * 100
	fmt.Printf("  Sessions analyzed:        %d\n", len(results))
	fmt.Printf("  Total orders evaluated:   %d\n", totalOrders)
	fmt.Printf("  Orders BLOCKED:           %d (%.1f%%)\n", totalBlocked, interceptRate)
	fmt.Printf("  Orders ALLOWED:           %d (%.1f%%)\n", totalAllowed, float64(totalAllowed)/float64(totalOrders)*100)
	fmt.Println()

	fmt.Println("  ─── Rule Breakdown ───")
	fmt.Printf("  %-30s %8s %8s %8s %10s %10s\n", "Rule", "Total", "Passed", "Blocked", "Avg.Value", "Avg.Thresh")
	fmt.Println("  " + repeatChar("─", 80))
	for name, rs := range aggRules {
		avgV := rs.ValueSum / float64(rs.Total)
		avgT := rs.ThreshSum / float64(rs.Total)
		passRate := float64(rs.Passed) / float64(rs.Total) * 100
		fmt.Printf("  %-30s %8d %8d %8d %8.4f  %8.4f (%.0f%% pass)\n",
			name, rs.Total, rs.Passed, rs.Blocked, avgV, avgT, passRate)
	}
	fmt.Println()

	fmt.Println("  ─── Per-Session Summary (top 10) ───")
	fmt.Printf("  %-28s %8s %8s %8s %10s\n", "Session", "Orders", "Blocked", "Allowed", "BlockRate")
	fmt.Println("  " + repeatChar("─", 70))
	sort.Slice(results, func(i, j int) bool {
		return results[i].BlockedOrders > results[j].BlockedOrders
	})

	limit := min(10, len(results))
	for _, r := range results[:limit] {
		bRate := float64(r.BlockedOrders) / float64(max(1, r.TotalOrders)) * 100
		fmt.Printf("  %-28s %8d %8d %8d %8.1f%%\n",
			r.SessionID, r.TotalOrders, r.BlockedOrders, r.AllowedOrders, bRate)
	}
	fmt.Println()

	fmt.Println("  ─── Health Assessment ───")
	blockRate := float64(totalBlocked) / float64(totalOrders) * 100
	switch {
	case blockRate > 30:
		fmt.Println("  HIGH intercept rate (>30%) — thresholds may be too restrictive.")
		fmt.Println("  Consider reviewing MaxPositionPct, MaxSectorExposurePct, and MinCashBufferPct.")
	case blockRate < 1:
		fmt.Println("  LOW intercept rate (<1%) — thresholds may be too permissive.")
		fmt.Println("  Consider tightening risk limits to catch outlier orders.")
	default:
		fmt.Println("  Intercept rate in healthy range (1-30%).")
	}
	fmt.Println()
	fmt.Println("  TIP: To re-run with different parameters, edit configs/parameters.json")
	fmt.Println("       and restart the dashboard API server.")
	fmt.Println()
}

func repeatChar(c string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c[0]
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type postTradeSnap struct {
	SessionID    string
	PortfolioVal float64
	Cash         float64
	ReturnPct    float64
}

func validatePostTrade(sessions []sessionData) []sessionResult {
	postTrade := risk.NewPostTradeGate()

	var snaps []postTradeSnap
	for _, s := range sessions {
		if s.Summary.PortfolioValue > 0 {
			snaps = append(snaps, postTradeSnap{
				SessionID:    s.Summary.SessionID,
				PortfolioVal: s.Summary.PortfolioValue,
				Cash:         s.Summary.EndingCash,
			})
		}
	}

	if len(snaps) < 2 {
		return nil
	}

	for i := 1; i < len(snaps); i++ {
		prev := snaps[i-1].PortfolioVal
		if prev > 0 {
			snaps[i].ReturnPct = (snaps[i].PortfolioVal - prev) / prev
		}
	}

	var peak float64
	peak = snaps[0].PortfolioVal
	consecutiveLosses := 0
	var returns []float64
	var results []sessionResult

	for _, snap := range snaps {
		if snap.PortfolioVal > peak {
			peak = snap.PortfolioVal
		}
		drawdown := (peak - snap.PortfolioVal) / peak

		if snap.ReturnPct < 0 {
			consecutiveLosses++
		} else {
			consecutiveLosses = 0
		}

		returns = append(returns, snap.ReturnPct)
		rollingSharpe := computeRollingSharpe(returns, 20)

		input := risk.PostTradeInput{
			CurrentDrawdownPct: drawdown,
			RollingSharpe:      rollingSharpe,
			ConsecutiveLosses:  consecutiveLosses,
		}

		decision, err := postTrade.Evaluate(input, "NORMAL")
		if err != nil {
			continue
		}

		result := sessionResult{
			SessionID: snap.SessionID,
			RuleStats: make(map[string]*ruleStat),
		}
		if decision.Verdict == risk.VerdictHalt || decision.Verdict == risk.VerdictBlock {
			result.BlockedOrders = 1
		}
		result.TotalOrders = 1

		for _, rule := range decision.Details {
			rs, ok := result.RuleStats[rule.RuleName]
			if !ok {
				rs = &ruleStat{}
				result.RuleStats[rule.RuleName] = rs
			}
			rs.Total++
			if rule.Passed {
				rs.Passed++
			} else {
				rs.Blocked++
			}
			rs.ValueSum += rule.CurrentValue
			rs.ThreshSum += rule.Threshold
		}

		results = append(results, result)
	}

	return results
}

func computeRollingSharpe(returns []float64, window int) float64 {
	n := len(returns)
	if n < 2 {
		return 0
	}
	start := max(0, n-window)
	sample := returns[start:]
	m := len(sample)
	if m < 2 {
		return 0
	}
	mean := 0.0
	for _, r := range sample {
		mean += r
	}
	mean /= float64(m)

	variance := 0.0
	for _, r := range sample {
		d := r - mean
		variance += d * d
	}
	variance /= float64(m - 1)
	if variance <= 0 {
		return 0
	}
	stddev := math.Sqrt(variance)
	if stddev == 0 {
		return 0
	}
	return mean / stddev * math.Sqrt(252)
}

func printPostTradeReport(results []sessionResult, totalSessions int) {
	if len(results) == 0 {
		fmt.Println("  ─── Post-Trade Validation ───")
		fmt.Println("  Not enough session data for post-trade validation (need >= 2 sessions with PortfolioValue).")
		fmt.Println()
		return
	}

	totalAlerts := 0
	aggRules := make(map[string]*ruleStat)

	for _, r := range results {
		for name, rs := range r.RuleStats {
			ars, ok := aggRules[name]
			if !ok {
				ars = &ruleStat{}
				aggRules[name] = ars
			}
			ars.Total += rs.Total
			ars.Blocked += rs.Blocked
			ars.Passed += rs.Passed
			ars.ValueSum += rs.ValueSum
			ars.ThreshSum += rs.ThreshSum
		}
		if r.BlockedOrders > 0 || r.RuleStats["drawdown_halt"] != nil && r.RuleStats["drawdown_halt"].Blocked > 0 {
			totalAlerts++
		}
	}

	fmt.Println("  ─── Post-Trade Validation ───")
	fmt.Printf("  Sessions analyzed:        %d / %d\n", len(results), totalSessions)
	fmt.Printf("  Mode change alerts:       %d\n", totalAlerts)
	fmt.Println()

	fmt.Printf("  %-30s %8s %8s %8s %10s %10s\n", "Rule", "Total", "Passed", "Blocked", "Avg.Value", "Avg.Thresh")
	fmt.Println("  " + repeatChar("─", 80))
	for name, rs := range aggRules {
		avgV := rs.ValueSum / float64(rs.Total)
		avgT := rs.ThreshSum / float64(rs.Total)
		passRate := float64(rs.Passed) / float64(rs.Total) * 100
		fmt.Printf("  %-30s %8d %8d %8d %8.4f  %8.4f (%.0f%% pass)\n",
			name, rs.Total, rs.Passed, rs.Blocked, avgV, avgT, passRate)
	}
	fmt.Println()
}

func main() {
	help := flag.Bool("help", false, "show usage")
	stateDir := flag.String("state", "data/state", "path to state directory (with sessions/)")
	flag.Parse()

	if *help {
		fmt.Println("Usage: go run ./cmd/experimental/validate-risk-gate [--state <path>]")
		fmt.Println()
		fmt.Println("Validates the Risk Gate (PreTradeGate) against historical session data.")
		fmt.Println("Scans all session directories under data/state/sessions/, runs")
		fmt.Println("PreTradeCheck on each recommendation outcome, and reports effectiveness.")
		fmt.Println()
		fmt.Println("Flags:")
		fmt.Println("  --state   path to state directory (default: data/state)")
		os.Exit(0)
	}

	sessions := loadSessions(*stateDir)
	fmt.Fprintf(os.Stderr, "Loaded %d session directories\n", len(sessions))

	if len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "No sessions found under %s/sessions/. ", *stateDir)
		fmt.Fprintf(os.Stderr, "Run a simulation first: go run ./cmd/atlas -api\n")
		os.Exit(1)
	}

	if err := os.Chdir(findRepoRoot()); err != nil {
		fmt.Fprintf(os.Stderr, "error changing working directory: %v\n", err)
		os.Exit(1)
	}

	results := validateSessions(sessions)
	postTradeResults := validatePostTrade(sessions)
	printReport(results)
	printPostTradeReport(postTradeResults, len(sessions))
}

// findRepoRoot locates the project root by searching for go.mod upward.
func findRepoRoot() string {
	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
	}
	for _, arg := range os.Args[1:] {
		if arg == "--state" || arg == "-state" {
			continue
		}
		if info, err := os.Stat(arg); err == nil && info != nil {
			dir := arg
			for range 5 {
				if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
					return dir
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}
