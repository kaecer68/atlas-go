package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// loadRecOverrides reads human interventions from the ledger and returns a map
// of overrides for consume by collectRecommendations. Only non-expired
// approve_rec / reject_rec interventions are considered.
// Key format: "agentID:symbol", value: "approved" or "rejected".
func loadRecOverrides(store ledger.OutcomeStore) map[string]string {
	if store == nil {
		return nil
	}
	interventions, err := store.LoadHumanInterventions()
	if err != nil || len(interventions) == 0 {
		return nil
	}
	result := make(map[string]string)
	for _, iv := range interventions {
		if iv.IsExpired() || iv.TargetSymbol == "" {
			continue
		}
		switch iv.Type {
		case "approve_rec":
			key := iv.TargetAgentID + ":" + iv.TargetSymbol
			result[key] = "approved"
		case "reject_rec":
			key := iv.TargetAgentID + ":" + iv.TargetSymbol
			result[key] = "rejected"
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func filterMutedAgents(registry domain.AgentRegistry, plugins *PluginRegistry) domain.AgentRegistry {
	if plugins == nil || plugins.healthManager == nil {
		return registry
	}
	filtered := make([]domain.AgentSpec, 0, len(registry.Agents))
	for _, agent := range registry.Agents {
		if !plugins.IsAgentHealthy(agent.ID) {
			health := plugins.healthManager.GetHealth(agent.ID)
			score := 0.0
			if health != nil {
				score = health.CompositeScore
			}
			logging.Info("executors", "agent_muted", logging.AgentID(agent.ID), "composite_score", score)
			continue
		}
		filtered = append(filtered, agent)
	}
	return domain.AgentRegistry{
		Version: registry.Version,
		Agents:  filtered,
	}
}
