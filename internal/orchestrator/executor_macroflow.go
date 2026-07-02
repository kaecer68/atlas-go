package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

// NewDefaultMacroFlowStrategy creates a DefaultMacroFlowStrategy with a live
// macroflow.Engine using the default 7-day max-stale window.
func NewDefaultMacroFlowStrategy() DefaultMacroFlowStrategy {
	return DefaultMacroFlowStrategy{
		engine: macroflow.NewEngine(0), // 0 → default 7d
	}
}

// resolveMacroRiskLevel is a pass-through identity placeholder for future
// regime → macroflow.RiskLevel mapping (e.g., incorporating regime-confidence,
// narrative-event fusion, or an external risk signal). Currently returns the
// input unchanged; the pipeline uses a safe default of macroflow.RiskYellow.
func resolveMacroRiskLevel(regime macroflow.RiskLevel) macroflow.RiskLevel {
	return regime
}
