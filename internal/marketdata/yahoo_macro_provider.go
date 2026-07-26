package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math"
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

var yahooSharedLimiter = rate.NewLimiter(rate.Every(100*time.Millisecond), 5)

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
	// JPY=X has been removed from this provider — frankfurter_fx channel
	// is now the sole authoritative source for USD/JPY via api.frankfurter.app.
	symbols := map[string]string{
		"DX-Y.NYB": "dxy",
		"^TNX":     "us10y",
		"^VIX":     "vix",
		"CL=F":     "oil",
		"GC=F":     "gold",
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
		// range=1mo (vs 5d) ensures Yahoo Finance returns ≥2 daily closes for
		// sparse forex tickers (USDTWD=X, USDMXN=X, etc.) where range=5d
		// sometimes returns only the latest close — making ChangePct=0 and
		// breaking USD_TWD routing downstream (see decision
		// 2026-07-13-usd-twd-routing-recurring-bug-root-cause.md).
		"range": "1mo",
	}

	// Check shared US market cache (P2: integrates with Yahoo fetch cache layer).
	var body []byte
	if cached := usCache.get(ticker, params["interval"], params["range"]); cached != nil {
		body = cached
	} else {
		var err error
		body, err = y.session.fetchWithFallback(ctx, ticker, params)
		if err != nil {
			return MacroDataPoint{}, err
		}
		usCache.set(ticker, params["interval"], params["range"], body)
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

	var prev float64
	// 零值防禦 v2: 零值不再是錯誤 — 改從 closes 陣列末尾往回找上一個非零值。
	// Yahoo Finance 在非 US 交易時段對 forex/commodity tickers
	// (DX-Y.NYB, CL=F, GC=F, etc.) 可能回傳 closes: [0.0, ..., 0.0]，
	// 但更早的歷史資料中仍有上一個交易日的有效收盤價(range=1mo)。
	// 只有當 closes 陣列中完全沒有非零值時才拒絕。
	if latest == 0 {
		latest, prev = findLastValidClose(closes)
		if latest == 0 {
			return MacroDataPoint{}, fmt.Errorf("zero latest price for %s (all closes zero)", ticker)
		}
		logging.Warn("yahoo_macro_provider", "zero_fallback",
			"ticker", ticker,
			"fallback_value", fmt.Sprintf("%.4f", latest))
	} else {
		prev = latest
		if len(closes) > 1 {
			candidate := closes[len(closes)-2]
			if !math.IsNaN(candidate) && !math.IsInf(candidate, 0) && candidate != 0 {
				prev = candidate
			}
		}
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataPoint{}, fmt.Errorf("invalid change percentage for %s: %v", ticker, changePct)
	}
	// Bounds check: per marketdata/AGENTS.md, daily moves > ±30% are data
	// errors (stock split, parser glitch, forex holiday gap). Reject so we
	// don't pollute stress-index / yield-spread / US-TW spread downstream.
	if math.Abs(changePct) > 30 {
		return MacroDataPoint{}, fmt.Errorf("outlier daily change for %s: %.2f%% (likely data error)", ticker, changePct)
	}

	point := MacroDataPoint{
		Symbol:    ticker,
		Value:     latest,
		ChangePct: changePct,
		Timestamp: result[0].Meta.RegularMarketTime,
	}

	return point, nil
}

// findLastValidClose walks backwards through closes to find the last two
// non-zero, non-NaN values. Returns (0, 0) if none found. This is used when
// the latest close is zero (Yahoo Finance off-hours) — we fall back to the
// most recent valid trading-day close from the historical data (range=1mo).
func findLastValidClose(closes []float64) (latest, prev float64) {
	for i := len(closes) - 1; i >= 0; i-- {
		v := closes[i]
		if math.IsNaN(v) || math.IsInf(v, 0) || v == 0 {
			continue
		}
		if latest == 0 {
			latest = v
		} else {
			prev = v
			return
		}
	}
	return
}

// MockMacroProvider returns deterministic mock data for tests.
type MockMacroProvider struct {
	Snapshot MacroDataSnapshot
	Err      error
}

func (m *MockMacroProvider) Name() string { return "mock" }
func (m *MockMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return m.Snapshot, m.Err
}
