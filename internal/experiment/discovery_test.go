package experiment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLatestExperiment_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := FindLatestExperiment(dir)
	if result != "" {
		t.Errorf("expected empty string for empty dir, got %q", result)
	}
}

func TestFindLatestExperiment_NoJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b,c"), 0o644)

	result := FindLatestExperiment(dir)
	if result != "" {
		t.Errorf("expected empty string when no .json files, got %q", result)
	}
}

func TestFindLatestExperiment_ExcludesTestExperiment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test-experiment.json"), []byte("{}"), 0o644)

	result := FindLatestExperiment(dir)
	if result != "" {
		t.Errorf("expected empty string when only test-experiment.json exists, got %q", result)
	}
}

func TestFindLatestExperiment_NonDirExcluded(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "result-1700000000.json"), []byte("{}"), 0o644)

	result := FindLatestExperiment(dir)
	if result == "" {
		t.Error("expected a file to be found, got empty string")
	}
}

func TestFindLatestExperiment_TimestampSortOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "result-100.json"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dir, "result-200.json"), []byte("newer"), 0o644)
	os.WriteFile(filepath.Join(dir, "result-300.json"), []byte("newest"), 0o644)

	result := FindLatestExperiment(dir)
	expected := filepath.Join(dir, "result-300.json")
	if result != expected {
		t.Errorf("expected newest file %q, got %q", expected, result)
	}
}

func TestFindLatestExperiment_MixedPrefixes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "experiment-500.json"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "result-400.json"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "xxx-100.json"), []byte("c"), 0o644)

	// Should sort by timestamp regardless of prefix
	result := FindLatestExperiment(dir)
	expected := filepath.Join(dir, "experiment-500.json")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFindLatestExperiment_NonExistentDir(t *testing.T) {
	result := FindLatestExperiment("/nonexistent/path/12345")
	if result != "" {
		t.Errorf("expected empty string for nonexistent dir, got %q", result)
	}
}

func TestFindLatestExperiment_SingleFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "result-1700000000.json"), []byte("{}"), 0o644)

	result := FindLatestExperiment(dir)
	expected := filepath.Join(dir, "result-1700000000.json")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		filename string
		want     int64
	}{
		{"result-1700000000.json", 1700000000},
		{"experiment-100.json", 100},
		{"no-timestamp.json", 0},
		{"prefix-abc-123.json", 123},
		{"123.json", 123},
		{"", 0},
		{"just-hyphen-.json", 0},
		{"result-0.json", 0},
		{"result--5.json", 5}, // split yields ["result","","5"], last="5" → 5
	}

	for _, tc := range tests {
		got := extractTimestamp(tc.filename)
		if got != tc.want {
			t.Errorf("extractTimestamp(%q) = %d, want %d", tc.filename, got, tc.want)
		}
	}
}
