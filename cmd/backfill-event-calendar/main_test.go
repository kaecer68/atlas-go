package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ---------------------------------------------------------------------------
// fake store
// ---------------------------------------------------------------------------

// fakeStore implements ledger.HistoricalStore with recording upserts.
type fakeStore struct {
	ledger.HistoricalStore
	eventRows []ledger.EventCalendarRow
	err       error
}

func (f *fakeStore) UpsertEventCalendar(_ context.Context, row ledger.EventCalendarRow) error {
	if f.err != nil {
		return f.err
	}
	f.eventRows = append(f.eventRows, row)
	return nil
}

// ---------------------------------------------------------------------------
// stub providers
// ---------------------------------------------------------------------------

type stubProvider struct {
	name   string
	events map[int][]marketdata.CalendarProviderData
	err    map[int]error
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) FetchEvents(_ context.Context, year int) ([]marketdata.CalendarProviderData, error) {
	if s.err != nil && s.err[year] != nil {
		return nil, s.err[year]
	}
	return s.events[year], nil
}

func fixedProviderFactory(np namedProvider) providerFactory {
	return func(_ runConfig, _ int) []namedProvider { return []namedProvider{np} }
}

// sampleProviderEvents returns 3 events across 2023-2024 for the given name.
func sampleProviderEvents(name string) map[int][]marketdata.CalendarProviderData {
	return map[int][]marketdata.CalendarProviderData{
		2023: {
			{Date: "2023-06-13", EventType: "ex_dividend", Name: "台積電 除權息", Symbol: "2330", Direction: "mixed", Weight: 0.4, Description: "現金除息", Source: name},
			{Date: "2023-05-22", EventType: "shareholder_meeting", Name: "台積電 股東會", Symbol: "2330", Direction: "bullish", Weight: 0.25, Description: "股東會", Source: name},
		},
		2024: {
			{Date: "2024-06-13", EventType: "ex_dividend", Name: "台積電 除權息", Symbol: "2330", Direction: "mixed", Weight: 0.4, Description: "現金除息", Source: name},
			{Date: "2024-08-30", EventType: "msci_rebalance", Name: "MSCI 季度調整 2024-08-30", Symbol: "", Direction: "mixed", Weight: 0.9, Description: "MSCI 調整", Source: name},
		},
	}
}

// ---------------------------------------------------------------------------
// unit tests
// ---------------------------------------------------------------------------

func TestCollectYears(t *testing.T) {
	got := collectYears(2023, 2026)
	want := []int{2023, 2024, 2025, 2026}
	if len(got) != len(want) {
		t.Fatalf("collectYears len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("years[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if got := collectYears(2024, 2024); len(got) != 1 || got[0] != 2024 {
		t.Errorf("single-year range = %v, want [2024]", got)
	}
}

func TestToEventRow(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	// symbol event → id includes symbol (date,event_id) uniqueness
	row, ok := toEventRow("twse_openapi", marketdata.CalendarProviderData{
		Date: "2026-06-13", EventType: "ex_dividend", Symbol: "2330",
	}, now)
	if !ok {
		t.Fatal("expected ok=true for valid date")
	}
	if row.EventID != "twse_openapi_backfill_ex_dividend_2026-06-13_2330" {
		t.Errorf("event id = %q", row.EventID)
	}
	if row.IsSynthetic != 1 {
		t.Errorf("IsSynthetic = %d, want 1 (backfill marker)", row.IsSynthetic)
	}
	if row.Source != "twse_openapi" {
		t.Errorf("source = %q, want twse_openapi", row.Source)
	}
	if row.ActiveTheme != "ex_dividend" {
		t.Errorf("active_theme = %q, want ex_dividend", row.ActiveTheme)
	}

	// market-wide event (no symbol) → id without symbol
	row2, ok := toEventRow("msci_static", marketdata.CalendarProviderData{
		Date: "2026-08-31", EventType: "msci_rebalance",
	}, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if row2.EventID != "msci_static_backfill_msci_rebalance_2026-08-31" {
		t.Errorf("event id = %q", row2.EventID)
	}

	// invalid date → ok=false
	if _, ok := toEventRow("x", marketdata.CalendarProviderData{Date: "not-a-date"}, now); ok {
		t.Error("expected ok=false for invalid date")
	}
}

func TestRunDryRun(t *testing.T) {
	np := namedProvider{name: "stub", provider: &stubProvider{
		name:   "stub",
		events: sampleProviderEvents("stub"),
	}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2024,
		dryRun:    true,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.eventsFetched["stub"] != 4 {
		t.Errorf("fetched = %d, want 4", stats.eventsFetched["stub"])
	}
	if stats.eventsWritten["stub"] != 0 {
		t.Errorf("written = %d, want 0 in dry-run", stats.eventsWritten["stub"])
	}
	if stats.eventsSkipped["stub"] != 0 {
		t.Errorf("skipped = %d, want 0", stats.eventsSkipped["stub"])
	}
}

func TestRunWritesToStore(t *testing.T) {
	store := &fakeStore{}
	np := namedProvider{name: "stub", provider: &stubProvider{
		name:   "stub",
		events: sampleProviderEvents("stub"),
	}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2024,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.eventsWritten["stub"] != 4 {
		t.Errorf("written = %d, want 4", stats.eventsWritten["stub"])
	}
	if len(store.eventRows) != 4 {
		t.Fatalf("store rows = %d, want 4", len(store.eventRows))
	}
	for _, row := range store.eventRows {
		if row.IsSynthetic != 1 {
			t.Errorf("row %s IsSynthetic = %d, want 1", row.EventID, row.IsSynthetic)
		}
		if row.Source != "stub" {
			t.Errorf("row %s source = %q, want stub", row.EventID, row.Source)
		}
		if row.CapturedAt.IsZero() {
			t.Errorf("row %s captured_at is zero", row.EventID)
		}
	}
}

func TestRunSkipsDuplicateEvents(t *testing.T) {
	store := &fakeStore{}
	// A provider returning the same event twice (e.g. overlapping API rows)
	// must write only one row — dedup key is (date, event_id).
	events := map[int][]marketdata.CalendarProviderData{
		2024: {
			{Date: "2024-08-30", EventType: "msci_rebalance", Name: "MSCI", Source: "stub"},
			{Date: "2024-08-30", EventType: "msci_rebalance", Name: "MSCI", Source: "stub"},
		},
	}
	np := namedProvider{name: "stub", provider: &stubProvider{name: "stub", events: events}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2024,
		endYear:   2024,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.eventsWritten["stub"] != 1 {
		t.Errorf("written = %d, want 1", stats.eventsWritten["stub"])
	}
	if stats.eventsSkipped["stub"] != 1 {
		t.Errorf("skipped = %d, want 1", stats.eventsSkipped["stub"])
	}
	if len(store.eventRows) != 1 {
		t.Errorf("store rows = %d, want 1", len(store.eventRows))
	}
}

func TestRunProviderFetchErrorIsCounted(t *testing.T) {
	store := &fakeStore{}
	np := namedProvider{name: "stub", provider: &stubProvider{
		name: "stub",
		err:  map[int]error{2023: context.DeadlineExceeded},
	}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2023,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run should not fail on provider error: %v", err)
	}
	if stats.errors != 1 {
		t.Errorf("errors = %d, want 1", stats.errors)
	}
	if len(stats.errorProviders) != 1 || stats.errorProviders[0] != "stub/2023" {
		t.Errorf("errorProviders = %v", stats.errorProviders)
	}
}

func TestRunSkipsInvalidDates(t *testing.T) {
	store := &fakeStore{}
	np := namedProvider{name: "stub", provider: &stubProvider{
		name: "stub",
		events: map[int][]marketdata.CalendarProviderData{
			2023: {
				{Date: "not-a-date", EventType: "ex_dividend", Symbol: "2330", Source: "stub"},
				{Date: "2023-06-13", EventType: "ex_dividend", Symbol: "2330", Source: "stub"},
			},
		},
	}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2023,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.eventsSkipped["stub"] != 1 {
		t.Errorf("skipped = %d, want 1 (invalid date)", stats.eventsSkipped["stub"])
	}
	if stats.eventsWritten["stub"] != 1 {
		t.Errorf("written = %d, want 1", stats.eventsWritten["stub"])
	}
}

func TestDefaultFactoryModes(t *testing.T) {
	cfg := runConfig{}
	// auto → twse_openapi + msci_static + nsf_static
	auto := defaultFactory(cfg, 2026)
	if len(auto) != 3 {
		t.Fatalf("auto providers = %d, want 3", len(auto))
	}
	names := map[string]bool{}
	for _, np := range auto {
		names[np.name] = true
	}
	if !names["twse_openapi"] || !names["msci_static"] || !names["nsf_static"] {
		t.Errorf("auto providers = %v, want twse_openapi + msci_static + nsf_static", names)
	}

	cfg.provider = "msci"
	if msci := defaultFactory(cfg, 2026); len(msci) != 1 || msci[0].name != "msci_static" {
		t.Errorf("msci providers = %+v, want 1 msci_static", msci)
	}
	cfg.provider = "twse"
	if twse := defaultFactory(cfg, 2026); len(twse) != 1 || twse[0].name != "twse" {
		t.Errorf("twse providers = %+v, want 1 twse", twse)
	}
	cfg.provider = "nsf"
	if nsf := defaultFactory(cfg, 2026); len(nsf) != 1 || nsf[0].name != "nsf_static" {
		t.Errorf("nsf providers = %+v, want 1 nsf_static", nsf)
	}
}

func TestRunWithRealSQLiteStore(t *testing.T) {
	sqlDB, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlDB.Close()
	if err := ledger.InitSchema(sqlDB); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(sqlDB)

	np := namedProvider{name: "stub", provider: &stubProvider{
		name:   "stub",
		events: sampleProviderEvents("stub"),
	}}
	stats, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2024,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.eventsWritten["stub"] != 4 {
		t.Fatalf("written = %d, want 4", stats.eventsWritten["stub"])
	}

	// Reload through the real store and verify contents.
	rows, err := store.LoadEventCalendarRangeAll(context.Background(), "2023-01-01", "2024-12-31", 100)
	if err != nil {
		t.Fatalf("load range: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("loaded rows = %d, want 4", len(rows))
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.EventID, "stub_backfill_") {
			t.Errorf("event id = %q, want stub_backfill_ prefix", row.EventID)
		}
		if row.IsSynthetic != 1 {
			t.Errorf("row %s is_synthetic = %d, want 1", row.EventID, row.IsSynthetic)
		}
	}

	// Re-run is idempotent (upsert by (date, event_id)) — no new rows.
	stats2, err := run(context.Background(), runConfig{
		startYear: 2023,
		endYear:   2024,
		store:     store,
		now:       func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) },
		factory:   fixedProviderFactory(np),
	})
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if stats2.eventsWritten["stub"] != 4 {
		t.Errorf("re-run written = %d, want 4 (upsert, still counted)", stats2.eventsWritten["stub"])
	}
	rows2, err := store.LoadEventCalendarRangeAll(context.Background(), "2023-01-01", "2024-12-31", 100)
	if err != nil {
		t.Fatalf("load range 2: %v", err)
	}
	if len(rows2) != 4 {
		t.Errorf("rows after re-run = %d, want 4 (idempotent upsert)", len(rows2))
	}
}
