package live

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type AgentRunner struct {
	stateStore *livestore.StateStore
	marketData interface {
		GetQuotes(ctx context.Context, t time.Time, symbols []string) ([]domain.Quote, error)
	}
	system interface {
		Registry() domain.AgentRegistry
		GetPlugins() *orchestrator.PluginRegistry
		GetExecutionPolicy() domain.ExecutionPolicy
	}
	effectiveBrokerMode string
	eventBus            *ChannelEventBus
	metrics             MetricsRecorder
}

func NewAgentRunner(
	stateStore *livestore.StateStore,
	marketData interface {
		GetQuotes(ctx context.Context, t time.Time, symbols []string) ([]domain.Quote, error)
	},
	system interface {
		Registry() domain.AgentRegistry
		GetPlugins() *orchestrator.PluginRegistry
		GetExecutionPolicy() domain.ExecutionPolicy
	},
	effectiveBrokerMode string,
) *AgentRunner {
	return &AgentRunner{
		stateStore:          stateStore,
		marketData:          marketData,
		system:              system,
		effectiveBrokerMode: effectiveBrokerMode,
	}
}

func (r *AgentRunner) SetEventBus(eb *ChannelEventBus) {
	r.eventBus = eb
}

func (r *AgentRunner) SetMetrics(m MetricsRecorder) {
	r.metrics = m
}

func (r *AgentRunner) ApplyExecutionInput(ctx context.Context, input ExecutionInput) error {
	r.stateStore.SetCurrentRegime(input.Regime)

	if len(input.RawRecommendations) > 0 {
		r.stateStore.SetPendingRecommendations(input.RawRecommendations)
	}

	if len(input.FinalRecommendations) > 0 {
		r.stateStore.SetFilteredRecommendations(input.FinalRecommendations)
	}

	r.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"regime":        string(input.Regime),
			"raw_count":     len(input.RawRecommendations),
			"final_count":   len(input.FinalRecommendations),
			"determined_by": input.DeterminedBy,
			"type":          "execution_input_applied",
		},
	})

	logging.Info("agent_runner", "applied_execution_input",
		"regime", input.Regime, "raw_count", len(input.RawRecommendations), "final_count", len(input.FinalRecommendations))
	return nil
}

func (r *AgentRunner) RunContextAgent(ctx context.Context, watchlist []string) error {
	if r.system == nil {
		return fmt.Errorf("system not initialized")
	}
	if len(watchlist) == 0 {
		watchlist = []string{"0050.TW", "0056.TW", "2330.TW", "2317.TW", "2454.TW"}
	}

	quotes, err := r.marketData.GetQuotes(ctx, time.Now(), watchlist)
	if err != nil {
		return fmt.Errorf("fetch quotes for regime inference: %w", err)
	}

	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	registry := r.system.Registry()
	plugins := r.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}

	regime := r.inferRegime(registry, quoteMap, plugins)
	r.stateStore.SetCurrentRegime(regime)

	r.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"regime": string(regime),
			"type":   "regime_inference",
		},
	})

	logging.Info("context_agent", "regime_inferred", "regime", regime)
	return nil
}

func (r *AgentRunner) inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *orchestrator.PluginRegistry) domain.Regime {
	score := 0
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerContext {
			continue
		}
		prompt := plugins.ResolvePrompt(agent, nil)
		score += plugins.RegimeScore(agent, quotes, prompt)
	}
	switch {
	case score > 0:
		return domain.RegimeRiskOn
	case score < 0:
		return domain.RegimeRiskOff
	default:
		return domain.RegimeNeutral
	}
}

func (r *AgentRunner) RunStyleAndSectorAgents(ctx context.Context, watchlist []string) error {
	if r.system == nil {
		return fmt.Errorf("system not initialized")
	}
	if len(watchlist) == 0 {
		return nil
	}

	quotes, err := r.marketData.GetQuotes(ctx, time.Now(), watchlist)
	if err != nil {
		return fmt.Errorf("fetch quotes for agent analysis: %w", err)
	}

	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	registry := r.system.Registry()
	plugins := r.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}
	regime := r.stateStore.GetCurrentRegime()

	var recommendations []domain.Recommendation
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		prompt := plugins.ResolvePrompt(agent, nil)
		symbols := agent.Universe
		if len(symbols) == 0 {
			symbols = watchlist
		}

		for _, symbol := range symbols {
			quote, ok := quoteMap[symbol]
			if !ok || !quote.IsTradable {
				continue
			}
			passed, err := plugins.Screen(ctx, agent, symbol, quoteMap)
			if err != nil || !passed {
				continue
			}
			rec, ok := plugins.Recommendation(agent, quote, prompt, regime)
			if !ok {
				continue
			}
			recommendations = append(recommendations, rec)
		}
	}

	if len(recommendations) > 0 {
		r.stateStore.SetPendingRecommendations(recommendations)
		logging.Info("style_sector_agents", "generated_recommendations", "count", len(recommendations))
	}

	r.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"recommendation_count": len(recommendations),
			"type":                 "agent_recommendations",
		},
	})

	return nil
}

// ApplyRiskFilters runs the LayerControl (CRO/CIO) agents over the pending
// recommendations and stores the survivors as the filtered set.
//
// NOT A LIVE RISK GATE (FU-20261016-32, verified 2026-10-16): nothing in
// production calls this method, and calling it would apply the control layer a
// second time. The controlled path is the analysis pipeline:
// orchestrator/executor_pipeline.go:131 -> DefaultControlLayerStrategy.ApplyControl
// -> PluginRegistry.ApplyControl (executor_control.go:104). The live runner then
// receives that already-filtered result as ExecutionInput.FinalRecommendations
// and stores it verbatim in ApplyExecutionInput (agent_runner.go:65).
//
// The state this method writes (StateStore.SetFilteredRecommendations) has no
// production reader either, so leaving it uncalled loses no filtering:
// GetFilteredRecommendations is only used by tests.
//
// Re-certification conditions: only if the live path stops consuming
// pre-filtered recommendations. Then this method must (a) be the single place
// that applies the control layer for the live path, (b) leave pending
// recommendations untouched rather than re-deriving them, (c) come with an
// end-to-end test that a blocked recommendation never reaches the order path,
// and (d) drop its entry from internal/risk/exported_risk_levers_wiring_test.go
// together with closing FU-20261016-32.
func (r *AgentRunner) ApplyRiskFilters(ctx context.Context) error {
	if r.system == nil {
		return fmt.Errorf("system not initialized")
	}

	recommendations := r.stateStore.GetPendingRecommendations()
	if len(recommendations) == 0 {
		return nil
	}

	registry := r.system.Registry()
	plugins := r.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}
	policy := r.system.GetExecutionPolicy()
	regime := r.stateStore.GetCurrentRegime()

	filtered := recommendations
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerControl {
			continue
		}
		filtered = plugins.ApplyControl(agent, filtered, policy, regime)
	}

	r.stateStore.SetFilteredRecommendations(filtered)
	blockedCount := len(recommendations) - len(filtered)

	logging.Info("risk_filters", "applied_cro_cio_filters", "passed", len(filtered), "blocked", blockedCount)

	if blockedCount > 0 {
		r.publishEvent(BusEvent{
			ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Type:      EventRiskAlert,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"blocked_count": blockedCount,
				"passed_count":  len(filtered),
				"type":          "risk_filter",
			},
		})
	}

	return nil
}

func (r *AgentRunner) publishEvent(event BusEvent) {
	if r.eventBus == nil {
		return
	}
	r.eventBus.Publish(event)
}
