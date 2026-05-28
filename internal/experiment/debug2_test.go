//go:build debug

package experiment

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func TestDebugConstraintScores(t *testing.T) {
	replayPath := filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv")
	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	fmt.Printf("Dates in dataset: %d\n", len(ds.Dates))
	for _, d := range ds.Dates {
		fmt.Printf("  %s\n", d.Format("2006-01-02"))
	}

	policy := baseline.DefaultPolicy()
	baselineConstraints := policy.Constraints

	candidateConstraints1 := baseline.ApplyConstraintCandidate(policy.Constraints, `
conviction_floor: 99
liquidity_floor: 999999999
reject_on_weak_close: true
`)

	window := domain.BacktestWindowSummary{
		StartDate: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}

	// Manually run scoreConstraintWindowWithObservations logic
	registry := orchestrator.SeedRegistry()
	symbols := orchestrator.RegistrySymbols(registry)

	for _, date := range ds.Dates {
		if date.Before(window.StartDate) || date.After(window.EndDate) {
			continue
		}
		nextDate, ok := ds.NextDate(date, 1)
		if !ok || nextDate.After(window.EndDate) {
			continue
		}

		quotes := ds.QuotesForDate(date, symbols)
		nextQuotes := ds.QuotesForDate(nextDate, symbols)

		fmt.Printf("\nDate: %s, Next: %s, Quotes: %d\n", date.Format("2006-01-02"), nextDate.Format("2006-01-02"), len(quotes))

		for _, c := range []struct {
			name string
			cons domain.SimulationConstraints
		}{
			{"baseline", baselineConstraints},
			{"candidate1", candidateConstraints1},
		} {
			execPolicy := baseline.ExecutionPolicyFromConstraints(c.cons)
			_, rawRecs, activeRecs := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, nil, execPolicy)
			filtered := filterRecommendationsForConstraints(activeRecs, c.cons)
			engine := sim.NewEngine(c.cons)
			result := engine.Run(domain.RegimeNeutral, quotes, filtered)
			score := scoreSimulationResult(result, nextQuotes, c.cons.StartingCash)

			fmt.Printf("  %s: raw=%d active=%d filtered=%d score=%f cash=%f positions=%d\n",
				c.name, len(rawRecs), len(activeRecs), len(filtered), score, result.EndingCash, len(result.Positions))
			for _, p := range result.Positions {
				fmt.Printf("    pos: %s qty=%d mv=%f\n", p.Symbol, p.Quantity, p.MarketValue)
			}
		}
	}
}
