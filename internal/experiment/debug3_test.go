//go:build debug

package experiment

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func TestDebugWhyActiveZero(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	ds, _ := replay.LoadTWSEOpenDataCSV(replayPath)

	policy := baseline.DefaultPolicy()
	constraints := policy.Constraints
	execPolicy := baseline.ExecutionPolicyFromConstraints(constraints)

	registry := orchestrator.SeedRegistry()
	symbols := orchestrator.RegistrySymbols(registry)

	date := ds.Dates[0]
	quotes := ds.QuotesForDate(date, symbols)

	regime, rawRecs, finalRecs := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, nil, execPolicy)

	fmt.Printf("Regime: %s\n", regime)
	fmt.Printf("Raw: %d\n", len(rawRecs))
	for _, r := range rawRecs {
		fmt.Printf("  %s %s conv=%d skill=%s agent=%s\n", r.Symbol, r.Side, r.Conviction, r.Skill, r.Agent)
	}
	fmt.Printf("Final: %d\n", len(finalRecs))
	for _, r := range finalRecs {
		fmt.Printf("  %s %s conv=%d skill=%s agent=%s\n", r.Symbol, r.Side, r.Conviction, r.Skill, r.Agent)
	}
}
