package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// SOXIndexProvider fetches the Philadelphia Semiconductor Index (^SOX) from Yahoo Finance.
type SOXIndexProvider struct {
	session *yahooSession
}

// NewSOXIndexProvider creates a new SOX index provider.
func NewSOXIndexProvider() *SOXIndexProvider {
	return &SOXIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *SOXIndexProvider) Name() string {
	return "sox_index"
}

// FetchSnapshot retrieves the latest ^SOX value and daily change percentage.
func (p *SOXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "5d",
	}

	body, err := p.session.fetchWithFallback(ctx, "^SOX", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: invalid latest price: %v", latest)
	}

	// Daily change: compare latest close to the previous trading day's close.
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
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: invalid change percentage: %v", changePct)
	}

	// Reject implausible daily changes (typical SOX daily range ±5%,
	// allowing ±30% as a conservative hard cap for extreme market events).
	if math.Abs(changePct) > maxDailyChangePct {
		return MacroDataSnapshot{}, fmt.Errorf("sox_index: implausible daily change %.2f%% (>|%.1f%%|)",
			changePct, maxDailyChangePct)
	}

	return MacroDataSnapshot{
		SOXIndex: MacroDataPoint{
			Symbol:    "^SOX",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
