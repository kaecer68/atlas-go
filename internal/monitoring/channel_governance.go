// channel_governance.go — the "a permanently broken channel must be retired or
// fixed" criterion (issue #2138), in mechanical form.
//
// WHY this exists
// ---------------
// #2134 retired twse_oddlot after its upstream had been gone for 60+ days. The
// data showed the failure shape was not accidental: the upstream was
// PERMANENTLY unavailable, a replacement input already existed, and yet the
// channel kept a permanently-firing ChannelHealthStatusError for two months
// because "known issue" was only a UI badge and nothing ever forced a decision.
// This file turns "known but unhandled" into something a machine can gate.
//
// THE CRITERION (all three must hold)
// -----------------------------------
//  1. the upstream is PERMANENTLY unavailable, with first-party evidence
//     (KnownIssue.UpstreamRemovedAt non-empty);
//  2. a replacement input ALREADY exists (KnownIssue.ReplacementInput non-empty)
//     — without one there is nothing to retire TO and the channel is a
//     monitor/fix case;
//  3. the channel has been `degraded`/`error` with data older than
//     N × its own contract window.
//
// Plus a human deadline (`ActionBy`) that must either ship (`RetiredAt`) or be
// renewed explicitly, so the case cannot sit silently between 1–3 and a decision.
//
// WHAT IS FORBIDDEN
// -----------------
// Loosening the rule, suppressing the alert, or widening the window to make the
// symptom disappear. That is concealment: the criterion exists precisely so an
// unhandled known failure stays visible until it is retired or fixed.
//
// WHY N = 2 CONTRACT WINDOWS (and not 1, and not 60 days)
// -------------------------------------------------------
//
//	· N = 1 is NOT a criterion: "degraded AND data older than one window" is
//	  exactly the E29-3 rule-2b escalation point — i.e. "this channel is error
//	  right now". Using it would demand retirement for every transient (an
//	  upstream that simply published nothing today, a schema blip), which is
//	  the opposite of a governance criterion.
//	· N = 2 buys exactly one more full window of "it did not recover" as
//	  evidence, while bounding the cost: every extra window is another window
//	  with a permanently-firing alert. The measured case burnt 60 days; N = 2
//	  (≈96h on the default 48h window) bounds that by roughly 15×.
//	· Expressing N in CONTRACT WINDOWS rather than days is what makes it safe
//	  for slow upstreams: a weekly channel (TDCC, 8-day window) gets ≈16 days,
//	  a monthly one longer still — its own cadence decides.
//	· The old 60-day figure is an OBSERVATION of neglect, not a target. It is
//	  quoted here so nobody mistakes it for a calibrated threshold.
package monitoring

import (
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

const GovernanceWindowMultiplier = 2

// governanceExcludedStatuses are the verdicts that mean "no usable data landed"
// — the only statuses the dynamic condition can hold for. `inactive` is the
// retirement outcome (the criterion is already satisfied by then) and `ok` is
// by definition not a failure.
func governanceStatusIsFailure(status string) bool {
	return status == apigateway.StatusDegraded || status == apigateway.StatusError
}

// GovernanceDeadline is the STATIC half of the criterion: what the registry
// itself must declare, and whether the declared deadline has passed. It needs no
// channel record, which is what makes it evaluable in CI.
type GovernanceDeadline struct {
	// Channel is the channel ID (or the dash-alias) this entry describes.
	Channel string
	// AvailabilityCase is true when the entry declares first-party evidence of a
	// permanent upstream removal. False means "not applicable" (a dead alias, a
	// condition that is not about availability) — a positive declaration.
	AvailabilityCase bool
	// HasReplacement mirrors KnownIssue.ReplacementInput != "". Only an
	// availability case with a replacement is a retire candidate.
	HasReplacement bool
	// ActionBy is the declared decision deadline (zero when none is declared).
	ActionBy time.Time
	// RetiredAt is the declared ship date (zero when the disposition is open).
	RetiredAt time.Time
	// DaysSinceDeclared is how long the removal has been on record. It is
	// reported even when a deadline was renewed, so repeated renewals cannot hide
	// the total elapsed time.
	DaysSinceDeclared float64
	// Overdue is the CI-gating verdict: a declared deadline in the past with no
	// RetiredAt. Deterministic in `now` — never read from the wall clock here.
	Overdue bool
	// Deficiencies lists static contract problems (the entry cannot be judged).
	Deficiencies []string
}

// EvaluateGovernanceDeadline applies the static contract to one registry entry.
//
// Contract:
//   - an entry with no UpstreamRemovedAt is NOT an availability case: it opts
//     out of the criterion, and that is a declaration, not an omission (this is
//     how the taifex-daily dead alias is excluded without inventing a second
//     field);
//   - an availability case must declare a disposition: RetiredAt (shipped) or
//     ActionBy (decision due). Neither ⇒ deficiency (the entry is unjudgeable);
//   - a declared ActionBy in the past with no RetiredAt ⇒ Overdue.
//
// Renewal is allowed: pushing ActionBy forward while the disposition is still
// open is a legal, green-making action — as long as the same change re-examines
// UpstreamRemovedAt / ReplacementInput and the PR says why. DaysSinceDeclared is
// printed by the caller so the total time stays visible either way.
func EvaluateGovernanceDeadline(channelID string, issue KnownIssue, now time.Time) GovernanceDeadline {
	out := GovernanceDeadline{Channel: channelID}
	if issue.UpstreamRemovedAt == "" {
		return out
	}
	out.AvailabilityCase = true
	out.HasReplacement = issue.ReplacementInput != ""

	if removedAt, err := time.Parse(time.RFC3339, issue.UpstreamRemovedAt); err == nil {
		out.DaysSinceDeclared = now.Sub(removedAt).Hours() / 24
	}
	if actionBy, err := time.Parse(time.RFC3339, issue.ActionBy); err == nil {
		out.ActionBy = actionBy
	}
	if retiredAt, err := time.Parse(time.RFC3339, issue.RetiredAt); err == nil {
		out.RetiredAt = retiredAt
	}

	retired := !out.RetiredAt.IsZero()
	hasDeadline := !out.ActionBy.IsZero()
	switch {
	case !retired && !hasDeadline:
		out.Deficiencies = append(out.Deficiencies,
			"尚未宣告處置：必須二選一 —— RetiredAt（已退役/已修復的日期）或 ActionBy（決議期限）；"+
				"禁止以放寬規則／抑制告警／延長窗口讓它消失")
	case !retired && hasDeadline && now.After(out.ActionBy):
		out.Overdue = true
	}
	return out
}

// GovernanceDecision is the DYNAMIC half: the three-condition judgement for one
// channel at one instant, computed from the registry entry plus the live
// channel-health record.
type GovernanceDecision struct {
	Channel string
	// AvailabilityCase / HasReplacement mirror the registry declarations.
	AvailabilityCase bool
	HasReplacement   bool
	// Status is the record's stored verdict ("" when there is no record).
	Status string
	// DataAgeWindows is the data age in units of the channel's own contract
	// window; 0 when the age cannot be determined.
	DataAgeWindows float64
	// Timable is false when the record carries no data stamp at all — the case
	// then stays unjudged (reported, never guessed).
	Timable bool
	// ShouldRetire is the criterion: all three conditions hold.
	ShouldRetire bool
	// Overdue is the registry deadline verdict (see EvaluateGovernanceDeadline).
	Overdue bool
	// Reasons explains the verdict for a human/log line.
	Reasons []string
}

// EvaluateChannelGovernance judges one channel against the criterion.
//
// Pure and deterministic: `now` is injected, the contract is passed in, and no
// clock, registry singleton or I/O is read.
func EvaluateChannelGovernance(issue *KnownIssue, rec *apigateway.ChannelHealthRecord, contract apigateway.ChannelContract, now time.Time) GovernanceDecision {
	out := GovernanceDecision{}
	if issue == nil {
		out.Reasons = append(out.Reasons, "no known-issue entry: the criterion needs first-party evidence of the removal")
		return out
	}
	deadline := EvaluateGovernanceDeadline(contract.ChannelID, *issue, now)
	out.Channel = firstNonEmpty(contract.ChannelID, issue.Key)
	out.AvailabilityCase = deadline.AvailabilityCase
	out.HasReplacement = deadline.HasReplacement
	out.Overdue = deadline.Overdue

	if !deadline.AvailabilityCase {
		out.Reasons = append(out.Reasons, "not an availability case (no UpstreamRemovedAt declared) — criterion not applicable")
		return out
	}
	if !deadline.HasReplacement {
		out.Reasons = append(out.Reasons, "no replacement input exists — there is nothing to retire TO: monitor/fix, not retire")
	}
	if rec == nil {
		out.Reasons = append(out.Reasons, "no channel-health record: the dynamic condition cannot be judged")
		return out
	}
	out.Status = rec.Status

	window := contract.EffectiveFreshnessWindow()
	if age, _, ok := apigateway.DataAge(rec, now); ok && window > 0 {
		out.Timable = true
		out.DataAgeWindows = age.Hours() / window.Hours()
	} else if window <= 0 {
		out.Reasons = append(out.Reasons, "contract has no freshness window: the data age cannot be expressed in windows")
	} else {
		out.Reasons = append(out.Reasons, "record carries no data timestamp: the data age cannot be determined")
	}

	switch {
	case !governanceStatusIsFailure(rec.Status):
		out.Reasons = append(out.Reasons, fmt.Sprintf("status=%s is not a failure verdict (degraded/error) — no governance case", rec.Status))
	default:
		if out.Timable && out.DataAgeWindows > float64(GovernanceWindowMultiplier) {
			out.Reasons = append(out.Reasons, fmt.Sprintf(
				"status=%s and data is %.1f contract windows old (> %d) — permanent-or-not must be decided",
				rec.Status, out.DataAgeWindows, GovernanceWindowMultiplier))
			out.ShouldRetire = out.AvailabilityCase && out.HasReplacement
		} else if out.Timable {
			out.Reasons = append(out.Reasons, fmt.Sprintf(
				"status=%s but data is only %.1f contract windows old (≤ %d) — a transient failure must not be retired",
				rec.Status, out.DataAgeWindows, GovernanceWindowMultiplier))
		}
	}
	if out.Overdue {
		out.Reasons = append(out.Reasons, fmt.Sprintf(
			"registry deadline ActionBy=%s has passed with no RetiredAt: choose retire-or-fix (or renew explicitly with the reason)",
			issue.ActionBy))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// GovernanceNotifier implements the "remind ONCE, not continuously" requirement
// of issue #2138: it reports the transition into the overdue state, not every
// observation of it. A permanently repeating reminder is just a second paging
// channel — the exact fatigue the criterion exists to prevent.
//
// Semantics (all deterministic over the injected `now`, so it is testable):
//   - the FIRST observation of an overdue channel also emits (it is a
//     transition from "unknown" to "overdue");
//   - while it stays overdue, nothing more is emitted;
//   - when it stops being overdue the state is cleared, so a recurrence emits
//     again;
//   - a nil notifier emits nothing (callers that only want the gauge pass nil).
type GovernanceNotifier struct {
	mu       sync.Mutex
	overdue  map[string]bool
	observed map[string]time.Time
}

// NewGovernanceNotifier creates an empty notifier.
func NewGovernanceNotifier() *GovernanceNotifier {
	return &GovernanceNotifier{overdue: map[string]bool{}, observed: map[string]time.Time{}}
}

// Note records one observation and reports whether it just became overdue.
func (n *GovernanceNotifier) Note(channel string, overdue bool, now time.Time) bool {
	if n == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.overdue == nil {
		n.overdue = map[string]bool{}
	}
	if n.observed == nil {
		n.observed = map[string]time.Time{}
	}
	was := n.overdue[channel]
	n.overdue[channel] = overdue
	n.observed[channel] = now
	if overdue && !was {
		// Keep the current state recorded, but mark it as already reported so the
		// next observation does not emit again.
		n.overdue[channel] = true
		return true
	}
	return false
}
