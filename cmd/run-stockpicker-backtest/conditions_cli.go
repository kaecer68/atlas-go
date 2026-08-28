// conditions_cli.go — PR 2a CLI helpers for the configurable condition
// engine: rendering the -list-conditions output (including the fundamentals
// live_observe_only placeholder, P0-1) and the per-condition coverage print.
// Condition-ID resolution against the registry lives in
// internal/stockpicker.selectConditions (shared with the scheduler).
package main

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// listConditionsText renders the available conditions for -list-conditions.
// The default registry conditions are listed first, then the fundamentals
// placeholder marked live_observe_only (never backtested, P0-1).
func listConditionsText(reg *stockpicker.ConditionRegistry) string {
	var b strings.Builder
	for _, c := range reg.All() {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", c.ID, c.Name, c.Type)
	}
	ph := stockpicker.NewFundamentalPlaceholder()
	fmt.Fprintf(&b, "%s\t%s\t%s (live_observe_only)\n", ph.ID, ph.Name, ph.Type)
	return b.String()
}

// printCoverage prints the per-condition sample counts (PR verification
// input; kept here so main.go stays within the per-file line budget).
func printCoverage(rep stockpicker.CoverageReport) {
	fmt.Printf("coverage: asof=%s universe=%d triggers=%s..%s total_outcomes=%d\n",
		rep.AsOf, rep.UniverseSize, rep.Start, rep.End, rep.TotalOutcomes)
	for src, n := range rep.BySource {
		fmt.Printf("  %-40s %d\n", src, n)
	}
}
