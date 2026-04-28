package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ExportStatisticsProvider fetches Taiwan export statistics.
// TWSE FAS210 has been decommissioned; FinMind does not provide aggregate export data.
// This provider returns a "no data available" error to signal the channel as degraded.
// Available alternatives:
//   - Government customs API (manual, not real-time)
//   - g0v mirror: https://portal-api.g0v.ronny.tw/api/goodid/0 (JSON, relatively fresh)
type ExportStatisticsProvider struct {
	client     *http.Client
	storageDir string
	baseURL    string
}

// NewExportStatisticsProvider creates a new export statistics provider.
func NewExportStatisticsProvider(storageDir string) *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client:     &http.Client{Timeout: 20 * time.Second},
		storageDir: storageDir,
	}
}

// Name returns the provider name.
func (e *ExportStatisticsProvider) Name() string {
	return "export_statistics"
}

// FetchSnapshot returns an error because TWSE FAS210 (export statistics)
// has been decommissioned with no confirmed replacement endpoint.
func (e *ExportStatisticsProvider) FetchSnapshot(context.Context) (MacroDataSnapshot, error) {
	return MacroDataSnapshot{}, fmt.Errorf("export_statistics: TWSE FAS210 endpoint decommissioned; " +
		"FinMind does not provide aggregate export data; use government customs API or g0v mirror")
}

// ExportStatisticsProviderWithClient creates a provider with custom HTTP client (for testing).
func ExportStatisticsProviderWithClient(client *http.Client, storageDir string) *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client:     client,
		storageDir: storageDir,
	}
}
