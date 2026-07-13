package eventquality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQualityLog_FileAppendsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quality.jsonl")

	first, err := NewFileQualityLog(path)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	first.Record(ValidationResult{EventID: "evt-1", Accepted: false, Rule: "required", Field: "event_id", Reason: "missing"})
	_ = first.Close()

	second, err := NewFileQualityLog(path)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	second.Record(ValidationResult{EventID: "evt-2", Accepted: true, Rule: "ok", Field: "", Reason: ""})
	_ = second.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], `"event_id":"evt-1"`) || !strings.Contains(lines[0], `"accepted":false`) {
		t.Errorf("line 0 missing evt-1/accepted=false: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"event_id":"evt-2"`) || !strings.Contains(lines[1], `"accepted":true`) {
		t.Errorf("line 1 missing evt-2/accepted=true: %s", lines[1])
	}
}

func TestQualityLog_FileCreatesIfMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "quality.jsonl")

	ql, err := NewFileQualityLog(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer ql.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s: %v", path, err)
	}
}
