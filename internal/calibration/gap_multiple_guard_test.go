package calibration

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadReturnsFromCSV_RejectsGapMultiple locks k3 decision 3-1 in the
// calibration label loader: a corrupted stored multiple (FU-20260930-03) is made
// of two usable closes, so the price guard passes and the +900% / -90% pairs
// used to enter the label series that calibration fits.
func TestLoadReturnsFromCSV_RejectsGapMultiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gap_multiple.csv")
	rows := []string{"Date,Code,Name,TradeVolume,Open,High,Low,Close"}
	for i, day := range []string{"01", "02", "03", "08", "09", "10", "11", "12", "15"} {
		closePrice := "500"
		open, high, low := "500", "510", "495"
		if i == 7 { // 8th usable session: corrupted 10x multiple
			closePrice, open, high, low = "5000", "5000", "5010", "4995"
		}
		rows = append(rows, "2024-01-"+day+",2330,TSMC,1000,"+open+","+high+","+low+","+closePrice)
	}
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	returns, _, err := LoadReturnsFromCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 9 usable closes => 8 pairs; the pair INTO the corrupted close (500 ->
	// 5000) and the pair OUT of it (5000 -> 500, a poisoned previous-close
	// chain) are both rejected, leaving 6.
	if len(returns) != 6 {
		t.Fatalf("got %d returns %v, want 6 (the two pairs touching the 10x close contribute nothing)", len(returns), returns)
	}
	for i, r := range returns {
		if math.Abs(r) > 0.5 {
			t.Errorf("return[%d] = %v: a corrupted multiple became a return", i, r)
		}
	}
}
