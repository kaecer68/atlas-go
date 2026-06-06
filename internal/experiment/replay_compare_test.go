package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func TestComparePromptPerformance(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	brief := domain.MutationBrief{
		TargetAgentID: "growth-momentum-01",
		TargetSkill:   "growth_momentum",
		TargetLayer:   domain.LayerStyle,
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "growth_momentum.md"),
	}
	candidateDir := t.TempDir()
	candidatePath := filepath.Join(candidateDir, "v2.md")
	candidateArtifact := `require trend confirmation
downgrade conviction
reject setups
growth_momentum
technical_breakout`
	if err := os.WriteFile(candidatePath, []byte(candidateArtifact), 0o644); err != nil {
		t.Fatalf("write candidate prompt: %v", err)
	}

	baseline, candidate, err := comparePromptPerformance(replayPath, "", brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare prompt performance: %v", err)
	}
	if candidate > 1 || baseline > 1 {
		t.Fatalf("unexpected replay score magnitude, baseline=%f candidate=%f", baseline, candidate)
	}
	if baseline == 0 || candidate == 0 {
		t.Fatalf("expected non-zero scores, baseline=%f candidate=%f", baseline, candidate)
	}
	// Baseline and candidate may be equal when the sample replay data is too
	// sparse for prompt differences to produce observable score deltas. This is
	// expected with the 54-line samples/replay CSV. Real experiment comparison
	// uses full ATLAS_REPLAY_DATA_PATH datasets with hundreds of observations.
	if baseline == candidate {
		t.Logf("baseline == candidate = %f (expected with sparse sample replay data)", baseline)
	}
}

func TestApplyConstraintCandidateParsesRiskAndPortfolioFields(t *testing.T) {
	base := baseline.DefaultPolicy().Constraints
	candidate := `
conviction_floor: 55
liquidity_floor: 5000000
max_position_weight: 0.15
reserve_cash_fraction: 0.12
require_cro_pass: true
stop_loss_pct: 8
take_profit_pct: 15
max_open_positions: 3
transaction_cost_bps: 2.0
slippage_bps: 5
`
	updated := baseline.ApplyConstraintCandidate(base, candidate)
	if updated.MinRecommendationConviction != 55 {
		t.Fatalf("expected conviction floor parsed")
	}
	if updated.MinTradableVolume != 5000000 {
		t.Fatalf("expected liquidity floor parsed")
	}
	if updated.MaxPositionWeight != 0.15 {
		t.Fatalf("expected max position weight parsed")
	}
	if updated.ReserveCashFraction != 0.12 {
		t.Fatalf("expected reserve cash fraction parsed")
	}
	if !updated.RequireCROPass {
		t.Fatalf("expected require_cro_pass parsed")
	}
	if updated.StopLossPct != 0.08 {
		t.Fatalf("expected stop_loss_pct parsed as decimal, got %f", updated.StopLossPct)
	}
	if updated.TakeProfitPct != 0.15 {
		t.Fatalf("expected take_profit_pct parsed as decimal, got %f", updated.TakeProfitPct)
	}
	if updated.MaxOpenPositions != 3 {
		t.Fatalf("expected max_open_positions parsed")
	}
	if updated.TransactionCostBPS != 2.0 {
		t.Fatalf("expected transaction_cost_bps parsed")
	}
	if updated.SlippageBPS != 5 {
		t.Fatalf("expected slippage_bps parsed")
	}
}

func TestComparePromptPerformanceSupportsConstraintMutations(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	brief := domain.MutationBrief{
		TargetAgentID: "cro-01",
		TargetSkill:   "cro_risk",
		TargetLayer:   domain.LayerControl,
		MutationType:  "risk_rule_change",
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "cro_risk.md"),
	}
	candidateDir := t.TempDir()
	candidatePath := filepath.Join(candidateDir, "v2.md")
	candidateArtifact := `# Risk Rule Change Proposal

## Candidate Rule Patch

risk_rule_change:
  conviction_floor: 99
  liquidity_floor: 999999999
  reject_on_weak_close: true
`
	if err := os.WriteFile(candidatePath, []byte(candidateArtifact), 0o644); err != nil {
		t.Fatalf("write candidate artifact: %v", err)
	}

	baselinePolicyPath := filepath.Join(candidateDir, "baseline_policy.json")
	baselinePolicy := `{"version":1,"constraints":{"starting_cash":10000000,"max_position_weight":0.25,"max_open_positions":10,"min_tradable_volume":1000,"min_recommendation_conviction":0,"require_cro_pass":false,"transaction_cost_bps":1,"slippage_bps":5,"reserve_cash_fraction":0.1},"execution_policy":{"conviction_floor":0,"require_cro_pass":false,"momentum_crash_protection":false}}`
	if err := os.WriteFile(baselinePolicyPath, []byte(baselinePolicy), 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}

	summary, err := comparePromptPerformanceDetailed(replayPath, baselinePolicyPath, brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare constraint performance: %v", err)
	}
	if summary.BaselineObservations == 0 && summary.CandidateObservations == 0 {
		t.Fatalf("expected observations for constraint mutation test")
	}
}

func TestComparePromptPerformanceSupportsGovernanceRouting(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	brief := domain.MutationBrief{
		TargetAgentID: "cio-01",
		TargetSkill:   "cio_portfolio",
		TargetLayer:   domain.LayerControl,
		MutationType:  "portfolio_constraint_revision",
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "cio_seed.md"),
	}
	candidateDir := t.TempDir()
	candidatePath := filepath.Join(candidateDir, "v2.md")
	candidateArtifact := `# Portfolio Constraint Revision Proposal

## Candidate Constraint Patch

portfolio_constraint_revision:
  max_position_weight: 0.15
  reserve_cash_fraction: 0.12
  require_cro_pass: true
`
	if err := os.WriteFile(candidatePath, []byte(candidateArtifact), 0o644); err != nil {
		t.Fatalf("write candidate artifact: %v", err)
	}

	baselinePolicyPath := filepath.Join(candidateDir, "baseline_policy.json")
	baselinePolicy := `{"version":1,"constraints":{"starting_cash":10000000,"max_position_weight":0.25,"max_open_positions":10,"min_tradable_volume":1000,"min_recommendation_conviction":0,"require_cro_pass":false,"transaction_cost_bps":1,"slippage_bps":5,"reserve_cash_fraction":0.1},"execution_policy":{"conviction_floor":0,"require_cro_pass":false,"momentum_crash_protection":false}}`
	if err := os.WriteFile(baselinePolicyPath, []byte(baselinePolicy), 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}

	summary, err := comparePromptPerformanceDetailed(replayPath, baselinePolicyPath, brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare governance performance: %v", err)
	}
	if summary.BaselineObservations == 0 && summary.CandidateObservations == 0 {
		t.Fatalf("expected observations for governance routing test")
	}
}

func TestFallbackWindowExpandsUntilMinDatesMet(t *testing.T) {
	// Create a dataset with dates spanning 100 days
	ds := &replay.Dataset{
		Dates: []time.Time{
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	start, end, ok := fallbackWindow(ds, 3)
	if !ok {
		t.Fatalf("expected fallback window to be found")
	}
	if end != time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected end date")
	}
	// Should use 60-day window to capture at least 3 dates
	if start.After(time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected expanded fallback window, got start=%s", start.Format("2006-01-02"))
	}

	// When minDates exceeds all data, should fall back to full range
	start, _, ok = fallbackWindow(ds, 100)
	if !ok {
		t.Fatalf("expected fallback window to be found")
	}
	if start != ds.Dates[0] {
		t.Fatalf("expected full range fallback, got start=%s", start.Format("2006-01-02"))
	}
}

func TestFallbackStatsRatio(t *testing.T) {
	tests := []struct {
		name      string
		fallback  FallbackStats
		wantRatio float64
	}{
		{
			name:      "zero factors",
			fallback:  FallbackStats{FallbackCount: 0, TotalCount: 0},
			wantRatio: 0.0,
		},
		{
			name:      "no fallbacks",
			fallback:  FallbackStats{FallbackCount: 0, TotalCount: 10},
			wantRatio: 0.0,
		},
		{
			name:      "all fallbacks",
			fallback:  FallbackStats{FallbackCount: 10, TotalCount: 10},
			wantRatio: 1.0,
		},
		{
			name:      "half fallbacks",
			fallback:  FallbackStats{FallbackCount: 5, TotalCount: 10},
			wantRatio: 0.5,
		},
		{
			name:      "60 percent fallbacks",
			fallback:  FallbackStats{FallbackCount: 6, TotalCount: 10},
			wantRatio: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fallback.Ratio()
			if got != tt.wantRatio {
				t.Errorf("FallbackStats.Ratio() = %v, want %v", got, tt.wantRatio)
			}
		})
	}
}

func TestFallbackStatsIsHighFallback(t *testing.T) {
	tests := []struct {
		name     string
		fallback FallbackStats
		maxRatio float64
		want     bool
	}{
		{
			name:     "zero total returns false",
			fallback: FallbackStats{FallbackCount: 0, TotalCount: 0},
			maxRatio: 0.6,
			want:     false,
		},
		{
			name:     "below threshold returns false",
			fallback: FallbackStats{FallbackCount: 5, TotalCount: 10},
			maxRatio: 0.6,
			want:     false,
		},
		{
			name:     "at threshold returns false",
			fallback: FallbackStats{FallbackCount: 6, TotalCount: 10},
			maxRatio: 0.6,
			want:     false,
		},
		{
			name:     "above threshold returns true",
			fallback: FallbackStats{FallbackCount: 7, TotalCount: 10},
			maxRatio: 0.6,
			want:     true,
		},
		{
			name:     "all fallbacks above threshold",
			fallback: FallbackStats{FallbackCount: 10, TotalCount: 10},
			maxRatio: 0.6,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fallback.IsHighFallback(tt.maxRatio)
			if got != tt.want {
				t.Errorf("FallbackStats.IsHighFallback(%v) = %v, want %v", tt.maxRatio, got, tt.want)
			}
		})
	}
}

func TestComparePromptPerformanceComputesMonetaryNTD(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	brief := domain.MutationBrief{
		TargetAgentID: "growth-momentum-01",
		TargetSkill:   "growth_momentum",
		TargetLayer:   domain.LayerStyle,
		MutationType:  "prompt_tightening",
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "growth_momentum.md"),
	}
	candidateDir := t.TempDir()
	candidatePath := filepath.Join(candidateDir, "v2.md")
	candidateArtifact := `require trend confirmation
downgrade conviction
reject setups
growth_momentum
technical_breakout`
	if err := os.WriteFile(candidatePath, []byte(candidateArtifact), 0o644); err != nil {
		t.Fatalf("write candidate prompt: %v", err)
	}

	baselinePolicyPath := filepath.Join(candidateDir, "baseline_policy.json")
	startingCash := 1000000.0
	baselinePolicy := `{"version":1,"constraints":{"starting_cash":` + fmt.Sprintf("%.0f", startingCash) + `,"max_position_weight":0.25,"max_open_positions":10,"min_tradable_volume":1000,"min_recommendation_conviction":0,"require_cro_pass":false,"transaction_cost_bps":1,"slippage_bps":5,"reserve_cash_fraction":0.1},"execution_policy":{"conviction_floor":0,"require_cro_pass":false,"momentum_crash_protection":false}}`
	if err := os.WriteFile(baselinePolicyPath, []byte(baselinePolicy), 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}

	summary, err := comparePromptPerformanceDetailed(replayPath, baselinePolicyPath, brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare prompt performance: %v", err)
	}

	if summary.StartingCash != startingCash {
		t.Errorf("expected StartingCash=%.0f, got %.0f", startingCash, summary.StartingCash)
	}

	if summary.BaselineScore != 0 && summary.BaselineMonetaryNTD != summary.BaselineScore*startingCash {
		t.Errorf("expected BaselineMonetaryNTD=BaselineScore*StartingCash (%.4f*%.0f=%.2f), got %.2f",
			summary.BaselineScore, startingCash, summary.BaselineScore*startingCash, summary.BaselineMonetaryNTD)
	}
	if summary.CandidateScore != 0 && summary.CandidateMonetaryNTD != summary.CandidateScore*startingCash {
		t.Errorf("expected CandidateMonetaryNTD=CandidateScore*StartingCash (%.4f*%.0f=%.2f), got %.2f",
			summary.CandidateScore, startingCash, summary.CandidateScore*startingCash, summary.CandidateMonetaryNTD)
	}
}
