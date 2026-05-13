package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// TAIEXReturnCalculator calculates recent TAIEX returns using Yahoo Finance.
type TAIEXReturnCalculator struct {
	client *http.Client
}

// NewTAIEXReturnCalculator creates a new calculator.
func NewTAIEXReturnCalculator() *TAIEXReturnCalculator {
	return &TAIEXReturnCalculator{
		client: httpclient.NewFactory().NewClient(15 * time.Second),
	}
}

// Get1MonthReturn fetches the 1-month return of TAIEX (^TWII).
func (t *TAIEXReturnCalculator) Get1MonthReturn(ctx context.Context) (float64, error) {
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
	url := "https://query1.finance.yahoo.com/v8/finance/chart/^TWII?interval=1d&range=1d"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var result taiexChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Chart.Result) == 0 {
		return 0, fmt.Errorf("no chart data")
	}

	prices := result.Chart.Result[0].Meta.RegularMarketPrice
	if prices > 0 {
		return prices, nil
	}

	if len(result.Chart.Result[0].Indicators.Quote) > 0 {
		closes := result.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 {
				return closes[i], nil
			}
		}
	}

	return 0, fmt.Errorf("no price data available")
}

func (t *TAIEXReturnCalculator) fetchPastPrice(ctx context.Context, daysAgo int) float64 {
	period2 := time.Now().AddDate(0, 0, -daysAgo).Unix()
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/^TWII?interval=1d&period2=%d", period2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	var result taiexChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0
	}

	if len(result.Chart.Result) == 0 {
		return 0
	}

	if len(result.Chart.Result[0].Indicators.Quote) > 0 {
		closes := result.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 {
				return closes[i]
			}
		}
	}

	return 0
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
