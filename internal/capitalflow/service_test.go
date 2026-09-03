package capitalflow

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type stubProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return s.snap, s.err
}

func TestService_LatestDaily_AssemblesReport(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	svc := NewService(provider, 0, nil)
	report, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	if report.Date.Unix() != recordedAt {
		t.Errorf("Date = %v, want %v", report.Date.Unix(), recordedAt)
	}
	if len(report.Forces) == 0 {
		t.Errorf("Forces empty after Extract")
	}
	if report.Resonance.Direction == "" {
		t.Errorf("Resonance.Direction empty after ComputeResonance")
	}
}

func TestService_LatestDaily_ProviderError(t *testing.T) {
	provider := &stubProvider{err: context.DeadlineExceeded}
	svc := NewService(provider, 0, nil)
	if _, err := svc.LatestDaily(context.Background()); err == nil {
		t.Errorf("expected error from provider")
	}
}

func TestService_Summary_DerivesFromLatestDaily(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	svc := NewService(provider, 0, nil)
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Date.Unix() != recordedAt {
		t.Errorf("Date = %v, want %v", summary.Date.Unix(), recordedAt)
	}
	if summary.QualityLabel == "" {
		t.Errorf("QualityLabel empty after GenerateSummaryReport")
	}
	// M6 (audit): with an empty rolling store every Z is pinned to 0, so no
	// actor/signal reading exists — the summary must NOT fabricate a
	// DominantForce (the pre-M6 ForceRetail fallback made this non-empty).
	if summary.DominantForce != "" {
		t.Errorf("DominantForce = %q on a zero-history snapshot; M6 removed the fabricated retail fallback", summary.DominantForce)
	}
	if summary.DominantActor != "" || summary.DominantSignal != "" {
		t.Errorf("DominantActor/DominantSignal = %q/%q on a zero-history snapshot, want empty/empty", summary.DominantActor, summary.DominantSignal)
	}
	if summary.ResonanceDir == "" {
		t.Errorf("ResonanceDir empty after GenerateSummaryReport")
	}
}

func TestService_Summary_PropagatesProviderError(t *testing.T) {
	provider := &stubProvider{err: context.DeadlineExceeded}
	svc := NewService(provider, 0, nil)
	_, err := svc.Summary(context.Background())
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Summary error does not wrap context.DeadlineExceeded: %v", err)
	}
}

func TestService_Summary_SharesForcesWithDailyReport(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 200},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -100},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 50},
	}}
	svc := NewService(provider, 0, nil)
	daily, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !daily.Date.Equal(summary.Date) {
		t.Errorf("Date mismatch: daily=%v summary=%v", daily.Date, summary.Date)
	}
	if daily.Resonance.Direction != summary.ResonanceDir {
		t.Errorf("ResonanceDir mismatch: daily=%q summary=%q",
			daily.Resonance.Direction, summary.ResonanceDir)
	}
	if daily.QualityScore != summary.QualityScore {
		t.Errorf("QualityScore mismatch: daily=%v summary=%v",
			daily.QualityScore, summary.QualityScore)
	}
}

// TestService_LatestDailyIsIdempotent verifies spec §8.1 / CF-INV-04:
// calling LatestDaily twice on the same backing snapshot must
// return deeply equal DailyReports, and the rolling store must
// observe zero UpsertDay calls — read paths must be pure reads.
//
// The test uses the store-aware constructor so it can plug in
// stubRollingStore (defined in rolling_store_test.go) and assert
// that LatestDaily never writes to the persistent window.
func TestService_LatestDailyIsIdempotent(t *testing.T) {
	recordedAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := &stubRollingStore{}
	svc := NewServiceWithStore(provider, 0, store, nil)

	first, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("first LatestDaily: %v", err)
	}
	second, err := svc.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("second LatestDaily: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two LatestDaily calls returned different reports:\nfirst=%+v\nsecond=%+v", first, second)
	}

	// Spec §8.1: HTTP/MCP/UI/recommender reads must not mutate the
	// rolling window. The stub counts UpsertDay calls so we can
	// prove LatestDaily never wrote to the store.
	if store.upsertCalls != 0 {
		t.Errorf("LatestDaily called UpsertDay %d times, want 0 (read path must be pure)", store.upsertCalls)
	}
	if got := len(store.samples); got != 0 {
		t.Errorf("LatestDaily persisted %d samples, want 0 (read path must be pure)", got)
	}
}

// TestService_RefreshSameDayDoesNotGrowWindow verifies spec §8.2 /
// CF-INV-05: refreshing the same trading date more than once must
// not grow the rolling sample count. Each (dimension, trading_date)
// pair is collapsed to a single sample via the store's
// last-write-wins rule (BK-15 / spec §8.4).
//
// The store is consulted after each Refresh with a strictly-after
// `beforeDate` so the persisted sample is visible — History uses
// TradingDate < beforeDate, not <=.
func TestService_RefreshSameDayDoesNotGrowWindow(t *testing.T) {
	tradingDate := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         tradingDate.Add(9 * time.Hour).Unix(),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	ctx := context.Background()

	for i := range 3 {
		if err := svc.Refresh(ctx); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
	}

	nextDay := tradingDate.AddDate(0, 0, 1).Format("2006-01-02")
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, err := store.History(ctx, dim, nextDay, 60)
		if err != nil {
			t.Fatalf("History %s: %v", dim, err)
		}
		if len(got) != 1 {
			t.Errorf("%s window has %d samples after 3 Refresh calls, want 1 (CF-INV-05 last-write-wins)", dim, len(got))
		}
	}
}

func TestRefresh_KeyMatchesRecordedAt(t *testing.T) {
	recordedAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	want := time.Unix(recordedAt, 0).In(time.FixedZone("Asia/Taipei", 8*3600)).Format("2006-01-02")
	got, err := store.History(context.Background(), ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 1 || got[0].TradingDate != want {
		t.Errorf("sample date = %v, want %v (data-driven keying CF-INV-15)", got, want)
	}
}

func TestRefresh_SkipOnWeekend(t *testing.T) {
	saturday := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         saturday.Unix(),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	cal := industry.NewEventCalendarWithProvider(nil)
	svc := NewServiceWithStore(provider, 0, store, cal)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh on Saturday should skip-and-log, got err: %v", err)
	}
	got, err := store.History(context.Background(), ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no samples on weekend, got %d (CF-INV-16)", len(got))
	}
}

func TestRefresh_IdempotentSameDay(t *testing.T) {
	recordedAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	for i := range 3 {
		if err := svc.Refresh(context.Background()); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
	}
	nextDay := time.Unix(recordedAt, 0).AddDate(0, 0, 1).Format("2006-01-02")
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, err := store.History(context.Background(), dim, nextDay, 60)
		if err != nil {
			t.Fatalf("History %s: %v", dim, err)
		}
		if len(got) != 1 {
			t.Errorf("%s window has %d samples after 3 Refresh, want 1 (CF-INV-05)", dim, len(got))
		}
	}
}

func TestRefresh_TimezoneOffset(t *testing.T) {
	recordedAt := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	want := "2026-07-16"
	got, err := store.History(context.Background(), ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 1 || got[0].TradingDate != want {
		t.Errorf("UTC 16:00 should map to Taipei next day: got %v, want %s", got, want)
	}
}

// TestDeriveTradingDate_TaipeiBoundary locks audit M4: the read path
// trading-date derivation must use Asia/Taipei (same clock Refresh uses
// to key persisted samples). The canonical regression is a snapshot
// recorded 2026-09-03T23:30:00Z — Taipei 2026-09-04 07:30 — which a UTC
// derivation would mislabel as 2026-09-03.
func TestDeriveTradingDate_TaipeiBoundary(t *testing.T) {
	cases := []struct {
		name string
		utc  time.Time
		want string
	}{
		{"taipei_morning_before_8am", time.Date(2026, 9, 3, 23, 30, 0, 0, time.UTC), "2026-09-04"},
		{"taipei_noon_same_utc_day", time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC), "2026-09-04"},
		{"utc_evening_rolls_taipei_day", time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC), "2026-07-16"},
		{"taipei_late_night_same_day", time.Date(2026, 9, 4, 15, 59, 59, 0, time.UTC), "2026-09-04"},
		{"utc_epoch_zero", time.Unix(0, 0), "1970-01-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveTradingDate(c.utc.Unix()); got != c.want {
				t.Errorf("deriveTradingDate(%v) = %q, want %q (Asia/Taipei)", c.utc, got, c.want)
			}
		})
	}
}

// TestService_ReadPathDateKeyAlignsWithRefreshWrite locks audit M4
// end-to-end: Refresh persists the snapshot under its Taipei trading
// date, and LatestDaily must read with the same Taipei as-of date so
// the (write key, read upper bound, force AsOfTradingDate) all agree.
// Before the fix, the UTC-derived read date was one day behind the
// Taipei write key for Taiwan mornings before 08:00.
func TestService_ReadPathDateKeyAlignsWithRefreshWrite(t *testing.T) {
	recordedAt := time.Date(2026, 9, 3, 23, 30, 0, 0, time.UTC).Unix() // Taipei 09-04 07:30
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:         recordedAt,
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -50},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	ctx := context.Background()

	if err := svc.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	samples, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(samples) != 1 || samples[0].TradingDate != "2026-09-04" {
		t.Fatalf("Refresh wrote samples %v, want TradingDate=2026-09-04 (Taipei)", samples)
	}

	report, err := svc.LatestDaily(ctx)
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	asOf := ""
	for _, f := range report.Forces {
		if f.Force == ForceForeign {
			asOf = f.AsOfTradingDate
		}
	}
	if asOf != "2026-09-04" {
		t.Errorf("LatestDaily force AsOfTradingDate = %q, want %q (must match Refresh write key)", asOf, "2026-09-04")
	}
}

// TestRefresh_PersistsProvenanceUnits locks audit M3 on the write path:
// every RollingSample Refresh persists must carry exactly the
// (unit, source_id) of ComputeForceProvenance — the single provenance
// table — for all seven dimensions.
func TestRefresh_PersistsProvenanceUnits(t *testing.T) {
	recordedAt := time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC).Unix()
	provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
		RecordedAt:          recordedAt,
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 100},
		DomesticFundNet:     marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 20},
		DealerNet:           marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -30},
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "ForeignFuturesOINet", Value: -8000},
		TSMADR:              marketdata.MacroDataPoint{Symbol: "TSMADR", ChangePct: 1.2},
		GovernmentNet:       marketdata.MacroDataPoint{Symbol: "GovernmentNet", Value: 5},
		RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "RetailMarginBalance", ChangePct: -0.5},
		RetailShortBalance:  marketdata.MacroDataPoint{Symbol: "RetailShortBalance", ChangePct: 0.3},
	}}
	store := NewMemoryRollingSampleStore(60)
	svc := NewServiceWithStore(provider, 0, store, nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	dims := []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	}
	for _, dim := range dims {
		samples, err := store.History(context.Background(), dim, "2099-12-31", 10)
		if err != nil {
			t.Fatalf("History(%s): %v", dim, err)
		}
		if len(samples) != 1 {
			t.Fatalf("History(%s) len = %d, want 1", dim, len(samples))
		}
		prov := ComputeForceProvenance(dim)
		if samples[0].Unit != prov.Unit || samples[0].SourceID != prov.SourceID {
			t.Errorf("%s sample provenance = (%q, %q), want (%q, %q) from ComputeForceProvenance",
				dim, samples[0].Unit, samples[0].SourceID, prov.Unit, prov.SourceID)
		}
	}
}
