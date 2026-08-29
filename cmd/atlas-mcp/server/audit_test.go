package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditWriter_AppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := range 3 {
		if wErr := w.Write(AuditEntry{Tool: "t", Status: "ok", DurationMS: int64(i)}); wErr != nil {
			t.Fatalf("write %d: %v", i, wErr)
		}
	}
	if cErr := w.Close(); cErr != nil {
		t.Fatalf("close: %v", cErr)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d (%q)", len(lines), string(raw))
	}
	for i, line := range lines {
		var e AuditEntry
		if jErr := json.Unmarshal([]byte(line), &e); jErr != nil {
			t.Fatalf("line %d not valid JSON: %v (%q)", i, jErr, line)
		}
		if e.Tool != "t" || e.Status != "ok" {
			t.Fatalf("line %d unexpected: %+v", i, e)
		}
	}
}

func TestAuditWriter_DoubleCloseIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	if err := w.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second close (idempotent): %v", err)
	}
}

func TestAuditWriter_PopulatesTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()
	if err := w.Write(AuditEntry{Tool: "x", Status: "ok"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"ts":`) {
		t.Fatalf("expected auto-populated ts field, got %s", string(raw))
	}
}

func TestAuditWriter_RequiresParentDir(t *testing.T) {
	// Tests an explicit failure case: open on a path whose parent can't be
	// created (regular file in place of directory).
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	bad := filepath.Join(blocker, "audit.log")
	if _, err := NewAuditWriter(bad); err == nil {
		t.Fatal("expected error creating audit log under non-directory blocker")
	}
}

// === Phase 3: audit log retention ===

func TestAuditWriter_Cleanup_RemovesOldEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	now := time.Now().UTC()
	entries := []AuditEntry{
		{TS: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "old", Status: "ok"},
		{TS: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "mid", Status: "ok"},
		{TS: now.Add(-1 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "new", Status: "ok"},
	}
	for _, e := range entries {
		if wErr := w.Write(e); wErr != nil {
			t.Fatalf("write: %v", wErr)
		}
	}
	w.Close() // release handle so Cleanup can rewrite atomically

	w2, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	removed, cErr := w2.Cleanup(30, now)
	w2.Close()
	if cErr != nil {
		t.Fatalf("cleanup: %v", cErr)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	raw, rErr := os.ReadFile(path)
	if rErr != nil {
		t.Fatalf("read: %v", rErr)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after cleanup, got %d: %q", len(lines), string(raw))
	}
	if !strings.Contains(string(raw), `"tool":"mid"`) || !strings.Contains(string(raw), `"tool":"new"`) {
		t.Fatalf("expected mid+new to remain, got: %s", string(raw))
	}
	if strings.Contains(string(raw), `"tool":"old"`) {
		t.Fatalf("old entry should have been removed: %s", string(raw))
	}
}

func TestAuditWriter_Cleanup_DisabledWhenZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()
	w.Write(AuditEntry{Tool: "t", Status: "ok"})

	removed, err := w.Cleanup(0, time.Now())
	if err != nil {
		t.Fatalf("cleanup with 0 days: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected 0 removed, got %d", removed)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"tool":"t"`) {
		t.Fatalf("file should be unchanged when retention disabled: %s", string(raw))
	}
}

func TestAuditWriter_Cleanup_NegativeDaysIsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()
	w.Write(AuditEntry{Tool: "t", Status: "ok"})

	removed, err := w.Cleanup(-5, time.Now())
	if err != nil || removed != 0 {
		t.Fatalf("cleanup with -5 days should be no-op: removed=%d err=%v", removed, err)
	}
}

func TestAuditWriter_Cleanup_EmptyFileIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	removed, err := w.Cleanup(30, time.Now())
	if err != nil {
		t.Fatalf("cleanup on empty file: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected 0 removed, got %d", removed)
	}
}

func TestAuditWriter_Cleanup_WriterRemainsUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	oldTS := time.Now().UTC().Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano)
	w.Write(AuditEntry{Tool: "old", Status: "ok", TS: oldTS})

	removed, err := w.Cleanup(30, time.Now())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	// Writer must remain usable after cleanup (file reopened for append).
	if wErr := w.Write(AuditEntry{Tool: "new", Status: "ok"}); wErr != nil {
		t.Fatalf("write after cleanup: %v", wErr)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"tool":"new"`) {
		t.Fatalf("post-cleanup write missing: %s", string(raw))
	}
	if strings.Contains(string(raw), `"tool":"old"`) {
		t.Fatalf("old entry should have been removed: %s", string(raw))
	}
}

func TestAuditWriter_Cleanup_KeepsMalformedLines(t *testing.T) {
	// Corruption visible > corruption hidden: malformed lines are kept.
	path := filepath.Join(t.TempDir(), "a.log")
	w, _ := NewAuditWriter(path)

	now := time.Now().UTC()
	w.Write(AuditEntry{TS: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "old", Status: "ok"})
	// Inject a malformed line directly (bypass the JSON encoder).
	f, fErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if fErr != nil {
		t.Fatalf("open for malformed inject: %v", fErr)
	}
	if _, wErr := f.WriteString("this is not json\n"); wErr != nil {
		t.Fatalf("inject malformed: %v", wErr)
	}
	f.Close()
	w.Write(AuditEntry{Tool: "new", Status: "ok"})
	w.Close()

	w2, _ := NewAuditWriter(path)
	defer w2.Close()
	removed, _ := w2.Cleanup(30, now)

	raw, _ := os.ReadFile(path)
	if removed != 1 {
		t.Fatalf("expected 1 removed (only old valid), got %d", removed)
	}
	if !strings.Contains(string(raw), "this is not json") {
		t.Fatalf("malformed line should be kept (visible corruption): %s", string(raw))
	}
	if !strings.Contains(string(raw), `"tool":"new"`) {
		t.Fatalf("new entry should be kept: %s", string(raw))
	}
	if strings.Contains(string(raw), `"tool":"old"`) {
		t.Fatalf("old entry should have been removed: %s", string(raw))
	}
}
