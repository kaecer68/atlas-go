package feature

import (
	"math"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Func computes one feature value from a bar at position idx in a sorted bar slice.
type Func func(bar domain.DailyBar, idx int, bars []domain.DailyBar) float64

// Registry maps feature names to their computation functions.
var Registry = map[string]Func{
	// --- Original 7 features ---
	"close": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		return b.Close
	},
	"volume": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Volume <= 0 {
			return 0
		}
		return math.Log(float64(b.Volume))
	},
	"return_1d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx > 0 && bars[idx-1].Close > 0 {
			return (b.Close - bars[idx-1].Close) / bars[idx-1].Close
		}
		return 0
	},
	"return_5d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx >= 5 && bars[idx-5].Close > 0 {
			return (b.Close - bars[idx-5].Close) / bars[idx-5].Close
		}
		return 0
	},
	"hl_ratio": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Close > 0 {
			return (b.High - b.Low) / b.Close
		}
		return 0
	},
	"ma_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		if sum > 0 {
			return b.Close / (sum / 20.0)
		}
		return 1.0
	},
	"volume_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 || b.Volume <= 0 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += float64(bars[j].Volume)
		}
		avg := sum / 20.0
		if avg > 0 {
			return float64(b.Volume) / avg
		}
		return 1.0
	},

	// --- Phase C1: 15 technical indicators ---

	// adx_14: Average Directional Index (14-period Wilder's smoothing).
	"adx_14": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 28 {
			return 0.0
		}
		period := 14

		// Compute +DM, -DM, TR for each bar starting at idx-period*2+1.
		start := idx - period*2 + 1
		trVals := make([]float64, 0, period*2)
		pdmVals := make([]float64, 0, period*2)
		ndmVals := make([]float64, 0, period*2)
		for i := start; i <= idx; i++ {
			tr := bars[i].High - bars[i].Low
			upMove := bars[i].High - bars[i-1].High
			downMove := bars[i-1].Low - bars[i].Low
			if i > start {
				tr = math.Max(tr, math.Abs(bars[i].High-bars[i-1].Close))
				tr = math.Max(tr, math.Abs(bars[i].Low-bars[i-1].Close))
			}
			var pdm, ndm float64
			if upMove > downMove && upMove > 0 {
				pdm = upMove
			}
			if downMove > upMove && downMove > 0 {
				ndm = downMove
			}
			trVals = append(trVals, tr)
			pdmVals = append(pdmVals, pdm)
			ndmVals = append(ndmVals, ndm)
		}

		// Wilder's smoothing: first value is simple average, then EMA-like.
		atr := mean(trVals[:period])
		smPDM := mean(pdmVals[:period])
		smNDM := mean(ndmVals[:period])
		for i := period; i < len(trVals); i++ {
			atr = (atr*float64(period-1) + trVals[i]) / float64(period)
			smPDM = (smPDM*float64(period-1) + pdmVals[i]) / float64(period)
			smNDM = (smNDM*float64(period-1) + ndmVals[i]) / float64(period)
		}

		// Compute DX values over the second half.
		dxVals := make([]float64, 0, period)
		atr2 := mean(trVals[:period])
		smPDM2 := mean(pdmVals[:period])
		smNDM2 := mean(ndmVals[:period])
		for i := period; i < len(trVals); i++ {
			atr2 = (atr2*float64(period-1) + trVals[i]) / float64(period)
			smPDM2 = (smPDM2*float64(period-1) + pdmVals[i]) / float64(period)
			smNDM2 = (smNDM2*float64(period-1) + ndmVals[i]) / float64(period)
			var diPlus, diMinus float64
			if atr2 > 0 {
				diPlus = 100.0 * smPDM2 / atr2
				diMinus = 100.0 * smNDM2 / atr2
			}
			denom := diPlus + diMinus
			if denom > 0 {
				dxVals = append(dxVals, 100.0*math.Abs(diPlus-diMinus)/denom)
			} else {
				dxVals = append(dxVals, 0.0)
			}
		}

		// Smooth DX to get ADX.
		if len(dxVals) == 0 {
			return 0.0
		}
		adx := mean(dxVals[:min(period, len(dxVals))])
		for i := period; i < len(dxVals); i++ {
			adx = (adx*float64(period-1) + dxVals[i]) / float64(period)
		}
		return adx
	},

	// amihud: Amihud illiquidity measure.
	"amihud": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if b.Close <= 0 || b.Volume <= 0 {
			return 0.0
		}
		ret := 0.0
		if idx > 0 && bars[idx-1].Close > 0 {
			ret = math.Abs((b.Close - bars[idx-1].Close) / bars[idx-1].Close)
		}
		return ret / (b.Close * float64(b.Volume)) * 1e6
	},

	// atr_14: Average True Range over 14 periods.
	"atr_14": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 14 {
			return 0.0
		}
		sum := 0.0
		for i := idx - 13; i <= idx; i++ {
			tr := bars[i].High - bars[i].Low
			if i > 0 {
				tr = math.Max(tr, math.Abs(bars[i].High-bars[i-1].Close))
				tr = math.Max(tr, math.Abs(bars[i].Low-bars[i-1].Close))
			}
			sum += tr
		}
		return sum / 14.0
	},

	// bb_pct_b: Bollinger Bands %B.
	"bb_pct_b": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 0.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		ma := sum / 20.0
		vr := 0.0
		for j := idx - 19; j <= idx; j++ {
			d := bars[j].Close - ma
			vr += d * d
		}
		std := math.Sqrt(vr / 20.0)
		if std < 1e-12 {
			return 0.0
		}
		return (b.Close - ma) / (2.0 * std)
	},

	// hl_range_pct: High-Low range as percentage of 20-day MA close.
	"hl_range_pct": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 0.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		ma := sum / 20.0
		if ma <= 0 {
			return 0.0
		}
		return (b.High - b.Low) / ma
	},

	// kurtosis_20d: Excess kurtosis of log returns over 20 days.
	"kurtosis_20d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 20 {
			return 0.0
		}
		lr := make([]float64, 20)
		for j := range 20 {
			pos := idx - 19 + j
			if bars[pos-1].Close > 0 && bars[pos].Close > 0 {
				lr[j] = math.Log(bars[pos].Close / bars[pos-1].Close)
			}
		}
		n := float64(len(lr))
		meanLR := 0.0
		for _, v := range lr {
			meanLR += v
		}
		meanLR /= n
		m2, m4 := 0.0, 0.0
		for _, v := range lr {
			d := v - meanLR
			d2 := d * d
			m2 += d2
			m4 += d2 * d2
		}
		m2 /= n
		m4 /= n
		if m2 < 1e-15 {
			return 0.0
		}
		return m4/(m2*m2) - 3.0
	},

	// liquidity: log(1 + Volume) from ml_scorer.
	"liquidity": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Volume <= 0 {
			return 0.0
		}
		return math.Log(1.0 + float64(b.Volume))
	},

	// macd: 12-period EMA minus 26-period EMA of Close.
	"macd": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 25 {
			return 0.0
		}
		ema12 := emaClose(bars, idx, 12)
		ema26 := emaClose(bars, idx, 26)
		return ema12 - ema26
	},

	// macd_signal: 9-period EMA of MACD.
	"macd_signal": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 34 {
			return 0.0
		}
		// Compute MACD history for the last 9+ values.
		macdVals := make([]float64, 9)
		for j := range 9 {
			pos := idx - 8 + j
			ema12 := emaClose(bars, pos, 12)
			ema26 := emaClose(bars, pos, 26)
			macdVals[j] = ema12 - ema26
		}
		return emaOf(macdVals, 9)
	},

	// momentum_intra: (Close - Open) / Open from ml_scorer.
	"momentum_intra": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		open := b.Open
		if open == 0 {
			return 0.0
		}
		return (b.Close - open) / open
	},

	// obv: On-Balance Volume (cumulative).
	"obv": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx == 0 {
			return 0.0
		}
		prev := 0.0
		// Compute cumulative OBV from start.
		for i := 1; i <= idx; i++ {
			if bars[i].Close > bars[i-1].Close {
				prev += float64(bars[i].Volume)
			} else if bars[i].Close < bars[i-1].Close {
				prev -= float64(bars[i].Volume)
			}
		}
		return prev
	},

	// price_position: (Close - MA20) / Close.
	"price_position": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 0.0
		}
		if b.Close == 0 {
			return 0.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		ma := sum / 20.0
		return (b.Close - ma) / b.Close
	},

	// quality_intra: 1 - (High-Low)/Close from ml_scorer.
	"quality_intra": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Close == 0 {
			return 0.0
		}
		return 1.0 - (b.High-b.Low)/b.Close
	},

	// return_autocorr: 1-lag autocorrelation of returns over 20 days.
	"return_autocorr": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 21 {
			return 0.0
		}
		rets := make([]float64, 20)
		for j := range 20 {
			pos := idx - 19 + j
			if bars[pos-1].Close > 0 {
				rets[j] = (bars[pos].Close - bars[pos-1].Close) / bars[pos-1].Close
			}
		}
		// Series A: rets[0..18], Series B: rets[1..19].
		n := 19.0
		meanA, meanB := 0.0, 0.0
		for j := range 19 {
			meanA += rets[j]
			meanB += rets[j+1]
		}
		meanA /= n
		meanB /= n
		cov, varA, varB := 0.0, 0.0, 0.0
		for j := range 19 {
			da := rets[j] - meanA
			db := rets[j+1] - meanB
			cov += da * db
			varA += da * da
			varB += db * db
		}
		denom := math.Sqrt(varA * varB)
		if denom < 1e-15 {
			return 0.0
		}
		return cov / denom
	},

	// rsi_14: Relative Strength Index over 14 periods.
	"rsi_14": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 14 {
			return 50.0
		}
		gain, loss := 0.0, 0.0
		for i := idx - 13; i <= idx; i++ {
			chg := bars[i].Close - bars[i-1].Close
			if chg > 0 {
				gain += chg
			} else {
				loss += -chg
			}
		}
		avgGain := gain / 14.0
		avgLoss := loss / 14.0
		if avgLoss < 1e-15 {
			return 100.0
		}
		rs := avgGain / avgLoss
		return 100.0 - 100.0/(1.0+rs)
	},

	// skewness_20d: Skewness of log returns over 20 days.
	"skewness_20d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 20 {
			return 0.0
		}
		lr := make([]float64, 20)
		for j := range 20 {
			pos := idx - 19 + j
			if bars[pos-1].Close > 0 && bars[pos].Close > 0 {
				lr[j] = math.Log(bars[pos].Close / bars[pos-1].Close)
			}
		}
		n := float64(len(lr))
		meanLR := 0.0
		for _, v := range lr {
			meanLR += v
		}
		meanLR /= n
		m2, m3 := 0.0, 0.0
		for _, v := range lr {
			d := v - meanLR
			m2 += d * d
			m3 += d * d * d
		}
		m2 /= n
		m3 /= n
		if m2 < 1e-15 {
			return 0.0
		}
		return m3 / math.Pow(m2, 1.5)
	},

	// value_intra: Close / Open from ml_scorer.
	"value_intra": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		open := b.Open
		if open == 0 {
			return 1.0
		}
		return b.Close / open
	},

	// volatility_20d: Annualized volatility from 20-day log returns.
	"volatility_20d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 20 {
			return 0.0
		}
		lr := make([]float64, 20)
		for j := range 20 {
			pos := idx - 19 + j
			if bars[pos-1].Close > 0 && bars[pos].Close > 0 {
				lr[j] = math.Log(bars[pos].Close / bars[pos-1].Close)
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
	},

	// volume_trend: Short-term / long-term volume ratio.
	"volume_trend": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 1.0
		}
		sum5 := 0.0
		for j := idx - 4; j <= idx; j++ {
			sum5 += float64(bars[j].Volume)
		}
		sum20 := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum20 += float64(bars[j].Volume)
		}
		avg20 := sum20 / 20.0
		if avg20 <= 0 {
			return 1.0
		}
		return (sum5 / 5.0) / avg20
	},
}

// --- Helper functions for technical indicators ---

// mean computes the arithmetic mean of a slice.
func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// emaClose computes the exponential moving average of Close at position idx
// with the given period. Seeds with SMA of first 'period' bars, then
// propagates EMA forward to idx.
func emaClose(bars []domain.DailyBar, idx int, period int) float64 {
	if idx < period-1 {
		return 0
	}
	alpha := 2.0 / float64(period+1)

	// Seed: SMA of bars[0..period-1].
	sum := 0.0
	for j := range period {
		sum += bars[j].Close
	}
	ema := sum / float64(period)

	// Propagate from bar[period] to bar[idx].
	for j := period; j <= idx; j++ {
		ema = alpha*bars[j].Close + (1-alpha)*ema
	}
	return ema
}

// emaOf computes the EMA of a pre-computed series using the given period.
// The series should have at least 'period' elements.
func emaOf(vals []float64, period int) float64 {
	if len(vals) < period {
		return 0
	}
	alpha := 2.0 / float64(period+1)

	// Seed with SMA of first 'period' values.
	sum := 0.0
	for i := range period {
		sum += vals[i]
	}
	ema := sum / float64(period)

	for i := period; i < len(vals); i++ {
		ema = alpha*vals[i] + (1-alpha)*ema
	}
	return ema
}

// Available returns all registered feature names in sorted order.
func Available() []string {
	n := make([]string, 0, len(Registry))
	for k := range Registry {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Validate returns any feature names not in the registry.
func Validate(names []string) []string {
	var u []string
	for _, n := range names {
		if _, ok := Registry[n]; !ok {
			u = append(u, n)
		}
	}
	return u
}

// ParseNames splits a comma-separated string, trimming whitespace.
func ParseNames(raw string) []string {
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

// MakeExtractor returns a function that extracts the named features from a bar slice.
func MakeExtractor(names []string) func(bars []domain.DailyBar) [][]float64 {
	return func(bars []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(bars))
		for i, bar := range bars {
			row := make([]float64, len(names))
			for j, n := range names {
				row[j] = Registry[n](bar, i, bars)
			}
			f[i] = row
		}
		return f
	}
}

// ForwardReturnLabel returns a label extractor that computes forward 1-day returns.
func ForwardReturnLabel() func(bars []domain.DailyBar) []float64 {
	return func(bars []domain.DailyBar) []float64 {
		l := make([]float64, len(bars))
		for i := 0; i < len(bars)-1; i++ {
			if bars[i].Close > 0 {
				l[i] = (bars[i+1].Close - bars[i].Close) / bars[i].Close
			}
		}
		if len(bars) > 0 {
			l[len(bars)-1] = 0
		}
		return l
	}
}
