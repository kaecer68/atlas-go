package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInMemoryOncestampStore_DistinctKeys(t *testing.T) {
	store := NewInMemoryOncestampStore()
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)

	run, ok := store.TryClaim("k1", now, sameDay)
	if !ok || !run {
		t.Fatalf("first claim for k1: run=%v ok=%v want true/true", run, ok)
	}
	run, ok = store.TryClaim("k2", now, sameDay)
	if !ok || !run {
		t.Fatalf("first claim for k2: run=%v ok=%v want true/true", run, ok)
	}
}

func TestInMemoryOncestampStore_SamePeriodSuppresses(t *testing.T) {
	store := NewInMemoryOncestampStore()
	day1 := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	day1Later := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)

	if run, _ := store.TryClaim("k", day1, sameDay); !run {
		t.Fatalf("first claim expected to run")
	}
	run, ok := store.TryClaim("k", day1Later, sameDay)
	if !ok || run {
		t.Fatalf("same-day second claim: run=%v ok=%v want false/true", run, ok)
	}
}

func TestInMemoryOncestampStore_NextPeriodRuns(t *testing.T) {
	store := NewInMemoryOncestampStore()
	day1 := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 14, 6, 0, 0, 0, time.UTC)

	if run, _ := store.TryClaim("k", day1, sameDay); !run {
		t.Fatalf("day1 expected to run")
	}
	run, _ := store.TryClaim("k", day2, sameDay)
	if !run {
		t.Fatalf("day2 expected to run")
	}
}

func TestFileOncestampStore_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	store1, err := NewFileOncestampStore(dir)
	if err != nil {
		t.Fatalf("NewFileOncestampStore: %v", err)
	}
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	if run, ok := store1.TryClaim("restart-survives", now, sameDay); !ok || !run {
		t.Fatalf("first claim: run=%v ok=%v want true/true", run, ok)
	}

	store2, err := NewFileOncestampStore(dir)
	if err != nil {
		t.Fatalf("NewFileOncestampStore #2: %v", err)
	}
	run, ok := store2.TryClaim("restart-survives", now.Add(2*time.Hour), sameDay)
	if !ok || run {
		t.Fatalf("post-restart same-day: run=%v ok=%v want false/true (claim must survive process restart)", run, ok)
	}
}

func TestFileOncestampStore_AtomicWriteNoTmpLeft(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileOncestampStore(dir)
	if err != nil {
		t.Fatalf("NewFileOncestampStore: %v", err)
	}
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	if _, ok := store.TryClaim("k", now, sameDay); !ok {
		t.Fatalf("claim failed")
	}

	for _, ent := range readDir(t, dir) {
		if endsWith(ent, ".tmp") {
			t.Fatalf("atomic write left tmp file: %s", ent)
		}
	}
}

func TestDailyOnceGuardWithStore_RunsOncePerDayAndSurvivesRecreation(t *testing.T) {
	dir := t.TempDir()
	loc := fixedTimeZone(t)
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, loc)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return now }

	store1, _ := NewFileOncestampStore(dir)
	guard := dailyOnceGuardWithStore(loc, 6, 0, "stage3.daily", store1, sameDay)
	if !guard() {
		t.Fatalf("first 06:00 expected to run")
	}
	if guard() {
		t.Fatalf("duplicate 06:00 expected to suppress")
	}

	store2, _ := NewFileOncestampStore(dir)
	guard2 := dailyOnceGuardWithStore(loc, 6, 0, "stage3.daily", store2, sameDay)
	if guard2() {
		t.Fatalf("post-restart 06:00 expected to suppress (claim must survive)")
	}

	now = time.Date(2026, 7, 14, 6, 0, 0, 0, loc)
	if !guard2() {
		t.Fatalf("next-day 06:00 expected to run")
	}
}

func TestDailyGuardFor_NilStoreFallsBackToClosure(t *testing.T) {
	loc := fixedTimeZone(t)
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, loc)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return now }

	deps := Stage3TaskDeps{TimeZone: loc, OncestampStore: nil}
	guard := dailyGuardFor(deps, 6, 0)
	if !guard() {
		t.Fatalf("first claim expected to run")
	}
	if guard() {
		t.Fatalf("second claim same day expected to suppress")
	}
}

func TestDailyGuardFor_WithStoreUsesPersistence(t *testing.T) {
	dir := t.TempDir()
	loc := fixedTimeZone(t)
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, loc)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return now }

	store, _ := NewFileOncestampStore(dir)
	deps := Stage3TaskDeps{TimeZone: loc, OncestampStore: store}
	guard := dailyGuardFor(deps, 6, 0)
	if !guard() {
		t.Fatalf("first claim expected to run")
	}

	guards := dailyGuardFor(deps, 6, 0)
	if guards() {
		t.Fatalf("second guard using same store: expected suppress")
	}
}

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := readDirFilenames(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	return entries
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestFileOncestampStore_RecoversFromStaleTmpAfterSimulatedCrash(t *testing.T) {
	dir := t.TempDir()
	tmpPath := filepath.Join(dir, "stage3_oncestamps.json.tmp")
	if err := os.WriteFile(tmpPath, []byte("{partial-write-corrupted"), 0o644); err != nil {
		t.Fatalf("seed stale tmp: %v", err)
	}

	store, err := NewFileOncestampStore(dir)
	if err != nil {
		t.Fatalf("NewFileOncestampStore: %v", err)
	}

	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	run, ok := store.TryClaim("daily-resume", now, sameDay)
	if !ok || !run {
		t.Fatalf("expected claim to succeed despite stale tmp; got run=%v ok=%v", run, ok)
	}

	entries, err := readDirFilenames(dir)
	if err != nil {
		t.Fatalf("readDirFilenames: %v", err)
	}
	for _, ent := range entries {
		if endsWith(ent, ".tmp") {
			t.Fatalf("expected stale .tmp gone after rename; still present: %s", ent)
		}
	}

	store2, err := NewFileOncestampStore(dir)
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}
	run, ok = store2.TryClaim("daily-resume", now.Add(2*time.Hour), sameDay)
	if !ok || run {
		t.Fatalf("post-crash same-day claim should suppress; got run=%v ok=%v", run, ok)
	}
}
