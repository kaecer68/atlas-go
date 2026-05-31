package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEOddLotChannelAdapter adapts the TWSE odd-lot provider.
type TWSEOddLotChannelAdapter struct {
	provider *marketdata.TWSEOddLotProvider
	limiter  *rate.Limiter
}

// NewTWSEOddLotChannelAdapter creates a new adapter.
func NewTWSEOddLotChannelAdapter() *TWSEOddLotChannelAdapter {
	return &TWSEOddLotChannelAdapter{
		provider: marketdata.NewTWSEOddLotProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *TWSEOddLotChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("odd-lot marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "twse-odd-lot", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *TWSEOddLotChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *TWSEOddLotChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEOddLotChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse-odd-lot",
		Country:    "台灣",
		Platform:   "TWSE",
		APIFormat:  "REST JSON",
		Path:       "www.twse.com.tw/exchangeReport/BFI84U",
		HasLimiter: true,
	}
}
