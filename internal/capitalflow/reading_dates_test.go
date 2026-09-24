package capitalflow

// Issue #1940 R2 regression tests: the value/date pairing of dated input
// channels (government 官股行庫 file, TAIFEX futures OI session).

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// pointOn builds a snapshot point the way the gateway does: Timestamp is
// midnight UTC of the reading's own date (time.Parse("20060102", date)).
func pointOn(symbol string, value float64, date string) marketdata.MacroDataPoint {
	ts, err := time.Parse("20060102", date)
	if err != nil {
		panic(err)
	}
	return marketdata.MacroDataPoint{Symbol: symbol, Value: value, Timestamp: ts.Unix()}
}

func taipeiInstant(date, hhmm string) int64 {
	ts, err := time.Parse("2006-01-02 15:04", date+" "+hhmm)
	if err != nil {
		panic(err)
	}
	return ts.Add(-8 * time.Hour).Unix() // same wall clock in Asia/Taipei
}

// TestDimensionSampleDate_UsesReadingOwnDate locks the pairing rule: a
// dated dimension is keyed by the date its reading describes, never by the
// refresh run's trading day.
func TestDimensionSampleDate_UsesReadingOwnDate(t *testing.T) {
	cases := []struct {
		name        string
		snap        marketdata.MacroDataSnapshot
		dim         ForceName
		tradingDate string
		want        string
	}{
		{
			// The issue's canonical case: on 2026-09-07 the newest government
			// file is 20260904.json, so the reading belongs to 09-04.
			name:        "government file published next business day",
			snap:        marketdata.MacroDataSnapshot{GovernmentNet: pointOn("GOV_FLOW_NET", -64.4126, "20260904")},
			dim:         ForceGovernment,
			tradingDate: "2026-09-07",
			want:        "2026-09-04",
		},
		{
			// Pre-publication read: TAIFEX still serves the previous session.
			name:        "futures session lags one day",
			snap:        marketdata.MacroDataSnapshot{ForeignFuturesOINet: pointOn("TX_FOREIGN_OI_NET", -86189, "20260717")},
			dim:         ForceFutures,
			tradingDate: "2026-07-20",
			want:        "2026-07-17",
		},
		{
			name:        "government reading on the same day",
			snap:        marketdata.MacroDataSnapshot{GovernmentNet: pointOn("GOV_FLOW_NET", 147.0597, "20260902")},
			dim:         ForceGovernment,
			tradingDate: "2026-09-02",
			want:        "2026-09-02",
		},
		{
			// Same-day feeds (T86 / MI_MARGN) carry no meaningful session
			// stamp of their own; they stay on the trading day.
			name:        "undated dimension keeps the trading day",
			snap:        marketdata.MacroDataSnapshot{ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100, Timestamp: taipeiInstant("2026-09-07", "09:00")}},
			dim:         ForceForeign,
			tradingDate: "2026-09-07",
			want:        "2026-09-07",
		},
		{
			name:        "missing timestamp falls back to the trading day",
			snap:        marketdata.MacroDataSnapshot{GovernmentNet: marketdata.MacroDataPoint{Symbol: "GOV_FLOW_NET", Value: 5}},
			dim:         ForceGovernment,
			tradingDate: "2026-09-07",
			want:        "2026-09-07",
		},
		{
			// Defensive: a reading stamped after the report's trading day
			// must not create a future-dated sample.
			name:        "reading dated after the trading day is clamped",
			snap:        marketdata.MacroDataSnapshot{GovernmentNet: pointOn("GOV_FLOW_NET", 5, "20260908")},
			dim:         ForceGovernment,
			tradingDate: "2026-09-07",
			want:        "2026-09-07",
		},
		{
			// A zero-valued government reading is not usable data (R1), so
			// it has no reading date either.
			name:        "zero government reading has no reading date",
			snap:        marketdata.MacroDataSnapshot{GovernmentNet: pointOn("GOV_FLOW_NET", 0, "20260728")},
			dim:         ForceGovernment,
			tradingDate: "2026-09-07",
			want:        "2026-09-07",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dimensionSampleDate(c.snap, c.dim, c.tradingDate); got != c.want {
				t.Errorf("dimensionSampleDate(%s) = %q, want %q", c.dim, got, c.want)
			}
		})
	}
}

// TestRefresh_PersistsDatedReadingUnderItsOwnDate is the R2 write-path
// regression: refreshing on 2026-09-07 with the 20260904.json reading must
// persist a (government, 2026-09-04) sample, not (government, 2026-09-07).
func TestRefresh_PersistsDatedReadingUnderItsOwnDate(t *testing.T) {
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:          taipeiInstant("2026-09-07", "15:10"),
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		DomesticFundNet:     marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 30},
		DealerNet:           marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
		ForeignFuturesOINet: pointOn("TX_FOREIGN_OI_NET", -82389, "20260904"),
		GovernmentNet:       pointOn("GOV_FLOW_NET", -64.4126, "20260904"),
	}}
	store := NewMemoryRollingSampleStore(252)
	svc := NewServiceWithStore(provider, 0, store, nil)
	ctx := context.Background()

	if err := svc.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	samples, err := store.History(ctx, ForceGovernment, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(government): %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("government samples = %v, want exactly 1", samples)
	}
	if samples[0].TradingDate != "2026-09-04" {
		t.Errorf("government sample date = %q, want %q (the file's own date, not the refresh day)",
			samples[0].TradingDate, "2026-09-04")
	}
	if samples[0].RawValue != -64.4126 {
		t.Errorf("government sample value = %v, want -64.4126", samples[0].RawValue)
	}

	futures, err := store.History(ctx, ForceFutures, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(futures): %v", err)
	}
	if len(futures) != 1 || futures[0].TradingDate != "2026-09-04" {
		t.Errorf("futures samples = %v, want one sample dated 2026-09-04", futures)
	}

	// Same-day feeds keep the trading day.
	foreign, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(foreign): %v", err)
	}
	if len(foreign) != 1 || foreign[0].TradingDate != "2026-09-07" {
		t.Errorf("foreign samples = %v, want one sample dated 2026-09-07", foreign)
	}
}

// TestRefresh_StaleReadingDoesNotFabricateDailySamples is the other half of
// R1's mechanism: while the producer publishes nothing new, the same file is
// re-read every 5 minutes. Date-keyed attribution collapses those reads into
// ONE sample (CF-INV-05) instead of one sample per calendar day.
func TestRefresh_StaleReadingDoesNotFabricateDailySamples(t *testing.T) {
	base := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 30},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
		GovernmentNet:      pointOn("GOV_FLOW_NET", -64.4126, "20260728"),
	}
	store := NewMemoryRollingSampleStore(252)
	ctx := context.Background()

	// Three consecutive trading days with no new government file.
	for _, day := range []string{"2026-07-29", "2026-07-30", "2026-07-31"} {
		snap := base
		snap.RecordedAt = taipeiInstant(day, "09:00")
		svc := NewServiceWithStore(&stubProvider{snap: snap}, 0, store, nil)
		if err := svc.Refresh(ctx); err != nil {
			t.Fatalf("Refresh(%s): %v", day, err)
		}
	}

	gov, err := store.History(ctx, ForceGovernment, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(government): %v", err)
	}
	if len(gov) != 1 || gov[0].TradingDate != "2026-07-28" {
		t.Errorf("government samples after 3 refreshes of one stale file = %v, want exactly one sample dated 2026-07-28", gov)
	}
	// The same-day dimensions still advance one sample per trading day.
	foreign, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History(foreign): %v", err)
	}
	if len(foreign) != 3 {
		t.Errorf("foreign samples = %d, want 3 (one per trading day)", len(foreign))
	}
}

// TestLatestDaily_DatedReadingNeverEntersItsOwnWindow locks spec §8.4 for
// dated dimensions: the reference window ends strictly before the READING's
// date, so the reading cannot standardize itself.
func TestLatestDaily_DatedReadingNeverEntersItsOwnWindow(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryRollingSampleStore(252)
	if err := store.ImportHistory(ctx, []RollingSample{
		{TradingDate: "2026-08-27", Dimension: ForceGovernment, RawValue: -103.8497, Unit: "twd", SourceID: SourceGovernmentOperator},
		{TradingDate: "2026-09-04", Dimension: ForceGovernment, RawValue: -64.4126, Unit: "twd", SourceID: SourceGovernmentOperator},
	}); err != nil {
		t.Fatalf("ImportHistory: %v", err)
	}

	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         taipeiInstant("2026-09-08", "09:00"),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 30},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
		GovernmentNet:      pointOn("GOV_FLOW_NET", -64.4126, "20260904"),
	}}
	svc := NewServiceWithStore(provider, 0, store, nil)

	report, err := svc.LatestDaily(ctx)
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	var gov ForceScore
	for _, f := range report.Forces {
		if f.Force == ForceGovernment {
			gov = f
		}
	}
	if gov.AsOfTradingDate != "2026-09-04" {
		t.Errorf("government AsOfTradingDate = %q, want 2026-09-04", gov.AsOfTradingDate)
	}
	if gov.SampleCount != 1 {
		t.Errorf("government SampleCount = %d, want 1 (the 2026-09-04 sample is the value's own date and must be excluded from its window)", gov.SampleCount)
	}
	if !gov.DataAvailable {
		t.Error("government reading should be available (a non-zero file reading exists)")
	}
}

// TestLatestDaily_UndatedDimensionsKeepTradingDayWindow guards the other
// direction: the per-dimension window must not shrink for same-day feeds.
func TestLatestDaily_UndatedDimensionsKeepTradingDayWindow(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryRollingSampleStore(252)
	var seeded []RollingSample
	for i, d := range []string{"2026-09-01", "2026-09-02", "2026-09-03"} {
		seeded = append(seeded, RollingSample{
			TradingDate: d, Dimension: ForceForeign,
			RawValue: float64(100 + i), Unit: "hundred_million_shares", SourceID: SourceTWSET86,
		})
	}
	if err := store.ImportHistory(ctx, seeded); err != nil {
		t.Fatalf("ImportHistory: %v", err)
	}
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         taipeiInstant("2026-09-04", "09:00"),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 30},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
	}}
	svc := NewServiceWithStore(provider, 0, store, nil)
	report, err := svc.LatestDaily(ctx)
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	for _, f := range report.Forces {
		if f.Force != ForceForeign {
			continue
		}
		if f.AsOfTradingDate != "2026-09-04" {
			t.Errorf("foreign AsOfTradingDate = %q, want 2026-09-04", f.AsOfTradingDate)
		}
		if f.SampleCount != 3 {
			t.Errorf("foreign SampleCount = %d, want 3", f.SampleCount)
		}
	}
}

// TestDimensionReadingDate_TaipeiMidnightStamp pins the timestamp
// convention tolerance: both a UTC-midnight stamp and an Asia/Taipei
// midnight stamp resolve to the same calendar date.
func TestDimensionReadingDate_TaipeiMidnightStamp(t *testing.T) {
	taipeiMidnight := time.Date(2026, 9, 4, 0, 0, 0, 0, taipeiZone)
	snap := marketdata.MacroDataSnapshot{GovernmentNet: marketdata.MacroDataPoint{
		Symbol: "GOV_FLOW_NET", Value: 5, Timestamp: taipeiMidnight.Unix(),
	}}
	if got := dimensionSampleDate(snap, ForceGovernment, "2026-09-07"); got != "2026-09-04" {
		t.Errorf("dimensionSampleDate(Taipei-midnight stamp) = %q, want 2026-09-04", got)
	}
}
