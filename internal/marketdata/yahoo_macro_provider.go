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

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
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

var yahooSharedLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

// YahooFinanceMacroProvider fetches macro indicators from Yahoo Finance.
type YahooFinanceMacroProvider struct {
	client    *http.Client
	baseURL   string
	limiter   *rate.Limiter
	bdiSymbol string // Yahoo Finance symbol for BDI (default: ^BDI, alternative: BDI, BALTIC)
}

func NewYahooFinanceMacroProvider() *YahooFinanceMacroProvider {
	return &YahooFinanceMacroProvider{
		client:    httpclient.NewFactory().NewClient(15 * time.Second),
		limiter:   yahooSharedLimiter,
		bdiSymbol: "^BDI",
	}
}

// SetBDISymbol overrides the default BDI symbol (^BDI). Use if Yahoo Finance
// does not serve ^BDI; common alternatives include "BDI" or "BALTT".
func (y *YahooFinanceMacroProvider) SetBDISymbol(ticker string) {
	y.bdiSymbol = ticker
}

func (y *YahooFinanceMacroProvider) Name() string {
	return "yahoo_finance"
}
func (y *YahooFinanceMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	symbols := map[string]string{
		"DX-Y.NYB":  "dxy",
		"^TNX":      "us10y",
		"^VIX":      "vix",
		"CL=F":      "oil",
		"GC=F":      "gold",
		"JPY=X":     "jpy",
		"USD/TWD=X": "usd_twd",
		y.bdiSymbol: "bdi",
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
			case "bdi":
				snap.BDI = point
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
	if err := y.limiter.Wait(ctx); err != nil {
		return MacroDataPoint{}, fmt.Errorf("rate limit: %w", err)
	}
	var lastErr error
	for _, host := range yahooHosts {
		point, err := y.fetchFromHost(ctx, host, ticker)
		if err == nil {
			return point, nil
		}
		lastErr = err
		logging.Warn("yahoo_macro_provider", "host_failed", "host", host, "error", err)
	}
	return MacroDataPoint{}, fmt.Errorf("all hosts failed for %s: %w", ticker, lastErr)
}

func (y *YahooFinanceMacroProvider) fetchFromHost(ctx context.Context, host, ticker string) (MacroDataPoint, error) {
	var u string
	if y.baseURL != "" {
		u = fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=2d", y.baseURL, ticker)
	} else {
		u = fmt.Sprintf("https://%s/v8/finance/chart/%s?interval=1d&range=2d", host, ticker)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return MacroDataPoint{}, err
	}
	ua := modernUserAgents[time.Now().UnixNano()%int64(len(modernUserAgents))]
	req.Header.Set("User-Agent", ua)

	resp, err := y.client.Do(req)
	if err != nil {
		return MacroDataPoint{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MacroDataPoint{}, fmt.Errorf("http status %d from %s", resp.StatusCode, host)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataPoint{}, err
	}

	if len(body) > 0 && body[0] == '<' {
		return MacroDataPoint{}, fmt.Errorf("HTML response from %s", host)
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return MacroDataPoint{}, fmt.Errorf("unmarshal: %w", err)
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
				RegularMarketTime  int64   `json:"regularMarketTime"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
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
