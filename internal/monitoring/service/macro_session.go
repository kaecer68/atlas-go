package service

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// Market-session-aware freshness evaluation for macro ingest indicators
// (#1762). A financial platform must never label the latest available session
// close as「資料待更新」: outside a market's session window the honest,
// professional status is「週末休市」/「休市」/「盤前」with the as-of close date.
// The previous fixed 24h wall-clock threshold flagged every US-market
// indicator as stale every weekend even though the data was correct.

const (
	macroMarketUS = "US"
	macroMarketTW = "TW"
)

// macroSymbolMarket maps each macro ingest symbol to the market session that
// governs its freshness expectations. Symbols absent from this map fall back
// to the legacy 24h wall-clock threshold (safe default for unknown feeds).
var macroSymbolMarket = map[string]string{
	"DX-Y.NYB": macroMarketUS, // 美元指數
	"^TNX":     macroMarketUS, // 美債10年期
	"^VIX":     macroMarketUS, // 波動率指數
	"CL=F":     macroMarketUS, // 原油
	"GC=F":     macroMarketUS, // 黃金
	"JPY=X":    macroMarketUS, // 日圓 (USD/JPY, 近 24/5 交易)
	"USDTWD=X": macroMarketTW, // 台幣匯率
}

type macroSessionSpec struct {
	loc          *time.Location
	openMin      int // session open, minutes since local midnight
	closeMin     int // session close, minutes since local midnight
	isTradingDay func(time.Time) bool
}

var (
	macroUSSession = &macroSessionSpec{
		loc:          mustLocation("America/New_York"),
		openMin:      9 * 60,  // 09:30 ET
		closeMin:     16 * 60, // 16:00 ET
		isTradingDay: macroUSTradingDay,
	}
	macroTWSession = &macroSessionSpec{
		loc:          mustLocation("Asia/Taipei"),
		openMin:      9 * 60,     // 09:00 TW
		closeMin:     13*60 + 30, // 13:30 TW
		isTradingDay: taiwanholidays.IsTradingDay,
	}
)

// macroUSTradingDay reports whether t (interpreted in the market's local
// calendar) is an expected US equity session day.
//
// Known limitation: no US public-holiday table is embedded (the repo's
// calendar convention leaves US holidays to upstream data providers, see
// internal/marketdata/calendar.go). On a US holiday the previous session's
// close is correctly shown as 休市-fresh over the weekend, but the first
// weekday after the holiday may briefly warn 資料待更新 until the next
// session completes — strictly better than the legacy every-weekend warn.
func macroUSTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w != time.Saturday && w != time.Sunday
}

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Fallback to UTC keeps evaluation functional (never panics) when
		// the host lacks tzdata; US DST edge may shift text by an hour.
		return time.UTC
	}
	return loc
}

// macroSessionRef describes where the freshness reference point sits.
type macroSessionRef struct {
	lastClose time.Time // most recent expected session close at/before now
	closedNow bool      // market currently inside a closed window
	closedWhy string    // "週末休市" / "休市" / "盤前" (empty when open)
}

// macroSessionRefFor resolves, for the given session spec and now, the most
// recent expected close and whether the market is currently closed.
func macroSessionRefFor(spec *macroSessionSpec, now time.Time) macroSessionRef {
	t := now.In(spec.loc)
	ref := macroSessionRef{}

	isTradingToday := spec.isTradingDay(t)
	clock := t.Hour()*60 + t.Minute()

	switch {
	case !isTradingToday:
		ref.closedNow = true
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			ref.closedWhy = "週末休市"
		} else {
			ref.closedWhy = "休市"
		}
	case clock < spec.openMin:
		ref.closedNow = true
		ref.closedWhy = "盤前"
	}

	// Most recent expected close at/before now: today's close when the
	// session has ended (or is running — reference then is yesterday's
	// close, an intraday quote is always newer than that), otherwise walk
	// back to the previous trading day.
	day := t
	if isTradingToday && clock >= spec.closeMin {
		ref.lastClose = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, spec.loc).Add(time.Duration(spec.closeMin) * time.Minute)
		return ref
	}
	for i := 0; i < 10; i++ {
		if spec.isTradingDay(day) && (day.Before(t) || i > 0) {
			ref.lastClose = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, spec.loc).Add(time.Duration(spec.closeMin) * time.Minute)
			return ref
		}
		day = day.AddDate(0, 0, -1)
	}
	// 10-day walkback exhausted (should be impossible with weekend-only or
	// holiday tables): fall back to "a week ago" so callers degrade to warn.
	ref.lastClose = t.Add(-7 * 24 * time.Hour)
	return ref
}

// macroQuoteGrace allows quote timestamps stamped slightly before the
// official close (15:59 last trade) to count as covering that session.
const macroQuoteGrace = 2 * time.Hour

// evaluateMacroFreshness performs session-aware freshness classification for
// one indicator. The caller has already handled the hard-error cases
// (missing symbol, zero timestamp, >7d age).
//
// Returns the final status + user-facing status text:
//   - ("ok", "正常")                  — market open, data current
//   - ("ok", "週末休市（8/28 收盤）")  — market in a closed window, data is the
//     latest session close (professional UX: never「待更新」for correct data)
//   - ("warn", "資料待更新 (N 小時前)") — a completed expected session passed
//     without new data (genuine staleness)
func evaluateMacroFreshness(symbol string, ts int64, now time.Time) (status, text string) {
	market, known := macroSymbolMarket[symbol]
	if !known {
		// Unknown feed: legacy 24h wall-clock threshold.
		if now.Unix()-ts > 86400 {
			return "warn", fmt.Sprintf("資料待更新 (%d 小時前)", (now.Unix()-ts)/3600)
		}
		return "ok", "正常"
	}

	spec := macroTWSession
	if market == macroMarketUS {
		spec = macroUSSession
	}
	ref := macroSessionRefFor(spec, now)

	if ts >= ref.lastClose.Add(-macroQuoteGrace).Unix() {
		// Data covers the most recent expected session — correct data.
		if ref.closedNow {
			return "ok", fmt.Sprintf("%s（%d/%d 收盤）", ref.closedWhy, int(ref.lastClose.Month()), ref.lastClose.Day())
		}
		return "ok", "正常"
	}
	return "warn", fmt.Sprintf("資料待更新 (%d 小時前)", (now.Unix()-ts)/3600)
}
