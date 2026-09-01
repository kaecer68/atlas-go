package monitoring

// Tests for the #1787 alert lifecycle redesign:
//   - per-condition identity in dedup keys
//   - unresolved-record reuse (one persistent condition = one row)
//   - ResolveByIdentity (per-identity and category-wide)
//   - TTL auto-archival

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func newLifecycleTestMonitor(t *testing.T) (*Monitor, *AlertStore) {
	t.Helper()
	store := newTestStore(t)
	m := NewMonitor()
	m.SetAlertStore(store)
	m.SetDeduplicator(NewAlertDeduplicator(1*time.Millisecond, store))
	return m, store
}

func TestAlertIdentity_Extraction(t *testing.T) {
	cases := []struct {
		md   map[string]any
		want string
	}{
		{map[string]any{"task": "health_check"}, "health_check"},
		{map[string]any{"channel": "capital_flow"}, "capital_flow"},
		{map[string]any{"pillar": "judge"}, "judge"},
		{map[string]any{"task": 123}, ""},
		{nil, ""},
		{map[string]any{"unrelated": "x"}, ""},
	}
	for _, c := range cases {
		if got := alertIdentity(c.md); got != c.want {
			t.Errorf("alertIdentity(%v) = %q, want %q", c.md, got, c.want)
		}
	}
}

func TestMonitor_UnresolvedRecordReuse(t *testing.T) {
	m, store := newLifecycleTestMonitor(t)

	md := func(task string) map[string]any { return map[string]any{"task": task} }
	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task alpha is stale: not run for 5m", md("alpha"), nil)
	waitForRecords(t, store, 1) // first save must land before recurrence
	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task alpha is stale: not run for 10m", md("alpha"), nil)

	recs := waitForRecords(t, store, 1)
	if len(recs) != 1 {
		t.Fatalf("persistent condition should produce ONE record, got %d", len(recs))
	}
	if recs[0].Count != 2 {
		t.Errorf("count = %d, want 2", recs[0].Count)
	}
	if recs[0].DedupKey != "background_task:WARNING:alpha" {
		t.Errorf("dedup key = %q, want condition-identity key", recs[0].DedupKey)
	}
}

func TestMonitor_DistinctIdentitiesDistinctRows(t *testing.T) {
	m, store := newLifecycleTestMonitor(t)

	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task alpha is stale", map[string]any{"task": "alpha"}, nil)
	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task beta is stale", map[string]any{"task": "beta"}, nil)

	recs := waitForRecords(t, store, 2)
	if len(recs) != 2 {
		t.Fatalf("two distinct conditions should produce TWO records, got %d", len(recs))
	}
}

func TestMonitor_ResolvedConditionStartsNewRow(t *testing.T) {
	m, store := newLifecycleTestMonitor(t)

	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task alpha is stale", map[string]any{"task": "alpha"}, nil)
	waitForRecords(t, store, 1)

	if n := m.ResolveByIdentity("background_task", "alpha", "task-success"); n != 1 {
		t.Fatalf("ResolveByIdentity resolved %d, want 1", n)
	}

	// Condition recurs after resolution -> NEW row, count restarts at 1.
	m.AlertWithBreakdown(AlertLevelWarning, "background_task",
		"Task alpha is stale again", map[string]any{"task": "alpha"}, nil)
	recs := waitForRecords(t, store, 2)
	for _, r := range recs {
		if r.Status != domain.AlertStatusTriggered {
			continue
		}
		if r.Count != 1 {
			t.Errorf("new row count = %d, want 1", r.Count)
		}
	}
}

func TestMonitor_ResolveByIdentity_CategoryWide(t *testing.T) {
	m, store := newLifecycleTestMonitor(t)

	m.AlertWithBreakdown(AlertLevelWarning, "evolution", "replay_data_stale", nil, nil)
	waitForRecords(t, store, 1) // first save must land before recurrence
	m.AlertWithBreakdown(AlertLevelWarning, "evolution", "replay_data_unavailable", nil, nil)
	// Identity-less family: same category+level collapses into one record
	// whose count grows (#1787 reuse) — that record is what resolves.
	recs := waitForRecords(t, store, 1)
	if recs[0].Count != 2 {
		t.Errorf("identity-less recurrence count = %d, want 2", recs[0].Count)
	}

	if n := m.ResolveByIdentity("evolution", "", "replay-fresh"); n != 1 {
		t.Fatalf("category-wide resolve = %d, want 1", n)
	}
}

func TestAlertTTLExpiry(t *testing.T) {
	store := newTestStore(t)

	old := time.Now().Add(-8 * 24 * time.Hour)
	staleWarning := makeAlert("ttl-warn-1")
	staleWarning.Severity = "WARNING"
	staleWarning.Rule = "data_gap"
	staleWarning.Status = domain.AlertStatusTriggered
	staleWarning.Timestamp = old
	staleErr := makeAlert("ttl-err-1")
	staleErr.Severity = "ERROR"
	staleErr.Rule = "background_task"
	staleErr.Status = domain.AlertStatusTriggered
	staleErr.Timestamp = time.Now().Add(-8 * 24 * time.Hour) // < 30d grace
	freshWarning := makeAlert("ttl-warn-2")
	freshWarning.Severity = "WARNING"
	freshWarning.Rule = "data_gap"
	freshWarning.Status = domain.AlertStatusTriggered
	resolvedAlready := makeAlert("ttl-resolved")
	resolvedAlready.Severity = "WARNING"
	resolvedAlready.Status = domain.AlertStatusResolved
	resolvedAlready.Timestamp = old

	for _, rec := range []domain.AlertRecord{staleWarning, staleErr, freshWarning, resolvedAlready} {
		if err := store.Save(rec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	runAlertTTLExpiry(store)

	recs, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]domain.AlertRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	if got := byID["ttl-warn-1"]; got.Status != domain.AlertStatusResolved {
		t.Errorf("8-day-old WARNING should be TTL-resolved, got %s", got.Status)
	}
	if got := byID["ttl-err-1"]; got.Status != domain.AlertStatusTriggered {
		t.Errorf("8-day-old ERROR (30d grace) should stay open, got %s", got.Status)
	}
	if got := byID["ttl-warn-2"]; got.Status != domain.AlertStatusTriggered {
		t.Errorf("fresh WARNING should stay open, got %s", got.Status)
	}
	if got := byID["ttl-resolved"]; got.Status != domain.AlertStatusResolved {
		t.Errorf("already-resolved record must not change, got %s", got.Status)
	}
}

func TestStartAlertTTLLifecycle_NilStoreSafe(t *testing.T) {
	StartAlertTTLLifecycle(nil, context.Background(), time.Hour) // must not panic
}

func waitForRecords(t *testing.T, store *AlertStore, n int) []domain.AlertRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recs, err := store.LoadAll()
		if err == nil && len(recs) >= n {
			return recs
		}
		time.Sleep(10 * time.Millisecond)
	}
	recs, _ := store.LoadAll()
	if len(recs) < n {
		t.Fatalf("expected >= %d records, got %d", n, len(recs))
	}
	return recs
}
