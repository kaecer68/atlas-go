package domain

import "time"

// RetailSentimentSnapshot captures retail investor sentiment metrics for Taiwan equities.
type RetailSentimentSnapshot struct {
	MarginBalance    int64     `json:"margin_balance"`    // in hundred millions TWD
	MarginChangePct  float64   `json:"margin_change_pct"` // day-over-day change
	DayTradingRatio  float64   `json:"day_trading_ratio"` // 0.0 - 1.0
	RetailFuturesOI  int64     `json:"retail_futures_oi"` // small TAIEX futures OI
	MarginPercentile float64   `json:"margin_percentile"` // 0.0 - 1.0 historical percentile
	SentimentScore   float64   `json:"sentiment_score"`   // -1.0 (fear) to +1.0 (frenzy)
	Timestamp        time.Time `json:"timestamp"`
}

// CalculateSentimentScore maps margin percentile to a -1..+1 sentiment score.
func (rs *RetailSentimentSnapshot) CalculateSentimentScore() float64 {
	rs.SentimentScore = (rs.MarginPercentile - 0.5) * 2.0
	return rs.SentimentScore
}

// ExtremeReading returns "frenzy", "fear", or "neutral" based on percentile thresholds.
func (rs *RetailSentimentSnapshot) ExtremeReading() string {
	if rs.MarginPercentile >= 0.90 {
		return "frenzy"
	}
	if rs.MarginPercentile <= 0.10 {
		return "fear"
	}
	return "neutral"
}
