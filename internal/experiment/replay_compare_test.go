package experiment

import (
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
		StartDate: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
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
	if baseline == candidate {
		t.Fatalf("expected candidate to differ from baseline, got baseline=%f candidate=%f", baseline, candidate)
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
		StartDate: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
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

	baseline, candidate, err := comparePromptPerformance(replayPath, "", brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare constraint performance: %v", err)
	}
	if baseline == candidate {
		t.Fatalf("expected constraint mutation to produce different replay score")
	}
}

func TestComparePromptPerformanceSupportsGovernanceRouting(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
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

	baseline, candidate, err := comparePromptPerformance(replayPath, "", brief, window, candidatePath)
	if err != nil {
		t.Fatalf("compare governance performance: %v", err)
	}
	if baseline == candidate {
		t.Fatalf("expected governance routing to produce different replay score")
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
