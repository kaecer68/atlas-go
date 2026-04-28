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
