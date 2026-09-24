// Store-level tests for the per-symbol flow store, including the freshness
// bookkeeping (NewestStoredDate) the daily refresh uses to pick its window.
package stockflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// TestNewestStoredDate covers the incremental window source: empty/missing
// store → not ok; otherwise the newest date across every symbol file.
func TestNewestStoredDate(t *testing.T) {
	workdir := t.TempDir()
	dir := Dir(workdir)

	if _, ok, err := NewestStoredDate(dir); err != nil || ok {
		t.Fatalf("empty store = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "2330.json"), "2330", "2026-08-26", "2026-08-27")
	writeFile(t, filepath.Join(dir, "2317.json"), "2317", "2026-08-25")
	// A temp file left by a crashed write must not count.
	writeFile(t, filepath.Join(dir, "9999.json.tmp"), "9999", "2026-09-30")

	newest, ok, err := NewestStoredDate(dir)
	if err != nil || !ok {
		t.Fatalf("NewestStoredDate = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if got := newest.Format("2006-01-02"); got != "2026-08-27" {
		t.Fatalf("NewestStoredDate = %s, want 2026-08-27 (ignoring .tmp)", got)
	}
}

// TestMergeSymbolFile_IdempotentAndSorted: merging one day twice neither
// duplicates nor reorders the series, and only the new date counts as added.
func TestMergeSymbolFile_IdempotentAndSorted(t *testing.T) {
	dir := t.TempDir()
	day := []marketdata.SymbolFlow{{Symbol: "2330", Date: "20260828", ForeignInvestorNet: 1.5}}

	added, total, err := MergeSymbolFile(dir, "2330", day)
	if err != nil || added != 1 || total != 1 {
		t.Fatalf("first merge = (added=%d, total=%d, err=%v), want (1, 1, nil)", added, total, err)
	}
	added, total, err = MergeSymbolFile(dir, "2330", append(day, marketdata.SymbolFlow{
		Symbol: "2330", Date: "20260827", ForeignInvestorNet: -3,
	}))
	if err != nil || added != 1 || total != 2 {
		t.Fatalf("second merge = (added=%d, total=%d, err=%v), want (1, 2, nil)", added, total, err)
	}

	f, err := LoadFile(filepath.Join(dir, "2330.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(f.Flows) != 2 || f.Flows[0].Date != "2026-08-27" || f.Flows[1].Date != "2026-08-28" {
		t.Fatalf("flows = %+v, want ascending 08-27, 08-28", f.Flows)
	}
}

// TestCountFlowPointsAndVerify: the post-run bookkeeping used by the CLI
// summary and the fake-success gate.
func TestCountFlowPointsAndVerify(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "2330.json"), "2330", "2026-08-27")
	writeFile(t, filepath.Join(dir, "2317.json"), "2317", "2026-08-26", "2026-08-27")

	total, err := CountFlowPoints(dir)
	if err != nil || total != 3 {
		t.Fatalf("CountFlowPoints = (%d, %v), want (3, nil)", total, err)
	}
	var verified int
	if err := VerifyFiles(dir, map[string]bool{"2330": true, "2317": true}, func(string, ...any) { verified++ }); err != nil {
		t.Fatalf("VerifyFiles: %v", err)
	}
	if verified != 2 {
		t.Fatalf("verified = %d, want 2", verified)
	}
	// An out-of-order file must be reported.
	writeFile(t, filepath.Join(dir, "2454.json"), "2454", "2026-08-28", "2026-08-27")
	if err := VerifyFiles(dir, map[string]bool{"2454": true}, nil); err == nil {
		t.Fatal("VerifyFiles on an unsorted file = nil, want error")
	}
}

func writeFile(t *testing.T, path, symbol string, dates ...string) {
	t.Helper()
	flows := make([]stockpicker.FlowPoint, 0, len(dates))
	for _, d := range dates {
		flows = append(flows, stockpicker.FlowPoint{Date: d, ForeignNet: 1000})
	}
	data, err := json.Marshal(stockpicker.FlowFile{Symbol: symbol, Flows: flows})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
