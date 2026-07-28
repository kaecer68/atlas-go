package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// MarketVolumeChannelAdapter adapts the TWSE market volume provider.
type MarketVolumeChannelAdapter struct {
	provider *marketdata.MarketVolumeProvider
	limiter  *rate.Limiter
}

// NewMarketVolumeChannelAdapter creates a new adapter.
func NewMarketVolumeChannelAdapter() *MarketVolumeChannelAdapter {
	return &MarketVolumeChannelAdapter{
		provider: marketdata.NewMarketVolumeProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *MarketVolumeChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	result, err := a.provider.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("market volume marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "market_volume",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *MarketVolumeChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchLatest(ctx)
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

func (a *MarketVolumeChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *MarketVolumeChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "market_volume", Country: "台灣", Platform: "TWSE", APIFormat: "REST JSON", Path: "www.twse.com.tw/exchangeReport/MI_INDEX?type=MS", HasLimiter: true}
}
