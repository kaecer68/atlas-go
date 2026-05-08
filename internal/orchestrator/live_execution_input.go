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
	if p.system.Sim().provider != nil {
		var err error
		quotes, err = p.system.Sim().provider.GetQuotes(ctx, p.system.Sim().session.SessionDate, symbols)
		if err != nil {
			return nil, err
		}
	}

	result := ExecuteWithContext(ExecutionContext{
		Registry:      p.system.Sim().registry,
		Quotes:        quotes,
		Policy:        p.system.Sim().policy.ExecutionPolicy,
		Plugins:       p.system.plugins,
		SessionID:     p.system.Sim().session.ID,
		WeightManager: p.system.Port().darwinian,
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

	input := &domain.ExecutionInput{
		Regime:               result.Regime,
		RawRecommendations:   result.RawRecommendations,
		FinalRecommendations: result.FinalRecommendations,
		GuardOutcomes:        result.GuardOutcomes,
		DeterminedBy:         "adapter-producer-v1",
	}

	return input, nil
}
