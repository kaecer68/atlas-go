package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type SOXIndexChannelAdapter struct {
	provider *marketdata.SOXIndexProvider
	limiter  *rate.Limiter
}

func NewSOXIndexChannelAdapter(p *marketdata.SOXIndexProvider) *SOXIndexChannelAdapter {
	return &SOXIndexChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *SOXIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("sox index marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "sox_index",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *SOXIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *SOXIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SOXIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "sox_index", Country: "美國", Platform: "Yahoo Finance", APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true}
}
