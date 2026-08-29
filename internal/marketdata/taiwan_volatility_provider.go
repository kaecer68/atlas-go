package marketdata

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
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
type TaiwanVolatilityProvider struct {
	// history 是 TAIEX 每日 close 的 file-backed store（B02）。Yahoo 成功時
	// 寫入當日 close；Yahoo transport error 時 fallback 用 store 的 closes
	// 計算 20 日波動率（資料時間戳較舊但非 transport failure）。nil 時無
	// fallback（測試/未接線路徑，行為與舊版一致）。
	history *TaiwanIndexHistoryStore
}

// NewTaiwanVolatilityProvider creates a TAIEX volatility data provider
// without history fallback.
func NewTaiwanVolatilityProvider() *TaiwanVolatilityProvider {
	return &TaiwanVolatilityProvider{}
}

// NewTaiwanVolatilityProviderWithStore creates a provider wired to a
// TaiwanIndexHistoryStore at path for Yahoo-failure fallback (B02).
func NewTaiwanVolatilityProviderWithStore(path string) *TaiwanVolatilityProvider {
	return &TaiwanVolatilityProvider{
		history: NewTaiwanIndexHistoryStore(path),
	}
}

// Name returns the data channel identifier.
func (p *TaiwanVolatilityProvider) Name() string { return taiwanVolatilityChannel }

// FetchSnapshot fetches ^TWII daily bars from Yahoo and computes historical volatility.
//
// PR-B (kaecer 2026-08-05): handles the "stale cache, fresh data available" race
// where the 60s TTL is still valid but RegularMarketTime is from a previous
// trading day. In that case the stale entry is invalidated and a single refetch
// is attempted. If the refetched response is also stale (Yahoo itself lagging,
// or upstream broken), the error is reported with a clear message identifying
// both the cache and refetch failure modes.
//
// Refetched-but-stale data is NEVER written back to the cache, so the next
// FetchSnapshot call gets a clean state (cache miss → fresh fetch → success).
func (p *TaiwanVolatilityProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	s := getYahooSession()
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s rate limit: %w", taiwanVolatilityChannel, err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    taiwanVolatilityRange,
	}

	var (
		body      []byte
		fromCache bool
		refetched bool
	)

	// Loop bounded to 2 iterations: initial fetch + (optional) one refetch
	// after stale-cache invalidation. A refetch that is also stale returns
	// the error directly, so the loop never spins.
	for {
		body = nil
		fromCache = false
		if cached := twiiCache.get(params["interval"], params["range"]); cached != nil {
			body = cached
			fromCache = true
		} else {
			fetched, err := s.fetchWithFallback(ctx, taiwanVolatilitySymbol, params)
			if err != nil {
				// B02：Yahoo transport error → 嘗試從 history store fallback
				// （資料時間戳較舊但非 transport failure；store 無足夠資料
				// 或未接線時回原 error）。
				return p.fallbackFromHistory(err)
			}
			body = fetched
		}

		chartResp, err := UnmarshalYahooChart(body)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, err)
		}

		result := chartResp.Chart.Result
		if len(result) == 0 {
			return MacroDataSnapshot{}, fmt.Errorf("%s: no chart result", taiwanVolatilityChannel)
		}

		timestamp := result[0].Meta.RegularMarketTime
		if twiiCacheTimestampIsCurrentTradingDay(timestamp) {
			// Acceptable. Write to cache only if we fetched it; never
			// overwrite with stale data.
			if !fromCache {
				twiiCache.set(body, params["interval"], params["range"])
			}
			break
		}

		// Stale timestamp. If we've already refetched once, both cache
		// and refetch are stale — the upstream has not yet published the
		// latest trading day's bar (pre-market or upstream lag).
		//
		// A04（2026-08-10 audit）：此情況是「資料時間戳較舊」，不是 transport
		// failure，不應觸發 circuit breaker。用最新可用 bars 計算波動率並
		// 回傳，Timestamp 保留 Yahoo 的 RegularMarketTime 讓下游可見資料
		// 時間；transport error（DNS/timeout/HTTP 5xx）仍走上方 error 分支。
		if refetched {
			logging.Warn(taiwanVolatilityChannel, "stale_data_accepted",
				"regular_market_time", timestamp,
				"note", "refetch also stale; using latest available bars (not a transport failure)")
			break
		}

		// First attempt: invalidate the stale entry and refetch once.
		twiiCache.invalidate(params["interval"], params["range"])
		refetched = true
	}

	// `body` is acceptable from this point forward.
	chartResp, _ := UnmarshalYahooChart(body)
	result := chartResp.Chart.Result
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
	latest := validCloses[len(validCloses)-1]
	timestamp := result[0].Meta.RegularMarketTime

	vol := computeAnnualizedVolatility20D(validCloses)
	if math.IsNaN(vol) || math.IsInf(vol, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid volatility result: %v", taiwanVolatilityChannel, vol)
	}

	// B02：Yahoo 資料可用時寫入 history store（同交易日覆寫），供未來
	// Yahoo 失效時 fallback。timestamp 為 Yahoo RegularMarketTime（UTC）。
	if p.history != nil {
		p.history.Append(time.Unix(timestamp, 0), latest)
	}

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

// fallbackFromHistory 在 Yahoo transport error 時用 history store 的 TAIEX
// daily closes 計算 20 日波動率（B02）。store 未接線或 closes < 21 筆時回
// 原 error。Timestamp 標記為最後一筆 store 資料的日期（非當前），下游可見
// 資料時間戳較舊。
func (p *TaiwanVolatilityProvider) fallbackFromHistory(upstreamErr error) (MacroDataSnapshot, error) {
	if p.history == nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, upstreamErr)
	}
	closes := p.history.RecentCloses(21)
	if len(closes) < 21 {
		logging.Warn(taiwanVolatilityChannel, "history_fallback_insufficient",
			"closes", len(closes), "need", 21)
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, upstreamErr)
	}
	vol := computeAnnualizedVolatility20D(closes)
	if math.IsNaN(vol) || math.IsInf(vol, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, upstreamErr)
	}
	lastDate, ok := p.history.LastDate()
	if !ok {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", taiwanVolatilityChannel, upstreamErr)
	}
	logging.Warn(taiwanVolatilityChannel, "history_fallback_used",
		"note", "Yahoo down; computed from persisted TAIEX closes",
		"last_date", lastDate.Format("2006-01-02"))
	return MacroDataSnapshot{
		RecordedAt: time.Now().Unix(),
		HistoricalVolatility: MacroDataPoint{
			Symbol:    taiwanVolatilitySymbol,
			Value:     closes[len(closes)-1],
			ChangePct: vol,
			Timestamp: lastDate.Unix(),
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
	for j := range 20 {
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
