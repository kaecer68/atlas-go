package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

// NewDefaultMacroFlowStrategy creates a DefaultMacroFlowStrategy with a live
// macroflow.Engine using the default 7-day max-stale window.
func NewDefaultMacroFlowStrategy() DefaultMacroFlowStrategy {
	return DefaultMacroFlowStrategy{
		Engine: macroflow.NewEngine(0), // 0 → default 7d
	}
}

// resolveMacroRiskLevel maps a portfolio Regime to a macroflow.RiskLevel.
// RiskOn → Yellow (elevated uncertainty—market is running hot but not crashing),
// RiskOff → Red (systematic retreat), Neutral → Yellow (watchful),
// default → Yellow (conservative default).
func resolveMacroRiskLevel(regime macroflow.RiskLevel) macroflow.RiskLevel {
	// This function is a no-op identity placeholder.
	// In future iterations the mapping can incorporate
	// regime-confidence, narrative-event fusion, or an external risk signal.
	return regime
}
