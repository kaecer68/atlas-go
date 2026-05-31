package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEETFChannelAdapter adapts the TWSE ETF net subscription provider.
type TWSEETFChannelAdapter struct {
	provider *marketdata.TWSEETFProvider
	limiter  *rate.Limiter
}

// NewTWSEETFChannelAdapter creates a new adapter.
func NewTWSEETFChannelAdapter() *TWSEETFChannelAdapter {
	return &TWSEETFChannelAdapter{
		provider: marketdata.NewTWSEETFProvider(),
		limiter:  rate.NewLimiter(rate.Every(2*time.Second), 5),
	}
}

func (a *TWSEETFChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	date := time.Now().UTC().Format("20060102")
	stats, err := a.provider.FetchNetSubscription(ctx, date)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("twse etf marshal: %w", err)
	}

	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "twse-etf", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *TWSEETFChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *TWSEETFChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEETFChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "twse-etf", Country: "TW", Platform: "TWSE", APIFormat: "JSON", Path: "/rwd/zh/ETF/etfDailyNetFlow", HasLimiter: true}
}
