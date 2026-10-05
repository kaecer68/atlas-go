package service

import (
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// 「需關注」的三種成因（2026-10-05）。
//
// 由來：業主在後台資料通道頁看到 twse_sbl / government_broker 於週末顯示為待處理
// 異常，而同一時間 finmind / tdcc_equity_dispersion 的 warn 也混在同一張清單裡。
// 三者的處置者完全不同，混在一起管理者無法一眼分辨「我們的問題 vs 上游的問題 vs
// 時間還沒到」——這正是本分類要修掉的東西：
//
//	system_error   系統錯誤：atlas 這一側的問題（傳輸、schema、設定、註冊），要有人動手。
//	upstream_limit 已知上游限制：配額 / tier / 上游自己停止發布，已登錄且已宣告，
//	               本質上不是 atlas 故障（只等上游恢復或預算重置）。
//	expected_wait  預期等待：日曆未到。通道宣告 PublishCalendar 且資料並未逾約
//	               （上游在該期間沒有發布機會），所以現在什麼都不必做。
//	retired        設計退休：上游永久消失、替代輸入已接線。不是事故，因此不進
//	               「需關注」清單（見 ChannelContract.Retirement）。
//
// 這四者是同一個判定的四種結果，所以只有這一份實作：後端算出 category，前端只負責
// 標題與排版（與 channel status 的 label/tone 同一分工原則）。
const (
	// AttentionCategorySystemError — atlas 這一側的問題。
	AttentionCategorySystemError = "system_error"
	// AttentionCategoryUpstreamLimit — 已知且已宣告的上游限制（配額／tier／上游停發）。
	AttentionCategoryUpstreamLimit = "upstream_limit"
	// AttentionCategoryExpectedWait — 日曆未到（合約宣告的發布日曆尚未要求新資料）。
	AttentionCategoryExpectedWait = "expected_wait"
	// AttentionCategoryRetired — 設計退休（不進「需關注」）。
	AttentionCategoryRetired = "retired"
)

// upstreamLimitMessagePatterns is the FALLBACK signal for a channel that has no
// ChannelContract.KnownUpstreamLimit declaration. It exists because the
// declaration list is deliberately conservative (declared only with evidence),
// while a typed upstream-limit condition can still reach the page from a channel
// nobody has reviewed yet. Matching a message is second-best; it is used here
// only to AVOID accusing atlas of a fault the message already attributes to the
// upstream.
var upstreamLimitMessagePatterns = []string{
	"quota",
	"額度",
	"rate limit",
	"rate limited",
	"ip banned",
	"empty quote",
	"402",
	"requests reach the upper limit",
}

// ClassifyChannelAttention answers WHY a non-ok channel needs attention.
//
// It returns "" for statuses that are not attention cases at all (ok / inactive /
// unknown): an operator-disabled channel is a decision, and a missing record is
// not a condition, so neither may be dressed up as one.
//
// Precedence is deliberate. Retirement first (a contract fact outranks any
// record), then the calendar (the only category that says "nothing to do NOW"),
// then the declared upstream limit, then — the default — atlas's own problem.
// The default matters: an undeclared condition is reported against atlas rather
// than excused, so this classifier can only ever make the page more honest.
func ClassifyChannelAttention(contract apigateway.ChannelContract, rec *apigateway.ChannelHealthRecord, status, lastError string, now time.Time) string {
	if contract.IsRetired() || status == apigateway.StatusRetired {
		return AttentionCategoryRetired
	}
	switch status {
	case "", apigateway.StatusOK, apigateway.StatusInactive, apigateway.StatusUnknown:
		return ""
	}
	if channelWaitsForCalendar(contract, rec, now) {
		return AttentionCategoryExpectedWait
	}
	if contract.KnownUpstreamLimit != "" || messageIsUpstreamLimit(lastError) {
		return AttentionCategoryUpstreamLimit
	}
	return AttentionCategorySystemError
}

// channelWaitsForCalendar reports whether a channel's non-ok verdict is fully
// explained by its publish calendar — i.e. the upstream has had no publish
// opportunity yet, so nothing is late.
//
// The measurement is the SAME contract judgment the alert path uses
// (apigateway.ChannelContract.StalenessOverageSeconds, fed by the same data
// stamps as apigateway.DataAge): a second, wall-clock-only implementation here
// is exactly the drift this change exists to remove.
func channelWaitsForCalendar(contract apigateway.ChannelContract, rec *apigateway.ChannelHealthRecord, now time.Time) bool {
	if rec == nil || contract.PublishCalendar != apigateway.PublishCalendarTWTradingDay {
		return false
	}
	age, at, ok := channelDataAgeAt(rec, now)
	if !ok {
		return false
	}
	return contract.StalenessOverageSeconds(age.Seconds(), at, now) <= 0
}

// channelDataAgeAt returns the age of the record's DATA and the parsed instant it
// was measured against, using apigateway.DataAge's anchors (the newest of
// LastDataAt / LastSuccessAt) and falling back to LastFetchAt when the record
// carries neither — a fetch-only record still says something about when the
// channel was last able to produce data.
func channelDataAgeAt(rec *apigateway.ChannelHealthRecord, now time.Time) (time.Duration, time.Time, bool) {
	if rec == nil {
		return 0, time.Time{}, false
	}
	if age, stamp, ok := apigateway.DataAge(rec, now); ok {
		if ts, err := time.Parse(time.RFC3339, stamp); err == nil {
			return age, ts, true
		}
	}
	if ts, err := time.Parse(time.RFC3339, rec.LastFetchAt); err == nil {
		return now.Sub(ts), ts, true
	}
	return 0, time.Time{}, false
}

// messageIsUpstreamLimit is the fallback classification signal (see
// upstreamLimitMessagePatterns). Comparisons are case-insensitive because the
// stored text comes from several layers (provider, gateway, DB mirror).
func messageIsUpstreamLimit(lastError string) bool {
	if lastError == "" {
		return false
	}
	lower := strings.ToLower(lastError)
	for _, p := range upstreamLimitMessagePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
