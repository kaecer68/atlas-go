package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// TAIEXIndexProvider fetches the Taiwan Stock Exchange Capitalization Weighted
// Stock Index (TAIEX, 加權股價指數) from Yahoo Finance. Symbol: ^TWII.
type TAIEXIndexProvider struct {
	session *yahooSession
}

// NewTAIEXIndexProvider creates a new TAIEX index provider.
func NewTAIEXIndexProvider() *TAIEXIndexProvider {
	return &TAIEXIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *TAIEXIndexProvider) Name() string {
	return "taiex_index"
}

// FetchSnapshot retrieves the latest ^TWII value and change percentage.
// If Yahoo Finance fails, it falls back to the TWSE OpenAPI MI_INDEX endpoint.
// The TWSE response date is validated against the requested date so that
// previous-day data is never written as today's value.
func (p *TAIEXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	// Weekend/holiday gate: on non-trading days Yahoo Finance pads the latest
	// ^TWII close with 0 (parseYahooTAIEX treats 0 as invalid) and TWSE
	// MI_INDEX has no row for the current date, so the naive path returns an
	// error and trips the channel circuit breaker across a long weekend.
	// Serving the most recent trading day's close is the correct behavior and
	// must not be recorded as a failure, so bypass Yahoo entirely.
	now := twseTAIEXTargetDate()
	if !isTaiwanTradingDay(now) {
		return p.fetchFallback(ctx, fmt.Errorf("non-trading day %s (weekend/holiday)", now.Format("2006-01-02")))
	}

	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "3mo", // P1 B03: extended from 1mo to share cache with tw_vol
	}

	// Check shared cache first (P1 B03: avoids duplicate ^TWII fetch)
	var body []byte
	if cached := twiiCache.get(params["interval"], params["range"]); cached != nil {
		body = cached
	} else {
		var err error
		body, err = p.session.fetchWithFallback(ctx, "^TWII", params)
		if err != nil {
			// Primary source failed: fall back to TWSE official source.
			return p.fetchFallback(ctx, err)
		}
		twiiCache.set(body, params["interval"], params["range"])
	}

	pt, err := p.parseYahooTAIEX(body)
	if err != nil {
		return p.fetchFallback(ctx, err)
	}

	return MacroDataSnapshot{
		TAIEX:      pt,
		RecordedAt: time.Now().Unix(),
	}, nil
}

// parseYahooTAIEX extracts the TAIEX MacroDataPoint from a Yahoo chart response.
func (p *TAIEXIndexProvider) parseYahooTAIEX(body []byte) (MacroDataPoint, error) {
	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataPoint{}, err
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataPoint{}, errors.New("no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataPoint{}, errors.New("no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataPoint{}, fmt.Errorf("invalid latest price: %v", latest)
	}

	prev := latest
	if len(closes) >= 2 {
		prev = closes[len(closes)-2]
	}
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataPoint{}, fmt.Errorf("invalid change percentage: %v", changePct)
	}

	return MacroDataPoint{
		Symbol:    "^TWII",
		Value:     latest,
		ChangePct: math.Round(changePct*100) / 100,
		Timestamp: result[0].Meta.RegularMarketTime,
	}, nil
}

// fetchFallback attempts the TWSE official source and wraps both errors for visibility.
func (p *TAIEXIndexProvider) fetchFallback(ctx context.Context, yahooErr error) (MacroDataSnapshot, error) {
	pt, err := fetchTWSETAIEXFallback(ctx)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: yahoo failed (%w) and twse fallback failed: %w", yahooErr, err)
	}
	return MacroDataSnapshot{
		TAIEX:      pt,
		RecordedAt: time.Now().Unix(),
	}, nil
}
