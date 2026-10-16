package ledger

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FU-20261016-03 regression suite (hermetic, no PostgreSQL required).
//
// Production carried 894/894 experiments rows with session_id = NULL because
// the session-scoped write was a bare INSERT against a UNIQUE experiment_id
// that RecordExperiment had already written, and the resulting error was
// discarded by the caller. These tests pin the two halves of the fix:
//   - the write is now a targeted upsert (backfill NULL, never re-label), and
//   - a real store failure is returned (so a caller cannot silently ignore it)
//     and counted in SessionExperimentLinkErrorTotal.

func newSQLiteExperimentStoreForLink(t *testing.T) (*SQLiteOutcomeStore, *sql.DB) {
	t.Helper()
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return NewSQLiteOutcomeStore(db), db
}

func linkTestRecord(id string) domain.ExperimentRecord {
	return domain.ExperimentRecord{
		ID:            id,
		TargetAgentID: "agent-link",
		Skill:         "sector-tech",
		MutationType:  "prompt_tweak",
		Status:        domain.ExperimentPlanned,
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

func storedSessionID(t *testing.T, db *sql.DB, experimentID string) sql.NullString {
	t.Helper()
	var sid sql.NullString
	if err := db.QueryRow(
		`SELECT session_id FROM experiments WHERE experiment_id = ?`, experimentID,
	).Scan(&sid); err != nil {
		t.Fatalf("select session_id for %s: %v", experimentID, err)
	}
	return sid
}

// TestSQLiteRecordSessionExperiment_BackfillsNullSessionID is the exact defect
// shape: RecordExperiment writes the row first (session_id NULL), then the
// session-scoped write for the SAME experiment_id must succeed and attach the
// session instead of failing on the UNIQUE index.
func TestSQLiteRecordSessionExperiment_BackfillsNullSessionID(t *testing.T) {
	store, db := newSQLiteExperimentStoreForLink(t)
	rec := linkTestRecord("exp-link-backfill")

	// Production order (orchestrator.NextExperimentCandidate): global write
	// first, session-scoped write second.
	if err := store.RecordExperiment(rec); err != nil {
		t.Fatalf("RecordExperiment (first write): %v", err)
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "session-A"}, rec); err != nil {
		t.Fatalf("RecordSessionExperiment on an existing experiment_id must not fail: %v", err)
	}

	got := storedSessionID(t, db, rec.ID)
	if !got.Valid || got.String != "session-A" {
		t.Fatalf("session_id = %+v, want session-A (the link must be backfilled)", got)
	}
}

// TestSQLiteRecordSessionExperiment_KeepsExistingSessionID locks the COALESCE
// semantics: an experiment already attributed to a session must never be
// re-labelled by a later write.
func TestSQLiteRecordSessionExperiment_KeepsExistingSessionID(t *testing.T) {
	store, db := newSQLiteExperimentStoreForLink(t)
	rec := linkTestRecord("exp-link-keep")

	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "session-first"}, rec); err != nil {
		t.Fatalf("first session write: %v", err)
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "session-second"}, rec); err != nil {
		t.Fatalf("second session write (different session) must not fail: %v", err)
	}

	got := storedSessionID(t, db, rec.ID)
	if !got.Valid || got.String != "session-first" {
		t.Fatalf("session_id = %+v, want session-first (existing value wins)", got)
	}
}

// TestSQLiteRecordSessionExperiment_StoreFailureIsReturnedAndCounted proves a
// genuine write failure is not swallowed at the store boundary: the error
// surfaces AND the observability counter advances.
func TestSQLiteRecordSessionExperiment_StoreFailureIsReturnedAndCounted(t *testing.T) {
	store, db := newSQLiteExperimentStoreForLink(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	before := SessionExperimentLinkErrorTotal()
	err := store.RecordSessionExperiment(domain.ReplaySession{ID: "session-A"}, linkTestRecord("exp-link-fail"))
	if err == nil {
		t.Fatal("a closed store must return an error, got nil")
	}
	if after := SessionExperimentLinkErrorTotal(); after != before+1 {
		t.Fatalf("SessionExperimentLinkErrorTotal = %d, want %d (the failure must be counted)", after, before+1)
	}
}

// TestSQLiteRecordExperiment_RepeatedIDIsNotAnError pins backend parity with
// PostgreSQL (#1780): experiment_id is retried by design, so a repeat is not an
// error.
//
// Q2 batch 2 unit A (2026-10-16) changed the second half of that contract from
// "first write wins" to "last write wins". Write-once made the row unable to
// carry a status transition, so `accepted` was structurally 0 no matter what
// the pipeline did — production read that as a 0% acceptance rate. The JSONL
// store is append-only with last-write-wins folding
// (LatestExperimentStatusByID), so last-write-wins is the parity target, not
// first-write-wins. The repeat must still not error.
func TestSQLiteRecordExperiment_RepeatedIDIsNotAnError(t *testing.T) {
	store, db := newSQLiteExperimentStoreForLink(t)
	rec := linkTestRecord("exp-repeat")
	if err := store.RecordExperiment(rec); err != nil {
		t.Fatalf("first RecordExperiment: %v", err)
	}

	second := rec
	second.Status = domain.ExperimentAccepted
	if err := store.RecordExperiment(second); err != nil {
		t.Fatalf("repeated experiment_id must not be an error (parity with PostgreSQL): %v", err)
	}

	var accepted sql.NullInt64
	if err := db.QueryRow(
		`SELECT accepted FROM experiments WHERE experiment_id = ?`, rec.ID,
	).Scan(&accepted); err != nil {
		t.Fatalf("select accepted: %v", err)
	}
	if accepted.Int64 != 1 {
		t.Fatalf("accepted = %d, want 1 (the accepted transition must land)", accepted.Int64)
	}
}

// TestSQLiteRecordExperiment_TransitionDoesNotClearSessionID is the guard rail
// on the upsert above: the session link is owned by RecordSessionExperiment, so
// a later session-less record write for the same id must update the record
// without wiping the link (a bare "replace the row" upsert would silently
// re-create the 894/894 NULL defect of FU-20261016-03).
func TestSQLiteRecordExperiment_TransitionDoesNotClearSessionID(t *testing.T) {
	store, db := newSQLiteExperimentStoreForLink(t)
	rec := linkTestRecord("exp-transition-keeps-session")
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "session-A"}, rec); err != nil {
		t.Fatalf("RecordSessionExperiment: %v", err)
	}

	accepted := rec
	accepted.Status = domain.ExperimentAccepted
	if err := store.RecordExperiment(accepted); err != nil {
		t.Fatalf("RecordExperiment (status transition): %v", err)
	}

	if got := storedSessionID(t, db, rec.ID); !got.Valid || got.String != "session-A" {
		t.Fatalf("session_id = %+v, want session-A (a status transition must not clear the link)", got)
	}
}
