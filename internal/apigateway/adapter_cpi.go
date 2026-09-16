package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// USCPIChannelAdapter adapts the BLS CPI provider to the DataProvider
// interface (channel id "us_cpi"). CPI-U is published monthly, so the limiter
// is deliberately slow (one request per hour) — the channel exists to seed
// MacroDataSnapshot.CPIYoY for the narrative inflation detectors, not for
// per-tick freshness.
type USCPIChannelAdapter struct {
	provider *marketdata.BLSCPIProvider
	limiter  *rate.Limiter
}

func NewUSCPIChannelAdapter(p *marketdata.BLSCPIProvider) *USCPIChannelAdapter {
	return &USCPIChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(time.Hour), 1),
	}
}

func (a *USCPIChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("us_cpi marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_cpi",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USCPIChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USCPIChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "us_cpi", Country: "美國", Platform: "BLS", APIFormat: "REST JSON", Path: "api.bls.gov", HasLimiter: true}
}

func (a *USCPIChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if _, err := a.provider.FetchSnapshot(ctx); err != nil {
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
