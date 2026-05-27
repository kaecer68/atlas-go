package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TSMCRevenueChannelAdapter adapts a TSMCRevenueProvider to the DataProvider interface.
type TSMCRevenueChannelAdapter struct {
	provider *marketdata.TSMCRevenueProvider
	limiter  *rate.Limiter
}

// NewTSMCRevenueChannelAdapter creates a new adapter for the TSMC revenue channel.
func NewTSMCRevenueChannelAdapter(provider *marketdata.TSMCRevenueProvider) *TSMCRevenueChannelAdapter {
	return &TSMCRevenueChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(2*time.Minute), 1),
	}
}

// Fetch retrieves the latest TSMC monthly revenue.
func (a *TSMCRevenueChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("tsmc_revenue fetch: %w", err)
	}
	data, err := json.Marshal(snap.TSMCRevenue)
	if err != nil {
		return nil, fmt.Errorf("tsmc_revenue marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "tsmc_revenue",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TSMCRevenueChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the TSMC revenue rate limiter.
func (a *TSMCRevenueChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TSMC revenue.
func (a *TSMCRevenueChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tsmc_revenue",
		Country:    "台灣",
		Platform:   "TWSE 台積電月營收",
		APIFormat:  "REST JSON / FinMind TWT49U",
		Path:       "api.finmindtrade.com / www.twse.com.tw",
		HasLimiter: true,
	}
}
