// Package main provides a thin CLI wrapper around the cmd/stress-test/internal/risktest
// stress-test scenario package.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/cmd/stress-test/internal/risktest"
)

func main() {
	var configFile, outputFile, scenario string
	flag.StringVar(&configFile, "config", "", "Path to JSON stress test config file (optional)")
	flag.StringVar(&outputFile, "output", "", "Path to write JSON output (default: stdout)")
	flag.StringVar(&scenario, "scenario", "all", "Scenario name: market_crash, sector_rotation, liquidity_crisis, all")
	flag.Parse()

	cfg := risktest.LoadConfig(configFile)
	var results []risktest.Result
	if scenario == "all" {
		results = risktest.RunAll(cfg)
	} else if r, ok := risktest.RunScenario(scenario, cfg); ok {
		results = []risktest.Result{r}
	} else {
		fmt.Fprintf(os.Stderr, "unknown scenario: %s (valid: market_crash, sector_rotation, liquidity_crisis, all)\n", scenario)
		os.Exit(1)
	}

	all := risktest.SummarizeResults(results)
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	if outputFile != "" {
		if err := os.WriteFile(outputFile, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to %s\n", outputFile)
	} else {
		fmt.Println(string(out))
	}
}
