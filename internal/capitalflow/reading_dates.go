package capitalflow

// Value/date pairing for dated input channels — issue #1940 R1/R2.
//
// Two of the seven dimensions do not read a same-day upstream: their
// snapshot point carries the date of the *session or file* the value came
// from, because the producer publishes after the fact.
//
//   - government (官股行庫): gateway_adapter.applyGovernmentFlow stamps
//     MacroDataPoint.Timestamp from the reading file's own `date` field
//     (data/state/government_flow/YYYYMMDD.json).
//   - futures (foreign futures OI): applyTaifexInstitutional stamps
//     Timestamp from the TAIFEX OpenAPI row's `Date` (latest published
//     session).
//
// The rest of the pipeline used to ignore that stamp and key every sample
// to the refresh run's trading day. That is issue #1940 R2: on 2026-09-07
// the newest government file was 20260904.json (it is written the NEXT
// business day — file 20260904.json has mtime 2026-09-07 15:19), so the
// 09-04 reading was persisted and reported as the 09-07 sample; the same
// +1 shift produced 09-08←20260907.json, 09-11←20260910.json, … and the
// futures leg got two samples with one value (2026-07-17 = 07-20 =
// -86189), which is what R3's z = 785200 came from.
//
// The fix is to make the reading's own date its sample key, in all three
// places that touch the (dimension, trading_date) pair:
//
//   - Service.Refresh (write): the sample is persisted under the reading date.
//   - Service.extractAsOf (read): the reference window ends strictly before
//     the reading date, so the value never enters its own window (§8.4).
//   - ForceExtractor.Score: ForceScore.AsOfTradingDate reports the reading date.
//
// One helper feeds all three (dimensionSampleDate), so the write key, the
// read upper bound and the reported as-of date cannot drift apart again.
//
// Why not "Latest() must compare against the requested day", the fix
// suggested in the issue? Because for government_flow that comparison
// always fails and silently deletes the dimension: GovernmentBrokerChannelAdapter
// deliberately aggregates PreviousTradingDay(now, 1), so the newest file
// on trading day T is the reading for T-1 (verified over 2026-09-02..09-17:
// every file's mtime is the next business day). A "reading date must equal
// the requested day" gate would therefore reject every reading, every day.
// Keying by the reading's own date keeps the data and makes the pairing
// exact — which is what CF-INV-05/06 require ("no zero-valued or
// mis-dated sample in the reference window"), not "no data at all".
//
// The same reasoning applies to a stale file that is never replaced: with
// date-keyed attribution, re-reading 20260728.json on 20 consecutive days
// upserts the SAME (government, 2026-07-28) sample instead of fabricating
// 20 samples — the R1 mechanism, which is date-shaped even when the
// value is not zero.

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// datedDimensions lists the dimensions whose reading carries an explicit
// observation date in the snapshot. Dimensions outside this set are
// attributed to the refresh run's trading day, which is what their
// producers mean (TWSE T86 / MI_MARGN are same-day feeds).
func isDatedDimension(dim ForceName) bool {
	switch dim {
	case ForceGovernment, ForceFutures:
		return true
	default:
		return false
	}
}

// dimensionSampleDate returns the (dimension, trading_date) key that the
// dimension's current reading belongs to: the reading's own date when the
// snapshot carries one, otherwise the report's trading day.
//
// Returning the reading date (not the trading day) is what makes the
// write path idempotent for dated channels: repeated reads of one file
// collapse into one sample (CF-INV-05), and a sample can never be
// back-dated or forward-dated onto a day whose session it does not
// describe (CF-INV-06).
//
// A reading dated after the trading day is not representable as that
// day's observation, so it falls back to the trading day; that keeps the
// returned key inside the horizon the caller asked about instead of
// writing a future-dated sample.
func dimensionSampleDate(snap marketdata.MacroDataSnapshot, dim ForceName, tradingDate string) string {
	readingDate, ok := dimensionReadingDate(snap, dim)
	if !ok {
		return tradingDate
	}
	if tradingDate != "" && readingDate > tradingDate {
		return tradingDate
	}
	return readingDate
}

// dimensionReadingDate extracts the observation date stamped on a dated
// dimension's snapshot point. ok is false when the dimension is not
// dated, the point is absent, the point carries no timestamp, or the
// value is unusable (see scoreGovernment for the government zero rule).
//
// The stamp is formatted in Asia/Taipei, which is correct for both
// midnight conventions the producers use: a UTC-midnight stamp
// (time.Parse("20060102", …), what the gateway writes today) is 08:00 on
// the same Taipei date, and a Taipei-midnight stamp is that date directly.
func dimensionReadingDate(snap marketdata.MacroDataSnapshot, dim ForceName) (string, bool) {
	if !isDatedDimension(dim) {
		return "", false
	}
	var pt marketdata.MacroDataPoint
	switch dim {
	case ForceGovernment:
		if snap.GovernmentNet.Symbol == "" || snap.GovernmentNet.Value == 0 {
			return "", false
		}
		pt = snap.GovernmentNet
	default:
		// ForceFutures is the only other dated dimension (isDatedDimension).
		pt = snap.ForeignFuturesOINet
	}
	if pt.Symbol == "" || pt.Timestamp <= 0 {
		return "", false
	}
	return time.Unix(pt.Timestamp, 0).In(taipeiZone).Format("2006-01-02"), true
}
