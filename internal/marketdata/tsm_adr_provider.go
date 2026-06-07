package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// TSMADRProvider fetches TSMC ADR (TSM on NYSE) from Yahoo Finance.
type TSMADRProvider struct {
	session *yahooSession
}

// NewTSMADRProvider creates a new TSM ADR provider.
func NewTSMADRProvider() *TSMADRProvider {
	return &TSMADRProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *TSMADRProvider) Name() string {
	return "tsm_adr"
}

// FetchSnapshot retrieves the latest TSM (ADR) value and year-over-year change percentage.
func (p *TSMADRProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "1y",
	}

	body, err := p.session.fetchWithFallback(ctx, "TSM", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: invalid latest price: %v", latest)
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
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		TSMADR: MacroDataPoint{
			Symbol:    "TSM",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
