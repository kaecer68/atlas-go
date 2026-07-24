package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ExportStatisticsChannelAdapter adapts an ExportStatisticsProvider to the DataProvider interface.
type ExportStatisticsChannelAdapter struct {
	provider *marketdata.ExportStatisticsProvider
	limiter  *rate.Limiter
}

// NewExportStatisticsChannelAdapter creates a new adapter for the export statistics channel.
func NewExportStatisticsChannelAdapter(provider *marketdata.ExportStatisticsProvider) *ExportStatisticsChannelAdapter {
	return &ExportStatisticsChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves the latest export statistics snapshot.
func (a *ExportStatisticsChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("export fetch: %w", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("export marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "export_statistics",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *ExportStatisticsChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the export statistics rate limiter.
func (a *ExportStatisticsChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for export statistics.
func (a *ExportStatisticsChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "export_statistics",
		Country:    "台灣",
		Platform:   "關務署",
		APIFormat:  "csv",
		Path:       "opendata.customs.gov.tw",
		HasLimiter: true,
	}
}
