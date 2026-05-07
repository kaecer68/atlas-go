package portfolio

import (
	"testing"
	"time"
)

func TestStyleRotationDetector_Basic(t *testing.T) {
	d := NewStyleRotationDetector()
	if leader := d.GetCurrentLeader(); leader != StyleGrowth {
		t.Errorf("initial leader = %s, want growth", leader)
	}
	d.SetParameters(0.10, 10)
	d.UpdateStyleReturn(StyleGrowth, 0.02)
	mom := d.GetStyleMomentum(StyleGrowth)
	if mom.Return20D <= 0 {
		t.Errorf("Return20D = %f, expected positive", mom.Return20D)
	}
	momentums := d.GetAllMomentums()
	if len(momentums) != 4 {
		t.Errorf("got %d momentums, want 4", len(momentums))
	}
	d.Reset()
	if leader := d.GetCurrentLeader(); leader != StyleGrowth {
		t.Errorf("after reset leader = %s", leader)
	}
}

func TestRotationSignal_IsValid(t *testing.T) {
	var nilSig *RotationSignal
	if nilSig.IsValid() {
		t.Error("nil signal should be invalid")
	}
	if (&RotationSignal{Diff: 0}).IsValid() {
		t.Error("zero diff invalid")
	}
	if !(&RotationSignal{Diff: 0.01}).IsValid() {
		t.Error("positive diff valid")
	}
}

func TestStyleRotationStrategy_Basic(t *testing.T) {
	s := NewStyleRotationStrategy()
	if s.GetDetector() == nil {
		t.Error("GetDetector nil")
	}
	if s.GetAllocator() == nil {
		t.Error("GetAllocator nil")
	}
	result := s.UpdateAndRotate(map[Style]float64{StyleGrowth: 0.01, StyleValue: 0.01})
	if result != nil {
		t.Error("should be nil when equal")
	}
	stats := s.GetStats()
	if stats.CurrentLeader == "" {
		t.Error("leader empty")
	}
}

func TestCalculateStyleExposures(t *testing.T) {
	pos := []PositionWithStyle{
		{Symbol: "2330.TW", Value: 500, Styles: []Style{StyleGrowth, StyleQuality}},
		{Symbol: "2317.TW", Value: 500, Styles: []Style{StyleValue}},
	}
	exp := CalculateStyleExposures(pos, 1000)
	if len(exp.Current) == 0 {
		t.Error("expected non-empty")
	}
}

func TestRotationSignal_AllFields(t *testing.T) {
	now := time.Now()
	sig := RotationSignal{FromStyle: StyleValue, ToStyle: StyleGrowth, Strength: 0.5, Diff: 0.3, Timestamp: now}
	if sig.FromStyle != StyleValue || sig.ToStyle != StyleGrowth {
		t.Error("fields mismatch")
	}
}
