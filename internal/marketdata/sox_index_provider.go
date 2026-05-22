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

// soxHosts lists Yahoo Finance API hosts tried in order on failure for SOX.
var soxHosts = yahooHosts // share the same host list as Yahoo macro provider

// SOXIndexProvider fetches the Philadelphia Semiconductor Index (^SOX) from Yahoo Finance.
type SOXIndexProvider struct {
	httpClient *http.Client
	hosts      []string
}

// NewSOXIndexProvider creates a new SOX index provider.
func NewSOXIndexProvider() *SOXIndexProvider {
	return &SOXIndexProvider{
		httpClient: httpclient.NewFactory().NewClient(15 * time.Second),
		hosts:      soxHosts,
	}
}

// Name returns the provider name.
func (p *SOXIndexProvider) Name() string {
	return "sox_index"
}

// FetchSnapshot retrieves the latest ^SOX value and change percentage.
func (p *SOXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index rate limit: %w", err)
	}

	var lastErr error
	for _, host := range p.hosts {
		point, err := p.fetchFromHost(ctx, host)
		if err == nil {
			return point, nil
		}
		lastErr = err
	}
	return MacroDataSnapshot{}, fmt.Errorf("sox_index: all hosts failed: %w", lastErr)
}

func (p *SOXIndexProvider) fetchFromHost(ctx context.Context, host string) (MacroDataSnapshot, error) {
	url := fmt.Sprintf("https://%s/v8/finance/chart/%%5ESOX?interval=1d&range=2d", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("create request: %w", err)
	}

	ua := modernUserAgents[time.Now().UnixNano()%int64(len(modernUserAgents))]
	req.Header.Set("User-Agent", ua)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("http status %d from %s", resp.StatusCode, host)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("read body: %w", err)
	}

	if len(body) > 0 && body[0] == '<' {
		return MacroDataSnapshot{}, fmt.Errorf("HTML response from %s", host)
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("unmarshal: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("invalid latest price: %v", latest)
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
		return MacroDataSnapshot{}, fmt.Errorf("invalid change percentage: %v", changePct)
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
