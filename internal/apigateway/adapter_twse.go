package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEChannelAdapter adapts a TWSEClient to the DataProvider interface.
type TWSEChannelAdapter struct {
	client  *marketdata.TWSEClient
	limiter *rate.Limiter
}

// NewTWSEChannelAdapter creates a new adapter for the TWSE channel.
func NewTWSEChannelAdapter(client *marketdata.TWSEClient) *TWSEChannelAdapter {
	return &TWSEChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(rate.Inf, 0), // file-based replay, no rate limit
	}
}

// Fetch retrieves all quotes from TWSE OpenAPI as a sample dataset.
func (a *TWSEChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quotes, err := a.client.GetQuotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("twse fetch: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("twse marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_replay",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by attempting a bulk quote fetch.
func (a *TWSEChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetQuotes(ctx)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// RateLimit returns the TWSE rate limiter from limits.go.
func (a *TWSEChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE.
func (a *TWSEChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw",
		HasLimiter: true,
	}
}
