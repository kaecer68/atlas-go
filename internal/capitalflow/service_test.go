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
	if summary.DominantForce == "" {
		t.Errorf("DominantForce empty after GenerateSummaryReport")
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
