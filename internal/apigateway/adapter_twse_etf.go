package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEETFChannelAdapter adapts the TWSE ETF provider.
type TWSEETFChannelAdapter struct {
	provider *marketdata.TWSEETFProvider
	limiter  *rate.Limiter
}

// NewTWSEETFChannelAdapter creates a new adapter.
func NewTWSEETFChannelAdapter() *TWSEETFChannelAdapter {
	return &TWSEETFChannelAdapter{
		provider: marketdata.NewTWSEETFProvider(),
		limiter:  rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

func (a *TWSEETFChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("twse etf marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "twse_etf", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *TWSEETFChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *TWSEETFChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEETFChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "twse_etf", Country: "台灣", Platform: "TWSE", APIFormat: "REST JSON", Path: "www.twse.com.tw/exchangeReport/TWT44U", HasLimiter: true}
}
