package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const exchangeRateEndpoint = "https://open.er-api.com/v6/latest/USD"

// ExchangeRateProvider fetches exchange rates from the free ExchangeRate-API.
// Supports TWD (not available in ECB/Frankfurter dataset) and JPY.
// No API key required, rate-limited to ~1 request/minute on free tier.
// Historical rates require paid plan — ChangePct falls back to 0 with a
// warning log. Use FrankfurterFXProvider for daily change tracking on JPY.
type ExchangeRateProvider struct {
	client    *http.Client
	latestURL string
}

func NewExchangeRateProvider() *ExchangeRateProvider {
	return &ExchangeRateProvider{
		client:    httpclient.NewFactory().NewClient(10 * time.Second),
		latestURL: exchangeRateEndpoint,
	}
}

func (e *ExchangeRateProvider) Name() string {
	return "exchange_rate_api"
}

func (e *ExchangeRateProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.latestURL, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate read body: %w", err)
	}

	var fxResp exchangeRateResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate unmarshal: %w", err)
	}

	if fxResp.Result != "success" {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate API error: %s", fxResp.Result)
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}

	if twdRate, ok := fxResp.Rates["TWD"]; ok && twdRate > 0 {
		snap.USD_TWD = MacroDataPoint{
			Symbol:    "USD/TWD=X",
			Value:     twdRate,
			ChangePct: 0,
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Warn("exchangerate_provider", "missing_or_zero_rate", "currency", "TWD")
	}

	if jpyRate, ok := fxResp.Rates["JPY"]; ok && jpyRate > 0 {
		snap.JPY = MacroDataPoint{
			Symbol:    "JPY=X",
			Value:     jpyRate,
			ChangePct: 0,
			Timestamp: time.Now().Unix(),
		}
		logging.Info("exchangerate_provider", "jpy_change_pct_unavailable",
			"reason", "free tier lacks historical endpoint",
			"recommendation", "use FrankfurterFXProvider for daily change tracking")
	}

	return snap, nil
}

type exchangeRateResponse struct {
	Result   string             `json:"result"`
	BaseCode string             `json:"base_code"`
	Rates    map[string]float64 `json:"rates"`
}
