package apigateway

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// Rate-limiting configuration for all 14 channels.
// These are hardcoded as required by the Constitution.
var (
	// YahooFinanceRate is shared between providers hitting Yahoo Finance v8 chart API.
	YahooFinanceRate  = rate.Every(1 * time.Second)
	YahooFinanceBurst = 1

	// TWSEOpenAPIRate: 5 req/sec per IP.
	TWSEOpenAPIRate  = rate.Every(200 * time.Millisecond)
	TWSEOpenAPIBurst = 5

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

	// GeopoliticalRate: RSS sources.
	GeopoliticalRate  = rate.Every(10 * time.Second)
	GeopoliticalBurst = 1

	// ExportStatisticsRate: manual triggers should not be overly restricted.
	ExportStatisticsRate  = rate.Every(5 * time.Second)
	ExportStatisticsBurst = 1

	// TEJRate: per-second + daily quota.
	TEJRate  = rate.Every(time.Second)
	TEJBurst = 1
)

// Shared limiters (same API endpoint)
var (
	// yahooSharedLimiter is used by providers that hit Yahoo Finance API.
	yahooSharedLimiter = rate.NewLimiter(YahooFinanceRate, YahooFinanceBurst)
)

// RateLimitManager manages all channel rate limiters.
type RateLimitManager struct {
	limiters map[string]*rate.Limiter
}

// NewRateLimitManager creates a manager with default limiters for all channels.
func NewRateLimitManager() *RateLimitManager {
	return &RateLimitManager{
		limiters: map[string]*rate.Limiter{
			"us_yahoo":            yahooSharedLimiter,
			"jpy_yahoo":           yahooSharedLimiter,
			"twse_replay":         rate.NewLimiter(rate.Inf, 0), // no limit for file-based
			"twse_capital_flow":   rate.NewLimiter(TWSECapitalFlowRate, TWSECapitalFlowBurst),
			"fugle":               rate.NewLimiter(FugleBasicRate, FugleBasicBurst),
			"fubon":               rate.NewLimiter(FugleBasicRate, FugleBasicBurst), // same tier
			"finmind":             rate.NewLimiter(FinMindFreeRate, FinMindFreeBurst),
			"geopolitical":        rate.NewLimiter(GeopoliticalRate, GeopoliticalBurst),
			"geopolitical_taiwan": rate.NewLimiter(GeopoliticalRate, GeopoliticalBurst),
			"twse_margin":         rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst),
			"export_statistics":   rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"tsmc_revenue":        rate.NewLimiter(FinMindFreeRate, FinMindFreeBurst), // inherits FinMind
			"janus_regime":        rate.NewLimiter(rate.Inf, 0),                       // no limit for compute
			"tej":                 rate.NewLimiter(TEJRate, TEJBurst),
			"exchange_rate":       rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"sox_index":           rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"sector_data":         rate.NewLimiter(rate.Inf, 0),
			"day_trading":         rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst), // same tier as TWSE margin
			"bdi":                 rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"dram_spot_price":     rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse_sector_index":   rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"taifex-daily":        rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse_oddlot":         rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
			"twse-etf":            rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
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
