package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"
)

// NVDAProvider fetches NVIDIA Corp (NVDA) from Yahoo Finance.
type NVDAProvider struct {
	session *yahooSession
}

func NewNVDAProvider() *NVDAProvider {
	return &NVDAProvider{session: getYahooSession()}
}

func (p *NVDAProvider) Name() string { return "us_nvda" }

// FetchSnapshot retrieves the latest NVDA value and daily change percentage.
func (p *NVDAProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSTechSnapshot(ctx, "NVDA", "us_nvda", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.NVDA })
}

// AAPLProvider fetches Apple Inc (AAPL) from Yahoo Finance.
type AAPLProvider struct {
	session *yahooSession
}

func NewAAPLProvider() *AAPLProvider {
	return &AAPLProvider{session: getYahooSession()}
}

func (p *AAPLProvider) Name() string { return "us_aapl" }

// FetchSnapshot retrieves the latest AAPL value and daily change percentage.
func (p *AAPLProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSTechSnapshot(ctx, "AAPL", "us_aapl", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.AAPL })
}

// MSFTProvider fetches Microsoft Corp (MSFT) from Yahoo Finance.
type MSFTProvider struct {
	session *yahooSession
}

func NewMSFTProvider() *MSFTProvider {
	return &MSFTProvider{session: getYahooSession()}
}

func (p *MSFTProvider) Name() string { return "us_msft" }

// FetchSnapshot retrieves the latest MSFT value and daily change percentage.
func (p *MSFTProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return fetchUSTechSnapshot(ctx, "MSFT", "us_msft", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.MSFT })
}

// fetchUSTechSnapshot retrieves a US tech stock value and daily change percentage
// from Yahoo Finance. It follows the same 5-day-window / previous-close pattern
// used by TSMADRProvider.
func fetchUSTechSnapshot(ctx context.Context, ticker, channelName string, targetField func(*MacroDataSnapshot) *MacroDataPoint) (MacroDataSnapshot, error) {
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

	// Reject implausible daily changes (typical US tech daily range ±10%,
	// allowing ±30% as a conservative hard cap for extreme market events).
	if math.Abs(changePct) > maxDailyChangePct {
		return MacroDataSnapshot{}, fmt.Errorf("%s: implausible daily change %.2f%% (>|%.1f%%|)",
			channelName, changePct, maxDailyChangePct)
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}
	*targetField(&snap) = MacroDataPoint{
		Symbol:    ticker,
		Value:     latest,
		ChangePct: math.Round(changePct*100) / 100,
		Timestamp: result[0].Meta.RegularMarketTime,
	}
	return snap, nil
}
