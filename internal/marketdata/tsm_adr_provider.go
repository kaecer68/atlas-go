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

// FetchSnapshot retrieves the latest TSM (ADR) value and daily change percentage.
func (p *TSMADRProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "5d",
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
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: invalid change percentage: %v", changePct)
	}

	// Reject implausible daily changes (typical TSM ADR daily range ±5%,
	// allowing ±30% as a conservative hard cap for extreme market events).
	if math.Abs(changePct) > maxDailyChangePct {
		return MacroDataSnapshot{}, fmt.Errorf("tsm_adr: implausible daily change %.2f%% (>|%.1f%%|)",
			changePct, maxDailyChangePct)
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
