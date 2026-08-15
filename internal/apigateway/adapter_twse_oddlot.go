package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	stats, err := a.provider.FetchLatest(ctx)
	if err != nil {
		// Non-trading days or holidays: TWSE returns no data for the past 7 days,
		// which is expected behavior. Return a stale result instead of an error
		// to avoid triggering the circuit breaker.
		if strings.Contains(err.Error(), "no TWSE") || strings.Contains(err.Error(), "no data") ||
			errors.Is(err, marketdata.ErrOddLotUpstreamRemoved) {
			return &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: "twse_oddlot", Timestamp: time.Now()}}, nil
		}
		return nil, err
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("odd-lot marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "twse_oddlot",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *TWSEOddLotChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *TWSEOddLotChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEOddLotChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_oddlot",
		Country:    "台灣",
		Platform:   "TWSE",
		APIFormat:  "REST JSON",
		Path:       "www.twse.com.tw/exchangeReport/BFI84U",
		HasLimiter: true,
	}
}
