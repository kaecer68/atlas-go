package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleManager_CleansOldSimTraces(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(tracesDir, "sim-20250101.jsonl")
	newFile := filepath.Join(tracesDir, "sim-20260708.jsonl")
	if err := os.WriteFile(oldFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	lm := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "traces", MaxAgeDays: 7, Pattern: "sim-*.jsonl"},
	})
	report, err := lm.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.TotalDeleted != 1 {
		t.Fatalf("expected 1 old sim trace deleted, got report: %+v", report)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("old sim trace should have been deleted")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("new sim trace should be kept: %v", err)
	}
}

func TestLifecycleManager_CleansOldSessionTraces(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(tracesDir, "session-20250101-daily.jsonl")
	newFile := filepath.Join(tracesDir, "session-20260708-daily.jsonl")
	if err := os.WriteFile(oldFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().AddDate(0, 0, -40)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	lm := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "traces", MaxAgeDays: 30, Pattern: "session-*.jsonl"},
	})
	if _, err := lm.Run(context.Background(), false); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("old session trace should have been deleted")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("new session trace should be kept: %v", err)
	}
}
