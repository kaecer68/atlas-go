// conditions_cli.go — PR 2a CLI helpers for the configurable condition
// engine: resolving the -conditions flag against the registry and rendering
// the -list-conditions output (including the fundamentals live_observe_only
// placeholder, P0-1).
package main

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// selectConditions resolves the -conditions flag (comma-separated IDs) to
// registered conditions. An empty/blank list resolves to the full default
// registry set (the PR 1c demo conditions). Unknown IDs are rejected.
func selectConditions(list string, reg *stockpicker.ConditionRegistry) ([]stockpicker.Condition, error) {
	if strings.TrimSpace(list) == "" {
		return reg.All(), nil
	}
	var out []stockpicker.Condition
	for _, id := range strings.Split(list, ",") {
		id = strings.TrimSpace(id)
		c, ok := reg.Lookup(id)
		if !ok {
			return nil, fmt.Errorf("unknown condition %q (use -list-conditions)", id)
		}
		out = append(out, *c)
	}
	return out, nil
}

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

// conditionIDs extracts the ID of each condition in order.
func conditionIDs(conds []stockpicker.Condition) []string {
	ids := make([]string, len(conds))
	for i, c := range conds {
		ids[i] = c.ID
	}
	return ids
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
