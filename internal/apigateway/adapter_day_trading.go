package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DayTradingChannelAdapter adapts the TWSE day trading provider.
type DayTradingChannelAdapter struct {
	provider *marketdata.DayTradingProvider
	limiter  *rate.Limiter
}

// NewDayTradingChannelAdapter creates a new adapter.
func NewDayTradingChannelAdapter() *DayTradingChannelAdapter {
	return &DayTradingChannelAdapter{
		provider: marketdata.NewDayTradingProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *DayTradingChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	stats, err := a.provider.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("day trading marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "day_trading",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *DayTradingChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *DayTradingChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *DayTradingChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "day_trading", Country: "台灣", Platform: "TWSE", APIFormat: "REST JSON", Path: "www.twse.com.tw/exchangeReport/TWTB4U", HasLimiter: true}
}
