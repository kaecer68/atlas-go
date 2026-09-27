package apigateway

import (
	"fmt"
	"time"
)

// Canonical channel status vocabulary.
//
// A record's status is a VERDICT, not a raw log line: recordInternal already
// derives "warn" vs "error" from the consecutive-failure streak (k3 audit R1,
// 2026-09-08). The same idea is extended here to data freshness: a channel
// whose last fetch succeeded but is older than its contract FreshnessWindow is
// "stale", not "ok".
//
// The two derived-only values (StatusStale / StatusUnknown) are never written
// by a fetch; they only come out of DeriveChannelStatus.
const (
	StatusOK       = "ok"
	StatusWarn     = "warn"
	StatusError    = "error"
	StatusDegraded = "degraded"
	StatusInactive = "inactive"
	// StatusStale is the verdict for a channel whose last fetch is older than
	// its contract FreshnessWindow (Issue #1086: ok-but-frozen channels).
	StatusStale = "stale"
	// StatusUnknown is the verdict for a channel that has no health record.
	StatusUnknown = "unknown"
)

// DeriveChannelStatus is THE single channel-status judgment in atlas.
//
// Background (2026-09-24, fix/20260924-channel-status-truth): one channel could
// carry two contradictory verdicts at the same instant —
//
//	channel_health record : twse_oddlot status=ok  last_fetch_at=2026-09-07T00:18:11Z
//	/admin/datachannels   : twse_oddlot "ok / 正常"   (resolveChannelStatusFromStore
//	                        returned "ok" for ANY record whose status was "ok",
//	                        bypassing the contract freshness window)
//	health summary log    : twse_oddlot:stale        (deriveStatusWithContract DID
//	                        apply the window)
//	channel_health (DB)   : twse_oddlot status=ok last_fetch_at=<sync time>
//	                        (recordToDB stamped time.Now() instead of the
//	                        record's own fetch time)
//
// Every consumer — the admin data-channel page, the home overview,
// /api/dashboard/channel-health, Gateway.Summary (health log + error counter),
// /api/health/aggregate Tier 2, the atlas_channel_health_status gauge and the
// channel_health DB mirror — MUST call this function so the record the gateway
// writes and the status a human reads can never disagree.
//
// Rules (in order):
//  1. no record                 → StatusUnknown
//  2. record.Status != "ok"     → passthrough (error/warn/inactive are already
//     verdicts written by the fetch path). "degraded" is the ONE exception, and
//     it escalates: a degraded record becomes StatusError once the DATA behind
//     it is older than contract.EffectiveFreshnessWindow(). "degraded" means the
//     fetch succeeded while the payload was empty/stale/partial, i.e. no real
//     data landed; for less than the channel's own freshness window that stays a
//     warning (whatever was cached is still within its contract), but beyond it
//     there is no valid cache left, so the channel is broken and the verdict must
//     be the one the alert path acts on (the gauge maps error → 2, which is what
//     ChannelHealthStatusError matches). E29-3, 2026-09-27: without this a
//     degraded record could never escalate — it keeps the fetch timestamp
//     moving, so no freshness gate saw it, and permanent schema drift was
//     discoverable only from a process exit code.
//  3. record.Status == "ok"     → StatusStale when the age of LastFetchAt
//     exceeds contract.EffectiveFreshnessWindow() (StaleDataThreshold = 48h
//     when the contract does not declare a window)
//  4. empty/unparseable LastFetchAt → StatusOK: the record is broken, but not
//     because of staleness, and mislabeling it "stale" would mislead on-call
//     (same rule the pre-existing deriveStatusWithContract documented).
//
// LastFetchAt — not LastDataAt — is the freshness anchor, deliberately: a
// record marked stale therefore always carries a LastFetchAt older than its
// window, which keeps it out of ChannelHealthStore.Alerts() (1h actionable
// threshold) and out of the alert stream. Legitimate "data timestamp lags the
// fetch time" cases (us10y/vix) are covered by
// atlas_channel_staleness_overage_seconds, which keys on LastDataAt.
func DeriveChannelStatus(rec *ChannelHealthRecord, contract ChannelContract, now time.Time) string {
	if rec == nil {
		return StatusUnknown
	}
	if rec.Status == StatusDegraded {
		// Rule 2b: a degraded verdict is about the DATA, so it must expire like
		// one. Anchor = the data's own timestamps, never LastFetchAt (a degraded
		// fetch still succeeds, so LastFetchAt keeps moving and would make this
		// branch unreachable — the reason the state could never escalate).
		if age, _, ok := degradedDataAge(rec, now); ok && age > contract.EffectiveFreshnessWindow() {
			return StatusError
		}
		return StatusDegraded
	}
	if rec.Status != StatusOK {
		return rec.Status
	}
	if rec.Provenance == ProvenanceDerived {
		// Derived indicator record (vix/us10y — a field of the us_yahoo
		// batch, written only on regime transitions): it has no fetch cadence
		// of its own, so "is the last fetch fresh" is not a question about
		// this record (k3 audit R3). Its owner channel's verdict is the one
		// that matters; keeping the record's own status here is what the
		// metrics export and the dashboard badge have always shown.
		return rec.Status
	}
	age, ok := lastFetchAge(rec, now)
	if !ok {
		return StatusOK
	}
	if age > contract.EffectiveFreshnessWindow() {
		return StatusStale
	}
	return StatusOK
}

// DeriveChannelStatusForID resolves the channel's contract from the static
// registry and derives the verdict. Callers that only have a channel ID (the
// dashboard payloads, the DB mirror, the metrics export) use this form.
func DeriveChannelStatusForID(rec *ChannelHealthRecord, channelID string, now time.Time) string {
	return DeriveChannelStatus(rec, ChannelContracts().Contract(channelID), now)
}

// DeriveChannelStatusReason returns the human-readable reason behind a derived
// non-ok verdict, or "" when the verdict is "ok". Surfaces WHY a channel is
// stale so the admin page and the dashboard payload stay readable instead of
// showing a bare red/grey pill.
func DeriveChannelStatusReason(rec *ChannelHealthRecord, contract ChannelContract, now time.Time) string {
	switch DeriveChannelStatus(rec, contract, now) {
	case StatusStale:
		age, ok := lastFetchAge(rec, now)
		if !ok {
			return ""
		}
		return fmt.Sprintf("資料已 %s 未更新，超過合約更新窗口 %s（最後一次成功抓取 %s）",
			humanizeAge(age), humanizeWindow(contract.EffectiveFreshnessWindow()), rec.LastFetchAt)
	case StatusError:
		// Degraded that outlived its window (rule 2b). Without a reason the
		// channel page would show a bare "error" that an operator cannot tell
		// apart from a transport failure.
		if rec == nil || rec.Status != StatusDegraded {
			return ""
		}
		age, stamp, ok := degradedDataAge(rec, now)
		if !ok {
			return ""
		}
		return fmt.Sprintf("degraded 已超過合約更新窗口 %s：資料已 %s 未落地（最後一次成功資料 %s）⇒ 升級為 error",
			humanizeWindow(contract.EffectiveFreshnessWindow()), humanizeAge(age), stamp)
	}
	return ""
}

// degradedDataAge returns how long ago the DATA behind a degraded record was
// last seen, plus the stamp it was read from. It reports the NEWEST of the two
// data stamps, so the escalation fires only when even the most recent evidence
// that data exists is older than the window.
//
// E29-3 (2026-09-27). The candidate stamps are the record's own data facts:
//
//	LastDataAt    — when the upstream itself produced the data;
//	LastSuccessAt — when a real payload last landed in atlas.
//
// LastFetchAt is deliberately NOT a fallback: on a degraded record the fetch
// succeeded, so it always looks fresh, and measuring anything against it is how
// a permanently degraded channel stayed invisible.
//
// Known residual gap (registered, not fixed here): a degraded record with
// NEITHER stamp — a channel that has never landed a payload — cannot be timed
// at all, so it stays degraded. Bounding that case needs either a new fact on
// the record ("degraded since") or a streak-based rule, and a streak-based rule
// would page for deliberately deferred upstreams (twse_oddlot and friends).
func degradedDataAge(rec *ChannelHealthRecord, now time.Time) (time.Duration, string, bool) {
	if rec == nil {
		return 0, "", false
	}
	var newest time.Time
	var stamp string
	for _, candidate := range []string{rec.LastDataAt, rec.LastSuccessAt} {
		if candidate == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, candidate)
		if err != nil {
			continue
		}
		if newest.IsZero() || ts.After(newest) {
			newest, stamp = ts, candidate
		}
	}
	if newest.IsZero() {
		return 0, "", false
	}
	return now.Sub(newest), stamp, true
}

// lastFetchAge returns how long ago the record's last fetch happened.
func lastFetchAge(rec *ChannelHealthRecord, now time.Time) (time.Duration, bool) {
	if rec == nil || rec.LastFetchAt == "" {
		return 0, false
	}
	ts, err := time.Parse(time.RFC3339, rec.LastFetchAt)
	if err != nil {
		return 0, false
	}
	return now.Sub(ts), true
}

// humanizeAge renders a duration as a compact, operator-readable age:
// "17 天 3 小時", "3 小時 12 分", "45 秒". Zero components are omitted.
func humanizeAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%d 天 %d 小時", days, hours)
	case days > 0:
		return fmt.Sprintf("%d 天", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%d 小時 %d 分", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%d 小時", hours)
	case minutes > 0:
		return fmt.Sprintf("%d 分", minutes)
	default:
		return fmt.Sprintf("%d 秒", secs)
	}
}

// humanizeWindow renders a contract freshness window the way an operator would
// declare it: hours for the sub-3-day windows in use (48h default, 72h replay),
// whole days above that (8 days for the TDCC weekly snapshot).
func humanizeWindow(d time.Duration) string {
	if d >= 72*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d 天", int(d.Hours())/24)
	}
	return fmt.Sprintf("%d 小時", int(d.Hours()))
}

// ChannelHealthSyncValues is one row of the channel_health DB mirror.
//
// The status column carries the DERIVED verdict (DeriveChannelStatus), the same
// string every UI surface shows, so a DB query and the admin page agree. The
// timestamp/streak columns carry FACTS copied from the record (never the sync
// clock — stamping time.Now() there was the 2026-09-24 "DB looks fresh while
// the data is 17 days old" bug).
type ChannelHealthSyncValues struct {
	ChannelID           string
	Status              string
	LastFetchAt         time.Time
	LastSuccessAt       *time.Time
	ConsecutiveFailures int
}

// ChannelHealthSyncValuesFor computes the DB-mirror values for one record.
//
// LastFetchAt falls back to now when the record carries no parseable timestamp:
// the column is NOT NULL, and a record without a fetch time is broken in a way
// the status column already reports (it cannot be derived as ok). LastSuccessAt
// is left nil when empty so the upsert's COALESCE keeps the previous value.
func ChannelHealthSyncValuesFor(channelID string, rec *ChannelHealthRecord, now time.Time) ChannelHealthSyncValues {
	v := ChannelHealthSyncValues{
		ChannelID: channelID,
		Status:    DeriveChannelStatusForID(rec, channelID, now),
	}
	if rec == nil {
		v.LastFetchAt = now
		return v
	}
	if ts, err := time.Parse(time.RFC3339, rec.LastFetchAt); err == nil {
		v.LastFetchAt = ts
	} else {
		v.LastFetchAt = now
	}
	if ts, err := time.Parse(time.RFC3339, rec.LastSuccessAt); err == nil {
		v.LastSuccessAt = &ts
	}
	v.ConsecutiveFailures = rec.ConsecutiveFailures
	return v
}

// FetchOutcomeStatus classifies the outcome of a fetch that did NOT error, so
// the record written for it tells the truth about the payload.
//
// An adapter returns FetchResult.Stale=true in two very different situations:
//
//   - Expected no-data: TWSE publishes nothing for the past 7 days on
//     non-trading days (twse_margin, twse_capital_flow). This is routine and
//     MUST stay "ok" — flagging it would light up every weekend.
//   - Upstream gone / empty payload: twse_oddlot (BFI84U repurposed, 2026-08)
//     and twse_etf (TWT44U removed, 2026-08) can never produce data again. The
//     contract declares DegradedOnEmpty for those channels, so an empty payload
//     is recorded as "degraded" with an explicit reason instead of "ok".
//
// Channels that do not declare DegradedOnEmpty keep the historical "ok"
// outcome (the raw facts — LastFetchAt, LastSuccessAt, the fetch log — still
// record what happened).
func FetchOutcomeStatus(result *FetchResult, contract ChannelContract) (status, reason string) {
	if result == nil || !result.Stale {
		return StatusOK, ""
	}
	if !contract.DegradedOnEmpty {
		// Routine no-new-data (non-trading day): a successful lookup that
		// found nothing new is not a degraded channel.
		return StatusOK, ""
	}
	reason = fmt.Sprintf("%s: 上游回傳空/停用資料（stale payload）", contract.ChannelID)
	if result.Meta.LastError != "" {
		reason += "：" + result.Meta.LastError
	}
	return StatusDegraded, reason
}
