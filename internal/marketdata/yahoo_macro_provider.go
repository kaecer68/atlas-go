package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// yahooHosts lists Yahoo Finance API hosts tried in order on failure.
var yahooHosts = []string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

// modernUserAgents holds recent Chrome User-Agent strings for rotation.
var modernUserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
}

var yahooSharedLimiter = rate.NewLimiter(rate.Every(500*time.Millisecond), 2)

// YahooFinanceMacroProvider fetches macro indicators from Yahoo Finance.
type YahooFinanceMacroProvider struct {
	session *yahooSession
	limiter *rate.Limiter
}

func NewYahooFinanceMacroProvider() *YahooFinanceMacroProvider {
	return &YahooFinanceMacroProvider{
		session: getYahooSession(),
		limiter: yahooSharedLimiter,
	}
}

func (y *YahooFinanceMacroProvider) Name() string {
	return "yahoo_finance"
}

func (y *YahooFinanceMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	// ^BDIY (Baltic Dry Index) is not available on Yahoo Finance.
	// See https://github.com/ranaroussi/yfinance/issues/1667
	symbols := map[string]string{
		"DX-Y.NYB": "dxy",
		"^TNX":     "us10y",
		"^VIX":     "vix",
		"CL=F":     "oil",
		"GC=F":     "gold",
		"JPY=X":    "jpy",
		"USDTWD=X": "usd_twd",
		"SI=F":     "silver",
		"HG=F":     "copper",
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	// Use a semaphore to limit concurrency and avoid Yahoo rate limiting.
	sem := make(chan struct{}, 3) // max 3 concurrent requests

	for ticker, key := range symbols {
		wg.Add(1)
		go func(ticker, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			point, err := y.fetchIndicator(ctx, ticker)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", ticker, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			switch key {
			case "dxy":
				snap.DXY = point
			case "us10y":
				snap.US10Y = point
			case "vix":
				snap.VIX = point
			case "oil":
				snap.Oil = point
			case "gold":
				snap.Gold = point
			case "jpy":
				snap.JPY = point
			case "usd_twd":
				snap.USD_TWD = point
			case "silver":
				snap.Silver = point
			case "copper":
				snap.Copper = point
			}
			mu.Unlock()
		}(ticker, key)
	}

	wg.Wait()

	if len(errs) > 0 {
		logging.Warn("yahoo_macro_provider", "partial_fetch_failures",
			"errors", fmt.Sprintf("%v", errs))
		if len(errs) == len(symbols) {
			return snap, fmt.Errorf("all indicators failed: %w", errors.Join(errs...))
		}
		return snap, errors.Join(errs...)
	}
	return snap, nil
}

func (y *YahooFinanceMacroProvider) fetchIndicator(ctx context.Context, ticker string) (MacroDataPoint, error) {
	if err := y.limiter.Wait(ctx); err != nil {
		return MacroDataPoint{}, fmt.Errorf("rate limit: %w", err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    "2d",
	}

	body, err := y.session.fetchWithFallback(ctx, ticker, params)
	if err != nil {
		return MacroDataPoint{}, err
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataPoint{}, fmt.Errorf("%s: %w", ticker, err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataPoint{}, fmt.Errorf("no chart result for %s", ticker)
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataPoint{}, fmt.Errorf("no close prices for %s", ticker)
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) {
		return MacroDataPoint{}, fmt.Errorf("invalid latest price for %s: %v", ticker, latest)
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
		return MacroDataPoint{}, fmt.Errorf("invalid change percentage for %s: %v", ticker, changePct)
	}

	point := MacroDataPoint{
		Symbol:    ticker,
		Value:     latest,
		ChangePct: changePct,
		Timestamp: result[0].Meta.RegularMarketTime,
	}

	if strings.Contains(ticker, "TNX") {
		// ^TNX is yield in percent; treat change as bps proxy.
		point.Value = changePct * 10 // rough proxy: 1% move = 100bps
	}

	return point, nil
}

// MockMacroProvider returns deterministic mock data for tests.
type MockMacroProvider struct {
	Snapshot MacroDataSnapshot
}

func (m *MockMacroProvider) Name() string { return "mock" }
func (m *MockMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return m.Snapshot, nil
}
