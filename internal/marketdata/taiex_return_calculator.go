package marketdata

import (
	"context"
	"fmt"
	"math"
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
	return t.GetNDayReturn(ctx, 30)
}

// GetNDayReturn fetches the approximate N-day return of TAIEX (^TWII).
// Uses a single Yahoo Finance fetch (range=3mo or 1y) and parses the
// daily closes array to find both current and past prices. Result is
// cached via twiiCache (interval+range composite key) for 60 seconds.
//
// Precision: converts calendar days to approximate trading-day offset
// (days * 5/7). This trades ~1-2 day precision for a 50% reduction in
// Yahoo API calls (2→1) compared to the previous two-fetch approach.
func (t *TAIEXReturnCalculator) GetNDayReturn(ctx context.Context, days int) (float64, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limit: %w", err)
	}

	// Determine range. 3mo (~65 trading days) covers Get1MonthReturn and
	// most seasonal patterns. Fall back to 1y for longer windows.
	rangeStr := "3mo"
	tradingDayOffset := days * 5 / 7 // ~71% of calendar days
	if tradingDayOffset > 55 {
		rangeStr = "1y"
	}

	params := map[string]string{
		"interval": "1d",
		"range":    rangeStr,
	}

	// Check twiiCache (multi-entry, keyed by interval+range).
	var body []byte
	if cached := twiiCache.get(params["interval"], params["range"]); cached != nil {
		body = cached
	} else {
		var err error
		body, err = t.session.fetchWithFallback(ctx, "^TWII", params)
		if err != nil {
			return 0, fmt.Errorf("fetch TAIEX: %w", err)
		}
		twiiCache.set(body, params["interval"], params["range"])
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return 0, fmt.Errorf("parse TAIEX: %w", err)
	}

	if len(chartResp.Chart.Result) == 0 || len(chartResp.Chart.Result[0].Indicators.Quote) == 0 {
		return 0, fmt.Errorf("no chart data for ^TWII")
	}

	closes := chartResp.Chart.Result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return 0, fmt.Errorf("no close prices for ^TWII")
	}

	// Current price: last valid close.
	current := 0.0
	for i := len(closes) - 1; i >= 0; i-- {
		if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
			current = closes[i]
			break
		}
	}
	if current == 0 {
		return 0, fmt.Errorf("no valid current price")
	}

	// Past price: approximate N calendar days as trading-day offset.
	idx := max(len(closes)-1-tradingDayOffset, 0)
	past := closes[idx]
	if past <= 0 || math.IsNaN(past) || math.IsInf(past, 0) {
		// Fallback: scan backwards for the first valid price at or before the offset.
		for i := idx; i >= 0; i-- {
			if closes[i] > 0 && !math.IsNaN(closes[i]) && !math.IsInf(closes[i], 0) {
				past = closes[i]
				break
			}
		}
	}
	if past <= 0 || math.IsNaN(past) || math.IsInf(past, 0) {
		return 0, fmt.Errorf("no valid historical price for %d-day window", days)
	}

	return (current - past) / past, nil
}
