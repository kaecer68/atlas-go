package orchestrator

import (
	"context"
	"errors"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

var ErrSystemNotInitialized = errors.New("system not initialized")

type LiveExecutionInputProvider interface {
	Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error)
}

type liveExecutionInputProvider struct {
	system *System
}

func NewLiveExecutionInputProvider(system *System) *liveExecutionInputProvider {
	return &liveExecutionInputProvider{system: system}
}

func (p *liveExecutionInputProvider) Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error) {
	if p.system == nil {
		return nil, ErrSystemNotInitialized
	}

	var quotes []domain.Quote
	if p.system.provider != nil {
		var err error
		quotes, err = p.system.provider.GetQuotes(ctx, p.system.session.SessionDate, symbols)
		if err != nil {
			return nil, err
		}
	}

	result := ExecuteWithContext(ExecutionContext{
		Registry:      p.system.registry,
		Quotes:        quotes,
		Policy:        p.system.policy.ExecutionPolicy,
		Plugins:       p.system.plugins,
		SessionID:     p.system.session.ID,
		WeightManager: p.system.darwinian,
		Context:       ctx,
	})

	return &domain.ExecutionInput{
		Regime:               result.Regime,
		RawRecommendations:   result.RawRecommendations,
		FinalRecommendations: result.FinalRecommendations,
		GuardOutcomes:        result.GuardOutcomes,
		DeterminedBy:         "orchestrator-pipeline",
	}, nil
}

type AdapterProducer struct {
	marketData marketdata.Provider
	system     *System
}

func NewAdapterProducer(marketData marketdata.Provider, system *System) *AdapterProducer {
	return &AdapterProducer{
		marketData: marketData,
		system:     system,
	}
}

func (a *AdapterProducer) Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error) {
	if a.system == nil {
		return nil, ErrSystemNotInitialized
	}

	var quotes []domain.Quote
	if a.marketData != nil {
		var err error
		quotes, err = a.marketData.GetQuotes(ctx, a.system.session.SessionDate, symbols)
		if err != nil {
			return nil, err
		}
	}

	result := ExecuteWithContext(ExecutionContext{
		Registry:      a.system.registry,
		Quotes:        quotes,
		Policy:        a.system.policy.ExecutionPolicy,
		Plugins:       a.system.plugins,
		SessionID:     a.system.session.ID,
		WeightManager: a.system.darwinian,
		Context:       ctx,
	})

	input := &domain.ExecutionInput{
		Regime:               result.Regime,
		RawRecommendations:   result.RawRecommendations,
		FinalRecommendations: result.FinalRecommendations,
		GuardOutcomes:        result.GuardOutcomes,
		DeterminedBy:         "adapter-producer-v1",
	}

	return input, nil
}
