package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type DRAMSpotPriceChannelAdapter struct {
	provider *marketdata.DRAMSpotPriceProvider
	limiter  *rate.Limiter
}

func NewDRAMSpotPriceChannelAdapter(p *marketdata.DRAMSpotPriceProvider) *DRAMSpotPriceChannelAdapter {
	return &DRAMSpotPriceChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *DRAMSpotPriceChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("dram spot price marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "dram_spot_price",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *DRAMSpotPriceChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *DRAMSpotPriceChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *DRAMSpotPriceChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "dram_spot_price", Country: "美國", Platform: "Yahoo Finance", APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true}
}
