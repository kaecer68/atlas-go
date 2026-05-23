package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"golang.org/x/time/rate"
)

// GatewayBackedProvider implements marketdata.Provider with an independent rate
// limiter for the simulation/backtest path. It wraps the same underlying provider
// types (Fugle, TWSE, Hybrid) that selectProvider() would create, but with a
// separate rate limit budget that does not compete with DashboardAPI channels.
//
// This is a structural bridge: the provider lifecycle is managed through the
// Gateway configuration path in main.go, not by the orchestrator. When the
// Gateway later supports parameterized quote channels, this provider can switch
// to route through them without changing the orchestrator.
type GatewayBackedProvider struct {
	cfg config.Config

	inner marketdata.Provider // lazily created on first GetQuotes call
	once  sync.Once

	limiter     *rate.Limiter
	limiterOnce sync.Once
}

// NewGatewayBackedProvider creates a new provider with an independent rate limiter.
func NewGatewayBackedProvider(cfg config.Config) *GatewayBackedProvider {
	return &GatewayBackedProvider{cfg: cfg}
}

func (p *GatewayBackedProvider) Name() string {
	p.initProvider()
	return "gateway-backed-" + p.inner.Name()
}

func (p *GatewayBackedProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	p.initLimiter()
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("gateway-backed provider rate limit: %w", err)
	}

	p.initProvider()

	quotes, err := p.inner.GetQuotes(ctx, asOf, symbols)
	if err != nil {
		logging.Error("gateway_provider", "get_quotes_failed",
			"provider", p.inner.Name(),
			"symbols", len(symbols),
			"err", err.Error())
		return nil, err
	}

	logging.Info("gateway_provider", "get_quotes_ok",
		"provider", p.inner.Name(),
		"symbols", len(quotes))

	return quotes, nil
}

func (p *GatewayBackedProvider) initLimiter() {
	p.limiterOnce.Do(func() {
		// Simulation path gets generous limits — 50 req/s, burst 10.
		// This is far higher than DashboardAPI's per-channel limits (1-5 req/s)
		// and allows normal simulation throughput without competing with the
		// Gateway's DashboardAPI rate limiting.
		p.limiter = rate.NewLimiter(50, 10)
	})
}

func (p *GatewayBackedProvider) initProvider() {
	p.once.Do(func() {
		p.inner = selectProvider(p.cfg)
		logging.Info("gateway_provider", "initialized",
			"inner", p.inner.Name(),
			"provider_cfg", p.cfg.MarketDataProvider)
	})
}
