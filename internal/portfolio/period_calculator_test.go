package portfolio

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// helpers to build test entries quickly
func entry(taiex, sox, tsmadr, usdtwd, vol float64) SnapshotEntry {
	return SnapshotEntry{
		TAIEX:        taiex,
		SOX:          sox,
		TSMADR:       tsmadr,
		USDTWD:       usdtwd,
		MarketVolume: vol,
	}
}

func makeEntries(vals []float64, fn func(v float64) SnapshotEntry) []SnapshotEntry {
	out := make([]SnapshotEntry, len(vals))
	for i, v := range vals {
		out[i] = fn(v)
	}
	return out
}

func floatEq(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

// ── TAIEX MA ──

func TestCalculator_TAIEXMA5_Exact5Days(t *testing.T) {
	c := NewCalculator()
	entries := makeEntries([]float64{100, 102, 104, 106, 108}, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// (100+102+104+106+108)/5 = 104
	if !floatEq(ind.TAIEXMA5, 104, 0.01) {
		t.Errorf("TAIEXMA5 = %v, want 104", ind.TAIEXMA5)
	}
	if ind.TAIEXMA20 != 0 {
		t.Errorf("TAIEXMA20 should be 0 with only 5 days, got %v", ind.TAIEXMA20)
	}
}

func TestCalculator_TAIEXMA5_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	entries := makeEntries([]float64{100, 102, 104, 106}, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.TAIEXMA5 != 0 {
		t.Errorf("TAIEXMA5 should be 0 with only 4 days, got %v", ind.TAIEXMA5)
	}
}

func TestCalculator_TAIEXMA20_Exact20Days(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = float64(20000 + i*10) // 20000,20010,...,20190
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// sum of 20000..20190 = 20*(20000+20190)/2 = 20*20095 = 401900
	// avg = 401900/20 = 20095
	if !floatEq(ind.TAIEXMA20, 20095, 0.01) {
		t.Errorf("TAIEXMA20 = %v, want 20095", ind.TAIEXMA20)
	}
}

// ── SOX MA ──

func TestCalculator_SOXMA20_Exact20Days(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = float64(5000 + i*5) // 5000,5005,...,5095
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, v, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// sum = 20*(5000+5095)/2 = 20*5047.5 = 100950
	// avg = 100950/20 = 5047.5
	if !floatEq(ind.SOXMA20, 5047.5, 0.01) {
		t.Errorf("SOXMA20 = %v, want 5047.5", ind.SOXMA20)
	}
	if ind.SOXMA50 != 0 {
		t.Errorf("SOXMA50 should be 0 with only 20 days")
	}
}

func TestCalculator_SOXMA50_Exact50Days(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 50)
	for i := range vals {
		vals[i] = float64(5500 + i) // 5500..5549
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, v, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// sum = 50*(5500+5549)/2 = 50*5524.5 = 276225
	// avg = 276225/50 = 5524.5
	if !floatEq(ind.SOXMA50, 5524.5, 0.01) {
		t.Errorf("SOXMA50 = %v, want 5524.5", ind.SOXMA50)
	}
}

func TestCalculator_SOXMA50_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = float64(5000 + i)
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, v, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.SOXMA50 != 0 {
		t.Errorf("SOXMA50 should be 0 with 40 days, got %v", ind.SOXMA50)
	}
}

// ── TSM ADR 5-day high ──

func TestCalculator_TSMADRHigh5(t *testing.T) {
	c := NewCalculator()
	entries := []SnapshotEntry{
		entry(0, 0, 150, 0, 0),
		entry(0, 0, 155, 0, 0),
		entry(0, 0, 148, 0, 0),
		entry(0, 0, 160, 0, 0),
		entry(0, 0, 152, 0, 0),
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if !floatEq(ind.TSMADRHigh5, 160, 0.01) {
		t.Errorf("TSMADRHigh5 = %v, want 160", ind.TSMADRHigh5)
	}
}

func TestCalculator_TSMADRHigh5_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	entries := []SnapshotEntry{
		entry(0, 0, 150, 0, 0),
		entry(0, 0, 155, 0, 0),
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.TSMADRHigh5 != 0 {
		t.Errorf("TSMADRHigh5 should be 0 with only 2 days, got %v", ind.TSMADRHigh5)
	}
}

// ── Market Volume ──

func TestCalculator_MarketVolumeMA20(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = float64(3000 + i*50) // 3000,3050,...,3950
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, 0, 0, 0, v)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// sum = 20*(3000+3950)/2 = 20*3475 = 69500
	// avg = 69500/20 = 3475
	if !floatEq(ind.MarketVolumeMA20, 3475, 0.01) {
		t.Errorf("MarketVolumeMA20 = %v, want 3475", ind.MarketVolumeMA20)
	}
}

func TestCalculator_MarketVolumeMA20_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 15)
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, 0, 0, 0, v)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.MarketVolumeMA20 != 0 {
		t.Errorf("MarketVolumeMA20 should be 0 with 15 days, got %v", ind.MarketVolumeMA20)
	}
}

// ── TWD ──

func TestCalculator_TWDMA20(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = 32.0 + float64(i)*0.01 // 32.00,32.01,...,32.19
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, 0, 0, v, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// sum = 20*(32.00+32.19)/2 = 20*32.095 = 641.9
	// avg = 641.9/20 = 32.095
	if !floatEq(ind.TWDMA20, 32.095, 0.001) {
		t.Errorf("TWDMA20 = %v, want 32.095", ind.TWDMA20)
	}
}

func TestCalculator_TWDMA20_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 10)
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(0, 0, 0, v, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.TWDMA20 != 0 {
		t.Errorf("TWDMA20 should be 0 with 10 days, got %v", ind.TWDMA20)
	}
}

func TestCalculator_TWDChangePct(t *testing.T) {
	c := NewCalculator()
	// 6 days of USD/TWD: 31.80, 32.00, 32.10, 31.90, 32.20, 32.05
	entries := []SnapshotEntry{
		entry(0, 0, 0, 31.80, 0),
		entry(0, 0, 0, 32.00, 0),
		entry(0, 0, 0, 32.10, 0),
		entry(0, 0, 0, 31.90, 0),
		entry(0, 0, 0, 32.20, 0),
		entry(0, 0, 0, 32.05, 0),
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// TWDChange1D: (32.05 - 32.20) / 32.20 * 100 = -0.466% (appreciation)
	if !floatEq(ind.TWDChange1D, -0.4658, 0.01) {
		t.Errorf("TWDChange1D = %v, want ~-0.466", ind.TWDChange1D)
	}
	// TWDChange3D: (32.05 - 32.10) / 32.10 * 100 = -0.156%
	if !floatEq(ind.TWDChange3D, -0.1558, 0.01) {
		t.Errorf("TWDChange3D = %v, want ~-0.156", ind.TWDChange3D)
	}
	// TWDChange5D: (32.05 - 31.80) / 31.80 * 100 = +0.786% (depreciation)
	if !floatEq(ind.TWDChange5D, 0.7862, 0.01) {
		t.Errorf("TWDChange5D = %v, want ~0.786", ind.TWDChange5D)
	}
}

func TestCalculator_TWDChangePct_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	entries := []SnapshotEntry{
		entry(0, 0, 0, 32.00, 0),
		entry(0, 0, 0, 32.05, 0),
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// TWDChange1D should be filled (2 days)
	if !floatEq(ind.TWDChange1D, (32.05-32.00)/32.00*100, 0.01) {
		t.Errorf("TWDChange1D should be computed with 2 days")
	}
	// TWDChange3D should be 0 (only 2 days)
	if ind.TWDChange3D != 0 {
		t.Errorf("TWDChange3D should be 0 with only 2 days")
	}
	// TWDChange5D should be 0
	if ind.TWDChange5D != 0 {
		t.Errorf("TWDChange5D should be 0 with only 2 days")
	}
}

// ── TAIEX MA20 Slope ──

func TestCalculator_TAIEXMA20Slope_Uptrend(t *testing.T) {
	c := NewCalculator()
	// 24 entries with rising TAIEX: 19000, 19010, ..., 19230
	vals := make([]float64, 24)
	for i := range vals {
		vals[i] = float64(19000 + i*10)
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.TAIEXMA20Slope <= 0 {
		t.Errorf("TAIEXMA20Slope should be positive for uptrend, got %v", ind.TAIEXMA20Slope)
	}
}

func TestCalculator_TAIEXMA20Slope_Flat(t *testing.T) {
	c := NewCalculator()
	// 24 entries with constant TAIEX
	vals := make([]float64, 24)
	for i := range vals {
		vals[i] = 20000
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if !floatEq(ind.TAIEXMA20Slope, 0, 0.01) {
		t.Errorf("TAIEXMA20Slope should be 0 for flat trend, got %v", ind.TAIEXMA20Slope)
	}
}

func TestCalculator_TAIEXMA20Slope_InsufficientHistory(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = float64(20000 + i*10)
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	if ind.TAIEXMA20Slope != 0 {
		t.Errorf("TAIEXMA20Slope should be 0 with only 20 days (need 24), got %v", ind.TAIEXMA20Slope)
	}
}

// ── EntriesFromSnapshots ──

func TestEntriesFromSnapshots(t *testing.T) {
	snaps := []marketdata.MacroDataSnapshot{
		{
			TAIEX:        marketdata.MacroDataPoint{Value: 23000},
			SOXIndex:     marketdata.MacroDataPoint{Value: 5500},
			TSMADR:       marketdata.MacroDataPoint{Value: 160},
			USD_TWD:      marketdata.MacroDataPoint{Value: 32.1},
			MarketVolume: marketdata.MacroDataPoint{Value: 3500},
		},
	}
	entries := EntriesFromSnapshots(snaps)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.TAIEX != 23000 || e.SOX != 5500 || e.TSMADR != 160 || e.USDTWD != 32.1 || e.MarketVolume != 3500 {
		t.Errorf("entries = %+v, want all fields mapped", e)
	}
}

// ── Zero values with zero snapshots ──

func TestCalculator_ZeroSnapshots(t *testing.T) {
	c := NewCalculator()
	ind := &PeriodIndicators{}
	c.Enrich(ind, nil)
	// All computed fields should remain zero
	if ind.TAIEXMA5 != 0 || ind.TAIEXMA20 != 0 || ind.TAIEXMA20Slope != 0 ||
		ind.SOXMA50 != 0 || ind.SOXMA20 != 0 || ind.TSMADRHigh5 != 0 ||
		ind.MarketVolumeMA20 != 0 || ind.TWDMA20 != 0 ||
		ind.TWDChange1D != 0 || ind.TWDChange3D != 0 || ind.TWDChange5D != 0 {
		t.Error("all computed fields should be 0 with no history")
	}
}

// ── Determinism ──

func TestCalculator_Determinism(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 50)
	for i := range vals {
		vals[i] = float64(20000 + i*10)
	}
	entryBuilder := func(idx int, taiexVal float64) SnapshotEntry {
		return entry(taiexVal, 5500+float64(idx)*5, 150+float64(idx), 32.0+float64(idx)*0.01, 3000+float64(idx)*50)
	}
	entries := make([]SnapshotEntry, 50)
	for idx := range 50 {
		entries[idx] = entryBuilder(idx, float64(20000+idx*10))
	}

	result1 := &PeriodIndicators{}
	c.Enrich(result1, entries)
	result2 := &PeriodIndicators{}
	c.Enrich(result2, entries)
	result3 := &PeriodIndicators{}
	c.Enrich(result3, entries)

	fields1 := []float64{result1.TAIEXMA5, result1.TAIEXMA20, result1.TAIEXMA20Slope,
		result1.SOXMA50, result1.SOXMA20, result1.TSMADRHigh5,
		result1.MarketVolumeMA20, result1.TWDMA20,
		result1.TWDChange1D, result1.TWDChange3D, result1.TWDChange5D}
	fields2 := []float64{result2.TAIEXMA5, result2.TAIEXMA20, result2.TAIEXMA20Slope,
		result2.SOXMA50, result2.SOXMA20, result2.TSMADRHigh5,
		result2.MarketVolumeMA20, result2.TWDMA20,
		result2.TWDChange1D, result2.TWDChange3D, result2.TWDChange5D}
	fields3 := []float64{result3.TAIEXMA5, result3.TAIEXMA20, result3.TAIEXMA20Slope,
		result3.SOXMA50, result3.SOXMA20, result3.TSMADRHigh5,
		result3.MarketVolumeMA20, result3.TWDMA20,
		result3.TWDChange1D, result3.TWDChange3D, result3.TWDChange5D}

	for i := range fields1 {
		if fields1[i] != fields2[i] || fields2[i] != fields3[i] {
			t.Errorf("non-deterministic result at field index %d: %v, %v, %v", i, fields1[i], fields2[i], fields3[i])
		}
	}
}
