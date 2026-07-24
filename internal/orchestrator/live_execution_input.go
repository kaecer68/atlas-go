package orchestrator

import (
	"context"
	"errors"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ErrSystemNotInitialized is returned when an AdapterProducer is used before
// its backing System is built.
var ErrSystemNotInitialized = errors.New("system not initialized")

// LiveExecutionInputProvider bridges the simulation recommendation pipeline to
// the live trading engine. It is implemented by AdapterProducer, which reuses
// ExecuteWithContext (screening → recommendation → guard filters) and emits
// a domain.ExecutionInput for the live Orchestrator to execute.
type LiveExecutionInputProvider interface {
	Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error)
}

// AdapterProducer adapts an orchestrator.System so the live trading engine can
// consume the same recommendation pipeline used by batch simulations.
//
// This is the intentional bridge between the two execution engines: the
// simulation engine (orchestrator.System) produces screened recommendations,
// and the live engine (live.Orchestrator) executes them through a broker.
type AdapterProducer struct {
	marketData marketdata.Provider
	system     *System
}

// NewAdapterProducer creates a producer that fetches quotes from the given
// market-data provider and runs the full ExecuteWithContext pipeline on the
// provided system.
func NewAdapterProducer(marketData marketdata.Provider, system *System) *AdapterProducer {
	return &AdapterProducer{
		marketData: marketData,
		system:     system,
	}
}

// Produce implements LiveExecutionInputProvider.
func (a *AdapterProducer) Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error) {
	if a.system == nil {
		return nil, ErrSystemNotInitialized
	}

	var quotes []domain.Quote
	if a.marketData != nil {
		var err error
		quotes, err = a.marketData.GetQuotes(ctx, a.system.Sim().session.SessionDate, symbols)
		if err != nil {
			return nil, err
		}
	}

	result := ExecuteWithContext(ExecutionContext{
		Registry:      a.system.Sim().registry,
		Quotes:        quotes,
		Policy:        a.system.Sim().policy.ExecutionPolicy,
		Plugins:       a.system.plugins,
		SessionID:     a.system.Sim().session.ID,
		WeightManager: a.system.Port().darwinian,
		Context:       ctx,
	})

	return &domain.ExecutionInput{
		Regime:               result.Regime,
		RawRecommendations:   result.RawRecommendations,
		FinalRecommendations: result.FinalRecommendations,
		GuardOutcomes:        result.GuardOutcomes,
		DeterminedBy:         "adapter-producer-v1",
	}, nil
}
