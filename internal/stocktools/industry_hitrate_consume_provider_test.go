package stocktools

import (
	"context"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

type fakeIndustryWinRateProvider struct {
	report stockpicker.IndustryWinRateReport
	found  bool
	err    error
	calls  int
	last   [3]string // source, window, regime
}

func (f *fakeIndustryWinRateProvider) LoadIndustryWinRate(_ context.Context, source, window, regime string) (stockpicker.IndustryWinRateReport, bool, error) {
	f.calls++
	f.last = [3]string{source, window, regime}
	return f.report, f.found, f.err
}

func TestSectorAllocationHitRateProvider_MapsCanonicalRows(t *testing.T) {
	inner := &fakeIndustryWinRateProvider{
		found: true,
		report: stockpicker.IndustryWinRateReport{
			Source: "stockpicker-momentum-20d-positive",
			Industries: []stockpicker.IndustryWinRateSummary{
				{
					IndustryID:        "semiconductor",
					Direction:         "buy",
					Observations:      140,
					WinRate:           0.62,
					WilsonLower:       0.53,
					WilsonUpper:       0.70,
					CalibrationStatus: string(stockpicker.CalibrationEligible),
				},
			},
		},
	}
	provider := NewSectorAllocationHitRateProvider(inner)

	report, err := provider.LoadIndustryWinRate("stockpicker-momentum-20d-positive", "momentum-20d-positive", "120d")

	if err != nil {
		t.Fatalf("LoadIndustryWinRate: %v", err)
	}
	if inner.calls != 1 || inner.last[0] != "stockpicker-momentum-20d-positive" || inner.last[1] != "120d" || inner.last[2] != "" {
		t.Fatalf("inner call = %v (calls=%d), want the source/window passed through and the all-regimes stratum", inner.last, inner.calls)
	}
	if len(report.Industries) != 1 {
		t.Fatalf("rows = %+v, want 1", report.Industries)
	}
	row := report.Industries[0]
	if row.IndustryID != "semiconductor" || row.CalibrationStatus != sectorallocation.IndustryHitRateCalibrationEligible {
		t.Errorf("row = %+v, want the canonical id and calibration status preserved", row)
	}
	if row.WilsonLower != 0.53 || row.Observations != 140 {
		t.Errorf("row stats = %+v, want WilsonLower 0.53 / observations 140", row)
	}
}

// TestSectorAllocationHitRateProvider_NotFoundIsEmptyReport: "no outcomes in the
// window" is a normal answer, not an error (the consumer fails closed on the
// empty report).
func TestSectorAllocationHitRateProvider_NotFoundIsEmptyReport(t *testing.T) {
	inner := &fakeIndustryWinRateProvider{found: false}
	provider := NewSectorAllocationHitRateProvider(inner)

	report, err := provider.LoadIndustryWinRate("src", "cond", "120d")

	if err != nil {
		t.Fatalf("not-found must not error: %v", err)
	}
	if len(report.Industries) != 0 {
		t.Fatalf("rows = %+v, want none", report.Industries)
	}
}

func TestSectorAllocationHitRateProvider_ErrorIsWrapped(t *testing.T) {
	wantErr := errors.New("ledger closed")
	provider := NewSectorAllocationHitRateProvider(&fakeIndustryWinRateProvider{err: wantErr})

	_, err := provider.LoadIndustryWinRate("src", "cond", "120d")

	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
	}
}

func TestSectorAllocationHitRateProvider_NilInnerFailsClosed(t *testing.T) {
	provider := NewSectorAllocationHitRateProvider(nil)

	report, err := provider.LoadIndustryWinRate("src", "cond", "120d")

	if err != nil {
		t.Fatalf("nil inner must not error: %v", err)
	}
	if len(report.Industries) != 0 {
		t.Fatalf("rows = %+v, want none", report.Industries)
	}
}

func TestSectorAllocationHitRateProvider_SatisfiesPort(t *testing.T) {
	var _ sectorallocation.IndustryHitRateProvider = NewSectorAllocationHitRateProvider(nil)
}
