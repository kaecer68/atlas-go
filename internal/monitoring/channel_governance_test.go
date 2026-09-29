// channel_governance_test.go — issue #2138: the "permanently broken channel must
// be retired or fixed" criterion, pinned on the case that actually happened.
//
// The tests are written against the PRODUCTION SHAPE of twse_oddlot (degraded
// record, last real data 2026-09-07, 48h contract window, alert firing since
// 2026-09-29) because that is the observation the criterion exists for. Every
// verdict below therefore has to be explainable by pointing at those numbers.
package monitoring

import (
	"sort"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// productionOddLotIssue is the registry entry as it would have looked BEFORE the
// retirement shipped: evidence + replacement + a deadline, and no RetiredAt.
func productionOddLotIssue() KnownIssue {
	return KnownIssue{
		Key:               "twse_oddlot",
		Title:             "TWSE odd-lot trading report removed (BFI84U repurposed)",
		DocumentedAt:      "2026-08-05T00:00:00Z",
		UpstreamRemovedAt: "2026-08-01T00:00:00Z",
		ReplacementInput:  "twse_capital_flow 代理（monitoring.NewOddLotFetcher → oddLotFromCapitalFlow）",
		ActionBy:          "2026-09-29T00:00:00Z",
	}
}

// productionOddLotRecord is the record the alert path was firing on.
func productionOddLotRecord() *apigateway.ChannelHealthRecord {
	return &apigateway.ChannelHealthRecord{
		Status:        apigateway.StatusDegraded,
		LastFetchAt:   "2026-09-27T12:59:28Z",
		LastSuccessAt: "2026-09-07T00:18:11Z",
		LastError:     "twse_oddlot: 上游回傳空/停用資料（stale payload）",
	}
}

func oddLotContract() apigateway.ChannelContract {
	return apigateway.ChannelContracts().Contract("twse_oddlot")
}

func TestEvaluateChannelGovernance_WorkedExampleTwseOddlot(t *testing.T) {
	// The instant the criterion is judged at: after the deadline, before anyone
	// acted. This is the state that used to be able to persist for 60 days.
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	issue := productionOddLotIssue()

	got := EvaluateChannelGovernance(&issue, productionOddLotRecord(), oddLotContract(), now)

	if !got.AvailabilityCase {
		t.Fatal("AvailabilityCase = false: the entry declares first-party evidence of a permanent removal")
	}
	if !got.HasReplacement {
		t.Fatal("HasReplacement = false: the twse_capital_flow proxy already served the consumer")
	}
	if !got.ShouldRetire {
		t.Fatalf("ShouldRetire = false for the production shape that fired for 60+ days (reasons=%v)", got.Reasons)
	}
	if !got.Overdue {
		t.Fatalf("Overdue = false although ActionBy (2026-09-29) had passed with no RetiredAt (reasons=%v)", got.Reasons)
	}
	// 22.6 days of data over a 48h window ⇒ ~11.3 windows; the criterion's line is
	// 2 windows (see GovernanceWindowMultiplier), so this is not a near miss.
	if got.DataAgeWindows < 10 {
		t.Errorf("DataAgeWindows = %.2f, want > 10 (22.6 days / 2 days)", got.DataAgeWindows)
	}
	if !got.Timable {
		t.Error("Timable = false although the record carries last_success_at")
	}
}

func TestEvaluateChannelGovernance_ShippedDispositionTurnsItGreen(t *testing.T) {
	// The registry entry AS SHIPPED (retired 2026-09-29, #2136). Determinism
	// guardrail: the same `now` that produced "overdue" above must now be green,
	// otherwise a legal disposition could never clear the gate.
	issue := *LookupKnownIssue("twse_oddlot")
	if issue.RetiredAt == "" {
		t.Fatal("the retired entry must carry RetiredAt; without it CI can never go green again")
	}
	for _, now := range []time.Time{
		time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC),
		time.Date(2027, 9, 29, 7, 0, 0, 0, time.UTC),
	} {
		dl := EvaluateGovernanceDeadline("twse_oddlot", issue, now)
		if dl.Overdue {
			t.Errorf("Overdue = true at %s although RetiredAt=%s is declared", now.Format(time.RFC3339), issue.RetiredAt)
		}
		if len(dl.Deficiencies) != 0 {
			t.Errorf("Deficiencies = %v at %s, want none", dl.Deficiencies, now.Format(time.RFC3339))
		}
	}
}

func TestEvaluateChannelGovernance_NegativeCases(t *testing.T) {
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	contract := oddLotContract()

	t.Run("transient failure must not be retired", func(t *testing.T) {
		issue := productionOddLotIssue()
		// Same channel, same permanent evidence — but the data is only 6h old, so
		// "it did not recover" has not been demonstrated.
		rec := &apigateway.ChannelHealthRecord{
			Status:        apigateway.StatusDegraded,
			LastFetchAt:   "2026-09-29T06:00:00Z",
			LastSuccessAt: "2026-09-29T01:00:00Z",
		}
		got := EvaluateChannelGovernance(&issue, rec, contract, now)
		if got.ShouldRetire {
			t.Fatalf("ShouldRetire = true for a 6h-old failure (reasons=%v)", got.Reasons)
		}
		if got.DataAgeWindows >= float64(GovernanceWindowMultiplier) {
			t.Fatalf("DataAgeWindows = %.2f, want < %d", got.DataAgeWindows, GovernanceWindowMultiplier)
		}
	})

	t.Run("permanent removal without a replacement is not a retire case", func(t *testing.T) {
		// The bdi shape: CNBC `.BADI` has been price-less since 2026-09-20 and no
		// usable alternative exists, so there is nothing to retire TO.
		issue := *LookupKnownIssue("bdi")
		rec := &apigateway.ChannelHealthRecord{
			Status:        apigateway.StatusError,
			LastFetchAt:   "2026-09-29T06:00:00Z",
			LastSuccessAt: "2026-09-20T08:35:00Z",
		}
		got := EvaluateChannelGovernance(&issue, rec, contract, now)
		if !got.AvailabilityCase {
			t.Fatal("AvailabilityCase = false: bdi does declare an upstream removal")
		}
		if got.HasReplacement {
			t.Fatal("HasReplacement = true for bdi, but no usable alternative source exists")
		}
		if got.ShouldRetire {
			t.Fatalf("ShouldRetire = true without a replacement input (reasons=%v)", got.Reasons)
		}
	})

	t.Run("dead alias is not an availability case", func(t *testing.T) {
		issue := *LookupKnownIssue("taifex-daily")
		rec := &apigateway.ChannelHealthRecord{Status: apigateway.StatusError, LastSuccessAt: "2026-06-04T00:00:00Z"}
		got := EvaluateChannelGovernance(&issue, rec, contract, now)
		if got.AvailabilityCase {
			t.Fatal("AvailabilityCase = true for the dead alias: UpstreamRemovedAt is empty by declaration")
		}
		if got.ShouldRetire || got.Overdue {
			t.Fatalf("the dead alias must never be a retire case (reasons=%v)", got.Reasons)
		}
	})

	t.Run("already retired (inactive) is not a failure verdict", func(t *testing.T) {
		issue := productionOddLotIssue() // deadline already passed, so Overdue stays true
		rec := &apigateway.ChannelHealthRecord{
			Status:        apigateway.StatusInactive,
			LastFetchAt:   "2026-09-29T06:00:00Z",
			LastSuccessAt: "2026-09-07T00:18:11Z",
		}
		got := EvaluateChannelGovernance(&issue, rec, contract, now)
		if got.ShouldRetire {
			t.Fatalf("ShouldRetire = true for an inactive record: the retirement already happened (reasons=%v)", got.Reasons)
		}
	})

	t.Run("unknown data age is reported, never guessed", func(t *testing.T) {
		issue := productionOddLotIssue()
		rec := &apigateway.ChannelHealthRecord{Status: apigateway.StatusError}
		got := EvaluateChannelGovernance(&issue, rec, contract, now)
		if got.Timable {
			t.Error("Timable = true although the record carries no data timestamp")
		}
		if got.ShouldRetire {
			t.Fatalf("ShouldRetire = true without a measurable data age (reasons=%v)", got.Reasons)
		}
	})

	t.Run("no known-issue entry → no criterion", func(t *testing.T) {
		got := EvaluateChannelGovernance(nil, productionOddLotRecord(), contract, now)
		if got.ShouldRetire || got.AvailabilityCase {
			t.Fatalf("a channel without a known-issue entry must not be judged (reasons=%v)", got.Reasons)
		}
	})
}

func TestEvaluateChannelGovernance_WindowBoundary(t *testing.T) {
	// The boundary is `>`, not `>=`: at exactly N windows the evidence is "N
	// windows without recovery", which the criterion deliberately does not yet
	// call permanent. Pinned with an explicitly injected now.
	issue := productionOddLotIssue()
	contract := oddLotContract()
	window := contract.EffectiveFreshnessWindow()
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

	exactlyN := &apigateway.ChannelHealthRecord{
		Status:        apigateway.StatusDegraded,
		LastFetchAt:   now.Format(time.RFC3339),
		LastSuccessAt: now.Add(-time.Duration(GovernanceWindowMultiplier) * window).Format(time.RFC3339),
	}
	if got := EvaluateChannelGovernance(&issue, exactlyN, contract, now); got.ShouldRetire {
		t.Errorf("exactly %d windows must NOT be a retire case (DataAgeWindows=%.4f)", GovernanceWindowMultiplier, got.DataAgeWindows)
	}

	justOverN := &apigateway.ChannelHealthRecord{
		Status:        apigateway.StatusDegraded,
		LastFetchAt:   now.Format(time.RFC3339),
		LastSuccessAt: now.Add(-time.Duration(GovernanceWindowMultiplier)*window - time.Minute).Format(time.RFC3339),
	}
	if got := EvaluateChannelGovernance(&issue, justOverN, contract, now); !got.ShouldRetire {
		t.Errorf("one minute past %d windows must be a retire case (DataAgeWindows=%.4f, reasons=%v)", GovernanceWindowMultiplier, got.DataAgeWindows, got.Reasons)
	}
}

func TestEvaluateGovernanceDeadline_DeficiencyAndRenewal(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	t.Run("availability case with no disposition is a deficiency", func(t *testing.T) {
		got := EvaluateGovernanceDeadline("x", KnownIssue{Key: "x", UpstreamRemovedAt: "2026-09-01T00:00:00Z"}, now)
		if !got.AvailabilityCase {
			t.Fatal("AvailabilityCase = false")
		}
		if len(got.Deficiencies) == 0 {
			t.Fatal("an availability case with neither RetiredAt nor ActionBy must be a deficiency (the entry is unjudgeable)")
		}
		if got.Overdue {
			t.Error("Overdue must stay false: there is no declared deadline to be late for")
		}
	})

	t.Run("renewal is legal and makes the gate green again", func(t *testing.T) {
		overdue := KnownIssue{Key: "x", UpstreamRemovedAt: "2026-09-01T00:00:00Z", ActionBy: "2026-09-15T00:00:00Z"}
		if dl := EvaluateGovernanceDeadline("x", overdue, now); !dl.Overdue {
			t.Fatal("a past ActionBy with no RetiredAt must be Overdue")
		}
		renewed := overdue
		renewed.ActionBy = "2026-11-01T00:00:00Z"
		if dl := EvaluateGovernanceDeadline("x", renewed, now); dl.Overdue {
			t.Fatal("a renewed deadline must clear the gate")
		}
		// Anti-dilution: the elapsed time stays visible no matter how often it is renewed.
		if dl := EvaluateGovernanceDeadline("x", renewed, now); dl.DaysSinceDeclared < 29 {
			t.Errorf("DaysSinceDeclared = %.1f, want ≈30 (the report must always show how long this has been open)", dl.DaysSinceDeclared)
		}
	})

	t.Run("not an availability case has nothing to declare", func(t *testing.T) {
		got := EvaluateGovernanceDeadline("taifex-daily", KnownIssue{Key: "taifex-daily"}, now)
		if got.AvailabilityCase || len(got.Deficiencies) != 0 || got.Overdue {
			t.Fatalf("an entry without UpstreamRemovedAt opts out entirely, got %+v", got)
		}
	})
}

// TestKnownIssues_RegistrySatisfiesTheStaticContract locks the shipped registry
// itself: every availability case must carry a disposition, and the ids the
// governance report prints must stay stable (the report is diffed by humans).
func TestKnownIssues_RegistrySatisfiesTheStaticContract(t *testing.T) {
	entries := KnownIssueEntries()
	if len(entries) == 0 {
		t.Fatal("KnownIssueEntries() is empty")
	}
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ChannelID)
		if e.Issue.UpstreamRemovedAt == "" {
			continue
		}
		if dl := EvaluateGovernanceDeadline(e.ChannelID, e.Issue, now); len(dl.Deficiencies) != 0 {
			t.Errorf("%s: %v", e.ChannelID, dl.Deficiencies)
		}
	}
	// Determinism: the report prints this list, so the order must be sorted.
	sortedIDs := append([]string(nil), ids...)
	sort.Strings(sortedIDs)
	for i := range sortedIDs {
		if ids[i] != sortedIDs[i] {
			t.Fatalf("KnownIssueEntries() is not sorted: %v", ids)
		}
	}
	// SET EQUALITY, both directions. A registry entry that the accessor does not
	// return is a blind spot: it would never be judged, never printed, and never
	// gated (the "registered but unchecked" shape). An id the accessor returns
	// without a registry entry is impossible by construction, which is exactly
	// why the reverse direction is asserted rather than assumed.
	want := []string{"bdi", "taifex-daily", "twse-etf", "twse-oddlot", "twse_etf", "twse_oddlot"}
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("KnownIssueEntries() = %v, want the whole registry %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("KnownIssueEntries()[%d] = %q, want %q (set equality, not just membership)", i, ids[i], want[i])
		}
	}
	// Every entry must also be reachable through the lookup the RUNTIME uses:
	// LookupKnownIssue is what cmd/atlas consults, so an entry that the accessor
	// knows but the lookup does not is not actually governing anything.
	for _, id := range want {
		if LookupKnownIssue(id) == nil {
			t.Errorf("LookupKnownIssue(%q) = nil although the entry is registered: the accessor and the runtime lookup disagree", id)
		}
	}
}

func TestGovernanceNotifier_RemindsOnceNotContinuously(t *testing.T) {
	n := NewGovernanceNotifier()
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

	if !n.Note("twse_oddlot", true, now) {
		t.Fatal("the first observation of an overdue channel must emit (state was unknown before it)")
	}
	for i := 1; i <= 5; i++ {
		if n.Note("twse_oddlot", true, now.Add(time.Duration(i)*5*time.Minute)) {
			t.Fatalf("observation %d emitted again: a repeating reminder is just a second paging channel", i)
		}
	}
	// Recovering clears the state, so a recurrence is reported again.
	if n.Note("twse_oddlot", false, now.Add(time.Hour)) {
		t.Fatal("clearing the state must not emit")
	}
	if !n.Note("twse_oddlot", true, now.Add(2*time.Hour)) {
		t.Fatal("a recurrence after recovery must emit again")
	}
	// A nil notifier (gauge-only callers) emits nothing and must not panic.
	var nilNotifier *GovernanceNotifier
	if nilNotifier.Note("twse_oddlot", true, now) {
		t.Fatal("a nil notifier must never emit")
	}
	// Channels are tracked independently.
	if !n.Note("other_channel", true, now) {
		t.Fatal("a different channel's first observation must emit")
	}
}
