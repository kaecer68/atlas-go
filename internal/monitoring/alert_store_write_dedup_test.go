package monitoring

// Write-point dedup regression tests (#1787 follow-up, 2026-10-01).
//
// Production symptom: the "未處理告警" queue grew by same-type rows (e.g. 13
// windows rows for one channel family) although the condition had not changed.
// Two properties of AlertStore are guarded here:
//
//  1. Save itself collapses a re-emitted open condition onto its open row, so
//     duplicates cannot be appended by writers that have no AlertDeduplicator
//     (or by a second process writing the same JSONL).
//  2. The merge target is the NEWEST open row, not the first match in file
//     order. FindByDedupKey returns the first match, which is often an older
//     closed row; reusing that match fails, so every repeat appended a new row.
//
// History is never rewritten: closed rows (resolved/acknowledged/silenced) are
// left untouched and a new episode after closure still gets its own row.

import (
	"strconv"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func triggeredAlert(id, dedupKey string, at time.Time) domain.AlertRecord {
	return domain.AlertRecord{
		ID:        id,
		Timestamp: at,
		Rule:      "data_gap",
		Severity:  "WARNING",
		Message:   "channel government_flow has 18 missing coverage date(s)",
		Status:    domain.AlertStatusTriggered,
		Count:     1,
		DedupKey:  dedupKey,
	}
}

// TestAlertStore_Save_CollapsesRepeatedOpenCondition is the core regression: 13
// emissions of one condition must leave ONE open row whose count grew.
func TestAlertStore_Save_CollapsesRepeatedOpenCondition(t *testing.T) {
	store := newTestStore(t)
	const key = "data_gap:WARNING:government_flow"

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 13; i++ {
		rec := triggeredAlert("alert-"+strconv.Itoa(i), key, base.Add(time.Duration(i)*time.Hour))
		if err := store.Save(rec); err != nil {
			t.Fatalf("Save(%d): %v", i, err)
		}
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1 (repeated emissions of one open condition must collapse)", len(records))
	}
	if records[0].Count != 13 {
		t.Errorf("count = %d, want 13", records[0].Count)
	}
	if records[0].ID != "alert-0" {
		t.Errorf("merged row ID = %q, want alert-0 (the open row keeps its identity)", records[0].ID)
	}
	if records[0].LastSeen == nil {
		t.Fatal("LastSeen = nil, want the newest occurrence")
	}
	if want := base.Add(12 * time.Hour); !records[0].LastSeen.Equal(want) {
		t.Errorf("LastSeen = %v, want %v", records[0].LastSeen, want)
	}
}

// TestAlertStore_Save_MergesIntoNewestOpenRowNotFirstMatch reproduces the
// first-match defect: a closed row with the same key sits earlier in the file.
func TestAlertStore_Save_MergesIntoNewestOpenRowNotFirstMatch(t *testing.T) {
	store := newTestStore(t)
	const key = "data_gap:WARNING:government_flow"
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	closed := triggeredAlert("alert-closed", key, base)
	closed.Status = domain.AlertStatusResolved
	closed.ResolvedBy = "ttl-expiry"
	if err := store.Save(closed); err != nil {
		t.Fatalf("Save(closed): %v", err)
	}

	first := triggeredAlert("alert-open-1", key, base.Add(time.Hour))
	if err := store.Save(first); err != nil {
		t.Fatalf("Save(open-1): %v", err)
	}
	second := triggeredAlert("alert-open-2", key, base.Add(2*time.Hour))
	if err := store.Save(second); err != nil {
		t.Fatalf("Save(open-2): %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadAll len = %d, want 2 (one closed episode + one open row): %+v", len(records), records)
	}
	if records[0].ID != "alert-closed" || records[0].Status != domain.AlertStatusResolved {
		t.Errorf("closed row was rewritten: %+v", records[0])
	}
	if records[1].ID != "alert-open-1" {
		t.Errorf("open row ID = %q, want alert-open-1 (the newest open row is the merge target)", records[1].ID)
	}
	if records[1].Count != 2 {
		t.Errorf("open row count = %d, want 2", records[1].Count)
	}
}

// TestAlertStore_Save_NewEpisodeAfterClosure documents that dedup does not
// swallow a genuinely new episode: once the open row is closed, the next
// emission starts a new row. Both rows stay on disk (traceability).
func TestAlertStore_Save_NewEpisodeAfterClosure(t *testing.T) {
	store := newTestStore(t)
	const key = "data_gap:WARNING:government_flow"
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	if err := store.Save(triggeredAlert("alert-ep1", key, base)); err != nil {
		t.Fatalf("Save(ep1): %v", err)
	}
	if err := store.Resolve("alert-ep1", "ttl-expiry"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := store.Save(triggeredAlert("alert-ep2", key, base.Add(24*time.Hour))); err != nil {
		t.Fatalf("Save(ep2): %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadAll len = %d, want 2 (resolved episode + new episode)", len(records))
	}
	if records[0].ID != "alert-ep1" || records[0].Status != domain.AlertStatusResolved {
		t.Errorf("first episode history lost: %+v", records[0])
	}
	if records[1].ID != "alert-ep2" || records[1].Status != domain.AlertStatusTriggered {
		t.Errorf("second episode not appended: %+v", records[1])
	}
}

// TestAlertStore_Save_WithoutDedupKeyAlwaysAppends keeps identity-less alerts
// on the historical append path: with no dedup_key there is nothing to collapse.
func TestAlertStore_Save_WithoutDedupKeyAlwaysAppends(t *testing.T) {
	store := newTestStore(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		rec := triggeredAlert("alert-"+strconv.Itoa(i), "", base.Add(time.Duration(i)*time.Minute))
		if err := store.Save(rec); err != nil {
			t.Fatalf("Save(%d): %v", i, err)
		}
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("LoadAll len = %d, want 3", len(records))
	}
}

// TestAlertStore_Save_AcknowledgedRowIsNotMergedInto pins the boundary of the
// merge: a decided row is history. A re-emission after acknowledgement starts a
// new row so the decision is not silently re-opened.
func TestAlertStore_Save_AcknowledgedRowIsNotMergedInto(t *testing.T) {
	store := newTestStore(t)
	const key = "data_gap:WARNING:government_flow"
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	if err := store.Save(triggeredAlert("alert-ack", key, base)); err != nil {
		t.Fatalf("Save(ack): %v", err)
	}
	if err := store.Acknowledge("alert-ack", "admin"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	if err := store.Save(triggeredAlert("alert-next", key, base.Add(time.Hour))); err != nil {
		t.Fatalf("Save(next): %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadAll len = %d, want 2", len(records))
	}
	if records[0].Status != domain.AlertStatusAcknowledged {
		t.Errorf("acknowledged row status = %q, want acknowledged", records[0].Status)
	}
	if records[1].ID != "alert-next" {
		t.Errorf("new row ID = %q, want alert-next", records[1].ID)
	}
}
