package domain

import "time"

// 連續契約（continuous futures series）的領域型別。
//
// 名詞（見 docs/specs/futures-bars-firstparty-spec.md §6）：
//   - 前月（front month）：任一交易日中，到期月最近且尚未到期的月契約。
//   - 換倉日（roll date）：某月契約的最後交易日（表定＝該月第三個星期三，遇假順延）。
//   - 段（segment）：某一契約作為前月的連續交易日區間。

// AdjustMethod 是連續序列的價格調整法。
type AdjustMethod string

const (
	// AdjustNone 不做任何調整（raw splice）：換倉日會留下價差跳空。
	AdjustNone AdjustMethod = "none"
	// AdjustPriceDiff 是**價差調整**（difference / back-adjust）：把較舊的段整體平移，
	// 使序列在換倉日連續，並錨定在最新段的原始價位。本階段採用此法。
	AdjustPriceDiff AdjustMethod = "back_adjust_price_diff"
)

// FuturesRollover 是一次換倉 splice 的稽核記錄。
type FuturesRollover struct {
	Contract  string    `json:"contract"`
	RollDate  time.Time `json:"roll_date"`
	FromMonth string    `json:"from_month"`
	ToMonth   string    `json:"to_month"`
	// PriceDiff = close(to, RollDate) − close(from, RollDate)，兩者皆取 canonical
	// （一般時段）收盤。任一邊缺值 ⇒ nil（splice 不成立；**不補值、不插值**）。
	PriceDiff *float64 `json:"price_diff,omitempty"`
}

// SpliceValid 回報此 splice 是否可用於累積調整。
func (r FuturesRollover) SpliceValid() bool { return r.PriceDiff != nil }

// FuturesContinuousBar 是連續序列的一根 bar。
//
// Adjusted* 只調整價格：量與 OI 不調整（調整後的口數沒有意義）。
// 原始值同時保留，讓消費端能分辨「真實成交價」與「平移後的連續價」。
type FuturesContinuousBar struct {
	TradeDate     time.Time      `json:"trade_date"`
	Contract      string         `json:"contract"`
	ContractMonth string         `json:"contract_month"`
	Session       FuturesSession `json:"session"`
	Open          *float64       `json:"open,omitempty"`
	High          *float64       `json:"high,omitempty"`
	Low           *float64       `json:"low,omitempty"`
	Close         *float64       `json:"close,omitempty"`
	AdjustedOpen  *float64       `json:"adjusted_open,omitempty"`
	AdjustedHigh  *float64       `json:"adjusted_high,omitempty"`
	AdjustedLow   *float64       `json:"adjusted_low,omitempty"`
	AdjustedClose *float64       `json:"adjusted_close,omitempty"`
	Volume        *int64         `json:"volume,omitempty"`
	OpenInterest  *int64         `json:"open_interest,omitempty"`
	// CumulativeDiff 是套用到本根的累積平移量（最新段為 0）。
	CumulativeDiff float64 `json:"cumulative_diff"`
}
