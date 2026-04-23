package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func TestDebugWithCustomPolicy(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	ds, _ := replay.LoadTWSEOpenDataCSV(replayPath)

	candidateDir := t.TempDir()
	baselinePolicyPath := filepath.Join(candidateDir, "baseline_policy.json")
	baselinePolicy := `{"version":1,"constraints":{"starting_cash":10000000,"max_position_weight":0.25,"max_open_positions":10,"min_tradable_volume":1000,"min_recommendation_conviction":0,"require_cro_pass":false,"transaction_cost_bps":1,"slippage_bps":5,"reserve_cash_fraction":0.1},"execution_policy":{"conviction_floor":0,"require_cro_pass":false,"momentum_crash_protection":false}}`
	os.WriteFile(baselinePolicyPath, []byte(baselinePolicy), 0o644)

	policy, _ := baseline.Load(baselinePolicyPath)
	fmt.Printf("Loaded policy: RequireCROPass=%t ConvictionFloor=%d\n", policy.Constraints.RequireCROPass, policy.Constraints.MinRecommendationConviction)
	fmt.Printf("Exec policy: RequireCROPass=%t ConvictionFloor=%d\n", policy.ExecutionPolicy.RequireCROPass, policy.ExecutionPolicy.ConvictionFloor)

	constraints := policy.Constraints
	execPolicy := baseline.ExecutionPolicyFromConstraints(constraints)
	fmt.Printf("Exec policy from constraints: RequireCROPass=%t ConvictionFloor=%d\n", execPolicy.RequireCROPass, execPolicy.ConvictionFloor)

	registry := orchestrator.SeedRegistry()
	symbols := orchestrator.RegistrySymbols(registry)

	date := ds.Dates[0]
	quotes := ds.QuotesForDate(date, symbols)

	regime, rawRecs, finalRecs := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, nil, execPolicy)

	fmt.Printf("Regime: %s\n", regime)
	fmt.Printf("Raw: %d\n", len(rawRecs))
	fmt.Printf("Final: %d\n", len(finalRecs))

	filtered := filterRecommendationsForConstraints(finalRecs, constraints)
	fmt.Printf("Filtered: %d\n", len(filtered))

	engine := sim.NewEngine(constraints)
	result := engine.Run(regime, quotes, filtered)
	fmt.Printf("Result: cash=%f positions=%d\n", result.EndingCash, len(result.Positions))
	for _, p := range result.Positions {
		fmt.Printf("  pos: %s qty=%d mv=%f\n", p.Symbol, p.Quantity, p.MarketValue)
	}
}
