package orchestrator

import (
	"context"
	"os"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/screener"
)

type AgentExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool)
}

type RegimeExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Score(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int
}

type PluginRegistry struct {
	regimeExecutors  []RegimeExecutor
	agentExecutors   []AgentExecutor
	controlExecutors []ControlExecutor
	screener         screener.Screener
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		regimeExecutors: []RegimeExecutor{
			TaiwanMacroRegimeExecutor{},
			ForeignFlowRegimeExecutor{},
		},
		agentExecutors: []AgentExecutor{
			SemiconductorExecutor{},
			AISupplyChainExecutor{},
			ETFRotationExecutor{},
			FinancialsExecutor{},
			ShippingExecutor{},
			GrowthMomentumExecutor{},
			ValueYieldExecutor{},
			EarningsQualityExecutor{},
			TechnicalBreakoutExecutor{},
		},
		controlExecutors: []ControlExecutor{
			CRORiskExecutor{},
			CIOPortfolioExecutor{},
		},
	}
}

func (r *PluginRegistry) WithScreener(s screener.Screener) *PluginRegistry {
	r.screener = s
	return r
}

func (r *PluginRegistry) Screen(ctx context.Context, agent domain.AgentSpec, symbol string, quotes map[string]domain.Quote) (bool, error) {
	if r.screener == nil || !agent.ScreeningCriteria.HasFilters() {
		return true, nil
	}
	return r.screener.Screen(ctx, symbol, agent.ScreeningCriteria, quotes)
}

func (r *PluginRegistry) ResolvePrompt(agent domain.AgentSpec, overrides map[string]string) string {
	if override, ok := overrides[agent.ID]; ok && override != "" {
		return override
	}
	if override, ok := overrides[agent.Skill]; ok && override != "" {
		return override
	}
	bytes, err := os.ReadFile(agent.PromptFile)
	if err != nil {
		return ""
	}
	return strings.ToLower(string(bytes))
}

func (r *PluginRegistry) RegimeScore(agent domain.AgentSpec, quotes map[string]domain.Quote, prompt string) int {
	for _, exec := range r.regimeExecutors {
		if exec.Supports(agent) {
			return exec.Score(agent, quotes, prompt)
		}
	}
	return 0
}

func (r *PluginRegistry) Recommendation(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	for _, exec := range r.agentExecutors {
		if exec.Supports(agent) {
			return exec.Recommend(agent, quote, prompt, regime)
		}
	}
	return domain.Recommendation{}, false
}

func (r *PluginRegistry) ApplyControl(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	for _, exec := range r.controlExecutors {
		if exec.Supports(agent) {
			return exec.Apply(agent, recs, policy)
		}
	}
	return recs
}
