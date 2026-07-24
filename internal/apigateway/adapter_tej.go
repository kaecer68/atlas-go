package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TEJChannelAdapter adapts a TEJClient to the DataProvider interface.
type TEJChannelAdapter struct {
	client  *marketdata.TEJClient
	limiter *rate.Limiter
}

// NewTEJChannelAdapter creates a new adapter for the TEJ channel.
func NewTEJChannelAdapter(client *marketdata.TEJClient) *TEJChannelAdapter {
	return &TEJChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(TEJRate, TEJBurst),
	}
}

// Fetch pings the TEJ API and fetches 2330 daily price as a representative sample.
func (a *TEJChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	startDate := start.AddDate(0, 0, -5).Format("2006-01-02")
	endDate := start.Format("2006-01-02")
	rows, err := a.client.GetStockPriceDaily(ctx, "2330", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("tej fetch: %w", err)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("tej marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "tej",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
			LatencyMs:   time.Since(start).Milliseconds(),
		},
	}, nil
}

// HealthCheck verifies connectivity to the TEJ API.
func (a *TEJChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := a.client.Ping(ctx); err != nil {
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

// RateLimit returns the TEJ rate limiter.
func (a *TEJChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TEJ.
func (a *TEJChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tej",
		Country:    "台灣",
		Platform:   "TEJ 台灣經濟新報",
		APIFormat:  "REST JSON",
		Path:       "api.tej.com.tw",
		HasLimiter: true,
	}
}
