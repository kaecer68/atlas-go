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
