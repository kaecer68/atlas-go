package ledger

// FU-20261016-07 — mutation_briefs had no unique key.
//
// window_summaries carried `window_id TEXT UNIQUE` plus an INSERT ... ON
// CONFLICT upsert from the start; its sibling table mutation_briefs carried a
// non-unique index and a plain INSERT. Re-running one window therefore
// accumulated brief rows, and the only thing keeping the backfill idempotent
// was a caller-side DELETE (cmd/migrate-data -windows). These tests pin the
// schema half of the fix: the unique index exists on both backends
// (sql/migrations/000029 + sqlite_core.go), the SQLite writer is an upsert, and
// a legacy file that already holds duplicates FAILS LOUD instead of being
// repaired by deleting rows.

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// legacyMutationBriefsSchema is the pre-000029 shape shipped by
// sqlite_core.go: window_id carries only a non-unique index. A legacy file
// really does accept a second row for the same window — that is the defect.
const legacyMutationBriefsSchema = `
CREATE TABLE mutation_briefs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	window_id TEXT NOT NULL,
	data_json TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX idx_mutation_briefs_window_id ON mutation_briefs(window_id);`

// mutationBriefFixture builds a brief whose payload is distinguishable per
// version, so a test can tell "the row was replaced" from "a second row was
// appended".
func mutationBriefFixture(windowID, mutationType string) domain.MutationBrief {
	return domain.MutationBrief{
		ContractVersion: 1,
		WindowID:        windowID,
		TargetAgentID:   "agent-" + mutationType,
		MutationType:    mutationType,
		GeneratedAt:     time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
	}
}

// windowIDIndexes reports the indexes on mutation_briefs as name -> unique.
// PRAGMA index_list is the only way to see a unique index that was not
// declared inline; the column list does not change when it is added.
func windowIDIndexes(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA index_list('mutation_briefs')`)
	if err != nil {
		t.Fatalf("pragma index_list: %v", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list: %v", err)
		}
		out[name] = unique == 1
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index_list: %v", err)
	}
	return out
}

func mutationBriefRowCount(t *testing.T, db *sql.DB, windowID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM mutation_briefs WHERE window_id = ?`, windowID).Scan(&n); err != nil {
		t.Fatalf("count mutation_briefs rows: %v", err)
	}
	return n
}

// TestRecordMutationBriefSQLiteUpsertReplacesInsteadOfAccumulating is the
// store-level acceptance from FU-20261016-07: writing the same window twice
// leaves ONE row whose payload is the second write.
func TestRecordMutationBriefSQLiteUpsertReplacesInsteadOfAccumulating(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	store, err := NewWindowStore(config.Config{StoreBackend: "sqlite", SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("NewWindowStore(sqlite): %v", err)
	}

	const windowID = "window-fu101607-rewrite"
	if err := store.RecordMutationBrief(windowID, mutationBriefFixture(windowID, "prompt_tightening")); err != nil {
		t.Fatalf("first RecordMutationBrief: %v", err)
	}
	if err := store.RecordMutationBrief(windowID, mutationBriefFixture(windowID, "threshold_shift")); err != nil {
		t.Fatalf("second RecordMutationBrief: %v", err)
	}
	if err := store.RecordMutationBrief("window-fu101607-other", mutationBriefFixture("window-fu101607-other", "agent_swap")); err != nil {
		t.Fatalf("RecordMutationBrief(other window): %v", err)
	}

	raw, err := OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = raw.Close() }()

	if got := mutationBriefRowCount(t, raw, windowID); got != 1 {
		t.Fatalf("mutation_briefs rows for %s = %d, want 1 (second write must replace, not accumulate)", windowID, got)
	}

	var payload string
	if err := raw.QueryRow(`SELECT data_json FROM mutation_briefs WHERE window_id = ?`, windowID).Scan(&payload); err != nil {
		t.Fatalf("read stored brief: %v", err)
	}
	var got domain.MutationBrief
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("stored brief must parse: %v", err)
	}
	if got.MutationType != "threshold_shift" || got.TargetAgentID != "agent-threshold_shift" {
		t.Fatalf("stored brief = %+v, want the second write (threshold_shift)", got)
	}
}

// TestInitSchemaMutationBriefsWindowIDIsUnique pins the schema half on a fresh
// database: the unique index exists and the database itself refuses a second
// row for one window.
func TestInitSchemaMutationBriefsWindowIDIsUnique(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	indexes := windowIDIndexes(t, db)
	unique, ok := indexes["ux_mutation_briefs_window_id"]
	if !ok {
		t.Fatalf("ux_mutation_briefs_window_id missing; indexes = %v", indexes)
	}
	if !unique {
		t.Fatalf("ux_mutation_briefs_window_id exists but is NOT unique; indexes = %v", indexes)
	}

	if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ('window-dup', '{}', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("first raw insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ('window-dup', '{}', '2026-01-01T00:00:00Z')`); err == nil {
		t.Fatal("second raw insert for the same window_id returned nil error, want a UNIQUE violation")
	} else if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
		t.Fatalf("second raw insert error = %q, want a UNIQUE constraint violation", err)
	}
	if got := mutationBriefRowCount(t, db, "window-dup"); got != 1 {
		t.Fatalf("rows for window-dup = %d, want 1", got)
	}
}

// TestInitSchemaHealsLegacyMutationBriefsTable is the deployment path: an
// existing atlas.db was created before 000029, so CREATE TABLE IF NOT EXISTS
// leaves the table alone. The unique index statement is what upgrades it.
func TestInitSchemaHealsLegacyMutationBriefsTable(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(legacyMutationBriefsSchema); err != nil {
		t.Fatalf("create legacy mutation_briefs: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ('window-legacy', '{"mutation_type":"prompt_tightening"}', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Precondition: the legacy shape really does accept duplicates, which is
	// why the store's old plain INSERT accumulated rows.
	if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ('window-legacy', '{}', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatalf("precondition broken: legacy table rejected a duplicate (%v)", err)
	}
	if got := mutationBriefRowCount(t, db, "window-legacy"); got != 2 {
		t.Fatalf("legacy duplicate rows = %d, want 2", got)
	}
	// Restore the single-row legacy state the migration is expected to meet.
	if _, err := db.Exec(`DELETE FROM mutation_briefs WHERE window_id = ?
		AND data_json = '{}'`, "window-legacy"); err != nil {
		t.Fatalf("reset legacy fixture: %v", err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema on the legacy file: %v", err)
	}

	indexes := windowIDIndexes(t, db)
	if !indexes["ux_mutation_briefs_window_id"] {
		t.Fatalf("InitSchema did not add the unique index to the legacy file; indexes = %v", indexes)
	}
	// The pre-existing row survives the heal.
	if got := mutationBriefRowCount(t, db, "window-legacy"); got != 1 {
		t.Fatalf("legacy rows after InitSchema = %d, want 1 (the heal must not touch rows)", got)
	}
	if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ('window-legacy', '{}', '2026-01-03T00:00:00Z')`); err == nil {
		t.Fatal("duplicate insert after the heal returned nil error, want a UNIQUE violation")
	}
}

// TestInitSchemaLegacyDuplicatesFailLoudWithoutDeleting rows is the data-safety
// pin for the migration: a legacy file that DOES hold duplicates cannot be
// upgraded silently, and the failure must not become a cleanup. Deleting
// production rows is an operator decision, never a schema step.
func TestInitSchemaLegacyDuplicatesFailLoudWithoutDeleting(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(legacyMutationBriefsSchema); err != nil {
		t.Fatalf("create legacy mutation_briefs: %v", err)
	}
	for _, payload := range []string{`{"mutation_type":"old"}`, `{"mutation_type":"new"}`} {
		if _, err := db.Exec(`INSERT INTO mutation_briefs (window_id, data_json, created_at)
			VALUES ('window-dup-legacy', ?, '2026-01-01T00:00:00Z')`, payload); err != nil {
			t.Fatalf("seed duplicate legacy row %s: %v", payload, err)
		}
	}

	err = InitSchema(db)
	if err == nil {
		t.Fatal("InitSchema on a legacy file with duplicate window_id rows returned nil, want a loud failure")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
		t.Fatalf("InitSchema error = %q, want it to name the UNIQUE constraint", err)
	}
	if got := mutationBriefRowCount(t, db, "window-dup-legacy"); got != 2 {
		t.Fatalf("rows after the failed heal = %d, want 2 (the schema step must never delete data)", got)
	}
}
