package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// TAIEXReturnCalculator calculates recent TAIEX returns using Yahoo Finance.
type TAIEXReturnCalculator struct {
	session *yahooSession
}

// NewTAIEXReturnCalculator creates a new calculator.
func NewTAIEXReturnCalculator() *TAIEXReturnCalculator {
	return &TAIEXReturnCalculator{
		session: getYahooSession(),
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
	params := map[string]string{
		"interval": "1d",
		"range":    "1d",
	}

	body, err := t.session.fetchWithFallback(ctx, "^TWII", params)
	if err != nil {
		return 0, err
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return 0, err
	}

	if len(chartResp.Chart.Result) == 0 {
		return 0, fmt.Errorf("no chart data")
	}

	metaPrice := chartResp.Chart.Result[0].Meta.RegularMarketPrice
	if metaPrice > 0 {
		return metaPrice, nil
	}

	if len(chartResp.Chart.Result[0].Indicators.Quote) > 0 {
		closes := chartResp.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
				return closes[i], nil
			}
		}
	}

	return 0, fmt.Errorf("no valid price data")
}

func (t *TAIEXReturnCalculator) fetchPastPrice(ctx context.Context, daysAgo int) float64 {
	params := map[string]string{
		"interval": "1d",
		"period1":  fmt.Sprintf("%d", time.Now().AddDate(0, 0, -daysAgo-5).Unix()),
		"period2":  fmt.Sprintf("%d", time.Now().AddDate(0, 0, -daysAgo+1).Unix()),
	}

	body, err := t.session.fetchWithFallback(ctx, "^TWII", params)
	if err != nil {
		return 0
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return 0
	}

	if len(chartResp.Chart.Result) == 0 {
		return 0
	}

	metaPrice := chartResp.Chart.Result[0].Meta.RegularMarketPrice
	if metaPrice > 0 {
		return metaPrice
	}

	if len(chartResp.Chart.Result[0].Indicators.Quote) > 0 {
		closes := chartResp.Chart.Result[0].Indicators.Quote[0].Close
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
				return closes[i]
			}
		}
	}

	return 0
}
