package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// SOXIndexProvider fetches the Philadelphia Semiconductor Index (^SOX) from Yahoo Finance.
type SOXIndexProvider struct {
	httpClient *http.Client
	baseURL    string
}

// NewSOXIndexProvider creates a new SOX index provider.
func NewSOXIndexProvider() *SOXIndexProvider {
	return &SOXIndexProvider{
		httpClient: httpclient.NewFactory().NewClient(15 * time.Second),
		baseURL:    "https://query1.finance.yahoo.com",
	}
}

// Name returns the provider name.
func (p *SOXIndexProvider) Name() string {
	return "sox_index"
}

// FetchSnapshot retrieves the latest ^SOX value and change percentage.
func (p *SOXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	url := p.baseURL + "/v8/finance/chart/%5ESOX?interval=1d&range=2d"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	prev := latest
	if len(closes) > 1 {
		candidate := closes[len(closes)-2]
		if !math.IsNaN(candidate) && !math.IsInf(candidate, 0) && candidate != 0 {
			prev = candidate
		}
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	return MacroDataSnapshot{
		SOXIndex: MacroDataPoint{
			Symbol:    "^SOX",
			Value:     latest,
			ChangePct: changePct,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
