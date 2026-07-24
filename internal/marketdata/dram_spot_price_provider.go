package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// DRAMSpotPriceProvider fetches Micron Technology (MU) stock price
// as a proxy for DRAM spot price trends. MU is the largest DRAM
// manufacturer and its stock price correlates ~85% with DRAM spot
// prices tracked by DRAMeXchange/InSpectrum.
//
// The ChangePct field reflects the day-over-day percentage change,
// which serves as a high-frequency DRAM price trend signal for the
// silicon cycle state machine.
type DRAMSpotPriceProvider struct {
	session *yahooSession
}

// NewDRAMSpotPriceProvider creates a new DRAM spot price provider
// using Micron Technology (MU) as the underlying proxy.
func NewDRAMSpotPriceProvider() *DRAMSpotPriceProvider {
	return &DRAMSpotPriceProvider{
		session: getYahooSession(),
	}
}

// Name returns the provider name.
func (p *DRAMSpotPriceProvider) Name() string {
	return "dram_spot_price"
}

// FetchSnapshot retrieves the latest MU price and daily change percentage.
func (p *DRAMSpotPriceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "2d",
	}

	// Check shared US market cache (P1 B04)
	var body []byte
	if cached := usCache.get("MU"); cached != nil {
		body = cached
	} else {
		var err error
		body, err = p.session.fetchWithFallback(ctx, "MU", params)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: %w", err)
		}
		usCache.set("MU", body)
	}
	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: %w", err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: no chart result")
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: invalid latest price: %v", latest)
	}

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
		return MacroDataSnapshot{}, fmt.Errorf("dram_spot_price: invalid change percentage: %v", changePct)
	}

	return MacroDataSnapshot{
		DRAMSpotPrice: MacroDataPoint{
			Symbol:    "MU",
			Value:     latest,
			ChangePct: math.Round(changePct*100) / 100,
			Timestamp: time.Now().Unix(),
		},
	}, nil
}
