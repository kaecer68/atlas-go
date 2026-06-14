package replay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateReplayVersion_HappyPath(t *testing.T) {
	dir := t.TempDir()
	// VERSION file references "twse_stock_day_all_sample.csv"
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("twse_stock_day_all_sample.csv"), 0o644); err != nil {
		t.Fatal(err)
	}
	// source file must exist
	if err := os.WriteFile(filepath.Join(dir, "twse_stock_day_all_sample.csv"), []byte("header\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateReplayVersion(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReplayVersion_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	err := ValidateReplayVersion(dir)
	if err == nil {
		t.Fatal("expected error for missing VERSION file")
	}
}

func TestValidateReplayVersion_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ValidateReplayVersion(dir)
	if err == nil {
		t.Fatal("expected error for empty VERSION file")
	}
}

func TestValidateReplayVersion_SourceFileMissing(t *testing.T) {
	dir := t.TempDir()
	// VERSION references a file that doesn't exist
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("nonexistent.csv"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ValidateReplayVersion(dir)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestValidateReplayVersion_WhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("  twse_stock_day_all_sample.csv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "twse_stock_day_all_sample.csv"), []byte("header\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateReplayVersion(dir); err != nil {
		t.Fatalf("unexpected error (whitespace should be trimmed): %v", err)
	}
}

func TestComputeChecksum_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	checksum, err := ComputeChecksum(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checksum == "" {
		t.Fatal("expected non-empty checksum")
	}
	if len(checksum) != 64 {
		t.Fatalf("expected 64-char SHA256 hex, got %d", len(checksum))
	}
}

func TestComputeChecksum_FileNotFound(t *testing.T) {
	_, err := ComputeChecksum("/nonexistent/path.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestComputeChecksum_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("deterministic"), 0o644); err != nil {
		t.Fatal(err)
	}

	c1, _ := ComputeChecksum(path)
	c2, _ := ComputeChecksum(path)
	if c1 != c2 {
		t.Fatalf("checksums differ: %s vs %s", c1, c2)
	}
}

func TestComputeChecksum_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	checksum, err := ComputeChecksum(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty file SHA256 is well-known
	if checksum != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("unexpected empty-file checksum: %s", checksum)
	}
}

func TestWriteVersion_NoChecksum(t *testing.T) {
	dir := t.TempDir()
	if err := WriteVersion(dir, "replay.csv", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replay.csv" {
		t.Fatalf("expected 'replay.csv', got '%s'", string(data))
	}
}

func TestWriteVersion_WithChecksum(t *testing.T) {
	dir := t.TempDir()
	if err := WriteVersion(dir, "replay.csv", "abc123"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "replay.csv\nchecksum=abc123"
	if string(data) != expected {
		t.Fatalf("expected '%s', got '%s'", expected, string(data))
	}
}

func TestWriteVersion_Overwrite(t *testing.T) {
	dir := t.TempDir()
	versionPath := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(versionPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Overwrite
	if err := WriteVersion(dir, "new.csv", "xyz"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new.csv\nchecksum=xyz" {
		t.Fatalf("expected overwritten content, got '%s'", string(data))
	}
}

func TestWriteVersion_InvalidDirectory(t *testing.T) {
	dir := t.TempDir()
	invalidDir := filepath.Join(dir, "nonexistent", "subdir")
	err := WriteVersion(invalidDir, "replay.csv", "")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestValidateReplayVersion_SlashedVersionPath(t *testing.T) {
	// VERSION is a directory, not a file — should get a read error
	dir := t.TempDir()
	versionPath := filepath.Join(dir, "VERSION")
	if err := os.MkdirAll(versionPath, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ValidateReplayVersion(dir)
	if err == nil {
		t.Fatal("expected error when VERSION is a directory")
	}
}
