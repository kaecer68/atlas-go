package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEInsiderChannelAdapter adapts the TWSE insider trading provider.
type TWSEInsiderChannelAdapter struct {
	provider *marketdata.TWSEInsiderProvider
	limiter  *rate.Limiter
}

// NewTWSEInsiderChannelAdapter creates a new adapter.
func NewTWSEInsiderChannelAdapter(storageDir string) *TWSEInsiderChannelAdapter {
	return &TWSEInsiderChannelAdapter{
		provider: marketdata.NewTWSEInsiderProvider(storageDir),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// SetProvider overrides the provider (for tests).
func (a *TWSEInsiderChannelAdapter) SetProvider(p *marketdata.TWSEInsiderProvider) {
	a.provider = p
}

func (a *TWSEInsiderChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("twse_insider rate limit: %w", err)
	}

	agg, err := a.provider.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(agg)
	if err != nil {
		return nil, fmt.Errorf("twse_insider marshal: %w", err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_insider",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

func (a *TWSEInsiderChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *TWSEInsiderChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEInsiderChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_insider",
		Country:    "台灣",
		Platform:   "TWSE OpenAPI",
		APIFormat:  "REST JSON",
		Path:       "openapi.twse.com.tw/v1/opendata/t187ap12_L",
		HasLimiter: true,
	}
}
