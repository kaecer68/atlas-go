package forecast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScore_BullishAligned(t *testing.T) {
	r := Score("20260716", Input{
		ForeignFuturesOIZ:  2.0,
		ForeignSpot5DSlope: 10.0,
		TSMADRChangePct:    1.5,
		SPXChangePct:       1.0,
		NDXChangePct:       1.5,
		USDTWDChangePct:    -0.3,
		VIX:                15,
	})
	if r.Direction != ForeignDirectionBullish {
		t.Errorf("expected bullish, got %s (prob=%.3f, score=%.3f)", r.Direction, r.Probability, r.Score)
	}
	if r.Probability < 0.6 {
		t.Errorf("prob=%.3f, want >= 0.6", r.Probability)
	}
}

func TestScore_BearishAligned(t *testing.T) {
	r := Score("20260716", Input{
		ForeignFuturesOIZ:  -2.5,
		ForeignSpot5DSlope: -12.0,
		TSMADRChangePct:    -2.0,
		SPXChangePct:       -2.0,
		NDXChangePct:       -2.5,
		USDTWDChangePct:    0.8,
		VIX:                30,
	})
	if r.Direction != ForeignDirectionBearish {
		t.Errorf("expected bearish, got %s (prob=%.3f)", r.Direction, r.Probability)
	}
}

func TestScore_NeutralWhenMixed(t *testing.T) {
	// Strong opposing signals cancel out — should land in the neutral band.
	r := Score("20260716", Input{
		ForeignFuturesOIZ:  1.5,  // bullish leading
		ForeignSpot5DSlope: -8.0, // bearish recent spot trend
		TSMADRChangePct:    -1.5, // bearish
		SPXChangePct:       0.5,
		NDXChangePct:       0.5,
		VIX:                22,
	})
	if r.Direction != ForeignDirectionNeutral {
		t.Errorf("expected neutral on cancelling signals, got %s (prob=%.3f)", r.Direction, r.Probability)
	}
}

func TestScore_NeutralAllZero(t *testing.T) {
	r := Score("20260716", Input{})
	if r.Direction != ForeignDirectionNeutral {
		t.Errorf("zero inputs should be neutral, got %s (prob=%.3f)", r.Direction, r.Probability)
	}
	if r.Probability != 0.5 {
		t.Errorf("zero-score probability should be exactly 0.5, got %.3f", r.Probability)
	}
}

func TestScore_ProbabilitiesBounded(t *testing.T) {
	cases := []Input{
		{ForeignFuturesOIZ: 99, ForeignSpot5DSlope: 99, TSMADRChangePct: 99, SPXChangePct: 99, NDXChangePct: 99, USDTWDChangePct: -99, VIX: 1},
		{ForeignFuturesOIZ: -99, ForeignSpot5DSlope: -99, TSMADRChangePct: -99, SPXChangePct: -99, NDXChangePct: -99, USDTWDChangePct: 99, VIX: 99},
		{},
	}
	for i, in := range cases {
		r := Score("20260716", in)
		if r.Probability < 0 || r.Probability > 1 {
			t.Errorf("case %d: prob=%.3f out of [0,1]", i, r.Probability)
		}
	}
}

func TestJudge(t *testing.T) {
	// Correct hit.
	prev := Record{Date: "20260715", Direction: ForeignDirectionBullish, Probability: 0.7}
	big := Judge(prev, 5_000_000_000)
	if big.ActualOutcome != ForeignDirectionBullish {
		t.Errorf("big buy should be bullish, got %s", big.ActualOutcome)
	}
	if big.Correct == nil || !*big.Correct {
		t.Error("bullish vs bullish should be correct")
	}

	// Direction mismatch: predicted bullish, realised small (neutral).
	small := Judge(prev, 500_000_000)
	if small.ActualOutcome != ForeignDirectionNeutral {
		t.Errorf("sub-threshold should be neutral, got %s", small.ActualOutcome)
	}
	if small.Correct == nil || *small.Correct {
		t.Error("bullish vs sub-threshold (neutral) must be incorrect — prediction was wrong")
	}

	// Wrong direction.
	prevBear := Record{Date: "20260715", Direction: ForeignDirectionBearish, Probability: 0.7}
	miss := Judge(prevBear, 4_000_000_000)
	if miss.Correct == nil || *miss.Correct {
		t.Error("bearish prediction vs bullish outcome should be incorrect")
	}

	// Both neutral → correct (no significant move either way).
	prevNeutral := Record{Date: "20260715", Direction: ForeignDirectionNeutral, Probability: 0.5}
	bothFlat := Judge(prevNeutral, 100_000_000)
	if bothFlat.Correct == nil || !*bothFlat.Correct {
		t.Error("neutral prediction vs sub-threshold actual must be correct")
	}
}

func TestCalibrate(t *testing.T) {
	// 60% hits over 100 samples → calibrated.
	recs := make([]Record, 0, 100)
	for i := range 100 {
		hit := i%10 < 6
		recs = append(recs, Record{Date: "20260716", Correct: &hit})
	}
	s := Calibrate(recs)
	if !s.Calibrated {
		t.Errorf("expected calibrated at 60%% over 100 samples, got %+v", s)
	}
	if s.Samples != 100 {
		t.Errorf("samples=%d", s.Samples)
	}

	// 40% hits → not calibrated.
	for i := range 100 {
		hit := i%10 < 4
		recs[i] = Record{Date: "20260716", Correct: &hit}
	}
	s2 := Calibrate(recs)
	if s2.Calibrated {
		t.Errorf("expected NOT calibrated at 40%%, got %+v", s2)
	}
	if s2.Reason == "" {
		t.Error("expected reason string")
	}

	// 30 samples (under min) → not calibrated.
	recs3 := make([]Record, 30)
	for i := range recs3 {
		h := true
		recs3[i] = Record{Date: "20260716", Correct: &h}
	}
	s3 := Calibrate(recs3)
	if s3.Calibrated {
		t.Errorf("expected NOT calibrated at 30 samples, got %+v", s3)
	}
	if !contains(s3.Reason, "90") {
		t.Errorf("reason should mention 90, got %s", s3.Reason)
	}
}

func TestLedger_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	if err := l.Write(Record{Date: "20260716", Direction: ForeignDirectionBearish, Probability: 0.7, Score: -0.4}); err != nil {
		t.Fatal(err)
	}
	if err := l.Write(Record{Date: "20260715", Direction: ForeignDirectionBullish, Probability: 0.65, Score: 0.3}); err != nil {
		t.Fatal(err)
	}
	recs, err := l.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0].Date != "20260715" || recs[1].Date != "20260716" {
		t.Errorf("ordering wrong: %s, %s", recs[0].Date, recs[1].Date)
	}

	// Overwrite
	if err := l.Write(Record{Date: "20260716", Direction: ForeignDirectionNeutral, Probability: 0.5, Score: 0}); err != nil {
		t.Fatal(err)
	}
	r, err := l.Load("20260716")
	if err != nil {
		t.Fatal(err)
	}
	if r.Direction != ForeignDirectionNeutral {
		t.Errorf("overwrite failed: %s", r.Direction)
	}
}

func TestLedger_ListEmptyDir(t *testing.T) {
	l := NewLedger(filepath.Join(t.TempDir(), "nonexistent"))
	recs, err := l.List(0)
	if err != nil {
		t.Fatalf("missing dir should be silent: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected no records, got %d", len(recs))
	}
	// Verify os import is used elsewhere if needed; this test ensures the
	// tempDir + nonexistent path is exercised.
	_ = os.MkdirAll
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
