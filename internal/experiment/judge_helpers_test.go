package experiment

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

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
	store := ledger.NewStore(dir).(ledger.ExperimentStore)
	j := NewJudge(store, dir, dir)
	if r := j.WithEventBus(nil); r == nil {
		t.Fatal("WithEventBus returned nil")
	}
	if r := j.WithParameters(nil); r == nil {
		t.Fatal("WithParameters returned nil")
	}
}
