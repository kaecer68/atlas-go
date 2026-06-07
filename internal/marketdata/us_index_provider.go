package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// SPXIndexProvider fetches the S&P 500 index (^GSPC) from Yahoo Finance.
type SPXIndexProvider struct {
	session *yahooSession
}

// NewSPXIndexProvider creates a new S&P 500 index provider.
func NewSPXIndexProvider() *SPXIndexProvider {
	return &SPXIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *SPXIndexProvider) Name() string {
	return "us_spx"
}

// FetchSnapshot retrieves the latest ^GSPC value and year-over-year change percentage.
func (p *SPXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "1y",
	}

	body, err := p.session.fetchWithFallback(ctx, "^GSPC", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: invalid latest price: %v", latest)
	}

	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("us_spx: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		SPXIndex: MacroDataPoint{
			Symbol:    "^GSPC",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}

// NDXIndexProvider fetches the Nasdaq Composite index (^IXIC) from Yahoo Finance.
type NDXIndexProvider struct {
	session *yahooSession
}

// NewNDXIndexProvider creates a new Nasdaq Composite index provider.
func NewNDXIndexProvider() *NDXIndexProvider {
	return &NDXIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *NDXIndexProvider) Name() string {
	return "us_ndx"
}

// FetchSnapshot retrieves the latest ^IXIC value and year-over-year change percentage.
func (p *NDXIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "1y",
	}

	body, err := p.session.fetchWithFallback(ctx, "^IXIC", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: invalid latest price: %v", latest)
	}

	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("us_ndx: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		NDXIndex: MacroDataPoint{
			Symbol:    "^IXIC",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}

// DJIIndexProvider fetches the Dow Jones Industrial Average index (^DJI) from Yahoo Finance.
type DJIIndexProvider struct {
	session *yahooSession
}

// NewDJIIndexProvider creates a new Dow Jones index provider.
func NewDJIIndexProvider() *DJIIndexProvider {
	return &DJIIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *DJIIndexProvider) Name() string {
	return "us_dji"
}

// FetchSnapshot retrieves the latest ^DJI value and year-over-year change percentage.
func (p *DJIIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "1y",
	}

	body, err := p.session.fetchWithFallback(ctx, "^DJI", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: invalid latest price: %v", latest)
	}

	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("us_dji: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		DJIIndex: MacroDataPoint{
			Symbol:    "^DJI",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
