package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
)

func main() {
	var format string
	flag.StringVar(&format, "format", "json", "Output format: json|markdown")
	flag.Parse()

	engine := janus.NewEngine()
	engine.EnsureAllRegimes()

	// Seed mock historical performance to produce meaningful weights.
	now := time.Now()
	seedCohort(engine, prism.RegimeRiskOn, 1.2, 0.62, 0.15, now)
	seedCohort(engine, prism.RegimeRiskOff, 0.4, 0.51, -0.05, now)
	seedCohort(engine, prism.RegimeHighVolatility, 0.3, 0.48, -0.08, now)
	seedCohort(engine, prism.RegimeLowVolatility, 0.9, 0.55, 0.10, now)
	seedCohort(engine, prism.RegimeTransition, 0.5, 0.50, 0.02, now)

	engine.Update()

	status := engine.GetStatus()

	switch format {
	case "markdown":
		printMarkdown(status)
	case "json":
		printJSON(status)
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s\n", format)
		os.Exit(1)
	}
}

func seedCohort(engine *janus.Engine, regime prism.RegimeType, sharpe, hitRate, totalReturn float64, baseTime time.Time) {
	// Seed 20 days of identical performance for stable window aggregates.
	for day := 0; day < 20; day++ {
		engine.RecordSnapshot(janus.CohortSnapshot{
			Regime:      regime,
			SharpeRatio: sharpe,
			HitRate:     hitRate,
			TotalReturn: totalReturn,
			Signals:     50,
			RecordedAt:  baseTime.Add(-time.Duration(20-day) * 24 * time.Hour),
		})
	}
}

func printJSON(status janus.Status) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(status)
}

func printMarkdown(status janus.Status) {
	fmt.Println("# JANUS Meta-Layer Status")
	fmt.Println()
	fmt.Printf("**Classification:** `%s`  \n", status.Classification)
	fmt.Printf("**Last Updated:** %s  \n", status.LastUpdated.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("## Cohort Weights")
	fmt.Println()
	fmt.Println("| Regime | Weight |")
	fmt.Println("|--------|--------|")
	for regime, weight := range status.Weights {
		fmt.Printf("| %s | %.4f |\n", regime, weight)
	}
	fmt.Println()
	if len(status.WindowPerformances) > 0 {
		fmt.Println("## Windowed Sharpe Ratios")
		fmt.Println()
		fmt.Println("| Regime | Short | Medium | Long |")
		fmt.Println("|--------|-------|--------|------|")
		for regime, perf := range status.WindowPerformances {
			fmt.Printf("| %s | %.3f | %.3f | %.3f |\n", regime, perf.ShortSharpe, perf.MedSharpe, perf.LongSharpe)
		}
		fmt.Println()
	}
	fmt.Println("---")
	fmt.Println("*JANUS detects emergent regimes by comparing short-window vs long-window cohort accuracy.*")
}
