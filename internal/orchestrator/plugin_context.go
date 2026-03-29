package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type TaiwanMacroRegimeExecutor struct{}

func (TaiwanMacroRegimeExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "taiwan_macro"
}

func (TaiwanMacroRegimeExecutor) Score(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int {
	if strings.Contains(prompt, "risk") {
		return 1
	}
	return 0
}

type ForeignFlowRegimeExecutor struct{}

func (ForeignFlowRegimeExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "foreign_flow"
}

func (ForeignFlowRegimeExecutor) Score(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int {
	if quote, ok := quotes["0050.TW"]; ok && quote.Last >= quote.Open {
		return 1
	}
	return -1
}
