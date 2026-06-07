package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// OTCIndexProvider fetches the Taiwan OTC (Over-The-Counter) index from Yahoo Finance.
// Symbol: ^TWOII — the TPEx (Taipei Exchange) capitalization-weighted index
// covering Taiwan's small-to-medium cap stocks. Essential for measuring the
// non-TSMC, non-blue-chip segment of Taiwan's equity market.
//
// The OTC index serves as a critical divergence signal: when TAIEX rises
// (TSMC-driven) but OTC falls (broader market weakness), the gap signals
// concentration risk as documented in the 2024-2025 US-TW linkage report.
type OTCIndexProvider struct {
	session *yahooSession
}

// NewOTCIndexProvider creates a new OTC index provider.
func NewOTCIndexProvider() *OTCIndexProvider {
	return &OTCIndexProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *OTCIndexProvider) Name() string {
	return "otc_index"
}

// FetchSnapshot retrieves the latest ^TWOII value and year-over-year change percentage.
func (p *OTCIndexProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "1y",
	}

	body, err := p.session.fetchWithFallback(ctx, "^TWOII", params)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: %w", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: invalid latest price: %v", latest)
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
		return MacroDataSnapshot{}, fmt.Errorf("otc_index: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		TAIEX: MacroDataPoint{
			Symbol:    "^TWOII",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: result[0].Meta.RegularMarketTime,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
