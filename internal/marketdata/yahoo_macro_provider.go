package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// YahooFinanceMacroProvider fetches macro indicators from Yahoo Finance.
type YahooFinanceMacroProvider struct {
	client *http.Client
}

// NewYahooFinanceMacroProvider creates a new Yahoo Finance macro provider.
func NewYahooFinanceMacroProvider() *YahooFinanceMacroProvider {
	return &YahooFinanceMacroProvider{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the provider name.
func (y *YahooFinanceMacroProvider) Name() string {
	return "yahoo_finance"
}

// FetchSnapshot retrieves DXY, ^TNX, VIX, Oil, Gold, JPY, USD/TWD from Yahoo Finance concurrently.
func (y *YahooFinanceMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	symbols := map[string]string{
		"DX-Y.NYB":  "dxy",
		"^TNX":      "us10y",
		"^VIX":      "vix",
		"CL=F":      "oil",
		"GC=F":      "gold",
		"JPY=X":     "jpy",
		"USD/TWD=X": "usd_twd",
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	for ticker, key := range symbols {
		wg.Add(1)
		go func(ticker, key string) {
			defer wg.Done()
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
			}
			mu.Unlock()
		}(ticker, key)
	}

	wg.Wait()

	if len(errs) > 0 {
		logging.Warn("yahoo_macro_provider", "partial_fetch_failures", "errors", fmt.Sprintf("%v", errs))
		if len(errs) == len(symbols) {
			return snap, fmt.Errorf("all indicators failed: %w", errors.Join(errs...))
		}
		return snap, errors.Join(errs...)
	}
	return snap, nil
}

func (y *YahooFinanceMacroProvider) fetchIndicator(ctx context.Context, ticker string) (MacroDataPoint, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=2d", ticker)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MacroDataPoint{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return MacroDataPoint{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataPoint{}, err
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return MacroDataPoint{}, err
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataPoint{}, fmt.Errorf("no chart result")
	}

	meta := result[0].Meta
	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataPoint{}, fmt.Errorf("no close prices")
	}

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) {
		return MacroDataPoint{}, fmt.Errorf("invalid latest price: %v", latest)
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
		return MacroDataPoint{}, fmt.Errorf("invalid change percentage: %v", changePct)
	}

	point := MacroDataPoint{
		Symbol:    ticker,
		Value:     latest,
		ChangePct: changePct,
		Timestamp: meta.RegularMarketTime,
	}

	if strings.Contains(ticker, "TNX") {
		// ^TNX is yield in percent; treat change as bps proxy.
		point.Value = changePct * 10 // rough proxy: 1% move = 100bps
	}

	return point, nil
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketTime int64 `json:"regularMarketTime"`
			} `json:"meta"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}

// MockMacroProvider returns deterministic mock data for tests.
type MockMacroProvider struct {
	Snapshot MacroDataSnapshot
}

func (m *MockMacroProvider) Name() string { return "mock" }
func (m *MockMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return m.Snapshot, nil
}
