package domain

import "testing"

func TestScreeningCriteria_MicrostructureFilters(t *testing.T) {
	minLiq := 30.0
	maxSpread := 0.01
	maxVol := 0.05
	excludeAbnormal := true

	sc := ScreeningCriteria{
		MinLiquidityScore:     &minLiq,
		MaxSpreadEstimate:     &maxSpread,
		MaxRealizedVolatility: &maxVol,
		ExcludeAbnormalVolume: &excludeAbnormal,
	}

	if !sc.HasFilters() {
		t.Error("expected HasFilters to be true")
	}
}

func TestScreeningCriteria_NoMicrostructureFilters(t *testing.T) {
	sc := ScreeningCriteria{}
	if sc.HasFilters() {
		t.Error("expected HasFilters to be false for empty criteria")
	}
}
