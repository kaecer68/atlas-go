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

func entryFull(taiex, sox, tsmadr, usdtwd, vol, foreignNet, futuresOI float64) SnapshotEntry {
	return SnapshotEntry{
		TAIEX:               taiex,
		SOX:                 sox,
		TSMADR:              tsmadr,
		USDTWD:              usdtwd,
		MarketVolume:        vol,
		ForeignInvestorNet:  foreignNet,
		ForeignFuturesOINet: futuresOI,
	}
}

func marginEntry(balance float64) MarginEntry {
	return MarginEntry{MarginBalance: balance}
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

// ── W1: Sparse history (input_available semantics) ──

func TestCalculator_SparseMarketVolume_ZeroWhenInsufficient(t *testing.T) {
	c := NewCalculator()
	// 20 entries total, but only 3 non-zero volume values
	entries := make([]SnapshotEntry, 20)
	for i := range 20 {
		if i%7 == 0 { // indices 0, 7, 14 = 3 non-zero
			entries[i] = entry(20000, 5000, 150, 32, 3000+float64(i)*50)
		} else {
			entries[i] = entry(20000, 5000, 150, 32, 0) // zero volume
		}
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// Non-zero count (3) < MinDaysMarketVolumeMA20 (20) → must stay 0
	if ind.MarketVolumeMA20 != 0 {
		t.Errorf("MarketVolumeMA20 should be 0 with only 3 non-zero points in 20-day window, got %v", ind.MarketVolumeMA20)
	}
}

func TestCalculator_SparseMarketVolume_ExactlyMinDays(t *testing.T) {
	c := NewCalculator()
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = 3000 + float64(i)*50
	}
	entries := makeEntries(vals, func(v float64) SnapshotEntry {
		return entry(20000, 5000, 150, 32, v)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// All 20 non-zero → should compute MA20 normally
	if ind.MarketVolumeMA20 == 0 {
		t.Errorf("MarketVolumeMA20 should be computed with 20 non-zero points, got 0")
	}
	expected := float64(3000+3950) / 2.0 // (first + last) / 2 for arithmetic progression
	if !floatEq(ind.MarketVolumeMA20, expected, 0.01) {
		t.Errorf("MarketVolumeMA20 = %v, want %v", ind.MarketVolumeMA20, expected)
	}
}

func TestCalculator_SparseTAIEX_ZeroWhenInsufficient(t *testing.T) {
	c := NewCalculator()
	// 20 entries, only 5 non-zero TAIEX values
	entries := make([]SnapshotEntry, 20)
	for i := range 20 {
		if i%4 == 0 { // indices 0,4,8,12,16 = 5 non-zero
			entries[i] = entry(20000+float64(i)*10, 5000, 150, 32, 3000)
		} else {
			entries[i] = entry(0, 5000, 150, 32, 3000) // zero TAIEX
		}
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// Non-zero count (5) < MinDaysTAIEXMA20 (20) → must stay 0
	if ind.TAIEXMA20 != 0 {
		t.Errorf("TAIEXMA20 should be 0 with only 5 non-zero points, got %v", ind.TAIEXMA20)
	}
	// TAIEXMA5 window is indices 15..19: only 1 non-zero (index 16) → < 5 → 0
	if ind.TAIEXMA5 != 0 {
		t.Errorf("TAIEXMA5 should be 0 with insufficient non-zero points, got %v", ind.TAIEXMA5)
	}
}

func TestCalculator_SparseTAIEX_ExactlyMinDays(t *testing.T) {
	c := NewCalculator()
	// 5 entries all non-zero
	entries := makeEntries([]float64{100, 102, 104, 106, 108}, func(v float64) SnapshotEntry {
		return entry(v, 0, 0, 0, 0)
	})
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)

	// All 5 non-zero → should compute MA5
	if ind.TAIEXMA5 == 0 {
		t.Errorf("TAIEXMA5 should be computed with 5 non-zero points, got 0")
	}
	expected := float64(100+102+104+106+108) / 5.0
	if !floatEq(ind.TAIEXMA5, expected, 0.01) {
		t.Errorf("TAIEXMA5 = %v, want %v", ind.TAIEXMA5, expected)
	}
}

// ── B5 Batch 2: Foreign capital ──

func TestCalculator_ForeignNet5DayAvg(t *testing.T) {
	c := NewCalculator()
	// 5 days: +100, -50, +200, -30, +80 → avg = 60
	entries := make([]SnapshotEntry, 5)
	vals := []float64{100, -50, 200, -30, 80}
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, v, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	if !floatEq(ind.ForeignNet5DayAvg, 60, 0.01) {
		t.Errorf("ForeignNet5DayAvg = %v, want 60", ind.ForeignNet5DayAvg)
	}
}

func TestCalculator_ForeignNet5DayAvg_Insufficient(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 4)
	for i := range 4 {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, 100, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	if ind.ForeignNet5DayAvg != 0 {
		t.Errorf("ForeignNet5DayAvg should be 0 with 4 entries, got %v", ind.ForeignNet5DayAvg)
	}
}

func TestCalculator_ForeignBuySellDays10(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 10)
	vals := []float64{100, -50, 200, -30, 80, -10, 0, 60, -90, 40}
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, v, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	if ind.ForeignBuyDays10 != 5 {
		t.Errorf("ForeignBuyDays10 = %d, want 5 (values >0: 100,200,80,60,40)", ind.ForeignBuyDays10)
	}
	if ind.ForeignSellDays10 != 4 {
		t.Errorf("ForeignSellDays10 = %d, want 4 (values <0: -50,-30,-10,-90)", ind.ForeignSellDays10)
	}
}

func TestCalculator_ForeignConsecDays(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 10)
	// Last 4 entries positive: 80, 60, 40, 20 → ConsecBuy = 4
	vals := []float64{-100, 50, -200, 30, -10, -5, 80, 60, 40, 20}
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, v, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	if ind.ForeignConsecBuyDays != 4 {
		t.Errorf("ForeignConsecBuyDays = %d, want 4", ind.ForeignConsecBuyDays)
	}
	if ind.ForeignConsecSellDays != 0 {
		t.Errorf("ForeignConsecSellDays = %d, want 0 (last is positive)", ind.ForeignConsecSellDays)
	}
}

func TestCalculator_ForeignConsecSellDays(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 10)
	// Last 5 entries negative: -80, -60, -40, -20, -10
	vals := []float64{100, -50, 200, -30, 9999, -80, -60, -40, -20, -10}
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, v, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	if ind.ForeignConsecSellDays != 5 {
		t.Errorf("ForeignConsecSellDays = %d, want 5", ind.ForeignConsecSellDays)
	}
	if ind.ForeignConsecBuyDays != 0 {
		t.Errorf("ForeignConsecBuyDays = %d, want 0 (last is negative)", ind.ForeignConsecBuyDays)
	}
}

func TestCalculator_ForeignNetPeakSell(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 10)
	vals := []float64{-100, 50, -400, 30, -200, -10, 80, 60, 40, 20}
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, v, 5000)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	// Most negative = -400
	if !floatEq(ind.ForeignNetPeakSell, -400, 0.01) {
		t.Errorf("ForeignNetPeakSell = %v, want -400", ind.ForeignNetPeakSell)
	}
}

// ── B5 Batch 2: Futures ──

func TestCalculator_FuturesOIPrev(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 2)
	entries[0] = entryFull(20000, 5000, 150, 32, 3000, 100, 5000)
	entries[1] = entryFull(20000, 5000, 150, 32, 3000, -50, 4800)
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	// Prev = entries[0].ForeignFuturesOINet = 5000
	if !floatEq(ind.ForeignFuturesOIPrev, 5000, 0.01) {
		t.Errorf("ForeignFuturesOIPrev = %v, want 5000", ind.ForeignFuturesOIPrev)
	}
}

func TestCalculator_FuturesOIDelta3(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 4)
	vals := []float64{5000, 5100, 5050, 5150} // +100, -50, +100 → net direction = 2 increases - 1 decrease = 1
	for i, v := range vals {
		entries[i] = entryFull(20000, 5000, 150, 32, 3000, 100, v)
	}
	ind := &PeriodIndicators{}
	c.Enrich(ind, entries)
	// delta: day2-day1=+100→+1, day3-day2=-50→-1, day4-day3=+100→+1 → total = 1
	if ind.ForeignFuturesOIDelta3 != 1 {
		t.Errorf("ForeignFuturesOIDelta3 = %d, want 1", ind.ForeignFuturesOIDelta3)
	}
}

// ── B5 Batch 2: Margin ──

func TestCalculator_MarginBalancePeak(t *testing.T) {
	c := NewCalculator()
	history := make([]MarginEntry, 30)
	for i := range 30 {
		history[i] = marginEntry(2000 + float64(i)*10) // peaks at 2290
	}
	ind := &PeriodIndicators{}
	c.EnrichMargin(ind, history)
	if !floatEq(ind.MarginBalancePeak, 2290, 0.01) {
		t.Errorf("MarginBalancePeak = %v, want 2290", ind.MarginBalancePeak)
	}
}

func TestCalculator_MarginBalanceChange5D(t *testing.T) {
	c := NewCalculator()
	history := make([]MarginEntry, 6)
	// Balance goes from 2000 to 2100 over 5 days → change = (2100-2000)/2000*100 = 5%
	history[0] = marginEntry(2000)
	history[1] = marginEntry(2020)
	history[2] = marginEntry(2040)
	history[3] = marginEntry(2060)
	history[4] = marginEntry(2080)
	history[5] = marginEntry(2100)
	ind := &PeriodIndicators{}
	c.EnrichMargin(ind, history)
	if !floatEq(ind.MarginBalanceChange5D, 5.0, 0.01) {
		t.Errorf("MarginBalanceChange5D = %v, want 5.0", ind.MarginBalanceChange5D)
	}
}

func TestCalculator_MarginInsufficientHistory_Peak(t *testing.T) {
	c := NewCalculator()
	history := make([]MarginEntry, 10) // only 10 entries, needs 30
	for i := range 10 {
		history[i] = marginEntry(2000 + float64(i)*10)
	}
	ind := &PeriodIndicators{}
	c.EnrichMargin(ind, history)
	if ind.MarginBalancePeak != 0 {
		t.Errorf("MarginBalancePeak should be 0 with only 10 entries, got %v", ind.MarginBalancePeak)
	}
}

func TestCalculator_ForeignDeterminism(t *testing.T) {
	c := NewCalculator()
	entries := make([]SnapshotEntry, 10)
	for i := range 10 {
		entries[i] = entryFull(20000+float64(i)*10, 5000, 150, 32, 3000, float64(100-i*10), 5000+float64(i)*10)
	}

	ind1 := &PeriodIndicators{}
	c.Enrich(ind1, entries)
	ind2 := &PeriodIndicators{}
	c.Enrich(ind2, entries)

	if ind1.ForeignNet5DayAvg != ind2.ForeignNet5DayAvg {
		t.Errorf("ForeignNet5DayAvg non-deterministic: %v vs %v", ind1.ForeignNet5DayAvg, ind2.ForeignNet5DayAvg)
	}
	if ind1.ForeignBuyDays10 != ind2.ForeignBuyDays10 {
		t.Errorf("ForeignBuyDays10 non-deterministic: %d vs %d", ind1.ForeignBuyDays10, ind2.ForeignBuyDays10)
	}
	if ind1.ForeignConsecBuyDays != ind2.ForeignConsecBuyDays {
		t.Errorf("ForeignConsecBuyDays non-deterministic: %d vs %d", ind1.ForeignConsecBuyDays, ind2.ForeignConsecBuyDays)
	}
}
