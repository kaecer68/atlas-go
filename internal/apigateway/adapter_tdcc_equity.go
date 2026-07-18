package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TDCClientChannelAdapter wraps TDCClient as a DataProvider (G01).
type TDCClientChannelAdapter struct {
	provider *marketdata.TDCClient
	limiter  *rate.Limiter
}

// NewTDCClientChannelAdapter creates a TDCC equity dispersion channel adapter.
func NewTDCClientChannelAdapter() *TDCClientChannelAdapter {
	p := marketdata.NewTDCClient()
	return &TDCClientChannelAdapter{provider: p, limiter: p.RateLimiter()}
}

func (a *TDCClientChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("tdcc rate limit: %w", err)
	}
	stats, err := a.provider.FetchDispersion(ctx, time.Now().Format("20060102"))
	if err != nil {
		return nil, fmt.Errorf("tdcc_equity_dispersion: %w", err)
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("tdcc marshal: %w", err)
	}
	return &FetchResult{
		Data: payload,
		Meta: FetchMetadata{
			ChannelID:          "tdcc_equity_dispersion",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

func (a *TDCClientChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{
		Status:    "ok",
		CheckType: "liveness",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}, nil
}

func (a *TDCClientChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TDCClientChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tdcc_equity_dispersion",
		Country:    "TW",
		Platform:   "TDCC",
		APIFormat:  "JSON",
		Path:       "/v1/equity-dispersion",
		HasLimiter: true,
	}
}
