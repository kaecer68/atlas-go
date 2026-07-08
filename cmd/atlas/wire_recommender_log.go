package main

import "github.com/kaecer68/atlas-go/internal/recommender"

// anyDepsWired returns a human-readable summary of which producer adapters
// were successfully attached to the recommender. Used in the boot log to
// make production wiring visible at startup.
func anyDepsWired(d recommender.HandlerDeps) string {
	switch {
	case d.Narrative != nil && d.StrategyComp != nil:
		return "narrative+strategy"
	case d.Narrative != nil:
		return "narrative"
	case d.StrategyComp != nil:
		return "strategy"
	case d.CapitalFlow != nil:
		return "capitalflow-only"
	default:
		return "fallback-only (no real services)"
	}
}
