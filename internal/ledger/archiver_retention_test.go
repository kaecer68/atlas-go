package ledger

// Retention / size-rotation tests for the ledger archiver (FU-20261012-05).
//
// Pre-fix shape (2026-10-16, origin/main 844f590f2): NewArchiver had zero call
// sites, so nothing ever archived anything; and its fixed 4-name list did not
// contain the files that actually grow (recommendation_outcomes.jsonl was
// 172 MB / 45,668 records) while naming alerts.jsonl, which lives in alerts/.
//
// Every test here asserts the two invariants the follow-up demands:
//   - archival MOVES data (the archive holds byte-identical content);
//   - a run is bounded and does nothing to files that are still live.

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const ledgerFixtureRecords = "{\"line\":\"one\"}\n{\"line\":\"two\"}\n{\"line\":\"three\"}\n"

// writeAgedLedgerFile writes content to baseDir/rel and backdates its mtime.
func writeAgedLedgerFile(t *testing.T, baseDir, rel, content string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(baseDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatalf("chtimes %s: %v", rel, err)
	}
	return path
}

// readGzip returns the decompressed bytes of a .gz file.
func readGzip(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader %s: %v", path, err)
	}
	defer func() { _ = gz.Close() }()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read gzip %s: %v", path, err)
	}
	return data
}

func TestArchiver_RunWithReport_MovesStaleLedgerFiles(t *testing.T) {
	baseDir := t.TempDir()
	stale := []string{"recommendation_outcomes.jsonl", "metrics.jsonl", "darwinian_history.jsonl"}
	for _, rel := range stale {
		writeAgedLedgerFile(t, baseDir, rel, ledgerFixtureRecords, 400*24*time.Hour)
	}
	freshPath := writeAgedLedgerFile(t, baseDir, "quotes.jsonl", ledgerFixtureRecords, time.Hour)

	a := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365})
	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if len(report.Archived) != 3 {
		t.Fatalf("archived %d files, want 3 (%+v)", len(report.Archived), report.Archived)
	}

	for _, rel := range stale {
		if _, err := os.Stat(filepath.Join(baseDir, rel)); !os.IsNotExist(err) {
			t.Errorf("%s must be moved out of the live path (err=%v)", rel, err)
		}
		ent := findArchived(report.Archived, rel)
		if ent == nil {
			t.Fatalf("%s missing from report.Archived: %+v", rel, report.Archived)
		}
		if ent.Lines != 3 {
			t.Errorf("%s archived line count = %d, want 3", rel, ent.Lines)
		}
		if got := string(readGzip(t, filepath.Join(baseDir, ent.Archive))); got != ledgerFixtureRecords {
			t.Errorf("%s archive content = %q, want the original bytes %q", rel, got, ledgerFixtureRecords)
		}
	}

	// A live file must not be touched.
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("fresh file disappeared: %v", err)
	}
	if got, err := os.ReadFile(freshPath); err != nil || string(got) != ledgerFixtureRecords {
		t.Errorf("fresh file content changed (err=%v)", err)
	}
}

func TestArchiver_RunWithReport_SkipsEmptyStaleFile(t *testing.T) {
	baseDir := t.TempDir()
	path := writeAgedLedgerFile(t, baseDir, "metrics.jsonl", "", 400*24*time.Hour)

	report, err := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365}).RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if len(report.Archived) != 0 {
		t.Fatalf("empty file must not be archived: %+v", report.Archived)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("empty stale file must be left in place: %v", err)
	}
}

func TestArchiver_RunWithReport_RespectsPerRunBudget(t *testing.T) {
	baseDir := t.TempDir()
	for _, rel := range []string{"a.jsonl", "b.jsonl", "c.jsonl"} {
		writeAgedLedgerFile(t, baseDir, rel, ledgerFixtureRecords, 400*24*time.Hour)
	}
	a := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365, MaxFilesPerRun: 2})
	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if len(report.Archived) != 2 {
		t.Fatalf("archived %d files with a per-run budget of 2: %+v", len(report.Archived), report.Archived)
	}
	// The deferred file stays on the live path for the next run.
	remaining := 0
	for _, rel := range []string{"a.jsonl", "b.jsonl", "c.jsonl"} {
		if _, err := os.Stat(filepath.Join(baseDir, rel)); err == nil {
			remaining++
		}
	}
	if remaining != 1 {
		t.Fatalf("%d live files left, want exactly 1", remaining)
	}
}

func TestArchiver_RunWithReport_ArchivesSubdirectoryFile(t *testing.T) {
	baseDir := t.TempDir()
	writeAgedLedgerFile(t, baseDir, "alerts/alerts.jsonl", ledgerFixtureRecords, 400*24*time.Hour)

	report, err := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365}).RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	ent := findArchived(report.Archived, "alerts/alerts.jsonl")
	if ent == nil {
		t.Fatalf("alerts/alerts.jsonl was not archived: %+v", report.Archived)
	}
	// The archive name must be flat: a relative path with a separator used to be
	// passed straight into the archive dir name (which fails to create).
	if strings.Contains(filepath.Base(ent.Archive), string(filepath.Separator)) || strings.Contains(ent.Archive, "alerts/alerts") {
		t.Errorf("archive path %q must be flattened", ent.Archive)
	}
	if got := string(readGzip(t, filepath.Join(baseDir, ent.Archive))); got != ledgerFixtureRecords {
		t.Errorf("archive content = %q, want %q", got, ledgerFixtureRecords)
	}
}

func TestArchiver_RunWithReport_KeepsArchivesByDefault(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldArchive := filepath.Join(a.archiveDir(), "metrics-20200101-000000.jsonl.gz")
	if err := os.WriteFile(oldArchive, []byte("old"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	stamp := time.Now().Add(-2000 * 24 * time.Hour)
	if err := os.Chtimes(oldArchive, stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if report.Pruned != 0 {
		t.Errorf("pruned = %d, want 0 (archives are kept unless prune days are configured)", report.Pruned)
	}
	if _, err := os.Stat(oldArchive); err != nil {
		t.Errorf("archive older than 730 days must survive the default policy: %v", err)
	}
}

func TestArchiver_RunWithReport_PrunesOnlyWhenConfigured(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir).WithPolicy(ArchivePolicy{ArchivePruneDays: 30})
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldArchive := filepath.Join(a.archiveDir(), "metrics-20200101-000000.jsonl.gz")
	newArchive := filepath.Join(a.archiveDir(), "metrics-20260101-000000.jsonl.gz")
	for _, p := range []string{oldArchive, newArchive} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	oldStamp := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(oldArchive, oldStamp, oldStamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if report.Pruned != 1 {
		t.Errorf("pruned = %d, want 1", report.Pruned)
	}
	if _, err := os.Stat(oldArchive); !os.IsNotExist(err) {
		t.Errorf("60-day-old archive should be pruned with a 30-day window (err=%v)", err)
	}
	if _, err := os.Stat(newArchive); err != nil {
		t.Errorf("recent archive must survive: %v", err)
	}
}

func TestArchiver_RunWithReport_RotatesOversizedLiveFile(t *testing.T) {
	baseDir := t.TempDir()
	livePath := writeAgedLedgerFile(t, baseDir, "recommendation_outcomes.jsonl", ledgerFixtureRecords, time.Hour)

	// 1 MB threshold is irrelevant here: use bytes instead by crafting a tiny
	// policy through the internal field (same as the existing tests do).
	a := NewArchiver(baseDir)
	a.maxFileSize = 10

	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if len(report.Rotated) != 1 {
		t.Fatalf("rotated %d files, want 1 (%+v)", len(report.Rotated), report.Rotated)
	}
	info, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("live file must survive rotation: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("live file size after rotation = %d, want 0", info.Size())
	}
	ent := report.Rotated[0]
	if ent.Lines != 3 {
		t.Errorf("rotated line count = %d, want 3", ent.Lines)
	}
	if got := string(readGzip(t, filepath.Join(baseDir, ent.Archive))); got != ledgerFixtureRecords {
		t.Errorf("rotated archive content = %q, want %q", got, ledgerFixtureRecords)
	}
}

func TestArchiver_RunWithReport_StaleOversizedFileIsMovedNotTruncated(t *testing.T) {
	baseDir := t.TempDir()
	live := writeAgedLedgerFile(t, baseDir, "metrics.jsonl", ledgerFixtureRecords, 400*24*time.Hour)

	a := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365})
	a.maxFileSize = 10

	report, err := a.RunWithReport()
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if len(report.Archived) != 1 || len(report.Rotated) != 0 {
		t.Fatalf("a stale file must be moved (not rotated): archived=%d rotated=%d", len(report.Archived), len(report.Rotated))
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("stale live file should be gone after the move (err=%v)", err)
	}
	if got := string(readGzip(t, filepath.Join(baseDir, report.Archived[0].Archive))); got != ledgerFixtureRecords {
		t.Errorf("moved archive content = %q, want %q", got, ledgerFixtureRecords)
	}
}

func TestArchiver_RunWithReport_HonorsContextCancellation(t *testing.T) {
	baseDir := t.TempDir()
	for _, rel := range []string{"a.jsonl", "b.jsonl"} {
		writeAgedLedgerFile(t, baseDir, rel, ledgerFixtureRecords, 400*24*time.Hour)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := NewArchiver(baseDir).WithPolicy(ArchivePolicy{RetentionDays: 365}).WithContext(ctx)
	report, err := a.RunWithReport()
	if err == nil {
		t.Fatal("expected a canceled-context error")
	}
	if len(report.Archived) != 0 {
		t.Errorf("canceled run archived %d files, want 0", len(report.Archived))
	}
}

func TestArchiver_compressVerified_ProducesReadableArchive(t *testing.T) {
	baseDir := t.TempDir()
	src := filepath.Join(baseDir, "src.jsonl")
	if err := os.WriteFile(src, []byte(ledgerFixtureRecords), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(baseDir, "archive.jsonl.gz")
	a := NewArchiver(baseDir)
	lines, err := a.compressVerified(src, dst)
	if err != nil {
		t.Fatalf("compressVerified: %v", err)
	}
	if lines != 3 {
		t.Errorf("verified lines = %d, want 3", lines)
	}
	if got := readGzip(t, dst); !bytes.Equal(got, []byte(ledgerFixtureRecords)) {
		t.Errorf("archive content = %q, want %q", got, ledgerFixtureRecords)
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary archive must be renamed away (err=%v)", err)
	}
}

func findArchived(files []ArchivedFile, rel string) *ArchivedFile {
	for i := range files {
		if files[i].Path == rel {
			return &files[i]
		}
	}
	return nil
}
