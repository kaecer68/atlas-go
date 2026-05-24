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

// TAIEXReturnCalculator calculates recent TAIEX returns using Yahoo Finance.
type TAIEXReturnCalculator struct {
	client *http.Client
	hosts  []string
}

// NewTAIEXReturnCalculator creates a new calculator.
func NewTAIEXReturnCalculator() *TAIEXReturnCalculator {
	return &TAIEXReturnCalculator{
		client: httpclient.NewFactory().NewClient(15 * time.Second),
		hosts:  yahooHosts,
	}
}

// Get1MonthReturn fetches the 1-month return of TAIEX (^TWII).
func (t *TAIEXReturnCalculator) Get1MonthReturn(ctx context.Context) (float64, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limit: %w", err)
	}

	current, err := t.fetchPrice(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch current price: %w", err)
	}

	past := t.fetchPastPrice(ctx, 30)
	if past <= 0 {
		return 0, fmt.Errorf("unable to fetch historical price")
	}

	return (current - past) / past, nil
}

func (t *TAIEXReturnCalculator) fetchPrice(ctx context.Context) (float64, error) {
	var lastErr error
	for _, host := range t.hosts {
		price, err := t.fetchPriceFromHost(ctx, host)
		if err == nil {
			return price, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("all hosts failed: %w", lastErr)
}

func (t *TAIEXReturnCalculator) fetchPriceFromHost(ctx context.Context, host string) (float64, error) {
	url := fmt.Sprintf("https://%s/v8/finance/chart/^TWII?interval=1d&range=1d", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	ua := modernUserAgents[time.Now().UnixNano()%int64(len(modernUserAgents))]
	req.Header.Set("User-Agent", ua)

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http status %d from %s", resp.StatusCode, host)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	if len(body) > 0 && body[0] == '<' {
		return 0, fmt.Errorf("HTML response from %s", host)
	}

	var result taiexChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Chart.Result) == 0 {
		return 0, fmt.Errorf("no chart data")
	}

	metaPrice := result.Chart.Result[0].Meta.RegularMarketPrice
	if metaPrice > 0 {
		return metaPrice, nil
	}

	if len(result.Chart.Result[0].Indicators.Quote) > 0 {
		closes := result.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
				return closes[i], nil
			}
		}
	}

	return 0, fmt.Errorf("no valid price data")
}

func (t *TAIEXReturnCalculator) fetchPastPrice(ctx context.Context, daysAgo int) float64 {
	for _, host := range t.hosts {
		price, err := t.fetchPastPriceFromHost(ctx, host, daysAgo)
		if err == nil {
			return price
		}
	}
	return 0
}

func (t *TAIEXReturnCalculator) fetchPastPriceFromHost(ctx context.Context, host string, daysAgo int) (float64, error) {
	period2 := time.Now().AddDate(0, 0, -daysAgo).Unix()
	url := fmt.Sprintf("https://%s/v8/finance/chart/^TWII?interval=1d&period2=%d", host, period2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	ua := modernUserAgents[time.Now().UnixNano()%int64(len(modernUserAgents))]
	req.Header.Set("User-Agent", ua)

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http status %d from %s", resp.StatusCode, host)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	if len(body) > 0 && body[0] == '<' {
		return 0, fmt.Errorf("HTML response from %s", host)
	}

	var result taiexChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal: %w", err)
	}

	if len(result.Chart.Result) == 0 {
		return 0, fmt.Errorf("no chart data")
	}

	metaPrice := result.Chart.Result[0].Meta.RegularMarketPrice
	if metaPrice > 0 {
		return metaPrice, nil
	}

	if len(result.Chart.Result[0].Indicators.Quote) > 0 {
		closes := result.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
				return closes[i], nil
			}
		}
	}

	return 0, fmt.Errorf("no valid close price")
}

type taiexChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}
