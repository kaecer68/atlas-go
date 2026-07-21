package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSESBLChannelAdapter wraps TWSESBLProvider as a DataProvider (G02).
type TWSESBLChannelAdapter struct {
	provider *marketdata.TWSESBLProvider
	limiter  *rate.Limiter
}

// NewTWSESBLChannelAdapter creates a TWSE SBL channel adapter.
func NewTWSESBLChannelAdapter() *TWSESBLChannelAdapter {
	p := marketdata.NewTWSESBLProvider(0.5)
	return &TWSESBLChannelAdapter{provider: p, limiter: p.RateLimiter()}
}

func (a *TWSESBLChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("twse_sbl rate limit: %w", err)
	}
	stats, err := a.provider.FetchSBLSummary(ctx, time.Now().Format("20060102"))
	if err != nil {
		return nil, fmt.Errorf("twse_sbl: %w", err)
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("twse_sbl marshal: %w", err)
	}
	return &FetchResult{
		Data: payload,
		Meta: FetchMetadata{
			ChannelID:          "twse_sbl",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

func (a *TWSESBLChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	// Stub: TWSE SBL endpoint not yet confirmed (G02). HealthCheck
	// returns "inactive" so the alerting path in
	// monitoring/service/data_channels.go skips this channel; the
	// dashboard renders it as "not yet live" via Metadata().Stub.
	return HealthStatus{
		Status:    "inactive",
		CheckType: "liveness",
		LastError: "stub: TWSE SBL endpoint not yet confirmed (G02) — see internal/marketdata/twse_sbl_provider.go",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}, nil
}

func (a *TWSESBLChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSESBLChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_sbl",
		Country:    "TW",
		Platform:   "TWSE",
		APIFormat:  "JSON",
		Path:       "/rwd/zh/lending/TWT93U",
		HasLimiter: true,
		// Stub: channel is registered so the dashboard can show
		// "not yet live" explicitly. Real implementation is gated
		// on G02 (TWSE SBL endpoint confirmation).
		Stub: true,
	}
}
