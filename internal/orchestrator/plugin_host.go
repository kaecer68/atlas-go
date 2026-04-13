package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PluginHost manages a chain of plugins and delegates lifecycle hooks.
type PluginHost struct {
	plugins []Plugin
}

// Register adds a plugin to the chain and attaches it to the given core.
func (ph *PluginHost) Register(p Plugin, core *SystemCore) {
	if ph == nil {
		return
	}
	ph.plugins = append(ph.plugins, p)
	if core != nil {
		p.Attach(core)
	}
}

// AttachAll calls Attach on every registered plugin.
func (ph *PluginHost) AttachAll(core *SystemCore) {
	if ph == nil {
		return
	}
	for _, p := range ph.plugins {
		p.Attach(core)
	}
}

// ProcessRecommendations runs all plugins in registration order.
func (ph *PluginHost) ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	if ph == nil {
		return recs
	}
	for _, p := range ph.plugins {
		recs = p.ProcessRecommendations(regime, recs)
	}
	return recs
}

// PostSimulation runs all plugins in registration order.
func (ph *PluginHost) PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	if ph == nil {
		return
	}
	for _, p := range ph.plugins {
		p.PostSimulation(quotes, regime, asOf)
	}
}
