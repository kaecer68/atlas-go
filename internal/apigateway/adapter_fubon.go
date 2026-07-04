package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FubonChannelAdapter adapts a FubonClient to the DataProvider interface.
type FubonChannelAdapter struct {
	client  *marketdata.FubonClient
	limiter *rate.Limiter
}

// NewFubonChannelAdapter creates a new adapter for the Fubon channel.
func NewFubonChannelAdapter(client *marketdata.FubonClient) *FubonChannelAdapter {
	return &FubonChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(FugleBasicRate, FugleBasicBurst), // same tier as Fugle
	}
}

// Fetch retrieves quotes for 2330 (台積電) and 0050 as representative samples.
func (a *FubonChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	if !a.client.IsHealthy() {
		return nil, fmt.Errorf("fubon proxy: health probe reports unhealthy, skipping fetch")
	}

	quotes, err := a.client.GetQuotes(ctx, []string{"2330", "0050"})
	if err != nil {
		return nil, fmt.Errorf("fubon fetch: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("fubon marshal: %w", err)
	}
	saveSnapshot("fubon", data)
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "fubon",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity to the Fubon proxy.
func (a *FubonChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := a.client.HealthCheck(ctx); err != nil {
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

// RateLimit returns the Fubon rate limiter.
func (a *FubonChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Fubon.
func (a *FubonChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "fubon",
		Country:    "台灣",
		Platform:   "富邦證券",
		APIFormat:  "REST JSON",
		Path:       "api.fubon.com.tw (via Python proxy)",
		HasLimiter: true,
	}
}
