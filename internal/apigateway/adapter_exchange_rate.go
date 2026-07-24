package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type ExchangeRateChannelAdapter struct {
	provider *marketdata.ExchangeRateProvider
	limiter  *rate.Limiter
}

func NewExchangeRateChannelAdapter(p *marketdata.ExchangeRateProvider) *ExchangeRateChannelAdapter {
	return &ExchangeRateChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *ExchangeRateChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("exchange rate marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "exchange_rate",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *ExchangeRateChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "liveness",
	}, nil
}

func (a *ExchangeRateChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *ExchangeRateChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "exchange_rate", Country: "全球", Platform: "Frankfurter/ECB", APIFormat: "REST JSON", Path: "api.frankfurter.dev", HasLimiter: true}
}
