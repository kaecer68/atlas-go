package eventdriven

import (
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// NarrativeProvider adapts narrative.NarrativeEngine to the local
// NarrativeModelProvider interface so Predictor can consume InvestmentModels.
type NarrativeProvider struct {
	Engine *narrative.NarrativeEngine
}

func (p *NarrativeProvider) ListModels() []ModelView {
	if p.Engine == nil {
		return nil
	}
	src := p.Engine.ListModels()
	out := make([]ModelView, len(src))
	for i, m := range src {
		out[i] = ModelView{
			ID:           m.ID,
			Name:         m.Name,
			Weight:       m.Weight,
			Direction:    directionFromPrediction(m.RecentPrediction),
			ActiveThemes: m.ActiveThemes,
		}
	}
	return out
}

func directionFromPrediction(prediction float64) string {
	switch {
	case prediction > 0:
		return "bullish"
	case prediction < 0:
		return "bearish"
	default:
		return "neutral"
	}
}
