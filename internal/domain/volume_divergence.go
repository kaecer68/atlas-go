package domain

import (
	"sort"
	"strconv"
)

// Volume-price divergence detection (量價背離).
//
// Semantics (30-trading-day panel, PIT-safe: the function reads only the
// bars it is given — callers passing a historical prefix get a point-in-time
// reading):
//
//   - 頂背離 (TopDivergence, bearish warning): the latest close is at or
//     near the window closing high (within divergencePriceTolerance) while
//     volume is declining (5-day average volume < 20-day average volume).
//     Interpretation: the rally is losing participation — momentum may be
//     exhausting. For a holder/buyer this is a caution signal.
//   - 底背離 (BottomDivergence, selling exhaustion): the latest close is at
//     or near the window closing low while volume is declining.
//     Interpretation: sell-off is losing participation — panic selling may
//     be exhausting.
//
// The MA-based volume proxy (volMA5 < volMA20) is used instead of comparing
// two successive peak volumes because it is robust to single-day spikes and
// stays explainable to retail users. Raw numbers are exposed in the result
// so callers can render the evidence behind the boolean flags.
const (
	// DivergenceDefaultWindowDays is the default lookback in trading days.
	DivergenceDefaultWindowDays = 30
	// DivergenceVolShortWindow / DivergenceVolLongWindow are the volume
	// moving-average windows used for the "volume declining" test.
	DivergenceVolShortWindow = 5
	DivergenceVolLongWindow  = 20
	// divergencePriceTolerance allows "near" highs/lows: a close within 1%
	// of the window extreme still counts as testing that extreme.
	divergencePriceTolerance = 0.01
)

// VolumeDivergenceResult is the outcome of DetectVolumeDivergence.
// JSON tags are snake_case per the repo API convention.
type VolumeDivergenceResult struct {
	Symbol     string `json:"symbol"`
	LatestDate string `json:"latest_date"` // YYYY-MM-DD of the last bar used
	WindowDays int    `json:"window_days"` // requested window
	BarsUsed   int    `json:"bars_used"`   // actual bars analyzed (<= WindowDays)

	Close      float64 `json:"close"`
	WindowHigh float64 `json:"window_high"` // highest close in window
	WindowLow  float64 `json:"window_low"`  // lowest close in window
	// Distance of the latest close from the window extremes, in percent
	// (0 = exactly at the extreme). Positive values only.
	CloseBelowHighPct float64 `json:"close_below_high_pct"`
	CloseAboveLowPct  float64 `json:"close_above_low_pct"`

	VolMA5  float64 `json:"vol_ma5"`
	VolMA20 float64 `json:"vol_ma20"`
	// VolumeDeclining is the shared precondition: volMA5 < volMA20.
	VolumeDeclining bool `json:"volume_declining"`

	// TopDivergence: price at/near window high AND volume declining.
	TopDivergence bool `json:"top_divergence"`
	// BottomDivergence: price at/near window low AND volume declining.
	BottomDivergence bool `json:"bottom_divergence"`

	// Interpretation is a human-readable zh-TW summary of the flags.
	Interpretation string `json:"interpretation"`

	// TradingDay, when non-nil and false, marks that the reading was served
	// on a non-trading day (weekend/holiday) so consumers do not mistake the
	// last available bar for a live signal. Serving layers set this; the
	// detector itself leaves it nil (mirrors Quote.TradingDay).
	TradingDay *bool `json:"trading_day,omitempty"`
}

// DetectVolumeDivergence analyzes bars for price/volume divergence over the
// most recent windowDays trading days. bars may be in any order (the
// function sorts a copy chronologically). Returns ok=false when there is
// insufficient data (< DivergenceVolLongWindow bars) or the price panel is
// degenerate (window high == window low).
func DetectVolumeDivergence(bars []DailyBar, windowDays int) (VolumeDivergenceResult, bool) {
	if windowDays <= 0 {
		windowDays = DivergenceDefaultWindowDays
	}
	if len(bars) < DivergenceVolLongWindow {
		return VolumeDivergenceResult{}, false
	}

	sorted := make([]DailyBar, len(bars))
	copy(sorted, bars)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	if len(sorted) > windowDays {
		sorted = sorted[len(sorted)-windowDays:]
	}

	latest := sorted[len(sorted)-1]
	res := VolumeDivergenceResult{
		Symbol:     latest.Symbol,
		LatestDate: latest.Date.Format("2006-01-02"),
		WindowDays: windowDays,
		BarsUsed:   len(sorted),
		Close:      latest.Close,
	}

	high, low := sorted[0].Close, sorted[0].Close
	for _, b := range sorted {
		if b.Close > high {
			high = b.Close
		}
		if b.Close < low {
			low = b.Close
		}
	}
	// Degenerate flat panel: no meaningful extreme to diverge from.
	if high <= low || high <= 0 || low <= 0 {
		return VolumeDivergenceResult{}, false
	}
	res.WindowHigh = high
	res.WindowLow = low
	res.CloseBelowHighPct = round2((high - latest.Close) / high * 100)
	res.CloseAboveLowPct = round2((latest.Close - low) / low * 100)

	res.VolMA5 = volMA(sorted, DivergenceVolShortWindow)
	res.VolMA20 = volMA(sorted, DivergenceVolLongWindow)
	// Zero-volume panels (data gaps) cannot support a declining-volume read.
	if res.VolMA20 > 0 {
		res.VolumeDeclining = res.VolMA5 < res.VolMA20
	}

	atHigh := latest.Close >= high*(1-divergencePriceTolerance)
	atLow := latest.Close <= low*(1+divergencePriceTolerance)
	res.TopDivergence = atHigh && res.VolumeDeclining
	res.BottomDivergence = atLow && res.VolumeDeclining

	switch {
	case res.TopDivergence:
		res.Interpretation = "頂背離：股價接近近" + strconv.Itoa(windowDays) + "日新高，但成交量遞減（5日均量低於20日均量），上漲動能可能衰竭，持有者宜提高警覺。"
	case res.BottomDivergence:
		res.Interpretation = "底背離：股價接近近" + strconv.Itoa(windowDays) + "日新低，但成交量遞減（5日均量低於20日均量），賣壓可能竭盡，跌勢可能趨緩。"
	case res.VolumeDeclining:
		res.Interpretation = "量能遞減中，但股價未處於區間極端，尚無明顯量價背離。"
	default:
		res.Interpretation = "無明顯量價背離。"
	}
	return res, true
}

// volMA returns the simple moving average of Volume over the last n bars.
// Callers guarantee len(bars) >= n (DetectVolumeDivergence requires >=
// DivergenceVolLongWindow bars and n <= DivergenceVolLongWindow).
func volMA(bars []DailyBar, n int) float64 {
	if len(bars) < n || n <= 0 {
		return 0
	}
	var sum float64
	for _, b := range bars[len(bars)-n:] {
		sum += float64(b.Volume)
	}
	return sum / float64(n)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
