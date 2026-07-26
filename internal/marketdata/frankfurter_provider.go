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
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	frankfurterLatestURL = "https://api.frankfurter.app/latest?from=USD&to=JPY"
	frankfurterBaseURL   = "https://api.frankfurter.app"
)

type FrankfurterFXProvider struct {
	client    *http.Client
	latestURL string
	baseURL   string
}

func NewFrankfurterFXProvider() *FrankfurterFXProvider {
	return &FrankfurterFXProvider{
		client:    httpclient.NewFactory().NewClient(10 * time.Second),
		latestURL: frankfurterLatestURL,
		baseURL:   frankfurterBaseURL,
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (f *FrankfurterFXProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		f.client = client
	}
}

func (f *FrankfurterFXProvider) Name() string {
	return "frankfurter_fx"
}

func (f *FrankfurterFXProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	jpyRate, err := f.fetchRate(ctx, f.latestURL)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("frankfurter fetch latest: %w", err)
	}

	changePct := 0.0
	prevRate, _, prevErr := f.fetchPreviousBusinessDayRate(ctx, jpyRate)
	if prevErr != nil {
		logging.Warn("frankfurter_provider", "previous_rate_unavailable",
			"error", prevErr.Error(), "fallback", "ChangePct=0")
	} else if prevRate > 0 {
		changePct = (jpyRate - prevRate) / prevRate * 100
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}
	if jpyRate > 0 {
		snap.JPY = MacroDataPoint{
			Symbol:    "JPY=X",
			Value:     jpyRate,
			ChangePct: changePct,
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Warn("frankfurter_provider", "missing_or_zero_rate", "currency", "JPY")
	}

	return snap, nil
}

func (f *FrankfurterFXProvider) fetchRate(ctx context.Context, url string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var fxResp frankfurterResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return 0, fmt.Errorf("unmarshal: %w", err)
	}

	rate, ok := fxResp.Rates["JPY"]
	if !ok {
		return 0, fmt.Errorf("JPY rate missing in response")
	}
	return rate, nil
}

// fetchPreviousBusinessDayRate walks back up to 7 business days and
// returns the first rate that DIFFERS from currentRate (0.01 tolerance).
// Returns the earliest available rate if all match (pegged currency).
// On weekends/holidays, Frankfurter's /latest endpoint still shows the
// last trading day's rate, so simply walking back 1 day would return
// the same value (changePct=0). Comparing against the most recent
// DIFFERENT rate produces a meaningful daily-change signal.
func (f *FrankfurterFXProvider) fetchPreviousBusinessDayRate(ctx context.Context, currentRate float64) (float64, string, error) {
	var firstRate float64
	var firstDate string
	for i := 1; i <= 7; i++ {
		date := PreviousTradingDay(time.Now(), i)
		url := fmt.Sprintf("%s/%s?from=USD&to=JPY", f.baseURL, date.Format("2006-01-02"))
		rate, err := f.fetchRate(ctx, url)
		if err != nil {
			continue
		}
		if firstRate == 0 {
			firstRate = rate
			firstDate = date.Format("2006-01-02")
		}
		if math.Abs(rate-currentRate) > 0.01 {
			return rate, date.Format("2006-01-02"), nil
		}
	}
	if firstRate > 0 {
		return firstRate, firstDate, nil
	}
	return 0, "", fmt.Errorf("no historical rate found in past 7 days")
}

type frankfurterResponse struct {
	Date  string             `json:"date"`
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}
