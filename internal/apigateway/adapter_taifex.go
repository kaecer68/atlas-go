package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TaifexChannelAdapter adapts the TAIFEX daily market provider.
type TaifexChannelAdapter struct {
	provider *marketdata.TaifexProvider
	limiter  *rate.Limiter
}

// NewTaifexChannelAdapter creates a new adapter.
func NewTaifexChannelAdapter() *TaifexChannelAdapter {
	return &TaifexChannelAdapter{
		provider: marketdata.NewTaifexProvider(),
		limiter:  rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

func (a *TaifexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	today := time.Now().Format("20060102")
	stats, err := a.provider.FetchDailyStats(ctx, today)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("taifex marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "taifex-daily", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *TaifexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *TaifexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TaifexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "taifex-daily", Country: "TW", Platform: "TAIFEX", APIFormat: "JSON", Path: "/cht/3/futDailyMarketReport", HasLimiter: true}
}
