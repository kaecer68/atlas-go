package orchestrator

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestMLScorer_TrainSkipsGapMultiple locks k3 decision 3-1 in the ML training
// set: a corrupted stored multiple (FU-20260930-03: 5904.TW stored at 720 for a
// session whose official close was 72) is made of two usable prices, so the
// price guard passed and the +800% label was fitted as if it were a real
// forward return. The feature row for that sample was perfectly normal, so
// nothing else in the pipeline could have caught it.
func TestMLScorer_TrainSkipsGapMultiple(t *testing.T) {
	bars := []domain.DailyBar{
		makeDailyBar("2330.TW", "2026-01-05", 100, 102, 99, 101, 1000),
		makeDailyBar("2330.TW", "2026-01-06", 101, 103, 100, 102, 1000),
		makeDailyBar("2330.TW", "2026-01-07", 102, 104, 101, 103, 1000),
		makeDailyBar("2330.TW", "2026-01-08", 103, 105, 102, 104, 1000),
		makeDailyBar("2330.TW", "2026-01-09", 104, 106, 103, 105, 1000),
		makeDailyBar("2330.TW", "2026-01-12", 105, 107, 104, 106, 1000),
		makeDailyBar("2330.TW", "2026-01-13", 106, 108, 105, 107, 1000),
		makeDailyBar("2330.TW", "2026-01-14", 1070, 1072, 1069, 1070, 1000), // corrupt 10x row
		makeDailyBar("2330.TW", "2026-01-15", 107, 109, 106, 108, 1000),
	}

	m := &capturingModel{}
	s := NewMLScorer(m)
	if err := s.Train(bars); err != nil {
		t.Fatalf("Train: %v", err)
	}

	// The corrupted bar poisons the pairs (01-13 -> 01-14) and (01-14 ->
	// 01-15); the remaining 6 pairs are usable.
	if len(m.Y) != 6 {
		t.Fatalf("trained on %d samples, want 6 (every pair touching the corrupted bar must be skipped)", len(m.Y))
	}
	if len(m.X) != len(m.Y) {
		t.Fatalf("feature/label length mismatch: %d features vs %d labels", len(m.X), len(m.Y))
	}
	for i, y := range m.Y {
		if math.Abs(y) > 0.5 || math.IsNaN(y) || math.IsInf(y, 0) {
			t.Errorf("label[%d] = %v: a corrupted multiple became a training label", i, y)
		}
	}

	// Control: without the corrupted bar every pair is used and the labels stay
	// exact.
	clean := make([]domain.DailyBar, 0, len(bars)-1)
	clean = append(clean, bars[:7]...)
	clean = append(clean, bars[8:]...)
	m2 := &capturingModel{}
	if err := NewMLScorer(m2).Train(clean); err != nil {
		t.Fatalf("Train (clean): %v", err)
	}
	if len(m2.Y) != 7 {
		t.Fatalf("clean data trained on %d samples, want 7 (normal data must not change)", len(m2.Y))
	}
	if want := 108.0/107.0 - 1; math.Abs(m2.Y[6]-want) > 1e-12 {
		t.Errorf("clean label[6] = %v, want %v", m2.Y[6], want)
	}
}
