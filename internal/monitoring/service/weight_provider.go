package service

import "github.com/kaecer68/atlas-go/internal/portfolio"

type WeightProvider interface {
	GetWeights(regime string) map[string]float64
}

// FactorWeightEngineWeightProvider adapts portfolio.FactorWeightEngine to the
// WeightProvider interface by converting map[FactorType]float64 into
// map[string]float64 keyed by FactorType.String().
type FactorWeightEngineWeightProvider struct {
	engine *portfolio.FactorWeightEngine
}

// NewFactorWeightEngineWeightProvider creates a production weight provider that
// wraps the given FactorWeightEngine.
func NewFactorWeightEngineWeightProvider(engine *portfolio.FactorWeightEngine) *FactorWeightEngineWeightProvider {
	return &FactorWeightEngineWeightProvider{engine: engine}
}

// GetWeights returns the engine's weights for the given regime as a
// map[string]float64. It returns nil when the engine is nil.
func (p *FactorWeightEngineWeightProvider) GetWeights(regime string) map[string]float64 {
	if p.engine == nil {
		return nil
	}

	src := p.engine.GetWeights(regime)
	out := make(map[string]float64, len(src))
	for ft, w := range src {
		out[string(ft)] = w
	}
	return out
}
