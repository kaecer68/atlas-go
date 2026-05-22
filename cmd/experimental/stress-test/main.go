package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/stress"
)

func main() {
	help := flag.Bool("help", false, "show usage")
	scenarioID := flag.String("scenario", "", "run a specific scenario by ID (empty = all)")
	flag.Parse()

	if *help {
		fmt.Println("Usage: go run ./cmd/experimental/stress-test [--scenario <id>]")
		fmt.Println()
		fmt.Println("Runs stress tests against built-in historical scenarios.")
		fmt.Println()
		fmt.Println("Available scenarios:")
		for _, s := range stress.AllScenarios() {
			fmt.Printf("  %-30s %s\n", s.ID, s.Name)
		}
		fmt.Println()
		fmt.Println("Flags:")
		fmt.Println("  --scenario  run specific scenario (default: all)")
		os.Exit(0)
	}

	registry := domain.AgentRegistry{}
	policy := domain.ExecutionPolicy{}
	runner := stress.NewRunner(registry, policy)

	dummyQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800.0, Open: 795.0, IsTradable: true},
		{Symbol: "0050.TW", Last: 150.0, Open: 149.0, IsTradable: true},
		{Symbol: "2303.TW", Last: 45.0, Open: 44.5, IsTradable: true},
		{Symbol: "2317.TW", Last: 105.0, Open: 104.0, IsTradable: true},
		{Symbol: "2881.TW", Last: 42.0, Open: 41.5, IsTradable: true},
	}
	dummyRecs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: "buy", Conviction: 75},
		{Symbol: "0050.TW", Side: "buy", Conviction: 60},
		{Symbol: "2303.TW", Side: "buy", Conviction: 55},
		{Symbol: "2317.TW", Side: "buy", Conviction: 50},
		{Symbol: "2881.TW", Side: "buy", Conviction: 68},
	}

	if *scenarioID != "" {
		sc, err := stress.GetScenarioByID(*scenarioID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		result := runner.RunScenario(sc, dummyQuotes, dummyRecs)
		fmt.Println(formatSingleResult(result))
	} else {
		report := runner.RunAll(dummyQuotes, dummyRecs)
		fmt.Println(stress.FormatReport(report))
	}
}

func formatSingleResult(r stress.ScenarioResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Scenario: %s (%s)\n", r.ScenarioName, r.ScenarioID))
	b.WriteString(fmt.Sprintf("  Return:     %.2f%%\n", r.TotalReturn*100))
	b.WriteString(fmt.Sprintf("  Drawdown:   %.2f%%\n", r.MaxDrawdown*100))
	b.WriteString(fmt.Sprintf("  VaR95:      %.2f%%\n", r.VaR95*100))
	b.WriteString(fmt.Sprintf("  Trades:     %d\n", r.TradeCount))
	b.WriteString(fmt.Sprintf("  Regime:     %s\n", r.FinalRegime))
	if r.MomentumDisabled {
		b.WriteString("  Momentum:   DISABLED\n")
	}
	return b.String()
}
