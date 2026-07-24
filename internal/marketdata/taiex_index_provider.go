package marketdata

import (
	"context"
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
func (p *TAIEXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
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
			return MacroDataSnapshot{}, fmt.Errorf("taiex_index: %w", err)
		}
		twiiCache.set(body, params["interval"], params["range"])
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: invalid latest price: %v", latest)
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
		return MacroDataSnapshot{}, fmt.Errorf("taiex_index: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		TAIEX: MacroDataPoint{
			Symbol:    "^TWII",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
