package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSESectorIndexChannelAdapter wraps TWSESectorIndexProvider as a Gateway channel.
// It fetches the Taiwan Semiconductor Index (TAISEMI, 半導體類指數) from TWSE
// OpenAPI v1 and returns it as a MacroDataPoint suitable for MacroDataSnapshot.
type TWSESectorIndexChannelAdapter struct {
	provider *marketdata.TWSESectorIndexProvider
	limiter  *rate.Limiter
}

// NewTWSESectorIndexChannelAdapter creates a channel adapter for TWSE sector indices.
func NewTWSESectorIndexChannelAdapter(provider *marketdata.TWSESectorIndexProvider) *TWSESectorIndexChannelAdapter {
	return &TWSESectorIndexChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves the latest Taiwan Semiconductor Index reading.
func (a *TWSESectorIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("twse_sector_index rate limit: %w", err)
	}

	today := time.Now()
	data, err := a.provider.FetchSectorIndices(ctx, today, today)
	if err != nil {
		return nil, fmt.Errorf("twse_sector_index: fetch: %w", err)
	}

	semiData, ok := data["semiconductor"]
	if !ok || len(semiData) == 0 {
		return nil, fmt.Errorf("twse_sector_index: semiconductor index not found in TWSE response")
	}

	latest := semiData[len(semiData)-1]

	point := marketdata.MacroDataPoint{
		Symbol:    "TAISEMI",
		Value:     latest.Index,
		ChangePct: latest.ReturnPct,
		Timestamp: time.Now().Unix(),
	}

	jsonData, err := json.Marshal(point)
	if err != nil {
		return nil, fmt.Errorf("twse_sector_index: marshal: %w", err)
	}

	return &FetchResult{
		Data: jsonData,
		Meta: FetchMetadata{
			ChannelID: "twse_sector_index",
			LatencyMs: time.Since(start).Milliseconds(),
			Timestamp: time.Now(),
		},
	}, nil
}

// HealthCheck verifies TWSE OpenAPI connectivity by fetching a single-day range.
func (a *TWSESectorIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	today := time.Now()
	_, err := a.provider.FetchSectorIndices(ctx, today, today)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// RateLimit returns the rate limiter.
func (a *TWSESectorIndexChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns channel metadata.
func (a *TWSESectorIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_sector_index",
		Country:    "台灣",
		Platform:   "TWSE OpenAPI v1",
		APIFormat:  "REST JSON",
		Path:       "openapi.twse.com.tw",
		HasLimiter: true,
	}
}
