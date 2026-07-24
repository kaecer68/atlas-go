package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FrankfurterFXChannelAdapter fetches USD/JPY exchange rate via Frankfurter API
// (api.frankfurter.app). This is the authoritative JPY source — us_yahoo no longer
// fetches JPY=X to avoid data overlap.
type FrankfurterFXChannelAdapter struct {
	provider *marketdata.FrankfurterFXProvider
	limiter  *rate.Limiter
}

// NewFrankfurterFXChannelAdapter creates a new adapter for the Frankfurter FX channel.
func NewFrankfurterFXChannelAdapter(provider *marketdata.FrankfurterFXProvider) *FrankfurterFXChannelAdapter {
	return &FrankfurterFXChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(FrankfurterFXRate, FrankfurterFXBurst),
	}
}

// Fetch retrieves the latest USD/JPY exchange rate.
func (a *FrankfurterFXChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("frankfurter_fx fetch: %w", err)
	}
	data, err := json.Marshal(snap.JPY)
	if err != nil {
		return nil, fmt.Errorf("frankfurter_fx marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "frankfurter_fx",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
			LatencyMs:          time.Since(start).Milliseconds(),
		},
	}, nil
}

// HealthCheck verifies the Frankfurter API is reachable.
func (a *FrankfurterFXChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the Frankfurter FX rate limiter.
func (a *FrankfurterFXChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Frankfurter FX.
func (a *FrankfurterFXChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "frankfurter_fx",
		Country:    "日本",
		Platform:   "Frankfurter (USD/JPY)",
		APIFormat:  "REST JSON",
		Path:       "api.frankfurter.app/latest?from=USD&to=JPY",
		HasLimiter: true,
	}
}
