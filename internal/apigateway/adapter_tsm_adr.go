package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TSMADRChannelAdapter adapts TSMADRProvider (TSMC ADR, NYSE:TSM) to the
// DataProvider interface. Uses yahooTechLimiter (1 req/1.5s) — grouped with
// NVDA / AAPL / MSFT to parallelize with the macro and index groups during
// us_market_refresh.
type TSMADRChannelAdapter struct {
	provider *marketdata.TSMADRProvider
	limiter  *rate.Limiter
}

func NewTSMADRChannelAdapter(p *marketdata.TSMADRProvider) *TSMADRChannelAdapter {
	return &TSMADRChannelAdapter{
		provider: p,
		limiter:  yahooTechLimiter,
	}
}

func (a *TSMADRChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("tsm_adr marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "tsm_adr",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *TSMADRChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *TSMADRChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TSMADRChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "tsm_adr", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}
