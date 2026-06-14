package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// intVal returns the int value from an interface{} that may be int or int64.
func intVal(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func TestNewArchiver(t *testing.T) {
	a := NewArchiver("/tmp/ledger")
	if a.baseDir != "/tmp/ledger" {
		t.Errorf("baseDir = %q, want /tmp/ledger", a.baseDir)
	}
	if a.maxFileSize == 0 {
		t.Error("maxFileSize should be non-zero")
	}
	if a.retentionDays == 0 {
		t.Error("retentionDays should be non-zero")
	}
}

func TestArchiver_ArchivableFiles(t *testing.T) {
	a := NewArchiver(t.TempDir())
	files := a.ArchivableFiles()
	if len(files) != 4 {
		t.Errorf("expected 4 archivable files, got %d: %v", len(files), files)
	}
	want := map[string]bool{
		"experiments.jsonl":         true,
		"human_interventions.jsonl": true,
		"spawn_records.jsonl":       true,
		"alerts.jsonl":              true,
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected archivable file: %q", f)
		}
	}
}

func TestArchiver_archiveDir(t *testing.T) {
	a := NewArchiver("/tmp/ledger")
	got := a.archiveDir()
	if !strings.HasSuffix(got, "archive") {
		t.Errorf("archiveDir = %q, want path ending with 'archive'", got)
	}
}

func TestArchiver_Stats_NoArchiveDir(t *testing.T) {
	a := NewArchiver(t.TempDir())
	stats, err := a.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if intVal(stats["archive_count"]) != 0 {
		t.Errorf("archive_count = %v, want 0", stats["archive_count"])
	}
	if intVal(stats["retention_days"]) != defaultRetentionDays {
		t.Errorf("retention_days = %v, want %d", stats["retention_days"], defaultRetentionDays)
	}
}

func TestArchiver_Stats_EmptyArchiveDir(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stats, err := a.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if intVal(stats["archive_count"]) != 0 {
		t.Errorf("archive_count = %v, want 0", stats["archive_count"])
	}
	if intVal(stats["total_size_mb"]) != 0 {
		t.Errorf("total_size_mb = %v, want 0", stats["total_size_mb"])
	}
}

func TestArchiver_Stats_WithFiles(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	archiveDir := a.archiveDir()
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "experiments-20260101-120000.jsonl.gz"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "spawn-20260101-120000.jsonl.gz"), []byte("world"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	stats, err := a.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if intVal(stats["archive_count"]) != 2 {
		t.Errorf("archive_count = %v, want 2", stats["archive_count"])
	}
}

func TestArchiver_Stats_SkipsDirs(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	archiveDir := a.archiveDir()
	if err := os.MkdirAll(filepath.Join(archiveDir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "file1.jsonl.gz"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	stats, err := a.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// Stats() uses len(entries) which includes dirs - existing behavior
	if intVal(stats["archive_count"]) != 2 {
		t.Errorf("archive_count = %v, want 2 (1 file + 1 dir)", stats["archive_count"])
	}
}

func TestArchiver_compressFile(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	src := filepath.Join(baseDir, "src.jsonl")
	if err := os.WriteFile(src, []byte(`{"id":1}
{"id":2}
`), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(baseDir, "archive.jsonl.gz")
	if err := a.compressFile(src, dst); err != nil {
		t.Fatalf("compressFile: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("archive file should exist: %v", err)
	}
	info, _ := os.Stat(dst)
	if info.Size() == 0 {
		t.Error("archived file should not be empty")
	}
}

func TestArchiver_compressFile_SourceNotFound(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	err := a.compressFile(filepath.Join(baseDir, "nonexistent.jsonl"), filepath.Join(baseDir, "archive.gz"))
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestArchiver_archiveIfNeeded_FileNotExist(t *testing.T) {
	a := NewArchiver(t.TempDir())
	if err := a.archiveIfNeeded("nonexistent.jsonl"); err != nil {
		t.Errorf("archiveIfNeeded for missing file should return nil: %v", err)
	}
}

func TestArchiver_archiveIfNeeded_FileTooSmall(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	path := filepath.Join(baseDir, "experiments.jsonl")
	if err := os.WriteFile(path, []byte("small file\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := a.archiveIfNeeded("experiments.jsonl"); err != nil {
		t.Errorf("archiveIfNeeded for small file should return nil: %v", err)
	}
	// File should be untouched
	info, _ := os.Stat(path)
	if info.Size() == 0 {
		t.Error("small file should not have been truncated")
	}
}

func TestArchiver_archiveIfNeeded_FileLargeEnough(t *testing.T) {
	baseDir := t.TempDir()
	// Use a tiny maxFileSize to force archival
	a := &Archiver{
		baseDir:       baseDir,
		maxFileSize:   10, // 10 bytes
		retentionDays: defaultRetentionDays,
	}
	// archiveIfNeeded requires the archive dir to exist (Run() creates it)
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	filename := "experiments.jsonl"
	path := filepath.Join(baseDir, filename)
	content := `{"id":1,"msg":"this is more than ten bytes"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := a.archiveIfNeeded(filename); err != nil {
		t.Fatalf("archiveIfNeeded: %v", err)
	}
	// Source file should be truncated
	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("source file should be truncated, got size %d", info.Size())
	}
	// Archive file should exist in archive dir
	entries, _ := os.ReadDir(a.archiveDir())
	if len(entries) == 0 {
		t.Error("expected archive file in archive dir")
	}
}

func TestArchiver_cleanupOldArchives_NoDir(t *testing.T) {
	a := NewArchiver(t.TempDir())
	if err := a.cleanupOldArchives(); err != nil {
		t.Errorf("cleanupOldArchives with no dir: %v", err)
	}
}

func TestArchiver_cleanupOldArchives_EmptyDir(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := a.cleanupOldArchives(); err != nil {
		t.Errorf("cleanupOldArchives empty dir: %v", err)
	}
}

func TestArchiver_cleanupOldArchives_SkipsDirs(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	archiveDir := a.archiveDir()
	if err := os.MkdirAll(filepath.Join(archiveDir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Should not error on subdirs
	if err := a.cleanupOldArchives(); err != nil {
		t.Errorf("cleanupOldArchives with subdir: %v", err)
	}
}

func TestArchiver_Run(t *testing.T) {
	baseDir := t.TempDir()
	a := NewArchiver(baseDir)
	if err := a.Run(); err != nil {
		t.Errorf("Run on empty dir: %v", err)
	}
	// Archive dir should be created
	if _, err := os.Stat(a.archiveDir()); os.IsNotExist(err) {
		t.Error("archive dir should have been created")
	}
}

func TestArchiver_Run_WithLargeFile(t *testing.T) {
	baseDir := t.TempDir()
	a := &Archiver{
		baseDir:       baseDir,
		maxFileSize:   10,
		retentionDays: defaultRetentionDays,
	}
	filename := "experiments.jsonl"
	path := filepath.Join(baseDir, filename)
	if err := os.WriteFile(path, []byte(`{"id":1,"data":"long enough to exceed 10 bytes"}
`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("source file should be truncated after Run, got size %d", info.Size())
	}
	entries, _ := os.ReadDir(a.archiveDir())
	if len(entries) == 0 {
		t.Error("expected archive file after Run")
	}
}

func TestArchiver_Run_AllArchivableFiles(t *testing.T) {
	baseDir := t.TempDir()
	a := &Archiver{
		baseDir:       baseDir,
		maxFileSize:   10,
		retentionDays: defaultRetentionDays,
	}
	// Create all 4 archivable files
	for _, name := range a.ArchivableFiles() {
		path := filepath.Join(baseDir, name)
		if err := os.WriteFile(path, []byte(`{"id":1,"data":"exceeds 10 byte limit for testing"}
`), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := a.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// All source files should be truncated
	for _, name := range a.ArchivableFiles() {
		info, _ := os.Stat(filepath.Join(baseDir, name))
		if info.Size() != 0 {
			t.Errorf("%s should be truncated, got size %d", name, info.Size())
		}
	}
}
