package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// SPXIndexProvider fetches the S&P 500 index (^GSPC) from Yahoo Finance.
type SPXIndexProvider struct{}

// NewSPXIndexProvider creates a new S&P 500 index provider.
func NewSPXIndexProvider() *SPXIndexProvider {
	return &SPXIndexProvider{}
}

// Name returns the provider name.
func (p *SPXIndexProvider) Name() string {
	return "us_spx"
}

// FetchSnapshot retrieves the latest ^GSPC value and daily change percentage.
func (p *SPXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSIndexSnapshot(ctx, "^GSPC", "us_spx", func(s *MacroDataSnapshot) *MacroDataPoint {
		return &s.SPXIndex
	})
}

// NDXIndexProvider fetches the Nasdaq Composite index (^IXIC) from Yahoo Finance.
type NDXIndexProvider struct{}

// NewNDXIndexProvider creates a new Nasdaq Composite index provider.
func NewNDXIndexProvider() *NDXIndexProvider {
	return &NDXIndexProvider{}
}

// Name returns the provider name.
func (p *NDXIndexProvider) Name() string {
	return "us_ndx"
}

// FetchSnapshot retrieves the latest ^IXIC value and daily change percentage.
func (p *NDXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSIndexSnapshot(ctx, "^IXIC", "us_ndx", func(s *MacroDataSnapshot) *MacroDataPoint {
		return &s.NDXIndex
	})
}

// DJIIndexProvider fetches the Dow Jones Industrial Average index (^DJI) from Yahoo Finance.
type DJIIndexProvider struct{}

// NewDJIIndexProvider creates a new Dow Jones index provider.
func NewDJIIndexProvider() *DJIIndexProvider {
	return &DJIIndexProvider{}
}

// Name returns the provider name.
func (p *DJIIndexProvider) Name() string {
	return "us_dji"
}

// FetchSnapshot retrieves the latest ^DJI value and daily change percentage.
func (p *DJIIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSIndexSnapshot(ctx, "^DJI", "us_dji", func(s *MacroDataSnapshot) *MacroDataPoint {
		return &s.DJIIndex
	})
}

// fetchUSIndexSnapshot retrieves a US index snapshot from Yahoo Finance and
// writes it into the field selected by targetField.
func fetchUSIndexSnapshot(ctx context.Context, ticker, channelName string, targetField func(*MacroDataSnapshot) *MacroDataPoint) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s rate limit: %w", channelName, err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "5d",
	}

	body, err := getYahooSession().fetchWithFallback(ctx, ticker, params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", channelName, err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", channelName, err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no chart result", channelName)
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no close prices", channelName)
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid latest price: %v", channelName, latest)
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
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid change percentage: %v", channelName, changePct)
	}

	// Reject implausible daily changes (typical US index daily range ±2%,
	// allowing ±30% as a conservative hard cap for extreme market events).
	const maxDailyChangePct = 30.0
	if math.Abs(changePct) > maxDailyChangePct {
		return MacroDataSnapshot{}, fmt.Errorf("%s: implausible daily change %.2f%% (>|%.1f%%|)",
			channelName, changePct, maxDailyChangePct)
	}

	snapshot := MacroDataSnapshot{}
	point := targetField(&snapshot)
	point.Symbol = ticker
	point.Value = latest
	point.ChangePct = math.Round(changePct*100) / 100
	point.Timestamp = result[0].Meta.RegularMarketTime
	snapshot.RecordedAt = time.Now().Unix()

	return snapshot, nil
}
