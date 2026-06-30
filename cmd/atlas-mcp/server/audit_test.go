package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditWriter_AppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := 0; i < 3; i++ {
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
