package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareDigestsMatch(t *testing.T) {
	want := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 10, s1: 7, s2: 10},
		{a: "condA", b: "2026-08"}: {n: 5, s1: 2, s2: 4},
	}
	if got := compareDigests(want, want); len(got) != 0 {
		t.Fatalf("identical digests must match, got %v", got)
	}
}

func TestCompareDigestsMismatchKinds(t *testing.T) {
	want := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 10, s1: 7, s2: 10},
		{a: "condB", b: "2026-09"}: {n: 3, s1: 1, s2: 3},
	}
	got := map[digestKey]spDigestRow{
		{a: "condA", b: "2026-09"}: {n: 9, s1: 7, s2: 10}, // differs
		{a: "condC", b: "2026-09"}: {n: 3, s1: 1, s2: 3},  // extra (condB missing)
	}
	msgs := compareDigests(want, got)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 mismatch messages, got %d: %v", len(msgs), msgs)
	}
	joined := strings.Join(msgs, "\n")
	for _, wantSub := range []string{"condA|2026-09 differs", "condB|2026-09 missing", "condC|2026-09 extra"} {
		if !strings.Contains(joined, wantSub) {
			t.Fatalf("expected message containing %q, got:\n%s", wantSub, joined)
		}
	}
}

func TestRequireStockpickerTargetAssertion(t *testing.T) {
	// Fail-closed pair (independent-review P1): -stockpicker without
	// -expect-db must abort; with it (or for other modes) it must pass.
	if err := requireStockpickerTargetAssertion(true, ""); err == nil {
		t.Fatal("-stockpicker without -expect-db must error (fail-closed)")
	} else if !strings.Contains(err.Error(), "-expect-db") {
		t.Fatalf("error must name -expect-db, got: %v", err)
	}
	if err := requireStockpickerTargetAssertion(true, "atlas"); err != nil {
		t.Fatalf("-stockpicker with -expect-db must pass, got: %v", err)
	}
	if err := requireStockpickerTargetAssertion(false, ""); err != nil {
		t.Fatalf("non-stockpicker modes keep legacy behavior (no -expect-db required), got: %v", err)
	}
}

func TestOpenStockpickerSource_MissingFile(t *testing.T) {
	_, err := openStockpickerSource(filepath.Join(t.TempDir(), "nope.db"))
	if err == nil {
		t.Fatal("missing source file must error, not create an empty DB")
	}
}

func TestOpenStockpickerSource_MissingTables(t *testing.T) {
	// An existing but table-less SQLite file must fail the schema probe
	// (silently migrating zero rows from a half-initialized DB is the
	// opens-or-creates footgun this guard exists to prevent).
	path := filepath.Join(t.TempDir(), "atlas.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create empty sqlite file: %v", err)
	}
	_, err := openStockpickerSource(path)
	if err == nil {
		t.Fatal("source without stockpicker tables must error")
	} else if !strings.Contains(err.Error(), "stock_signal_outcomes") {
		t.Fatalf("error must name the missing table, got: %v", err)
	}
}
