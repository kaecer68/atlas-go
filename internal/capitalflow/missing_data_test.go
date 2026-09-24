package capitalflow

// Issue #1940 R1/R3 regression tests: CF-INV-06 input hygiene (missing data
// is never stored or read as 0) and the Z-score degeneracy guard.

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// govSamples builds n consecutive zero-valued government samples, the shape
// found in data/state/capital_flow_rolling.json (2026-07-22 .. 2026-08-14).
func zeroSamples(n int, dim ForceName) []RollingSample {
	dates := []string{
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-27",
		"2026-07-28", "2026-07-29", "2026-07-30", "2026-07-31", "2026-08-03",
		"2026-08-04", "2026-08-05", "2026-08-06", "2026-08-07", "2026-08-10",
		"2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14",
	}
	out := make([]RollingSample, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, RollingSample{
			TradingDate: dates[i], Dimension: dim, RawValue: 0,
			Unit: "twd", SourceID: SourceGovernmentOperator,
		})
	}
	return out
}

func forceOf(forces []ForceScore, dim ForceName) ForceScore {
	for _, f := range forces {
		if f.Force == dim {
			return f
		}
	}
	return ForceScore{}
}

// TestZScoreFromSamples_DegenerateWindowIsZero is the R1/R3 core: a window
// that cannot standardize the value must yield z = 0, not a fake epsilon
// division.
func TestZScoreFromSamples_DegenerateWindowIsZero(t *testing.T) {
	t.Run("government 18 identical zeros (production 2026-08-27)", func(t *testing.T) {
		got := zScoreFromSamples(zeroSamples(18, ForceGovernment), -70.2846)
		if got != 0 {
			t.Errorf("z = %v, want 0 (pre-fix: -7028.5 from the max(0.01,std) clamp)", got)
		}
	})
	t.Run("futures two identical values (production 2026-07-21)", func(t *testing.T) {
		samples := []RollingSample{
			{TradingDate: "2026-07-17", Dimension: ForceFutures, RawValue: -86189, Unit: "contracts", SourceID: SourceTAIFEXInst},
			{TradingDate: "2026-07-20", Dimension: ForceFutures, RawValue: -86189, Unit: "contracts", SourceID: SourceTAIFEXInst},
		}
		got := zScoreFromSamples(samples, -78337)
		if got != 0 {
			t.Errorf("z = %v, want 0 (pre-fix: 785200 from the max(0.01,std) clamp)", got)
		}
	})
	t.Run("near-constant window", func(t *testing.T) {
		samples := []RollingSample{
			{TradingDate: "2026-09-01", Dimension: ForceRetail, RawValue: 100},
			{TradingDate: "2026-09-02", Dimension: ForceRetail, RawValue: 100.000001},
		}
		if got := zScoreFromSamples(samples, 105); got != 0 {
			t.Errorf("z = %v, want 0 (dispersion below the relative floor)", got)
		}
	})
	t.Run("fewer than two samples stays 0", func(t *testing.T) {
		if got := zScoreFromSamples(zeroSamples(1, ForceGovernment), 5); got != 0 {
			t.Errorf("z = %v, want 0", got)
		}
	})
}

// TestZScoreFromSamples_ClampsExtremeValues locks the backstop: a usable
// window can no longer emit an unbounded |z|.
func TestZScoreFromSamples_ClampsExtremeValues(t *testing.T) {
	// Retail keeps its zeros in the window (a flat percentage is data), so
	// this window is usable: mean 1, population std 1, raw 100 => z = 99.
	var samples []RollingSample
	for i := 0; i < 252; i++ {
		v := 0.0
		if i%2 == 0 {
			v = 2.0
		}
		samples = append(samples, RollingSample{
			TradingDate: dateFromIndex(i), Dimension: ForceRetail, RawValue: v,
		})
	}
	if degenerateReferenceWindow(samples, 100) {
		t.Fatal("window with real dispersion must not be reported as degenerate")
	}
	got := zScoreFromSamples(samples, 100)
	if got != maxAbsZScore {
		t.Errorf("z = %v, want the %v cap (uncapped z would be 99)", got, maxAbsZScore)
	}
	if neg := zScoreFromSamples(samples, -100); neg != -maxAbsZScore {
		t.Errorf("z = %v, want -%v", neg, maxAbsZScore)
	}
}

// dateFromIndex returns a synthetic ascending YYYY-MM-DD key.
func dateFromIndex(i int) string {
	day := i%28 + 1
	month := i/28%12 + 1
	return "2026-" + twoDigits(month) + "-" + twoDigits(day)
}

func twoDigits(v int) string {
	if v < 10 {
		return "0" + string(rune('0'+v))
	}
	return string(rune('0'+v/10)) + string(rune('0'+v%10))
}

// TestScore_LegacyZeroWindowIsUncalibrated covers the R1 window shape: once
// the legacy zeros are dropped the window holds no real observation, so the
// dimension is uncalibrated (spec §8.4) rather than "degraded" — and its
// Z is 0 instead of -7028.5.
func TestScore_LegacyZeroWindowIsUncalibrated(t *testing.T) {
	extractor := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		GovernmentNet: pointOn("GOV_FLOW_NET", -70.2846, "20260827"),
	}
	forces := extractor.Score(snap, "2026-08-27", map[ForceName][]RollingSample{
		ForceGovernment: zeroSamples(18, ForceGovernment),
	})
	gov := forceOf(forces, ForceGovernment)
	if gov.CalibrationStatus != CalibrationCalibrating {
		t.Errorf("CalibrationStatus = %q, want %q (no usable observation left in the window)", gov.CalibrationStatus, CalibrationCalibrating)
	}
	if gov.ZScore != 0 {
		t.Errorf("ZScore = %v, want 0", gov.ZScore)
	}
	if gov.Trend != "neutral" {
		t.Errorf("Trend = %q, want neutral", gov.Trend)
	}
	if gov.SampleCount != 0 {
		t.Errorf("SampleCount = %d, want 0 (all 18 legacy zeros are missing values, spec §8.3)", gov.SampleCount)
	}
}

// TestScore_MarksDegenerateWindowDegraded ties a degenerate *usable* window
// to the reported calibration status: z = 0 must not be read as "the market
// is calm" when the window simply has no dispersion.
func TestScore_MarksDegenerateWindowDegraded(t *testing.T) {
	extractor := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignFuturesOINet: pointOn("TX_FOREIGN_OI_NET", -78337, "20260721"),
	}
	forces := extractor.Score(snap, "2026-07-21", map[ForceName][]RollingSample{
		ForceFutures: {
			{TradingDate: "2026-07-17", Dimension: ForceFutures, RawValue: -86189, Unit: "contracts", SourceID: SourceTAIFEXInst},
			{TradingDate: "2026-07-20", Dimension: ForceFutures, RawValue: -86189, Unit: "contracts", SourceID: SourceTAIFEXInst},
		},
	})
	fut := forceOf(forces, ForceFutures)
	if fut.CalibrationStatus != CalibrationDegraded {
		t.Errorf("CalibrationStatus = %q, want %q", fut.CalibrationStatus, CalibrationDegraded)
	}
	if fut.ZScore != 0 {
		t.Errorf("ZScore = %v, want 0 (pre-fix: 785200)", fut.ZScore)
	}
	if fut.Trend != "neutral" {
		t.Errorf("Trend = %q, want neutral", fut.Trend)
	}
	if fut.SampleCount != 2 {
		t.Errorf("SampleCount = %d, want 2", fut.SampleCount)
	}
}

// TestScore_HealthyWindowStaysEligible guards against over-triggering the
// degraded label.
func TestScore_HealthyWindowStaysEligible(t *testing.T) {
	extractor := NewForceExtractor()
	var history []RollingSample
	for i := 0; i < 40; i++ {
		raw := 10.0 + float64(i%7)
		history = append(history, RollingSample{
			TradingDate: dateFromIndex(i), Dimension: ForceGovernment, RawValue: raw,
		})
	}
	snap := marketdata.MacroDataSnapshot{GovernmentNet: pointOn("GOV_FLOW_NET", 25.5, "20260827")}
	gov := forceOf(extractor.Score(snap, "2026-08-27", map[ForceName][]RollingSample{ForceGovernment: history}), ForceGovernment)
	if gov.CalibrationStatus != CalibrationEligible {
		t.Errorf("CalibrationStatus = %q, want %q", gov.CalibrationStatus, CalibrationEligible)
	}
	if gov.SampleCount != 40 {
		t.Errorf("SampleCount = %d, want 40", gov.SampleCount)
	}
}

// TestReferenceWindowFor_DropsLegacyMissingValues locks spec §8.3 on the
// read path: persisted zero-valued samples of a net-flow dimension must not
// enter the reference window, while ratio dimensions keep theirs.
func TestReferenceWindowFor_DropsLegacyMissingValues(t *testing.T) {
	mixed := []RollingSample{
		{TradingDate: "2025-09-17", Dimension: ForceInstitutional, RawValue: 0},
		{TradingDate: "2025-09-18", Dimension: ForceInstitutional, RawValue: 0},
		{TradingDate: "2026-05-14", Dimension: ForceInstitutional, RawValue: 0.001},
	}
	got := referenceWindowFor(mixed, ForceInstitutional)
	if len(got) != 1 || got[0].RawValue != 0.001 {
		t.Errorf("referenceWindowFor(institutional) = %v, want only the non-zero sample", got)
	}
	if len(referenceWindowFor(mixed, ForceRetail)) != 3 {
		t.Error("ratio dimensions (retail) must keep zero-valued samples: a flat percentage is data")
	}
	if len(referenceWindowFor(mixed, ForceTSMADR)) != 3 {
		t.Error("ratio dimensions (tsm_adr) must keep zero-valued samples")
	}
}

// TestScore_InstitutionalLegacyZeroWindow is the production regression: 148
// consecutive institutional zeros made a real reading score z = -38.67.
// With the zeros filtered the window is too short to standardize, so the
// honest answer is 0 / calibrating.
func TestScore_InstitutionalLegacyZeroWindow(t *testing.T) {
	var history []RollingSample
	for i := 0; i < 148; i++ {
		history = append(history, RollingSample{TradingDate: dateFromIndex(i), Dimension: ForceInstitutional, RawValue: 0})
	}
	history = append(history, RollingSample{TradingDate: "2026-05-14", Dimension: ForceInstitutional, RawValue: 0.001})

	snap := marketdata.MacroDataSnapshot{DomesticFundNet: marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: -0.3867209}}
	inst := forceOf(NewForceExtractor().Score(snap, "2026-05-15", map[ForceName][]RollingSample{ForceInstitutional: history}), ForceInstitutional)
	if inst.ZScore != 0 {
		t.Errorf("institutional ZScore = %v, want 0 (pre-fix: -38.67)", inst.ZScore)
	}
	if inst.SampleCount != 1 {
		t.Errorf("institutional SampleCount = %d, want 1", inst.SampleCount)
	}
	if inst.CalibrationStatus != CalibrationCalibrating {
		t.Errorf("CalibrationStatus = %q, want %q", inst.CalibrationStatus, CalibrationCalibrating)
	}
}

// TestScore_ZeroValuedNetFlowIsUnavailable locks the R1 write gate: a
// present-but-zero reading from a net-flow dimension is missing data, so no
// sample may be produced for it.
func TestScore_ZeroValuedNetFlowIsUnavailable(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 0},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 0},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: 0},
		ForeignFuturesOINet: marketdata.MacroDataPoint{
			Symbol: "TX_FOREIGN_OI_NET", Value: 0, Timestamp: pointOn("x", 0, "20260827").Timestamp,
		},
		GovernmentNet:       pointOn("GOV_FLOW_NET", 0, "20260827"),
		RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "RetailMarginBalance", ChangePct: 0},
		RetailShortBalance:  marketdata.MacroDataPoint{Symbol: "RetailShortBalance", ChangePct: 0},
		TSMADR:              marketdata.MacroDataPoint{Symbol: "TSMADR", ChangePct: 0},
	}
	forces := NewForceExtractor().Score(snap, "2026-08-27", nil)
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer, ForceGovernment, ForceFutures} {
		f := forceOf(forces, dim)
		if f.DataAvailable {
			t.Errorf("%s: DataAvailable = true for a zero-valued reading, want false (CF-INV-06)", dim)
		}
		if f.LeadingZ != 0 {
			t.Errorf("%s: LeadingZ = %v, want 0 (the futures reading is not usable)", dim, f.LeadingZ)
		}
	}
	// Ratio dimensions may legitimately be exactly zero.
	for _, dim := range []ForceName{ForceRetail, ForceTSMADR} {
		if f := forceOf(forces, dim); !f.DataAvailable {
			t.Errorf("%s: DataAvailable = false for a legitimate flat reading, want true", dim)
		}
	}
}

// TestRefresh_ZeroValuedReadingsNeverReachTheStore is the end-to-end R1
// regression on the write path.
func TestRefresh_ZeroValuedReadingsNeverReachTheStore(t *testing.T) {
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         taipeiInstant("2026-07-29", "09:00"),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 0},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
		GovernmentNet:      pointOn("GOV_FLOW_NET", 0, "20260728"),
	}}
	store := NewMemoryRollingSampleStore(252)
	svc := NewServiceWithStore(provider, 0, store, nil)
	ctx := context.Background()

	if err := svc.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	for _, dim := range []ForceName{ForceGovernment, ForceInstitutional} {
		samples, err := store.History(ctx, dim, "2099-12-31", 10)
		if err != nil {
			t.Fatalf("History(%s): %v", dim, err)
		}
		if len(samples) != 0 {
			t.Errorf("%s persisted %v, want no samples (zero-valued reading is missing data, CF-INV-06)", dim, samples)
		}
	}
	foreign, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(foreign): %v", err)
	}
	if len(foreign) != 1 {
		t.Errorf("foreign persisted %d samples, want 1 (non-zero readings are unaffected)", len(foreign))
	}
}
