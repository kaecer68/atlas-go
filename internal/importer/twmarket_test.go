package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportTWOpenDataCSVToJSONL(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bars.jsonl")
	err := ImportTWOpenDataCSVToJSONL("../../samples/replay/twse_stock_day_all_sample.csv", target)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}
