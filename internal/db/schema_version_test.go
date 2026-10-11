package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMigrations creates a migration directory containing name/body pairs so
// tests do not need the real sql/migrations tree.
func writeMigrations(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("-- test migration\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestLatestMigrationVersion_PicksHighest(t *testing.T) {
	dir := writeMigrations(t,
		"000001_create_channel_health.up.sql",
		"000001_create_channel_health.down.sql",
		"000002_create_timescale_tables.up.sql",
		"000010_period_detector_version.up.sql",
		"000010_period_detector_version.down.sql",
		"README.md",
	)
	got, err := LatestMigrationVersion(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 10 {
		t.Fatalf("LatestMigrationVersion = %d, want 10", got)
	}
}

func TestLatestMigrationVersion_IgnoresDownAndNonSQL(t *testing.T) {
	dir := writeMigrations(t,
		"000007_only_down.down.sql",
		"notes.txt",
	)
	if _, err := LatestMigrationVersion(dir); err == nil {
		t.Fatal("expected error when no *.up.sql file exists")
	}
}

func TestLatestMigrationVersion_MissingDirIsError(t *testing.T) {
	_, err := LatestMigrationVersion(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected error for missing migrations directory")
	}
	if !strings.Contains(err.Error(), "read migrations directory") {
		t.Fatalf("expected directory error, got: %v", err)
	}
}

func TestLatestMigrationVersion_EmptyDirIsErrorNotZero(t *testing.T) {
	// Guard against the "0 == nothing applied" confusion: an empty directory
	// must be an error, not a version of 0.
	if _, err := LatestMigrationVersion(t.TempDir()); err == nil {
		t.Fatal("expected error for empty migrations directory")
	}
}

func TestCheckSchemaVersion(t *testing.T) {
	cases := []struct {
		name       string
		state      SchemaState
		latest     uint
		wantErr    bool
		wantReason string
	}{
		{name: "in sync", state: SchemaState{Version: 28}, latest: 28},
		{name: "ahead of code (binary rollback) is not drift", state: SchemaState{Version: 30}, latest: 28},
		{name: "lagging", state: SchemaState{Version: 26}, latest: 28, wantErr: true, wantReason: "lagging"},
		{name: "never migrated", state: SchemaState{Version: 0}, latest: 28, wantErr: true, wantReason: "lagging"},
		{name: "dirty", state: SchemaState{Version: 28, Dirty: true}, latest: 28, wantErr: true, wantReason: "dirty"},
		{name: "dirty and lagging", state: SchemaState{Version: 27, Dirty: true}, latest: 28, wantErr: true, wantReason: "lagging_dirty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckSchemaVersion(tc.state, tc.latest)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected drift error")
			}
			if !IsSchemaVersionDrift(err) {
				t.Fatalf("expected *SchemaDrift, got %T (%v)", err, err)
			}
			var drift *SchemaDrift
			if !errors.As(err, &drift) {
				t.Fatalf("errors.As failed for %v", err)
			}
			if drift.Applied != tc.state.Version || drift.Latest != tc.latest || drift.Dirty != tc.state.Dirty {
				t.Fatalf("drift = %+v, want applied=%d latest=%d dirty=%v",
					drift, tc.state.Version, tc.latest, tc.state.Dirty)
			}
			if drift.Reason() != tc.wantReason {
				t.Errorf("Reason() = %q, want %q", drift.Reason(), tc.wantReason)
			}
		})
	}
}

func TestSchemaDrift_ErrorMessageMentionsVersions(t *testing.T) {
	err := CheckSchemaVersion(SchemaState{Version: 26}, 28)
	msg := err.Error()
	for _, want := range []string{"26", "28"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not mention %q", msg, want)
		}
	}
}

func TestVerifySchemaVersionWith_InjectedReader(t *testing.T) {
	dir := writeMigrations(t, "000026_futures_shadow.up.sql", "000028_break_even_cost_rate.up.sql")

	if err := verifySchemaVersion(dir, func() (SchemaState, error) {
		return SchemaState{Version: 28}, nil
	}); err != nil {
		t.Fatalf("in-sync reader should not report drift: %v", err)
	}

	err := verifySchemaVersion(dir, func() (SchemaState, error) {
		return SchemaState{Version: 25}, nil
	})
	if !IsSchemaVersionDrift(err) {
		t.Fatalf("expected drift for lagging reader, got %v", err)
	}
	var drift *SchemaDrift
	if !errors.As(err, &drift) || drift.Latest != 28 {
		t.Fatalf("drift = %+v, want latest=28", drift)
	}
}

func TestVerifySchemaVersionWith_ReaderErrorIsNotDrift(t *testing.T) {
	dir := writeMigrations(t, "000028_break_even_cost_rate.up.sql")
	sentinel := errors.New("connection refused")
	err := verifySchemaVersion(dir, func() (SchemaState, error) {
		return SchemaState{}, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected reader error to be propagated, got %v", err)
	}
	if IsSchemaVersionDrift(err) {
		t.Error("a reader failure must not be reported as schema drift")
	}
}

func TestVerifySchemaVersionWith_BogusMigrationsPath(t *testing.T) {
	err := verifySchemaVersion(filepath.Join(t.TempDir(), "missing"), func() (SchemaState, error) {
		return SchemaState{Version: 999}, nil
	})
	if err == nil {
		t.Fatal("expected error for missing migrations directory")
	}
	if IsSchemaVersionDrift(err) {
		t.Error("an unreadable migrations directory must not be reported as schema drift")
	}
}

func TestReadAppliedSchemaState_InvalidURL(t *testing.T) {
	dir := writeMigrations(t, "000001_create_channel_health.up.sql")
	_, err := ReadAppliedSchemaState("not-a-valid-url", dir)
	if err == nil {
		t.Fatal("expected error for invalid database URL")
	}
	if !strings.Contains(err.Error(), "create migrate instance") {
		t.Fatalf("expected migrate instance error, got: %v", err)
	}
}

func TestReadAppliedSchemaState_UnreachableDatabase(t *testing.T) {
	dir := writeMigrations(t, "000001_create_channel_health.up.sql")
	_, err := ReadAppliedSchemaState("postgres://localhost:1/db?sslmode=disable&connect_timeout=1", dir)
	if err == nil {
		t.Fatal("expected error for unreachable database")
	}
	if !strings.Contains(err.Error(), "create migrate instance") {
		t.Fatalf("expected migrate instance error, got: %v", err)
	}
	if IsSchemaVersionDrift(err) {
		t.Error("an unreachable database must not be reported as schema drift")
	}
}
