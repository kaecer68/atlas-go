package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FubonChannelAdapter adapts a FubonClient to the DataProvider interface.
type FubonChannelAdapter struct {
	client  *marketdata.FubonClient
	limiter *rate.Limiter
	// snapshotBase is the configured work dir the L3 channel snapshot is
	// written under (injected; see saveSnapshot for why never CWD-relative).
	snapshotBase string
}

// NewFubonChannelAdapter creates a new adapter for the Fubon channel.
// workDir is the configured work dir; it MUST be non-empty (see saveSnapshot).
func NewFubonChannelAdapter(client *marketdata.FubonClient, workDir string) *FubonChannelAdapter {
	return &FubonChannelAdapter{
		client:       client,
		limiter:      rate.NewLimiter(FugleBasicRate, FugleBasicBurst), // same tier as Fugle
		snapshotBase: workDir,
	}
}

func (a *FubonChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if !a.client.IsHealthy() {
		return nil, fmt.Errorf("fubon proxy: health probe reports unhealthy, skipping fetch")
	}

	quotes, err := a.client.GetQuotes(ctx, []string{"2330", "0050"})
	if err != nil {
		return nil, fmt.Errorf("fubon fetch: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("fubon marshal: %w", err)
	}
	saveSnapshot(a.snapshotBase, "fubon", data)
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "fubon",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity to the Fubon proxy.
func (a *FubonChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := a.client.HealthCheck(ctx); err != nil {
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

// RateLimit returns the Fubon rate limiter.
func (a *FubonChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Fubon.
func (a *FubonChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "fubon",
		Country:    "台灣",
		Platform:   "富邦證券",
		APIFormat:  "REST JSON",
		Path:       "api.fubon.com.tw (via Python proxy)",
		HasLimiter: true,
	}
}
