package marketdata

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type HybridProvider struct {
	finmindProvider *FinMindProvider
	fugleProvider   *FugleProvider
	twseClient      *TWSEClient

	finmindCB *CircuitBreaker
	fugleCB   *CircuitBreaker
	cbConfig  circuitBreakerConfig

	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int
}

func NewHybridProvider(finmindAPIKey, fugleAPIKey string) *HybridProvider {
	var finmindProvider *FinMindProvider
	if finmindAPIKey != "" {
		finmindProvider = NewFinMindProvider(finmindAPIKey)
	}

	var fugleProvider *FugleProvider
	if fugleAPIKey != "" {
		fugleProvider = NewFugleProviderWithAPIKey(fugleAPIKey)
	}

	config := defaultCircuitBreakerConfig()
	hp := &HybridProvider{
		finmindProvider: finmindProvider,
		fugleProvider:   fugleProvider,
		twseClient:      NewTWSEClient(),
		cbConfig:        config,
	}

	if finmindProvider != nil {
		hp.finmindCB = NewCircuitBreaker(config)
	}
	if fugleProvider != nil {
		hp.fugleCB = NewCircuitBreaker(config)
	}

	return hp
}

func (p *HybridProvider) Name() string {
	if p.finmindProvider != nil {
		return "hybrid-finmind"
	}
	if p.fugleProvider != nil {
		return "hybrid-fugle"
	}
	return "hybrid-twse"
}

func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if p.finmindProvider != nil && p.finmindCB.Allow() {
		quotes, err := p.finmindProvider.GetQuotes(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			p.finmindCB.RecordSuccess()
			return quotes, nil
		}
		p.finmindCB.RecordFailure()
		p.recordFallback()
		if err != nil {
			fmt.Printf("[HybridProvider] FinMind failed (%v), falling back to Fugle/TWSE\n", err)
		}
	}

	return p.getQuotesFromFugleOrTWSE(ctx, asOf, symbols)
}

func (p *HybridProvider) recordFallback() {
	p.fallbackCount++
	p.lastFallbackAt = time.Now()
}

func (p *HybridProvider) getQuotesFromFugleOrTWSE(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if p.fugleProvider != nil && p.fugleCB.Allow() {
		quotes, err := p.fugleProvider.GetQuotes(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			p.fugleCB.RecordSuccess()
			return quotes, nil
		}
		p.fugleCB.RecordFailure()
		if err != nil {
			fmt.Printf("[HybridProvider] Fugle failed (%v), falling back to TWSE\n", err)
		}
	}
	return p.getQuotesFromTWSE(ctx, symbols)
}

func (p *HybridProvider) getQuotesFromTWSE(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 1 {
		quote, err := p.twseClient.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}

	return p.twseClient.GetQuotesBySymbols(ctx, symbols)
}

func (p *HybridProvider) hasInvalidQuotes(quotes []domain.Quote) bool {
	for _, q := range quotes {
		if q.Last == 0 && q.Open == 0 && q.High == 0 && q.Low == 0 {
			return true
		}
		if q.Last < 0 || q.Open < 0 || q.High < 0 || q.Low < 0 {
			return true
		}
		if q.Volume < 0 {
			return true
		}
	}
	return false
}

func (p *HybridProvider) Reset() {
	if p.finmindCB != nil {
		p.finmindCB.Reset()
	}
	if p.fugleCB != nil {
		p.fugleCB.Reset()
	}
	p.fallbackCount = 0
	p.lastFallbackAt = time.Time{}
	p.recoveryAttempts = 0
}

func (p *HybridProvider) UseTWSE() {
	if p.finmindCB != nil {
		p.finmindCB.ForceOpen()
	}
	if p.fugleCB != nil {
		p.fugleCB.ForceOpen()
	}
}

func (p *HybridProvider) UseFugle() {
	if p.fugleCB != nil {
		p.fugleCB.Reset()
	}
}

func (p *HybridProvider) GetFinMindClient() *FinMindClient {
	if p.finmindProvider == nil {
		return nil
	}
	return p.finmindProvider.GetClient()
}

func (p *HybridProvider) GetTWSEClient() *TWSEClient {
	return p.twseClient
}

func (p *HybridProvider) GetFugleClient() *FugleClient {
	if p.fugleProvider == nil {
		return nil
	}
	return p.fugleProvider.GetClient()
}

func (p *HybridProvider) IsUsingTWSE() bool {
	finmindOpen := p.finmindCB == nil || p.finmindCB.State() == ProviderCircuitOpen
	fugleOpen := p.fugleCB == nil || p.fugleCB.State() == ProviderCircuitOpen
	return finmindOpen && fugleOpen
}

func (p *HybridProvider) CircuitBreakerStats() map[string]interface{} {
	stats := map[string]interface{}{
		"finmind_provider": p.finmindProvider != nil,
		"fugle_provider":   p.fugleProvider != nil,
	}
	if p.finmindCB != nil {
		for k, v := range p.finmindCB.Stats() {
			stats["finmind_"+k] = v
		}
	}
	if p.fugleCB != nil {
		for k, v := range p.fugleCB.Stats() {
			stats["fugle_"+k] = v
		}
	}
	return stats
}
