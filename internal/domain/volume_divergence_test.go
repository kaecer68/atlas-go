package domain

import (
	"testing"
	"time"
)

// mkBars builds n chronological daily bars ending at 2026-09-04 (a Friday).
// closes[i] and vols[i] set bar i; both slices must have length n.
func mkBars(closes []float64, vols []int64) []DailyBar {
	base := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	bars := make([]DailyBar, len(closes))
	for i := range closes {
		bars[i] = DailyBar{
			Date:   base.AddDate(0, 0, i-len(closes)+1),
			Symbol: "2330.TW",
			Close:  closes[i],
			Volume: vols[i],
		}
	}
	return bars
}

// ramp returns n values linearly interpolated from a to b (inclusive ends).
func ramp(a, b float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = a + (b-a)*float64(i)/float64(n-1)
	}
	return out
}

func rampInt(a, b int64, n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = a + int64(float64(b-a)*float64(i)/float64(n-1))
	}
	return out
}

func TestDetectVolumeDivergence_TopDivergence(t *testing.T) {
	// Price ramps 100 → 130 (latest close = window high), volume decays
	// 30000 → 10000 so volMA5 (last 5 avg) < volMA20.
	bars := mkBars(ramp(100, 130, 30), rampInt(30000, 10000, 30))
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !res.TopDivergence {
		t.Errorf("expected TopDivergence=true, got %+v", res)
	}
	if res.BottomDivergence {
		t.Errorf("unexpected BottomDivergence=true: %+v", res)
	}
	if !res.VolumeDeclining {
		t.Error("expected VolumeDeclining=true")
	}
	if res.CloseBelowHighPct != 0 {
		t.Errorf("expected close at window high, got %v%% below", res.CloseBelowHighPct)
	}
	if res.BarsUsed != 30 || res.WindowDays != 30 {
		t.Errorf("unexpected window bookkeeping: %+v", res)
	}
	if res.LatestDate != "2026-09-04" {
		t.Errorf("unexpected latest date %q", res.LatestDate)
	}
}

func TestDetectVolumeDivergence_BottomDivergence(t *testing.T) {
	// Price ramps 130 → 100 (latest close = window low), volume decays.
	bars := mkBars(ramp(130, 100, 30), rampInt(30000, 10000, 30))
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !res.BottomDivergence {
		t.Errorf("expected BottomDivergence=true, got %+v", res)
	}
	if res.TopDivergence {
		t.Errorf("unexpected TopDivergence=true: %+v", res)
	}
	if res.CloseAboveLowPct != 0 {
		t.Errorf("expected close at window low, got %v%% above", res.CloseAboveLowPct)
	}
}

func TestDetectVolumeDivergence_NoDivergenceWhenVolumeRising(t *testing.T) {
	// Price at window high but volume expanding → no divergence.
	bars := mkBars(ramp(100, 130, 30), rampInt(10000, 30000, 30))
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if res.TopDivergence || res.BottomDivergence {
		t.Errorf("expected no divergence with rising volume, got %+v", res)
	}
	if res.VolumeDeclining {
		t.Error("expected VolumeDeclining=false")
	}
}

func TestDetectVolumeDivergence_NoDivergenceMidRange(t *testing.T) {
	// Volume declining but price mid-range → no divergence flags.
	closes := ramp(100, 130, 30)
	// Push the latest close back to the middle of the range.
	closes[29] = 115
	bars := mkBars(closes, rampInt(30000, 10000, 30))
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if res.TopDivergence || res.BottomDivergence {
		t.Errorf("expected no divergence mid-range, got %+v", res)
	}
	if !res.VolumeDeclining {
		t.Error("expected VolumeDeclining=true (precondition still holds)")
	}
}

func TestDetectVolumeDivergence_InsufficientBars(t *testing.T) {
	bars := mkBars(ramp(100, 110, 10), rampInt(1000, 1000, 10))
	if _, ok := DetectVolumeDivergence(bars, 30); ok {
		t.Fatal("expected ok=false with < 20 bars")
	}
}

func TestDetectVolumeDivergence_UnsortedInput(t *testing.T) {
	bars := mkBars(ramp(100, 130, 30), rampInt(30000, 10000, 30))
	// Reverse the slice — the function must sort a copy, not trust order.
	for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
		bars[i], bars[j] = bars[j], bars[i]
	}
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok || !res.TopDivergence {
		t.Fatalf("expected top divergence on reversed input, got ok=%v %+v", ok, res)
	}
}

func TestDetectVolumeDivergence_FlatPanel(t *testing.T) {
	flat := make([]float64, 30)
	for i := range flat {
		flat[i] = 100
	}
	bars := mkBars(flat, rampInt(30000, 10000, 30))
	if _, ok := DetectVolumeDivergence(bars, 30); ok {
		t.Fatal("expected ok=false on flat price panel (high == low)")
	}
}

func TestDetectVolumeDivergence_ZeroVolumePanel(t *testing.T) {
	vols := make([]int64, 30) // all zero
	bars := mkBars(ramp(100, 130, 30), vols)
	res, ok := DetectVolumeDivergence(bars, 30)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if res.VolumeDeclining || res.TopDivergence {
		t.Errorf("zero-volume panel must not report declining volume, got %+v", res)
	}
}

func TestDetectVolumeDivergence_DefaultWindowAndTruncation(t *testing.T) {
	// 60 bars, window 30: only the last 30 bars matter. Make the first 30
	// bars exceed the later range; with window=30 they must be ignored.
	closes := append(ramp(200, 150, 30), ramp(100, 130, 30)...)
	vols := append(rampInt(50000, 50000, 30), rampInt(30000, 10000, 30)...)
	bars := mkBars(closes, vols)
	res, ok := DetectVolumeDivergence(bars, 0) // 0 → default 30
	if !ok {
		t.Fatal("expected ok=true")
	}
	if res.WindowDays != DivergenceDefaultWindowDays {
		t.Errorf("expected default window %d, got %d", DivergenceDefaultWindowDays, res.WindowDays)
	}
	if res.WindowHigh > 130.01 {
		t.Errorf("window truncation failed: high %v includes pre-window bars", res.WindowHigh)
	}
	if !res.TopDivergence {
		t.Errorf("expected top divergence on truncated window, got %+v", res)
	}
}
