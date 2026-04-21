package marketdata

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

// ExportStatisticsProvider fetches Taiwan export statistics from public APIs.
// It monitors electronics export data as a proxy for Taiwan's tech sector health.
type ExportStatisticsProvider struct {
	client *http.Client
}

// NewExportStatisticsProvider creates a new export statistics provider.
func NewExportStatisticsProvider() *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the provider name.
func (e *ExportStatisticsProvider) Name() string {
	return "export_statistics"
}

// FetchSnapshot retrieves Taiwan electronics export data.
// Currently uses a mock implementation that returns placeholder data.
// TODO: Integrate with actual Taiwan export data API (e.g., Taiwan Customs Administration).
func (e *ExportStatisticsProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}

	// Fetch electronics export data from Taiwan Customs Administration
	// This is a placeholder URL - in production, this would be the actual API endpoint
	url := "https://portal.sw.nat.gov.tw/PPL/ap/GB252A00"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("[ExportStatisticsProvider] failed to create request: %v", err)
		// Return mock data on error to allow system to continue
		return e.mockSnapshot(), nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		log.Printf("[ExportStatisticsProvider] request failed: %v", err)
		return e.mockSnapshot(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ExportStatisticsProvider] unexpected status: %d", resp.StatusCode)
		return e.mockSnapshot(), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ExportStatisticsProvider] failed to read body: %v", err)
		return e.mockSnapshot(), nil
	}

	// Try to parse the response as JSON
	var exportResp exportResponse
	if err := json.Unmarshal(body, &exportResp); err != nil {
		// If parsing fails, use mock data
		log.Printf("[ExportStatisticsProvider] failed to parse response: %v", err)
		return e.mockSnapshot(), nil
	}

	// Map the response to MacroDataPoint
	if len(exportResp.Data) > 0 {
		snap.ExportElectronics = MacroDataPoint{
			Symbol:    "TW_EXPORT_ELECTRONICS",
			Value:     exportResp.Data[0].Value,
			ChangePct: exportResp.Data[0].ChangePct,
			Timestamp: time.Now().Unix(),
		}
	} else {
		return e.mockSnapshot(), nil
	}

	return snap, nil
}

// mockSnapshot returns deterministic mock data for testing or when API is unavailable.
func (e *ExportStatisticsProvider) mockSnapshot() MacroDataSnapshot {
	return MacroDataSnapshot{
		RecordedAt: time.Now().Unix(),
		ExportElectronics: MacroDataPoint{
			Symbol:    "TW_EXPORT_ELECTRONICS",
			Value:     120.5, // billions USD (placeholder)
			ChangePct: 2.3,   // placeholder YoY change
			Timestamp: time.Now().Unix(),
		},
	}
}

type exportResponse struct {
	Data []struct {
		Value     float64 `json:"value"`
		ChangePct float64 `json:"change_pct"`
	} `json:"data"`
}

// ExportStatisticsProviderWithClient creates a provider with custom HTTP client (for testing).
func ExportStatisticsProviderWithClient(client *http.Client) *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client: client,
	}
}
