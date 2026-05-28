package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// JPYYahooChannelAdapter fetches USD/JPY exchange rate via Frankfurter API.
// Note: Despite the historical channel name "jpy_yahoo", this adapter uses
// the Frankfurter foreign exchange API (api.frankfurter.app), not Yahoo Finance.
type JPYYahooChannelAdapter struct {
	provider *marketdata.FrankfurterFXProvider
	limiter  *rate.Limiter
}

// NewJPYYahooChannelAdapter creates a new adapter for the JPY Yahoo channel.
func NewJPYYahooChannelAdapter(provider *marketdata.FrankfurterFXProvider) *JPYYahooChannelAdapter {
	return &JPYYahooChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(10*time.Second), 1),
	}
}

// Fetch retrieves the latest USD/JPY exchange rate.
func (a *JPYYahooChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("jpy_yahoo fetch: %w", err)
	}
	data, err := json.Marshal(snap.JPY)
	if err != nil {
		return nil, fmt.Errorf("jpy_yahoo marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "jpy_yahoo",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies the Frankfurter API is reachable.
func (a *JPYYahooChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the JPY Yahoo rate limiter.
func (a *JPYYahooChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for JPY Yahoo.
func (a *JPYYahooChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "jpy_yahoo",
		Country:    "日本",
		Platform:   "Frankfurter (USD/JPY)",
		APIFormat:  "REST JSON",
		Path:       "api.frankfurter.app/latest?from=USD&to=JPY",
		HasLimiter: true,
	}
}
