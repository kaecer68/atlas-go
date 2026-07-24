package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// BDIChannelAdapter wraps BDIProvider as a Gateway channel.
type BDIChannelAdapter struct {
	provider *marketdata.BDIProvider
	limiter  *rate.Limiter
}

// NewBDIChannelAdapter creates a new BDI channel adapter.
func NewBDIChannelAdapter(p *marketdata.BDIProvider) *BDIChannelAdapter {
	return &BDIChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves BDI data from the provider.
// Rate limiting is handled by the provider's bdiSharedLimiter (follows SOX/Yahoo pattern).
func (a *BDIChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("bdi marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "bdi",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

// HealthCheck verifies CNBC/Baltic Exchange connectivity by fetching a snapshot.
func (a *BDIChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the adapter's rate limiter.
func (a *BDIChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

// Metadata returns channel metadata.
func (a *BDIChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "bdi", Country: "全球", Platform: "CNBC", APIFormat: "REST JSON", Path: "quote.cnbc.com", HasLimiter: true}
}
