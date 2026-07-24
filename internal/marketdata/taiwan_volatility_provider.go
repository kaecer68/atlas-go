package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// TaiwanVolatilityProvider computes historical volatility (volatility_20d) for TAIEX.
// It fetches ^TWII daily bars from Yahoo Finance and computes annualized volatility
// from 20-day log returns: std(log_returns_20d) * sqrt(252).
//
// Uses range=3mo to ensure at least 21 trading days (20 returns) are available.
// Falls back gracefully: if fewer than 21 bars are returned, volatility is reported
// as 0.0 with the underlying Yahoo fetch error logged.

const (
	taiwanVolatilitySymbol  = "^TWII"  // Yahoo ticker for TAIEX
	taiwanVolatilityChannel = "tw_vol" // data channel identifier
	taiwanVolatilityRange   = "3mo"    // enough bars for 20-day returns with buffer
)

// TaiwanVolatilityProvider implements MacroDataProvider for TAIEX historical volatility.
type TaiwanVolatilityProvider struct{}

// NewTaiwanVolatilityProvider creates a TAIEX volatility data provider.
func NewTaiwanVolatilityProvider() *TaiwanVolatilityProvider {
	return &TaiwanVolatilityProvider{}
}

// Name returns the data channel identifier.
func (p *TaiwanVolatilityProvider) Name() string { return taiwanVolatilityChannel }

// FetchSnapshot fetches ^TWII daily bars from Yahoo and computes historical volatility.
func (p *TaiwanVolatilityProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	s := getYahooSession()
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s rate limit: %w", taiwanVolatilityChannel, err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    taiwanVolatilityRange,
	}

	// Check shared cache first (P1 B03: avoids duplicate ^TWII fetch)
	var body []byte
	if cached := twiiCache.get(params["interval"], params["range"]); cached != nil {
		body = cached
	} else {
		var err error
		body, err = s.fetchWithFallback(ctx, taiwanVolatilitySymbol, params)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, err)
		}
		twiiCache.set(body, params["interval"], params["range"])
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no chart result", taiwanVolatilityChannel)
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) < 21 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: insufficient bars (%d, need >=21) for volatility_20d",
			taiwanVolatilityChannel, len(closes))
	}

	// Filter out NaN/Inf values
	validCloses := make([]float64, 0, len(closes))
	for _, c := range closes {
		if !math.IsNaN(c) && !math.IsInf(c, 0) && c > 0 {
			validCloses = append(validCloses, c)
		}
	}
	if len(validCloses) < 21 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: insufficient valid bars (%d) for volatility_20d",
			taiwanVolatilityChannel, len(validCloses))
	}

	// Compute annualized 20-day volatility: std(log_returns) * sqrt(252)
	vol := computeAnnualizedVolatility20D(validCloses)
	if math.IsNaN(vol) || math.IsInf(vol, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid volatility result: %v", taiwanVolatilityChannel, vol)
	}

	latest := validCloses[len(validCloses)-1]
	timestamp := result[0].Meta.RegularMarketTime

	return MacroDataSnapshot{
		RecordedAt: time.Now().Unix(),
		HistoricalVolatility: MacroDataPoint{
			Symbol:    taiwanVolatilitySymbol,
			Value:     latest,
			ChangePct: vol,
			Timestamp: timestamp,
		},
	}, nil
}

// computeAnnualizedVolatility20D calculates annualized volatility from 20-day log returns.
// Uses the same formula as feature.Registry["volatility_20d"] in internal/feature/feature.go:
//
//	log_returns[i] = ln(close[i] / close[i-1]) for i = len(closes)-20 .. len(closes)-1
//	std = sqrt(variance of log_returns)
//	annualized = std * sqrt(252)
func computeAnnualizedVolatility20D(closes []float64) float64 {
	n := len(closes)
	if n < 21 {
		return 0.0
	}

	lr := make([]float64, 20)
	for j := 0; j < 20; j++ {
		pos := n - 20 + j
		if closes[pos-1] > 0 && closes[pos] > 0 {
			lr[j] = math.Log(closes[pos] / closes[pos-1])
		}
	}

	meanLR := 0.0
	for _, v := range lr {
		meanLR += v
	}
	meanLR /= 20.0

	vr := 0.0
	for _, v := range lr {
		d := v - meanLR
		vr += d * d
	}
	std := math.Sqrt(vr / 20.0)
	return std * math.Sqrt(252)
}
