package apigateway

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// Rate-limiting configuration for all 14 channels.
// These are hardcoded as required by the Constitution.
var (
	// YahooFinanceRate is the rate for Yahoo Finance macro channel (us_yahoo).
	// Yahoo Finance v8 chart API allows several requests per second, but we
	// stay conservative at 1 req/s for the macro channel because each call
	// fetches a batch of 9 macro indicators (VIX, DXY, US10Y, USD_TWD, Oil,
	// Gold, Silver, Copper, JPY) and we want predictable server behavior.
	YahooFinanceRate  = rate.Every(5 * time.Second)
	YahooFinanceBurst = 2

	// YahooIndexRate is the rate for US index channels (us_spx, us_ndx, us_dji).
	// 1 req/1.5s per channel — 3 channels share this limiter but do not serialize
	// against the macro or tech groups. Cold start: 0s, 1.5s, 3s (≈3s for 3 channels).
	YahooIndexRate  = rate.Every(1500 * time.Millisecond)
	YahooIndexBurst = 1

	// YahooTechRate is the rate for US tech stock + TSM ADR channels
	// (us_nvda, us_aapl, us_msft, tsm_adr).
	// 1 req/1.5s per channel — 4 channels share this limiter. Cold start:
	// 0s, 1.5s, 3s, 4.5s (≈4.5s for 4 channels). Combined with YahooIndexRate
	// (parallel group), all 7 US market channels complete in under 5s on first
	// refresh, instead of the previous serialized 7+ seconds.
	YahooTechRate  = rate.Every(1500 * time.Millisecond)
	YahooTechBurst = 1

	// TWSECapitalFlowRate: conservative.
	TWSECapitalFlowRate  = rate.Every(5 * time.Second)
	TWSECapitalFlowBurst = 1

	// TWSEMarginRate: same conservative approach.
	TWSEMarginRate  = rate.Every(5 * time.Second)
	TWSEMarginBurst = 1

	// FinMindFreeRate: free tier 600/hr.
	FinMindFreeRate  = rate.Every(6 * time.Second)
	FinMindFreeBurst = 1

	// FinMindPaidRate: paid tier 2000/hr.
	FinMindPaidRate  = rate.Every(1*time.Second + 800*time.Millisecond)
	FinMindPaidBurst = 1

	// FugleBasicRate: Basic tier 60/min.
	FugleBasicRate  = rate.Every(time.Second)
	FugleBasicBurst = 1

	// FrankfurterFXRate: Frankfurter API (api.frankfurter.app). Free tier, no auth.
	FrankfurterFXRate  = rate.Every(10 * time.Second)
	FrankfurterFXBurst = 1

	// GeopoliticalRate: RSS sources (1 req/min to avoid overwhelming feeds).
	GeopoliticalRate  = rate.Every(time.Minute)
	GeopoliticalBurst = 1

	// ExportStatisticsRate: manual triggers should not be overly restricted.
	ExportStatisticsRate  = rate.Every(5 * time.Second)
	ExportStatisticsBurst = 1

	// TEJRate: per-second + daily quota.
	TEJRate  = rate.Every(time.Second)
	TEJBurst = 1
)

// Group-scoped limiters for Yahoo Finance channels.
//
// History: all 8 Yahoo channels previously shared a single 1 req/s limiter
// (yahooSharedLimiter), which serialized the 8-channel us_market_refresh
// batch into an 8+ second cold start. Per Constitution Art. 2.3 ("共享
// limiter 用同一個 instance；不同 endpoint 可獨立 limiter"), we treat
// macro / index / tech as distinct endpoint groups — each is independently
// rate-limited so cold start and steady-state traffic are parallelized
// across the three groups.
var (
	// yahooMacroLimiter covers the macro channel (us_yahoo) which fetches
	// 9 indicators in one batch call. 1 req/s preserves existing semantics.
	yahooMacroLimiter = rate.NewLimiter(YahooFinanceRate, YahooFinanceBurst)

	// yahooIndexLimiter covers the 3 US index channels (us_spx, us_ndx, us_dji).
	yahooIndexLimiter = rate.NewLimiter(YahooIndexRate, YahooIndexBurst)

	// yahooTechLimiter covers the 4 US tech stock + ADR channels
	// (us_nvda, us_aapl, us_msft, tsm_adr).
	yahooTechLimiter = rate.NewLimiter(YahooTechRate, YahooTechBurst)

	// taiexIndexLimiter covers the Taiwan weighted index channel (taiex_index).
	taiexIndexLimiter = rate.NewLimiter(rate.Every(5*time.Second), 1)
)

// RateLimitManager manages all channel rate limiters.
type RateLimitManager struct {
	limiters map[string]*rate.Limiter
}

// NewRateLimitManager creates a manager with default limiters for all channels.
func NewRateLimitManager() *RateLimitManager {
	return &RateLimitManager{
		limiters: map[string]*rate.Limiter{
			"us_yahoo":               yahooMacroLimiter,
			"frankfurter_fx":         rate.NewLimiter(FrankfurterFXRate, FrankfurterFXBurst),
			"twse_replay":            rate.NewLimiter(rate.Inf, 0), // no limit for file-based
			"twse_capital_flow":      rate.NewLimiter(TWSECapitalFlowRate, TWSECapitalFlowBurst),
			"fugle":                  rate.NewLimiter(FugleBasicRate, FugleBasicBurst),
			"fubon":                  rate.NewLimiter(FugleBasicRate, FugleBasicBurst), // same tier
			"finmind":                rate.NewLimiter(FinMindFreeRate, FinMindFreeBurst),
			"geopolitical":           rate.NewLimiter(GeopoliticalRate, GeopoliticalBurst),
			"geopolitical_taiwan":    rate.NewLimiter(GeopoliticalRate, GeopoliticalBurst),
			"twse_margin":            rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst),
			"export_statistics":      rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"tsmc_revenue":           rate.NewLimiter(rate.Every(2*time.Minute), 1), // TSMC monthly revenue
			"janus_regime":           rate.NewLimiter(rate.Inf, 0),                  // no limit for compute
			"tej":                    rate.NewLimiter(TEJRate, TEJBurst),
			"exchange_rate":          rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"sox_index":              rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"tw_vol":                 rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst), // ^TWII 3mo bars → volatility_20d
			"sector_data":            rate.NewLimiter(rate.Inf, 0),
			"day_trading":            rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst), // same tier as TWSE margin
			"market_volume":          rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst), // same tier as TWSE margin/day_trading
			"bdi":                    rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"dram_spot_price":        rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse_sector_index":      rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"taifex_daily":           rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"taifex_institutional":   rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse_oddlot":            rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse_etf":               rate.NewLimiter(rate.Every(1*time.Second), 1), // adapter ground truth
			"twse_sbl":               rate.NewLimiter(rate.Every(2*time.Second), 1), // G02: TWSE SBL daily
			"twse_insider":           rate.NewLimiter(rate.Every(5*time.Second), 1), // TWSE OpenAPI t187ap12_L
			"tdcc_equity_dispersion": rate.NewLimiter(rate.Every(5*time.Second), 1), // G01: TDCC weekly
			"government_flow":        rate.NewLimiter(rate.Inf, 0),                  // file-backed, no upstream HTTP
			// US indexes + tech stocks + TSM ADR each use a group-scoped limiter
			// (see yahooIndexLimiter / yahooTechLimiter / taiexIndexLimiter above) so the 9-channel
			// us_market_refresh batch does not serialize at 1 req/s.
			"us_spx":      yahooIndexLimiter,
			"us_ndx":      yahooIndexLimiter,
			"us_dji":      yahooIndexLimiter,
			"taiex_index": taiexIndexLimiter,
			"us_nvda":     yahooTechLimiter,
			"us_aapl":     yahooTechLimiter,
			"us_msft":     yahooTechLimiter,
			"tsm_adr":     yahooTechLimiter,
		},
	}
}

// Get returns the limiter for a channel.
func (m *RateLimitManager) Get(channelID string) (*rate.Limiter, error) {
	l, ok := m.limiters[channelID]
	if !ok {
		return nil, fmt.Errorf("unknown channel: %s", channelID)
	}
	return l, nil
}

// Register adds or replaces a limiter for a channel.
func (m *RateLimitManager) Register(channelID string, limiter *rate.Limiter) {
	m.limiters[channelID] = limiter
}

// Remaining returns the approximate remaining tokens for a channel.
func (m *RateLimitManager) Remaining(channelID string) (float64, error) {
	l, err := m.Get(channelID)
	if err != nil {
		return 0, err
	}
	return l.Tokens(), nil
}

// Status returns rate limit status for all channels.
func (m *RateLimitManager) Status() map[string]RateLimitStatus {
	result := make(map[string]RateLimitStatus)
	for id, l := range m.limiters {
		result[id] = RateLimitStatus{
			ChannelID: id,
			Remaining: l.Tokens(),
			Burst:     l.Burst(),
		}
	}
	return result
}

// RateLimitStatus represents the current rate limit state.
type RateLimitStatus struct {
	ChannelID string  `json:"channel_id"`
	Remaining float64 `json:"remaining"`
	Burst     int     `json:"burst"`
}
