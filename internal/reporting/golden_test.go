package reporting

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Golden files lock the performance-report output contract. Every future
// change to report semantics (headline decontamination, regime attribution,
// equity-curve exclusion, synthetic annotation, SSoT source markers) must
// update these files deliberately via `go test ./internal/reporting -run
// TestGolden -update`, and the diff is reviewed in the PR — this is the
// regression guard that stops the "fix one data problem, break another"
// cycle on the performance report.
var updateGolden = flag.Bool("update", false, "update golden report files")

// writeGoldenFixture builds a JSONL ledger directory containing the full
// spectrum of production summary shapes the report must handle:
//
//  1. session-20260710 — real+ synthetic mixed ledger (headline = real only)
//  2. session-20260714 — RISK_OFF outcomes with outcome-level regimes
//  3. session-20260716 — synthetic-only session (zero headline contribution)
//  4. session-20260721 — LEGACY 0-VALUE summary (excluded from equity curve)
//  5. session-20260722 — NULL-regime summary (PV>0, regime="")
//
// Fixtures are written straight to disk (bypassing write-time validation)
// because they represent pre-existing legacy/backfilled data on the read
// path — the same data that triggered the #1431 / BL-01 / B02b / R01 /
// BL-06 / A7 fix cycle.
func writeGoldenFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeGoldenSession(t, dir, "session-20260710-daily", 2_980_000, 100_000, 1_000, domain.RegimeRiskOn, []domain.RecommendationOutcome{
		// Real trades — headline stats must come from these only.
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", Symbol: "2330", Side: "buy", Window: "session-20260710-daily", ForwardReturn: 0.05, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", Symbol: "2330", Side: "buy", Window: "session-20260710-daily", ForwardReturn: 0.03, PassedGuards: true},
		{AgentID: "agent-b", Skill: "value", Layer: "style", Symbol: "2881", Side: "buy", Window: "session-20260710-daily", ForwardReturn: -0.01, PassedGuards: true},
		// Synthetic evaluation trades — excluded from headline, counted in mix.
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", Symbol: "2330", Side: "buy", Window: "session-20260710-daily", ForwardReturn: 0.20, PassedGuards: true, IsSynthetic: true},
		{AgentID: "agent-b", Skill: "value", Layer: "style", Symbol: "2881", Side: "buy", Window: "session-20260710-daily", ForwardReturn: -0.15, PassedGuards: true, IsSynthetic: true},
	})
	writeGoldenSession(t, dir, "session-20260714-daily", 3_050_000, 90_000, 2_000, domain.RegimeRiskOff, []domain.RecommendationOutcome{
		// Outcome-level regime "RISK_OFF" — regime attribution must use the
		// outcome's own regime (R01), not the window→summary lookup.
		{AgentID: "agent-c", Skill: "macro", Layer: "macro", Symbol: "0050", Side: "buy", Window: "session-20260714-daily", ForwardReturn: 0.02, PassedGuards: true, Regime: "RISK_OFF"},
		{AgentID: "agent-c", Skill: "macro", Layer: "macro", Symbol: "0050", Side: "buy", Window: "session-20260714-daily", ForwardReturn: 0.10, PassedGuards: true, IsSynthetic: true, Regime: "RISK_OFF"},
		{AgentID: "agent-c", Skill: "macro", Layer: "macro", Symbol: "0050", Side: "buy", Window: "session-20260714-daily", ForwardReturn: -0.08, PassedGuards: true, IsSynthetic: true, Regime: "RISK_OFF"},
	})
	writeGoldenSession(t, dir, "session-20260716-daily", 2_950_000, 80_000, 0, domain.RegimeNeutral, []domain.RecommendationOutcome{
		// Outcome regime deliberately mismatches the summary regime (NEUTRAL)
		// — proves outcome.Regime wins over the summary lookup.
		{AgentID: "agent-d", Skill: "event", Layer: "context", Symbol: "2603", Side: "buy", Window: "session-20260716-daily", ForwardReturn: 0.04, PassedGuards: true, IsSynthetic: true, Regime: "RISK_ON"},
	})
	writeGoldenSession(t, dir, "session-20260721-daily", 0, 0, 0, "", nil)
	writeGoldenSession(t, dir, "session-20260722-daily", 3_200_000, 70_000, 0, "", []domain.RecommendationOutcome{
		// NULL-regime summary with outcome-level regime — bucket by the
		// outcome's regime (RISK_OFF), not "unknown".
		{AgentID: "agent-e", Skill: "quality", Layer: "style", Symbol: "2317", Side: "buy", Window: "session-20260722-daily", ForwardReturn: -0.005, PassedGuards: true, Regime: "RISK_OFF"},
		{AgentID: "agent-e", Skill: "quality", Layer: "style", Symbol: "2317", Side: "buy", Window: "session-20260722-daily", ForwardReturn: 0.03, PassedGuards: true, IsSynthetic: true, Regime: "RISK_OFF"},
	})

	return dir
}

func writeGoldenSession(t *testing.T, baseDir, sessionID string, portfolioValue, endingCash, totalTaxPaid float64, regime domain.Regime, outcomes []domain.RecommendationOutcome) {
	t.Helper()
	sessDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sessionID, err)
	}
	summary := domain.SessionSummary{
		SessionID:      sessionID,
		Regime:         regime,
		EndingCash:     endingCash,
		PortfolioValue: portfolioValue,
		OutcomeCount:   len(outcomes),
		TotalTaxPaid:   totalTaxPaid,
		RecordedAt:     time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal %s: %v", sessionID, err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "summary.json"), data, 0o644); err != nil {
		t.Fatalf("write summary %s: %v", sessionID, err)
	}
	if len(outcomes) == 0 {
		return
	}
	f, err := os.Create(filepath.Join(sessDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create outcomes %s: %v", sessionID, err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, oc := range outcomes {
		if err := enc.Encode(oc); err != nil {
			t.Fatalf("encode outcome %s: %v", sessionID, err)
		}
	}
}

func TestGolden_PerformanceReportJSON(t *testing.T) {
	dir := writeGoldenFixture(t)
	report, err := GenerateReport(ledger.NewStore(dir), dir, "all")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	report.GeneratedAt = time.Time{} // normalize volatile timestamp

	got, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	got = append(got, '\n')

	golden := filepath.Join("testdata", "golden_performance_report.json")
	if *updateGolden {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", golden)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("performance report JSON drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestGolden_PerformanceReportMarkdown(t *testing.T) {
	dir := writeGoldenFixture(t)
	report, err := GenerateReport(ledger.NewStore(dir), dir, "all")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	report.GeneratedAt = time.Time{} // normalize volatile timestamp

	got := GenerateMarkdownReport(report)

	golden := filepath.Join("testdata", "golden_performance_report.md")
	if *updateGolden {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", golden)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("performance report markdown drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestGolden_ContractAssertions verifies the fixture exercises every guard
// the golden locks: headline real-only, outcome-level regime attribution,
// zero-value equity exclusion, synthetic share annotation.
func TestGolden_ContractAssertions(t *testing.T) {
	dir := writeGoldenFixture(t)
	report, err := GenerateReport(ledger.NewStore(dir), dir, "all")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Equity curve: legacy 0-value session (20260721) excluded.
	if report.StartingValue != 2_980_000 {
		t.Errorf("starting_value = %v, want 2,980,000 (0-value session must be excluded)", report.StartingValue)
	}
	if report.EndingValue != 3_200_000 {
		t.Errorf("ending_value = %v, want 3,200,000 (0-value session must be excluded)", report.EndingValue)
	}
	if report.TotalReturn <= 0 {
		t.Errorf("total_return = %v, want positive (0-value session would collapse it)", report.TotalReturn)
	}

	// Headline: real trades only (5 real of 11 total: 3+1 real, 2+2+1+1 synthetic).
	if report.TotalTrades != 11 {
		t.Errorf("total_trades = %d, want 11", report.TotalTrades)
	}
	if report.RealTradeCount != 5 {
		t.Errorf("real_trade_count = %d, want 5", report.RealTradeCount)
	}
	if report.SyntheticTradeCount != 6 {
		t.Errorf("synthetic_trade_count = %d, want 6", report.SyntheticTradeCount)
	}
	if report.WinRate != 0.6 {
		t.Errorf("win_rate = %v, want 0.6 (real only: 3 wins / 5)", report.WinRate)
	}
	if report.ProfitFactor != 0.10/0.015 {
		t.Errorf("profit_factor = %v, want %v (real only)", report.ProfitFactor, 0.10/0.015)
	}

	// Regime attribution: outcome.Regime wins (R01). The 20260716 summary is
	// NEUTRAL but its outcome says RISK_ON; the 20260722 summary is NULL but
	// its outcome says RISK_OFF. The NULL-regime summary itself lands in
	// "unknown" only for SessionCount.
	rb := report.RegimeBreakdown.Regimes
	if rb["RISK_ON"].AggregateForwardReturn != 0.07 {
		t.Errorf("RISK_ON aggregate = %v, want 0.07 (+0.05+0.03-0.01; synthetic excluded)", rb["RISK_ON"].AggregateForwardReturn)
	}
	if rb["RISK_OFF"].AggregateForwardReturn != 0.015 {
		t.Errorf("RISK_OFF aggregate = %v, want 0.015 (+0.02-0.005; synthetic excluded)", rb["RISK_OFF"].AggregateForwardReturn)
	}
	if rb["unknown"].SessionCount != 1 {
		t.Errorf("unknown session_count = %d, want 1 (NULL-regime summary)", rb["unknown"].SessionCount)
	}
	if rb["unknown"].AggregateForwardReturn != 0 {
		t.Errorf("unknown aggregate = %v, want 0 (outcome regime resolves, nothing lands in unknown)", rb["unknown"].AggregateForwardReturn)
	}
	if rb["NEUTRAL"].SessionCount != 1 {
		t.Errorf("NEUTRAL session_count = %d, want 1", rb["NEUTRAL"].SessionCount)
	}
	if rb["NEUTRAL"].AggregateForwardReturn != 0 {
		t.Errorf("NEUTRAL aggregate = %v, want 0 (synthetic-only session contributes no returns)", rb["NEUTRAL"].AggregateForwardReturn)
	}

	// Synthetic share annotation in markdown (6/11 = 54.5%).
	md := GenerateMarkdownReport(report)
	if !containsAll(md, "Synthetic Share", "54.5%", "real trades only") {
		t.Errorf("markdown missing synthetic-share annotation:\n%s", md)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
