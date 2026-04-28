package strategy

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SelectorConfig struct {
	MinSwitchInterval time.Duration
	SwitchThreshold   float64
}

type Selector struct {
	registry   *Registry
	comparison *ComparisonEngine
	current    *Strategy
	config     SelectorConfig
	lastSwitch time.Time
}

func NewSelector(registry *Registry, comparison *ComparisonEngine) *Selector {
	return &Selector{
		registry:   registry,
		comparison: comparison,
		config: SelectorConfig{
			MinSwitchInterval: 5 * 24 * time.Hour,
			SwitchThreshold:   0.10,
		},
		lastSwitch: time.Time{},
	}
}

func (s *Selector) Select(ctx context.Context, vix float64, regime domain.Regime) (*Strategy, error) {
	candidates := s.registry.ListByRegime(regime)
	if len(candidates) == 0 {
		aw, ok := s.registry.Get("all_weather")
		if ok {
			return aw, nil
		}
		return &Strategy{
			ID:     "fallback",
			Name:   "Fallback",
			Agents: []string{"*"},
		}, nil
	}

	scores := make(map[string]float64)
	for _, c := range candidates {
		score, _ := s.comparison.GetScore(c.ID, 20)
		scores[c.ID] = score
	}

	var best *Strategy
	var bestScore float64
	for _, c := range candidates {
		score := scores[c.ID]
		if best == nil || score > bestScore {
			best = c
			bestScore = score
		}
	}

	if best != nil && s.current != nil && best.ID != s.current.ID {
		if !s.shouldSwitch(s.current, best, bestScore-scores[s.current.ID]) {
			return s.current, nil
		}
		s.lastSwitch = time.Now()
	}

	if best == nil {
		best, _ = s.registry.Get("all_weather")
	}
	s.current = best
	return best, nil
}

func (s *Selector) shouldSwitch(from, to *Strategy, scoreDelta float64) bool {
	if from.ID == to.ID {
		return false
	}
	if time.Since(s.lastSwitch) < s.config.MinSwitchInterval {
		return false
	}
	if scoreDelta < s.config.SwitchThreshold {
		return false
	}
	return true
}

func (s *Selector) GetCurrentStrategy() *Strategy {
	return s.current
}
