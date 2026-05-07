package experiment

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestMaxDrawdown_Helpers(t *testing.T) {
	if dd := maxDrawdown(nil); dd != 0 {
		t.Errorf("nil: got %f, want 0", dd)
	}
	if dd := maxDrawdown([]float64{}); dd != 0 {
		t.Errorf("empty: got %f, want 0", dd)
	}
	dd := maxDrawdown([]float64{0.01, -0.05, 0.02, -0.10, 0.03})
	if dd >= 0 {
		t.Errorf("negative returns: got %f, expected negative", dd)
	}
	dd = maxDrawdown([]float64{0.01, 0.02, 0.03})
	if math.Abs(dd) > 0.0001 {
		t.Errorf("positive returns: got %f, want ~0", dd)
	}
}

func TestCalculateVolatility_Helpers(t *testing.T) {
	if v := calculateVolatility(nil); v != 0 {
		t.Errorf("nil: got %f, want 0", v)
	}
	if v := calculateVolatility([]float64{0.01}); v != 0 {
		t.Errorf("single: got %f, want 0", v)
	}
	v := calculateVolatility([]float64{0.01, -0.02, 0.03, -0.01, 0.005})
	if v <= 0 {
		t.Errorf("expected positive volatility, got %f", v)
	}
}

func TestJudge_WithMethods(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir)
	j := NewJudge(store, dir, dir)
	if r := j.WithEventBus(nil); r == nil {
		t.Fatal("WithEventBus returned nil")
	}
	if r := j.WithParameters(nil); r == nil {
		t.Fatal("WithParameters returned nil")
	}
}
