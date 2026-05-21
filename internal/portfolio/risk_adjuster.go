package portfolio

import (
	"fmt"
	"maps"

	"github.com/kaecer68/atlas-go/internal/risk"
)

// PortfolioRiskAdjuster adjusts portfolio weights based on drawdown severity.
// It integrates with the MacroAwareDrawdownEngine to apply dynamic position
// capping, cyclical industry reduction, and defensive industry boosts.
type PortfolioRiskAdjuster struct {
	drawdownEngine   *risk.MacroAwareDrawdownEngine
	cyclicalSymbols  map[string]bool
	defensiveSymbols map[string]bool
}

// NewPortfolioRiskAdjuster creates a new adjuster backed by a drawdown engine.
// Industry classification sets are empty by default; use SetCyclicalSymbols
// and SetDefensiveSymbols to enable industry-specific adjustments.
func NewPortfolioRiskAdjuster(engine *risk.MacroAwareDrawdownEngine) *PortfolioRiskAdjuster {
	return &PortfolioRiskAdjuster{
		drawdownEngine:   engine,
		cyclicalSymbols:  make(map[string]bool),
		defensiveSymbols: make(map[string]bool),
	}
}

// SetCyclicalSymbols configures which symbols belong to cyclical industries.
// These symbols will have their weights reduced during moderate (20%) and
// severe (50%) drawdown scenarios.
func (a *PortfolioRiskAdjuster) SetCyclicalSymbols(symbols []string) {
	a.cyclicalSymbols = make(map[string]bool, len(symbols))
	for _, s := range symbols {
		a.cyclicalSymbols[s] = true
	}
}

// SetDefensiveSymbols configures which symbols belong to defensive industries.
// These symbols will have their weights increased during severe drawdown
// scenarios (20% boost).
func (a *PortfolioRiskAdjuster) SetDefensiveSymbols(symbols []string) {
	a.defensiveSymbols = make(map[string]bool, len(symbols))
	for _, s := range symbols {
		a.defensiveSymbols[s] = true
	}
}

// AdjustWeights adjusts portfolio weights based on the current drawdown severity.
// It returns a new map (the original weights map is never mutated) and a list
// of reasons documenting each adjustment for audit trail purposes.
//
// Adjustment rules by severity:
//
//	none:      no adjustments
//	light:     cap single positions above 15% to 15%
//	moderate:  cap single positions above 10% to 10%, reduce cyclical by 20%
//	severe:    cap single positions above 5% to 5%, reduce cyclical by 50%, boost defensive by 20%
//	emergency: liquidate all positions (set all weights to 0)
func (a *PortfolioRiskAdjuster) AdjustWeights(
	weights map[string]float64,
	severity risk.DrawdownAction,
) (adjusted map[string]float64, reasons []string) {
	adjusted = make(map[string]float64, len(weights))
	maps.Copy(adjusted, weights)

	switch severity {
	case risk.DrawdownNone:
		reasons = append(reasons, "severity=none: no adjustments applied")
		return adjusted, reasons

	case risk.DrawdownLight:
		return a.adjustLight(adjusted, reasons)

	case risk.DrawdownModerate:
		return a.adjustModerate(adjusted, reasons)

	case risk.DrawdownSevere:
		return a.adjustSevere(adjusted, reasons)

	case risk.DrawdownEmergency:
		return a.adjustEmergency(adjusted, reasons)

	default:
		reasons = append(reasons, fmt.Sprintf("severity=unknown(%d): no adjustments applied", severity))
		return adjusted, reasons
	}
}

// adjustLight caps individual position weights at 15%.
func (a *PortfolioRiskAdjuster) adjustLight(
	adjusted map[string]float64, reasons []string,
) (map[string]float64, []string) {
	const cap = 0.15
	anyCapped := false
	for symbol, w := range adjusted {
		if w > cap {
			adjusted[symbol] = cap
			reasons = append(reasons,
				fmt.Sprintf("light: capped %s from %.4f to %.4f (15%% limit)", symbol, w, cap))
			anyCapped = true
		}
	}
	if anyCapped {
		reasons = append(reasons, "light: single-position exposure limited to 15%")
	} else {
		reasons = append(reasons, "light: no positions exceeded 15% cap")
	}
	return adjusted, reasons
}

// adjustModerate caps single positions at 10% and reduces cyclical industry weights by 20%.
func (a *PortfolioRiskAdjuster) adjustModerate(
	adjusted map[string]float64, reasons []string,
) (map[string]float64, []string) {
	const cap = 0.10
	const cyclicalReduction = 0.80

	// Cap single positions at 10%
	anyCapped := false
	for symbol, w := range adjusted {
		if w > cap {
			adjusted[symbol] = cap
			reasons = append(reasons,
				fmt.Sprintf("moderate: capped %s from %.4f to %.4f (10%% limit)", symbol, w, cap))
			anyCapped = true
		}
	}
	if anyCapped {
		reasons = append(reasons, "moderate: single-position exposure limited to 10%")
	} else {
		reasons = append(reasons, "moderate: no positions exceeded 10% cap")
	}

	// Reduce cyclical industry weights by 20%
	if len(a.cyclicalSymbols) > 0 {
		cyclicalAdjusted := 0
		for symbol, w := range adjusted {
			if a.cyclicalSymbols[symbol] && w > 0 {
				adjusted[symbol] = w * cyclicalReduction
				reasons = append(reasons,
					fmt.Sprintf("moderate: reduced cyclical %s by 20%% (%.4f → %.4f)", symbol, w, adjusted[symbol]))
				cyclicalAdjusted++
			}
		}
		if cyclicalAdjusted > 0 {
			reasons = append(reasons,
				fmt.Sprintf("moderate: reduced %d cyclical industry position(s) by 20%%", cyclicalAdjusted))
		}
	}

	return adjusted, reasons
}

// adjustSevere caps single positions at 5%, reduces cyclical by 50%, and boosts defensive by 20%.
func (a *PortfolioRiskAdjuster) adjustSevere(
	adjusted map[string]float64, reasons []string,
) (map[string]float64, []string) {
	const cap = 0.05
	const cyclicalReduction = 0.50
	const defensiveBoost = 1.20

	// Cap single positions at 5%
	anyCapped := false
	for symbol, w := range adjusted {
		if w > cap {
			adjusted[symbol] = cap
			reasons = append(reasons,
				fmt.Sprintf("severe: capped %s from %.4f to %.4f (5%% limit)", symbol, w, cap))
			anyCapped = true
		}
	}
	if anyCapped {
		reasons = append(reasons, "severe: single-position exposure limited to 5%")
	} else {
		reasons = append(reasons, "severe: no positions exceeded 5% cap")
	}

	// Reduce cyclical industry weights by 50%
	if len(a.cyclicalSymbols) > 0 {
		cyclicalAdjusted := 0
		for symbol, w := range adjusted {
			if a.cyclicalSymbols[symbol] && w > 0 {
				adjusted[symbol] = w * cyclicalReduction
				reasons = append(reasons,
					fmt.Sprintf("severe: reduced cyclical %s by 50%% (%.4f → %.4f)", symbol, w, adjusted[symbol]))
				cyclicalAdjusted++
			}
		}
		if cyclicalAdjusted > 0 {
			reasons = append(reasons,
				fmt.Sprintf("severe: reduced %d cyclical industry position(s) by 50%%", cyclicalAdjusted))
		}
	}

	// Boost defensive industry weights by 20%
	if len(a.defensiveSymbols) > 0 {
		defensiveAdjusted := 0
		for symbol, w := range adjusted {
			if a.defensiveSymbols[symbol] && w > 0 {
				adjusted[symbol] = w * defensiveBoost
				reasons = append(reasons,
					fmt.Sprintf("severe: boosted defensive %s by 20%% (%.4f → %.4f)", symbol, w, adjusted[symbol]))
				defensiveAdjusted++
			}
		}
		if defensiveAdjusted > 0 {
			reasons = append(reasons,
				fmt.Sprintf("severe: boosted %d defensive industry position(s) by 20%%", defensiveAdjusted))
		}
	}

	return adjusted, reasons
}

// adjustEmergency liquidates all positions by setting all weights to zero.
func (a *PortfolioRiskAdjuster) adjustEmergency(
	adjusted map[string]float64, reasons []string,
) (map[string]float64, []string) {
	positionCount := 0
	for symbol, w := range adjusted {
		if w > 0 {
			reasons = append(reasons,
				fmt.Sprintf("emergency: liquidated %s (was %.4f)", symbol, w))
			positionCount++
		}
		adjusted[symbol] = 0
	}
	reasons = append(reasons,
		fmt.Sprintf("emergency: all %d position(s) liquidated, weights set to 0", positionCount))
	return adjusted, reasons
}
