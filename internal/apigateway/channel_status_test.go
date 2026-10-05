package apigateway

import (
	"strings"
	"testing"
	"time"
)

func mustRFC(t time.Time) string { return t.Format(time.RFC3339) }

// TestDeriveChannelStatus is the contract for the single channel-status
// judgment (2026-09-24 channel-status-truth): record facts in, one verdict out,
// for every consumer.
func TestDeriveChannelStatus(t *testing.T) {
	now := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	// A copy of the twse_oddlot contract with its RETIREMENT CLEARED. The
	// registry entry itself is retired since 2026-10-05 (contract.Retirement),
	// and rule 0 short-circuits every record rule for a retired channel — so the
	// general rule table below needs the pre-retirement shape to be observable at
	// all. The retirement case has its own table entry further down.
	oddlot := ChannelContracts().Contract("twse_oddlot") // window = 48h
	oddlot.Retirement = nil
	tdcc := ChannelContracts().Contract("tdcc_equity_dispersion")

	cases := []struct {
		name string
		rec  *ChannelHealthRecord
		c    ChannelContract
		want string
	}{
		{"no record", nil, oddlot, StatusUnknown},
		{"fresh ok", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-5 * time.Minute))}, oddlot, StatusOK},
		{"ok at the window edge", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-48 * time.Hour))}, oddlot, StatusOK},
		{"expired ok is stale", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}, oddlot, StatusStale},
		{"warn passthrough", &ChannelHealthRecord{Status: StatusWarn, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}, oddlot, StatusWarn},
		{"error passthrough", &ChannelHealthRecord{Status: StatusError, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}, oddlot, StatusError},
		{
			// E29-3: a degraded record with no data stamp cannot be judged, so
			// it stays degraded (never escalate on a fact that does not exist).
			"degraded without a data stamp stays degraded",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))},
			oddlot, StatusDegraded,
		},
		{
			"degraded with fresh data stays degraded (cache still valid)",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastSuccessAt: mustRFC(now.Add(-2 * time.Hour))},
			oddlot, StatusDegraded,
		},
		{
			// The state that had no way out before E29-3: the fetch keeps
			// succeeding (LastFetchAt fresh) while no real data lands.
			"degraded whose data outlived the window escalates",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastSuccessAt: mustRFC(now.Add(-17 * 24 * time.Hour))},
			oddlot, StatusError,
		},
		{
			"degraded anchors on the upstream data stamp",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastDataAt: mustRFC(now.Add(-30 * time.Minute)), LastSuccessAt: mustRFC(now.Add(-30 * 24 * time.Hour))},
			oddlot, StatusDegraded,
		},
		{
			// The NEWEST data stamp wins, so a stale leftover LastDataAt
			// cannot escalate a channel whose data actually landed minutes ago.
			"degraded uses the freshest data stamp, not the first one",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastDataAt: mustRFC(now.Add(-4 * 30 * 24 * time.Hour)), LastSuccessAt: mustRFC(now.Add(-5 * time.Minute))},
			oddlot, StatusDegraded,
		},
		{
			"long window keeps a recent degraded snapshot degraded",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastSuccessAt: mustRFC(now.Add(-3 * 24 * time.Hour))},
			tdcc, StatusDegraded,
		},
		{
			"long window still escalates when the data is over it",
			&ChannelHealthRecord{Status: StatusDegraded, LastFetchAt: mustRFC(now.Add(-5 * time.Minute)), LastSuccessAt: mustRFC(now.Add(-9 * 24 * time.Hour))},
			tdcc, StatusError,
		},
		{"inactive passthrough", &ChannelHealthRecord{Status: StatusInactive, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}, oddlot, StatusInactive},
		{"missing timestamp cannot be judged", &ChannelHealthRecord{Status: StatusOK}, oddlot, StatusOK},
		{"unparseable timestamp cannot be judged", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: "2026-09-07"}, oddlot, StatusOK},
		{"long window keeps slow snapshot ok", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-3 * 24 * time.Hour))}, tdcc, StatusOK},
		{"long window still stales when over", &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-9 * 24 * time.Hour))}, tdcc, StatusStale},
		{
			"derived indicator records are not channels",
			&ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-60 * time.Hour)), Provenance: ProvenanceDerived},
			oddlot, StatusOK,
		},
	}
	for _, tc := range cases {
		if got := DeriveChannelStatus(tc.rec, tc.c, now); got != tc.want {
			t.Errorf("%s: DeriveChannelStatus = %q, want %q", tc.name, got, tc.want)
		}
	}

	// The stale verdict must carry a readable reason (channel page readability).
	rec := &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}
	reason := DeriveChannelStatusReason(rec, oddlot, now)
	if reason == "" || !strings.Contains(reason, "17 天") {
		t.Errorf("stale reason = %q, want a human age (17 天) and the window", reason)
	}
	if got := DeriveChannelStatusReason(&ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now)}, oddlot, now); got != "" {
		t.Errorf("ok record must have no reason, got %q", got)
	}
}

// TestFetchOutcomeStatus pins the empty-payload rule: an adapter that flags a
// stale/empty payload only degrades the channel when the contract says empty
// data is a problem (twse_oddlot). Routine non-trading-day no-data
// (twse_margin / twse_capital_flow) stays ok — flagging it would light up
// every weekend.
func TestFetchOutcomeStatus(t *testing.T) {
	oddlot := ChannelContracts().Contract("twse_oddlot")
	margin := ChannelContracts().Contract("twse_margin")

	if got, reason := FetchOutcomeStatus(nil, oddlot); got != StatusOK || reason != "" {
		t.Errorf("nil result = (%q,%q), want ok", got, reason)
	}
	fresh := &FetchResult{Data: []byte("[]")}
	if got, _ := FetchOutcomeStatus(fresh, oddlot); got != StatusOK {
		t.Errorf("non-stale result = %q, want ok", got)
	}
	stale := &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: "twse_oddlot", LastError: "BFI84U repurposed"}}
	if got, _ := FetchOutcomeStatus(stale, margin); got != StatusOK {
		t.Errorf("expected-no-data channel = %q, want ok (non-trading day is not degraded)", got)
	}
	got, reason := FetchOutcomeStatus(stale, oddlot)
	if got != StatusDegraded {
		t.Errorf("removed-upstream channel = %q, want degraded (ok would hide the empty payload)", got)
	}
	if !strings.Contains(reason, "BFI84U repurposed") {
		t.Errorf("degraded reason = %q, want the adapter's error text attached", reason)
	}
	// The contract registry must keep the declaration the rule depends on.
	if !oddlot.DegradedOnEmpty {
		t.Error("twse_oddlot contract must declare DegradedOnEmpty (upstream removed, empty payload is a failure)")
	}
}

// TestChannelHealthSyncValuesFor pins what the DB mirror persists: the derived
// verdict plus the record's OWN timestamps. Stamping the sync clock into
// last_fetch_at is the bug that made a 17-day-old channel look freshly fetched
// in channel_health (2026-09-24).
func TestChannelHealthSyncValuesFor(t *testing.T) {
	now := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	rec := &ChannelHealthRecord{
		Status:              StatusOK,
		LastFetchAt:         mustRFC(now.Add(-17 * 24 * time.Hour)),
		LastSuccessAt:       mustRFC(now.Add(-17 * 24 * time.Hour)),
		ConsecutiveFailures: 0,
	}
	// Vehicle: a plain (non-retired) 48h channel. A retired channel reports
	// "retired" for EVERY record — that is its own assertion right below.
	const vehicle = "twse_margin"
	v := ChannelHealthSyncValuesFor(vehicle, rec, now)
	if v.Status != StatusStale {
		t.Errorf("mirrored status = %q, want stale", v.Status)
	}
	if !v.LastFetchAt.Equal(now.Add(-17 * 24 * time.Hour)) {
		t.Errorf("mirrored last_fetch_at = %s, want the record's own timestamp", v.LastFetchAt)
	}
	if v.LastSuccessAt == nil || !v.LastSuccessAt.Equal(now.Add(-17*24*time.Hour)) {
		t.Errorf("mirrored last_success_at = %v, want the record's own timestamp", v.LastSuccessAt)
	}

	// One verdict, every consumer, including the retired case: the DB mirror must
	// not say "stale" for a channel the page calls retired by design (2026-10-05).
	if retired := ChannelHealthSyncValuesFor("twse_oddlot", rec, now); retired.Status != StatusRetired {
		t.Errorf("mirrored status for the retired channel = %q, want %q", retired.Status, StatusRetired)
	}

	broken := ChannelHealthSyncValuesFor("twse_capital_flow", &ChannelHealthRecord{Status: StatusOK}, now)
	if !broken.LastFetchAt.Equal(now) || broken.LastSuccessAt != nil {
		t.Errorf("record without timestamps = %+v, want now + nil last_success (NOT NULL column)", broken)
	}
}

// TestDegradedSelfEscalatesWhenTheDataIsGone is the E29-3 contract: "degraded"
// is a verdict about the DATA, so it must expire like one, and the escalated
// verdict must be the one the alert path acts on (the gauge maps error → 2,
// which is what ChannelHealthStatusError matches). Before this, a degraded
// record could never escalate: it keeps LastFetchAt moving, so no freshness
// gate saw it and permanent schema drift was visible only in a process exit
// code.
func TestDegradedSelfEscalatesWhenTheDataIsGone(t *testing.T) {
	now := time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC)
	// Pre-retirement copy (see TestDeriveChannelStatus): rule 2b must be
	// observable, and the shipped twse_oddlot entry is now retired.
	oddlot := ChannelContracts().Contract("twse_oddlot") // window = 48h
	oddlot.Retirement = nil
	rec := &ChannelHealthRecord{
		Status:        StatusDegraded,
		LastFetchAt:   mustRFC(now.Add(-5 * time.Minute)), // the fetch still works
		LastSuccessAt: mustRFC(now.Add(-17 * 24 * time.Hour)),
	}

	if got := DeriveChannelStatus(rec, oddlot, now); got != StatusError {
		t.Fatalf("degraded with 17-day-old data = %q, want error (an alarmable verdict)", got)
	}
	reason := DeriveChannelStatusReason(rec, oddlot, now)
	if !strings.Contains(reason, "17 天") || !strings.Contains(reason, "升級為 error") {
		t.Errorf("escalation reason = %q, want the data age and the escalation", reason)
	}
	// One judgment, every consumer: the DB mirror carries the same verdict.
	if v := ChannelHealthSyncValuesFor("twse_margin", rec, now); v.Status != StatusError {
		t.Errorf("mirrored status = %q, want error", v.Status)
	}

	// Negative control: the same record with data inside the window is NOT
	// escalated, so the rule is about the data and not about "degraded exists".
	fresh := *rec
	fresh.LastSuccessAt = mustRFC(now.Add(-2 * time.Hour))
	if got := DeriveChannelStatus(&fresh, oddlot, now); got != StatusDegraded {
		t.Errorf("degraded with 2-hour-old data = %q, want degraded (cache still within contract)", got)
	}
	if r := DeriveChannelStatusReason(&fresh, oddlot, now); r != "" {
		t.Errorf("a degraded record inside its window must carry no error reason, got %q", r)
	}
}
