package marketdata

import (
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// 連續契約序列建構（規格 §6）。
//
// 這是**純函式**：輸入原始 bar、輸出調整後序列與 splice 事件，不落庫、不打網路。
// 調整法為價差 back-adjust（加法），錨定最新段：
//
//	adjusted(t) = raw_{segment(t)}(t) + Σ_{k >= segment(t)} diff_k
//
// 其中 diff_k = close(seg k+1, R_k) − close(seg k, R_k)，R_k 為段 k 的最後交易日。
// 最新段因此 diff 為 0（與現行市場價位一致），越舊的段被平移越多。
//
// 為何用加法而非比率（規格 §6.3 已記錄替代方案）：
// 階段 1 的用途是**點數層級**的門檻／基差／z-score；比率調整會扭曲絕對點數，
// 讓以點數定義的門檻失去意義。加法調整的已知代價：長序列的歷史絕對價會偏離當年真實指數。
func BuildContinuousSeries(bars []domain.FuturesBar, method domain.AdjustMethod) ([]domain.FuturesContinuousBar, []domain.FuturesRollover, error) {
	switch method {
	case domain.AdjustNone, domain.AdjustPriceDiff:
	default:
		return nil, nil, fmt.Errorf("continuous series: unsupported adjust method %q", method)
	}

	usable := canonicalMonthlyBars(bars)
	if len(usable) == 0 {
		return nil, nil, nil
	}

	segments, byKey := buildFrontMonthSegments(usable)
	if len(segments) == 0 {
		return nil, nil, nil
	}

	// 每個到期月的最後交易日（＝該月 as front month 的最後一日）＝ segment 最後一日。
	rollovers := make([]domain.FuturesRollover, 0, len(segments))
	for i := 0; i+1 < len(segments); i++ {
		cur, next := segments[i], segments[i+1]
		if cur.month == next.month {
			continue
		}
		ev := domain.FuturesRollover{
			Contract:  cur.contract,
			RollDate:  cur.lastDate,
			FromMonth: cur.month,
			ToMonth:   next.month,
		}
		// 換倉日當天兩個契約都有報價，但「新契約」在該日仍屬**舊段**（該日的前月
		// 還是舊契約）⇒ 查價必須用完整索引，不能用 next 段的 byDate。
		rollKey := cur.lastDate.Format("2006-01-02")
		fromBar, okFrom := byKey[cur.month+"|"+rollKey]
		toBar, okTo := byKey[next.month+"|"+rollKey]
		if okFrom && okTo && fromBar.Close != nil && toBar.Close != nil {
			diff := *toBar.Close - *fromBar.Close
			ev.PriceDiff = &diff
		}
		rollovers = append(rollovers, ev)
	}

	// 依段序列累積：段 j 的累積平移 = Σ_{k=j..n-2} diff_k（最後一段為 0）。
	// 無效 splice（PriceDiff == nil）以 0 計入並保留事件，讓稽核看得到缺口。
	//
	// 註：本函式目前只支援 AdjustPriceDiff 的全段累積；AdjustNone 時累積量恆為 0。
	out := make([]domain.FuturesContinuousBar, 0, len(usable))
	validDiffs := make([]float64, len(rollovers))
	for i, ev := range rollovers {
		if ev.PriceDiff != nil {
			validDiffs[i] = *ev.PriceDiff
		}
	}
	for segIdx, seg := range segments {
		cum := 0.0
		if method == domain.AdjustPriceDiff {
			for k := segIdx; k < len(validDiffs); k++ {
				cum += validDiffs[k]
			}
		}
		sort.SliceStable(seg.bars, func(i, j int) bool { return seg.bars[i].TradeDate.Before(seg.bars[j].TradeDate) })
		for _, b := range seg.bars {
			out = append(out, domain.FuturesContinuousBar{
				TradeDate:      b.TradeDate,
				Contract:       b.Contract,
				ContractMonth:  b.ContractMonth,
				Session:        b.Session,
				Open:           b.Open,
				High:           b.High,
				Low:            b.Low,
				Close:          b.Close,
				AdjustedOpen:   shiftFuturesPrice(b.Open, cum),
				AdjustedHigh:   shiftFuturesPrice(b.High, cum),
				AdjustedLow:    shiftFuturesPrice(b.Low, cum),
				AdjustedClose:  shiftFuturesPrice(b.Close, cum),
				Volume:         b.Volume,
				OpenInterest:   b.OpenInterest,
				CumulativeDiff: cum,
			})
		}
	}
	// 無效 splice（PriceDiff == nil）不補值、不插值：以 0 計入累積並保留事件，
	// 讓稽核看得到缺口（SpliceValid() 為 false）。
	return out, rollovers, nil
}

// futuresSegment 是一段「同一個月契約作為前月」的連續交易日。
type futuresSegment struct {
	contract string
	month    string
	lastDate time.Time
	bars     []domain.FuturesBar
	byDate   map[string]domain.FuturesBar
}

// canonicalMonthlyBars 只留下可比較的 bar：
//   - 契約月必須是標準月契約（排除週契約與價差組合）；
//   - 時段必須是一般（canonical；OI 只在該列有值）；
//   - 必須有收盤價（未成交列不構成序列）。
func canonicalMonthlyBars(bars []domain.FuturesBar) []domain.FuturesBar {
	out := make([]domain.FuturesBar, 0, len(bars))
	for _, b := range bars {
		if !domain.IsMonthlyContractMonth(b.ContractMonth) {
			continue
		}
		if b.Session != domain.SessionRegular {
			continue
		}
		if b.Close == nil {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].TradeDate.Equal(out[j].TradeDate) {
			return out[i].TradeDate.Before(out[j].TradeDate)
		}
		return out[i].ContractMonth < out[j].ContractMonth
	})
	return out
}

// buildFrontMonthSegments 依序掃過所有交易日，決定每天的前月契約，並切成連續的段。
//
// 前月定義（以資料為準）：在交易日 t，前月 = 「最後交易日 >= t」的月契約中月份最小者。
// 某契約的「最後交易日」以其**有資料的最後一日**推導（規格 §6.2：以資料驗證表定日期）。
//
// 第二個回傳值是「月契約|日期 → bar」的完整索引，供換倉 splice 查同日兩契約的收盤。
func buildFrontMonthSegments(bars []domain.FuturesBar) ([]futuresSegment, map[string]domain.FuturesBar) {
	lastDateByMonth := make(map[string]time.Time)
	for _, b := range bars {
		if cur, ok := lastDateByMonth[b.ContractMonth]; !ok || b.TradeDate.After(cur) {
			lastDateByMonth[b.ContractMonth] = b.TradeDate
		}
	}

	dates := make([]time.Time, 0, len(bars))
	seen := make(map[string]bool, len(bars))
	for _, b := range bars {
		d := b.TradeDate.Format("2006-01-02")
		if seen[d] {
			continue
		}
		seen[d] = true
		dates = append(dates, b.TradeDate)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	barsByKey := make(map[string]domain.FuturesBar, len(bars))
	for _, b := range bars {
		barsByKey[b.ContractMonth+"|"+b.TradeDate.Format("2006-01-02")] = b
	}

	var segments []futuresSegment
	for _, d := range dates {
		ds := d.Format("2006-01-02")
		front := frontMonthFor(d, lastDateByMonth)
		if front == "" {
			continue
		}
		bar, ok := barsByKey[front+"|"+ds]
		if !ok {
			// 前月當日無資料（例如該月契約在到期前已下市）⇒ 跳過該日。
			continue
		}
		if n := len(segments); n > 0 && segments[n-1].month == front {
			segments[n-1].bars = append(segments[n-1].bars, bar)
			segments[n-1].byDate[ds] = bar
			segments[n-1].lastDate = d
			continue
		}
		segments = append(segments, futuresSegment{
			contract: bar.Contract,
			month:    front,
			lastDate: d,
			bars:     []domain.FuturesBar{bar},
			byDate:   map[string]domain.FuturesBar{ds: bar},
		})
	}
	return segments, barsByKey
}

// frontMonthFor 回傳交易日 d 的前月契約月（YYYYMM）；找不到回空字串。
func frontMonthFor(d time.Time, lastDateByMonth map[string]time.Time) string {
	best := ""
	for month, last := range lastDateByMonth {
		if d.After(last) {
			continue // 已到期
		}
		if best == "" || month < best {
			best = month
		}
	}
	return best
}

func shiftFuturesPrice(v *float64, diff float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v + diff
	return &out
}
