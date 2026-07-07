package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLifecycleManager_HasDefaults(t *testing.T) {
	m := NewLifecycleManager(t.TempDir())
	if len(m.policies) != 7 {
		t.Fatalf("expected 7 default policies, got %d", len(m.policies))
	}

	want := map[string]string{
		"macro":        "20*.json",
		"margin":       "*_margin.json",
		"export":       "*_export.json",
		"capital_flow": "*.json",
		"tsmc_revenue": "*_revenue.json",
		"traces":       "sim-*.jsonl",
	}
	for _, p := range m.policies {
		if p.Dir == "traces" && p.Pattern == "session-*.jsonl" {
			// Two policies share the traces dir; only assert the first one above.
			continue
		}
		if p.Pattern != want[p.Dir] {
			t.Errorf("policy %s: pattern = %q, want %q", p.Dir, p.Pattern, want[p.Dir])
		}
	}
}

func TestNewLifecycleManagerWithPolicies(t *testing.T) {
	custom := []RetentionPolicy{
		{Dir: "custom", MaxAgeDays: 30, Pattern: "*.csv"},
	}
	m := NewLifecycleManagerWithPolicies(t.TempDir(), custom)
	if len(m.policies) != 1 {
		t.Fatalf("expected 1 custom policy, got %d", len(m.policies))
	}
	if m.policies[0].Dir != "custom" {
		t.Errorf("expected dir 'custom', got %q", m.policies[0].Dir)
	}
}

func TestRun_DryRun_DoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "macro")
	marginDir := filepath.Join(dir, "margin")
	os.MkdirAll(macroDir, 0o755)
	os.MkdirAll(marginDir, 0o755)

	oldTime := time.Now().AddDate(0, 0, -30)

	oldFile := filepath.Join(macroDir, "2026-04-01.json")
	os.WriteFile(oldFile, []byte("{}"), 0o644)
	os.Chtimes(oldFile, oldTime, oldTime)

	recentFile := filepath.Join(macroDir, "2026-05-14.json")
	os.WriteFile(recentFile, []byte("{}"), 0o644)

	latestFile := filepath.Join(macroDir, "latest.json")
	os.WriteFile(latestFile, []byte("{}"), 0o644)
	os.Chtimes(latestFile, oldTime, oldTime)

	oldMargin := filepath.Join(marginDir, "20260401_margin.json")
	os.WriteFile(oldMargin, []byte("{}"), 0o644)
	os.Chtimes(oldMargin, oldTime, oldTime)

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 7, Pattern: "20*.json", ExcludeFiles: []string{"latest.json"}},
		{Dir: "margin", MaxAgeDays: 7, Pattern: "*_margin.json"},
	})

	report, err := m.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run dryRun: %v", err)
	}

	if report.TotalDeleted != 2 {
		t.Errorf("dryRun deleted = %d, want 2", report.TotalDeleted)
	}
	if report.TotalKept != 2 {
		t.Errorf("dryRun kept = %d, want 2 (recent + latest)", report.TotalKept)
	}

	for _, f := range []string{oldFile, recentFile, latestFile, oldMargin} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("dryRun should not delete %s", f)
		}
	}
}

func TestRun_Delete_OldFiles(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "macro")
	os.MkdirAll(macroDir, 0o755)

	oldTime := time.Now().AddDate(0, 0, -30)
	oldFile := filepath.Join(macroDir, "2026-04-10.json")
	os.WriteFile(oldFile, []byte("{}"), 0o644)
	os.Chtimes(oldFile, oldTime, oldTime)

	newFile := filepath.Join(macroDir, "2026-05-14.json")
	os.WriteFile(newFile, []byte("{}"), 0o644)

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 7, Pattern: "20*.json"},
	})

	report, err := m.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.TotalDeleted != 1 {
		t.Errorf("deleted = %d, want 1", report.TotalDeleted)
	}
	if report.TotalKept != 1 {
		t.Errorf("kept = %d, want 1", report.TotalKept)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted")
	}
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("new file should be kept")
	}
}

func TestRun_ExcludeFiles(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "macro")
	os.MkdirAll(macroDir, 0o755)

	veryOld := time.Now().AddDate(0, 0, -200)
	latestFile := filepath.Join(macroDir, "latest.json")
	os.WriteFile(latestFile, []byte("{}"), 0o644)
	os.Chtimes(latestFile, veryOld, veryOld)

	oldDataFile := filepath.Join(macroDir, "2025-10-01.json")
	os.WriteFile(oldDataFile, []byte("{}"), 0o644)
	os.Chtimes(oldDataFile, veryOld, veryOld)

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 90, Pattern: "20*.json", ExcludeFiles: []string{"latest.json"}},
	})

	report, err := m.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.TotalDeleted != 1 {
		t.Errorf("deleted = %d, want 1 (old data file only)", report.TotalDeleted)
	}
	if report.TotalKept != 1 {
		t.Errorf("kept = %d, want 1 (latest.json excluded)", report.TotalKept)
	}

	if _, err := os.Stat(latestFile); os.IsNotExist(err) {
		t.Error("latest.json should never be deleted regardless of age")
	}
	if _, err := os.Stat(oldDataFile); !os.IsNotExist(err) {
		t.Error("old data file matching pattern should be deleted")
	}
}

func TestRun_NonexistentDir(t *testing.T) {
	dir := t.TempDir()
	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "nonexistent", MaxAgeDays: 7, Pattern: "*.json"},
	})

	report, err := m.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("nonexistent dir should not error: %v", err)
	}
	if report.TotalDeleted != 0 {
		t.Errorf("deleted = %d, want 0", report.TotalDeleted)
	}
	if report.TotalKept != 0 {
		t.Errorf("kept = %d, want 0", report.TotalKept)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "macro"), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 7, Pattern: "*.json"},
	})

	_, err := m.Run(ctx, false)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "macro")
	marginDir := filepath.Join(dir, "margin")
	os.MkdirAll(macroDir, 0o755)
	os.MkdirAll(marginDir, 0o755)

	os.WriteFile(filepath.Join(macroDir, "2026-05-01.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(macroDir, "2026-05-14.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(macroDir, "latest.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(marginDir, "20260514_margin.json"), []byte("{}"), 0o644)

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 90, Pattern: "20*.json", ExcludeFiles: []string{"latest.json"}},
		{Dir: "margin", MaxAgeDays: 90, Pattern: "*_margin.json"},
	})

	stats, err := m.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats["macro"] != 2 {
		t.Errorf("macro count = %d, want 2 (latest.json excluded, 2 date files matched)", stats["macro"])
	}
	if stats["margin"] != 1 {
		t.Errorf("margin count = %d, want 1", stats["margin"])
	}
}

func TestStats_NonexistentDir(t *testing.T) {
	m := NewLifecycleManagerWithPolicies(t.TempDir(), []RetentionPolicy{
		{Dir: "no_such_dir", MaxAgeDays: 90, Pattern: "*.json"},
	})

	stats, err := m.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats["no_such_dir"] != 0 {
		t.Errorf("nonexistent dir count = %d, want 0", stats["no_such_dir"])
	}
}

func TestPolicyReport_OldestKept(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "macro")
	os.MkdirAll(macroDir, 0o755)

	day1 := time.Now().AddDate(0, 0, -5)
	day2 := time.Now().AddDate(0, 0, -2)

	f1 := filepath.Join(macroDir, "2026-05-09.json")
	os.WriteFile(f1, []byte("{}"), 0o644)
	os.Chtimes(f1, day1, day1)

	f2 := filepath.Join(macroDir, "2026-05-12.json")
	os.WriteFile(f2, []byte("{}"), 0o644)
	os.Chtimes(f2, day2, day2)

	m := NewLifecycleManagerWithPolicies(dir, []RetentionPolicy{
		{Dir: "macro", MaxAgeDays: 7, Pattern: "20*.json"},
	})

	report, err := m.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(report.Policies))
	}
	if report.Policies[0].OldestKept != "2026-05-09.json" {
		t.Errorf("oldest kept = %q, want %q", report.Policies[0].OldestKept, "2026-05-09.json")
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		want    bool
	}{
		{"latest.json", []string{"latest.json"}, true},
		{"other.json", []string{"latest.json"}, false},
		{"test.json", nil, false},
		{"test.json", []string{}, false},
	}
	for _, tt := range tests {
		if got := isExcluded(tt.name, tt.exclude); got != tt.want {
			t.Errorf("isExcluded(%q, %v) = %v, want %v", tt.name, tt.exclude, got, tt.want)
		}
	}
}
