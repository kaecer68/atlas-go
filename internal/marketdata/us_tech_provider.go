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

func (p *NVDAProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda rate limit: %w", err)
	}
	body, err := p.session.fetchWithFallback(ctx, "NVDA", map[string]string{
		"interval": "1d", "range": "1y",
	})
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda: %w", err)
	}
	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda: %w", err)
	}
	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda: no chart result")
	}
	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda: no close prices")
	}
	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_nvda: invalid latest price: %v", latest)
	}
	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}
	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}
	return MacroDataSnapshot{
		NVDA: MacroDataPoint{
			Symbol:    "NVDA",
			Value:     latest,
			ChangePct: changePct,
			Timestamp: time.Now().Unix(),
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}

// AAPLProvider fetches Apple Inc (AAPL) from Yahoo Finance.
type AAPLProvider struct {
	session *yahooSession
}

func NewAAPLProvider() *AAPLProvider {
	return &AAPLProvider{session: getYahooSession()}
}

func (p *AAPLProvider) Name() string { return "us_aapl" }

func (p *AAPLProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl rate limit: %w", err)
	}
	body, err := p.session.fetchWithFallback(ctx, "AAPL", map[string]string{
		"interval": "1d", "range": "1y",
	})
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl: %w", err)
	}
	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl: %w", err)
	}
	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl: no chart result")
	}
	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl: no close prices")
	}
	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_aapl: invalid latest price: %v", latest)
	}
	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}
	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}
	return MacroDataSnapshot{
		AAPL: MacroDataPoint{
			Symbol:    "AAPL",
			Value:     latest,
			ChangePct: changePct,
			Timestamp: time.Now().Unix(),
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}

// MSFTProvider fetches Microsoft Corp (MSFT) from Yahoo Finance.
type MSFTProvider struct {
	session *yahooSession
}

func NewMSFTProvider() *MSFTProvider {
	return &MSFTProvider{session: getYahooSession()}
}

func (p *MSFTProvider) Name() string { return "us_msft" }

func (p *MSFTProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft rate limit: %w", err)
	}
	body, err := p.session.fetchWithFallback(ctx, "MSFT", map[string]string{
		"interval": "1d", "range": "1y",
	})
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft: %w", err)
	}
	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft: %w", err)
	}
	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft: no chart result")
	}
	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft: no close prices")
	}
	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("us_msft: invalid latest price: %v", latest)
	}
	prev := closes[0]
	if prev == 0 || math.IsNaN(prev) || math.IsInf(prev, 0) {
		prev = latest
	}
	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}
	return MacroDataSnapshot{
		MSFT: MacroDataPoint{
			Symbol:    "MSFT",
			Value:     latest,
			ChangePct: changePct,
			Timestamp: time.Now().Unix(),
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}
