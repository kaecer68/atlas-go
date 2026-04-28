package strategy

import (
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Registry struct {
	mu         sync.RWMutex
	strategies map[string]*Strategy
}

func NewRegistry() *Registry {
	return &Registry{
		strategies: make(map[string]*Strategy),
	}
}

func NewRegistryWithDefaults() *Registry {
	r := NewRegistry()

	strategies := []*Strategy{
		{
			ID:           "all_weather",
			Name:         "全天候",
			Description:  "所有 Agent，保守閾值",
			Enabled:      true,
			Agents:       []string{"*"},
			Priority:     10,
			RiskAppetite: RiskAppetiteBalanced,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeRiskOff, domain.RegimeNeutral},
		},
		{
			ID:           "growth",
			Name:         "成長動能",
			Description:  "動能 + AI supply chain",
			Enabled:      true,
			Agents:       []string{"momentum", "ai_supply_chain"},
			Priority:     20,
			RiskAppetite: RiskAppetiteAggressive,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral},
		},
		{
			ID:           "value",
			Name:         "價值投資",
			Description:  "Value + Quality",
			Enabled:      true,
			Agents:       []string{"value", "quality"},
			Priority:     30,
			RiskAppetite: RiskAppetiteConservative,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral},
		},
		{
			ID:           "defensive",
			Name:         "防御型",
			Description:  "高品質 + 低波動",
			Enabled:      true,
			Agents:       []string{"quality", "low_volatility"},
			Priority:     40,
			RiskAppetite: RiskAppetiteConservative,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOff, domain.RegimeNeutral},
		},
		{
			ID:           "momentum",
			Name:         "純動能",
			Description:  "僅動能因子",
			Enabled:      true,
			Agents:       []string{"momentum"},
			Priority:     50,
			RiskAppetite: RiskAppetiteAggressive,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn},
		},
	}

	for _, s := range strategies {
		r.strategies[s.ID] = s
	}
	return r
}

func (r *Registry) Register(s *Strategy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		return fmt.Errorf("strategy ID cannot be empty")
	}
	r.strategies[s.ID] = s
	return nil
}

func (r *Registry) Get(id string) (*Strategy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.strategies[id]
	return s, ok
}

func (r *Registry) List() []*Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		result = append(result, s)
	}
	return result
}

func (r *Registry) ListByRegime(regime domain.Regime) []*Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		if !s.Enabled {
			continue
		}
		for _, pref := range s.RegimePrefs {
			if pref == regime || pref == domain.RegimeNeutral {
				result = append(result, s)
				break
			}
		}
	}
	return result
}
