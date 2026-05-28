//go:build debug

package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDebugScores(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}

	// Test 1: Constraint mutations
	brief1 := domain.MutationBrief{
		TargetAgentID: "cro-01",
		TargetSkill:   "cro_risk",
		TargetLayer:   domain.LayerControl,
		MutationType:  "risk_rule_change",
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "cro_risk.md"),
	}
	candidateDir := os.TempDir()
	candidatePath := filepath.Join(candidateDir, "v2.md")
	candidateArtifact := `# Risk Rule Change Proposal

## Candidate Rule Patch

risk_rule_change:
  conviction_floor: 99
  liquidity_floor: 999999999
  reject_on_weak_close: true
`
	os.WriteFile(candidatePath, []byte(candidateArtifact), 0o644)

	base1, cand1, err := comparePromptPerformance(replayPath, "", brief1, window, candidatePath)
	fmt.Printf("Test 1: baseline=%f candidate=%f err=%v\n", base1, cand1, err)

	// Test 2: Governance routing
	brief2 := domain.MutationBrief{
		TargetAgentID: "cio-01",
		TargetSkill:   "cio_portfolio",
		TargetLayer:   domain.LayerControl,
		MutationType:  "portfolio_constraint_revision",
		PromptFile:    filepath.Join("..", "..", "prompts", "agents", "cio_seed.md"),
	}
	candidatePath2 := filepath.Join(candidateDir, "v2_gov.md")
	candidateArtifact2 := `# Portfolio Constraint Revision Proposal

## Candidate Constraint Patch

portfolio_constraint_revision:
  max_position_weight: 0.15
  reserve_cash_fraction: 0.12
  require_cro_pass: true
`
	os.WriteFile(candidatePath2, []byte(candidateArtifact2), 0o644)

	base2, cand2, err := comparePromptPerformance(replayPath, "", brief2, window, candidatePath2)
	fmt.Printf("Test 2: baseline=%f candidate=%f err=%v\n", base2, cand2, err)
}
