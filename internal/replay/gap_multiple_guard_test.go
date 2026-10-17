package replay

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestForwardReturn_RejectsGapMultiple locks k3 decision 3-1 at the replay
// dataset boundary: a corrupted stored multiple is made of two usable prices, so
// the price guard (FU-20260930-04) cannot reject it and the caller received a
// +900% forward return. The row sits past the 5-session IPO window.
func TestForwardReturn_RejectsGapMultiple(t *testing.T) {
	dir := t.TempDir()
	rows := []string{"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction"}
	for i, day := range []string{"10", "11", "12", "13", "14", "15", "16", "17"} {
		// Distinct OHLCV per row: identical consecutive rows would be rejected
		// by the separate stale/duplicated-OHLCV guard, not by this one.
		closePrice := fmt.Sprintf("%d", 100+i)
		open, high, low := closePrice, fmt.Sprintf("%d", 101+i), fmt.Sprintf("%d", 99+i)
		if i == 7 { // 8th session: corrupted 10x multiple
			closePrice, open, high, low = "1000", "1000", "1001", "999"
		}
		rows = append(rows, "2026-03-"+day+",0001,Gap,10000,100000,"+open+","+high+","+low+","+closePrice+",0,500")
	}
	path := filepath.Join(dir, "gap_multiple.csv")
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	// 2026-03-16 -> 2026-03-17 crosses into the corrupted 10x close.
	date, _ := time.Parse("2006-01-02", "2026-03-16")
	if _, ok := ds.ForwardReturn("0001.TW", date, 1); ok {
		t.Error("ForwardReturn reported success across a 10x stored close; want ok=false")
	}
	// Control: a clean pair keeps its exact value (105 -> 106).
	cleanDate, _ := time.Parse("2006-01-02", "2026-03-15")
	if got, ok := ds.ForwardReturn("0001.TW", cleanDate, 1); !ok || math.Abs(got-(106.0/105.0-1)) > 1e-12 {
		t.Errorf("clean ForwardReturn = (%v, %v), want (%v, true)", got, ok, 106.0/105.0-1)
	}
}
