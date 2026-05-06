package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveStateCreatesTimestampedCopy(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(src, "sessions", "s1"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "baseline_policy.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}

	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 timestamped archive dir, got %d", len(entries))
	}
}

func TestArchiveStatePreservesDirectoryStructure(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	sessionDir := filepath.Join(src, "sessions", "session-20260101")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	rootContent := `{"version":1,"constraints":{"starting_cash":1000000}}`
	if err := os.WriteFile(filepath.Join(src, "baseline_policy.json"), []byte(rootContent), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	outcomeLine := `{"agent_id":"agent-1","symbol":"2330.TW","conviction":80}` + "\n"
	if err := os.WriteFile(filepath.Join(src, "recommendation_outcomes.jsonl"), []byte(outcomeLine), 0o644); err != nil {
		t.Fatalf("write outcomes: %v", err)
	}

	sessionSummary := `{"session_id":"session-20260101","order_count":1}`
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte(sessionSummary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}

	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 archive dir, got %d", len(entries))
	}

	archiveDir := filepath.Join(archiveBase, entries[0].Name())

	// Verify root files
	for _, relPath := range []string{"baseline_policy.json", "recommendation_outcomes.jsonl"} {
		archivePath := filepath.Join(archiveDir, relPath)
		if _, err := os.Stat(archivePath); err != nil {
			t.Fatalf("expected %s in archive: %v", relPath, err)
		}
	}

	// Verify nested structure
	sessionSummaryArchive := filepath.Join(archiveDir, "sessions", "session-20260101", "summary.json")
	if _, err := os.Stat(sessionSummaryArchive); err != nil {
		t.Fatalf("expected session summary in archive: %v", err)
	}

	// Verify contents match
	wantBaseline, _ := os.ReadFile(filepath.Join(src, "baseline_policy.json"))
	gotBaseline, _ := os.ReadFile(filepath.Join(archiveDir, "baseline_policy.json"))
	if string(wantBaseline) != string(gotBaseline) {
		t.Fatalf("baseline_policy.json content mismatch")
	}

	wantOutcomes, _ := os.ReadFile(filepath.Join(src, "recommendation_outcomes.jsonl"))
	gotOutcomes, _ := os.ReadFile(filepath.Join(archiveDir, "recommendation_outcomes.jsonl"))
	if string(wantOutcomes) != string(gotOutcomes) {
		t.Fatalf("recommendation_outcomes.jsonl content mismatch")
	}
}

func TestArchiveStateRejectsMissingSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "nonexistent")
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout)
	if err == nil {
		t.Fatalf("expected error for missing source dir, got nil")
	}
	if !strings.Contains(err.Error(), "source") {
		t.Fatalf("expected error to mention source, got: %v", err)
	}
}

func TestArchiveStateRejectsFileAsSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(src, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout)
	if err == nil {
		t.Fatalf("expected error when source is a file, got nil")
	}
}

func TestArchiveStateDoesNotMutateSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "data.json"), []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Capture pre-archive state
	preStat, err := os.Stat(filepath.Join(src, "data.json"))
	if err != nil {
		t.Fatalf("stat pre: %v", err)
	}
	preModTime := preStat.ModTime()
	preSize := preStat.Size()

	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}

	// Verify source unchanged
	postStat, err := os.Stat(filepath.Join(src, "data.json"))
	if err != nil {
		t.Fatalf("stat post: %v", err)
	}
	if postStat.ModTime() != preModTime {
		t.Fatalf("source file modtime changed: %v -> %v", preModTime, postStat.ModTime())
	}
	if postStat.Size() != preSize {
		t.Fatalf("source file size changed: %d -> %d", preSize, postStat.Size())
	}

	// Verify content unchanged
	content, err := os.ReadFile(filepath.Join(src, "data.json"))
	if err != nil {
		t.Fatalf("read post: %v", err)
	}
	if string(content) != `{"key":"value"}` {
		t.Fatalf("source file content mutated")
	}
}

func TestArchiveStatePrintsArchivePath(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, archiveBase) {
		t.Fatalf("expected stdout to contain archive base path, got: %s", output)
	}
}

func TestArchiveStateDefaults(t *testing.T) {
	// With defaults (-src=data/state -dst-base=data/state-archive), if data/state
	// does not exist the command should error rather than panic or silently succeed.
	var stdout bytes.Buffer
	err := run([]string{}, &stdout)
	if err == nil {
		t.Fatalf("expected error when default source dir missing, got nil")
	}
}

func TestArchiveStateHandlesEmptySource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "empty-state")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive for empty source: %v", err)
	}

	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 archive dir even for empty source, got %d", len(entries))
	}

	archiveDir := filepath.Join(archiveBase, entries[0].Name())
	archiveEntries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(archiveEntries) != 0 {
		t.Fatalf("expected empty archive dir, got %d entries", len(archiveEntries))
	}
}

func TestArchiveStateTimestampIsUTC(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	before := time.Now().UTC()
	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}
	after := time.Now().UTC()

	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 archive dir, got %d", len(entries))
	}

	// Parse the timestamp directory name
	ts, err := time.Parse("20060102T150405Z", entries[0].Name())
	if err != nil {
		t.Fatalf("archive dir name is not valid UTC timestamp %q: %v", entries[0].Name(), err)
	}
	if ts.Before(before.Add(-time.Second)) || ts.After(after.Add(time.Second)) {
		t.Fatalf("timestamp %v out of range [%v, %v]", ts, before, after)
	}
}
