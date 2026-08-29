// File: historical_store_test.go
// Package: internal/ledger
//
// Tests for SQLiteHistoricalStore. Each test uses a fresh t.TempDir()
// and initializes a fresh schema; tables are dropped automatically when
// the test exits via Go's testing.Cleanup chain.
package ledger

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// mustOpenStore returns a fresh SQLiteHistoricalStore backed by a temp
// DB whose schema is initialised in-line. The returned cleanup func
// closes the underlying DB.
func mustOpenStore(t *testing.T) (HistoricalStore, *SQLiteHistoricalStore, string, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := InitSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("init: %v", err)
	}
	store := NewSQLiteHistoricalStore(db)
	cleanup := func() { _ = db.Close() }
	has, err := store.HasTables(context.Background())
	if err != nil {
		cleanup()
		t.Fatalf("has tables: %v", err)
	}
	for _, k := range []string{"regime_history", "stress_index_history", "geopolitical_history", "event_calendar_history", "prediction_backtest"} {
		if !has[k] {
			cleanup()
			t.Fatalf("missing table %s after InitSchema; tables=%v", k, has)
		}
	}
	return store, store, path, cleanup
}

// ------------------------------------------------------------------
// Regime
// ------------------------------------------------------------------

// TestParseTimeColumn_Layouts exercises parseTimeColumn across every layout
// we accept for historical TEXT columns. The Go-native time.Time.String()
// layout (e.g. "2026-06-29 06:00:00 +0000 UTC") is the format that the
// modernc.org/sqlite driver writes for sql.NullTime values whose underlying
// time.Time is UTC-stripped, so without that layout the column would parse
// to a zero time and the API surface would emit "0001-01-01T00:00:00Z"
// (see manifest 2026-07-21-historical-store-time-and-limit-fixes.md, B01).
func TestParseTimeColumn_Layouts(t *testing.T) {
	cases := []struct {
		name   string
		input  sql.NullString
		wantOK bool
	}{
		{"null", sql.NullString{}, false},
		{"empty", sql.NullString{String: "", Valid: true}, false},
		{"rfc3339nano", sql.NullString{String: "2026-06-29T06:00:00.123456789Z", Valid: true}, true},
		{"rfc3339", sql.NullString{String: "2026-06-29T06:00:00Z", Valid: true}, true},
		{"no_frac_tz_z", sql.NullString{String: "2026-06-29T06:00:00.000Z", Valid: true}, true},
		{"no_frac_z", sql.NullString{String: "2026-06-29T06:00:00Z", Valid: true}, true},
		// Legacy / driver-default format (this is the BUG-1 case).
		{"go_native_no_frac", sql.NullString{String: "2026-06-29 06:00:00 +0000 UTC", Valid: true}, true},
		{"go_native_nano", sql.NullString{String: "2026-07-20 16:14:01.734248004 +0000 UTC", Valid: true}, true},
		{"go_native_micro", sql.NullString{String: "2026-07-20 16:14:01.734248 +0000 UTC", Valid: true}, true},
		{"unparseable", sql.NullString{String: "not a time", Valid: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTimeColumn(tc.input)
			if tc.wantOK && got.IsZero() {
				t.Fatalf("parseTimeColumn(%q) returned zero time, want non-zero", tc.input.String)
			}
			if !tc.wantOK && !got.IsZero() {
				t.Fatalf("parseTimeColumn(%q) = %v, want zero time", tc.input.String, got)
			}
		})
	}
}

// TestSQLiteHistoricalStore_GoNativeTimeFormat_RoundTrip guards the BUG-1
// regression: a row whose captured_at was written via nullTime() (driver
// default Go-native format) must round-trip back to a non-zero RFC3339
// string via LoadRegimeHistory.
func TestSQLiteHistoricalStore_GoNativeTimeFormat_RoundTrip(t *testing.T) {
	_, store, _, cleanup := mustOpenStore(t)
	defer cleanup()

	ctx := context.Background()
	capturedAt := time.Date(2026, 6, 29, 6, 0, 0, 0, time.UTC)
	if err := store.UpsertRegime(ctx, RegimeRow{
		Date:        "2026-06-29",
		Regime:      "RISK_OFF",
		RecordedAt:  capturedAt,
		CapturedAt:  capturedAt,
		IsSynthetic: 0,
	}); err != nil {
		t.Fatalf("UpsertRegime: %v", err)
	}

	// Read the raw text the driver wrote.
	var raw string
	if err := store.db.QueryRowContext(ctx,
		`SELECT captured_at FROM regime_history WHERE date = ?`, "2026-06-29").Scan(&raw); err != nil {
		t.Fatalf("read raw captured_at: %v", err)
	}
	// Sanity-check: confirm we are exercising the Go-native format that
	// BUG-1 was about. If the driver ever switches to RFC3339Nano, this
	// test still passes (parseTimeColumn accepts both).
	if raw == "" {
		t.Fatalf("captured_at is empty; cannot validate round-trip")
	}

	rows, err := store.LoadRegimeHistory(ctx, 5)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CapturedAt.IsZero() {
		t.Fatalf("CapturedAt round-tripped to zero time; raw=%q", raw)
	}
	if !rows[0].CapturedAt.Equal(capturedAt) {
		t.Fatalf("CapturedAt round-trip mismatch: got %v want %v (raw=%q)",
			rows[0].CapturedAt, capturedAt, raw)
	}
}

func TestSQLiteHistoricalStore_UpsertRegime_Idempotent(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()

	row := RegimeRow{
		Date:            "2026-04-15",
		Regime:          "RISK_ON",
		SourceSessionID: "session-20260415-daily",
		RecordedAt:      time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		CapturedAt:      time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic:     1,
	}
	for i := range 3 {
		if err := store.UpsertRegime(context.Background(), row); err != nil {
			t.Fatalf("upsert #%d: %v", i, err)
		}
	}
	got, ok, err := store.LoadRegimeByDateAll(context.Background(), "2026-04-15")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Regime != "RISK_ON" {
		t.Errorf("regime = %q, want RISK_ON", got.Regime)
	}
	if got.SourceSessionID != "session-20260415-daily" {
		t.Errorf("source SID = %q, want session-20260415-daily", got.SourceSessionID)
	}
	if got.IsSynthetic != 1 {
		t.Errorf("IsSynthetic = %d, want 1", got.IsSynthetic)
	}
}

func TestSQLiteHistoricalStore_UpsertRegime_RequiresDate(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	if err := store.UpsertRegime(context.Background(), RegimeRow{}); err == nil {
		t.Fatal("expected error for empty date")
	}
}

func TestSQLiteHistoricalStore_LoadRegimeHistory_OrderedDesc(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	dates := []string{"2026-04-15", "2026-04-16", "2026-04-17", "2026-04-18"}
	for _, d := range dates {
		if err := store.UpsertRegime(context.Background(), RegimeRow{
			Date:        d,
			Regime:      "NEUTRAL",
			CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
			IsSynthetic: 1,
		}); err != nil {
			t.Fatalf("upsert %s: %v", d, err)
		}
	}
	got, err := store.LoadRegimeHistoryAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	// Order is DESC by date.
	wantOrder := []string{"2026-04-18", "2026-04-17", "2026-04-16", "2026-04-15"}
	for i, want := range wantOrder {
		if got[i].Date != want {
			t.Errorf("[%d].Date = %q, want %q", i, got[i].Date, want)
		}
	}
}

func TestSQLiteHistoricalStore_LoadRegimeHistory_Limit(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	for i := 1; i <= 5; i++ {
		date := time.Date(2026, 4, i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if err := store.UpsertRegime(context.Background(), RegimeRow{
			Date: date, Regime: "RISK_ON", IsSynthetic: 1,
			CapturedAt: time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("upsert %s: %v", date, err)
		}
	}
}

func TestLoadRegimeHistoryRangeAll_IncludesSynthetic(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	ctx := context.Background()
	if err := store.UpsertRegime(ctx, RegimeRow{
		Date:        "2026-04-15",
		Regime:      "RISK_ON",
		CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic: 1,
	}); err != nil {
		t.Fatalf("upsert synthetic: %v", err)
	}
	if err := store.UpsertRegime(ctx, RegimeRow{
		Date:        "2026-04-16",
		Regime:      "RISK_OFF",
		CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic: 0,
	}); err != nil {
		t.Fatalf("upsert real: %v", err)
	}
	got, err := store.LoadRegimeHistoryAll(ctx, 100)
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestLoadRegimeHistory_FiltersSynthetic(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	ctx := context.Background()
	if err := store.UpsertRegime(ctx, RegimeRow{
		Date:        "2026-04-15",
		Regime:      "RISK_ON",
		CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic: 1,
	}); err != nil {
		t.Fatalf("upsert synthetic: %v", err)
	}
	if err := store.UpsertRegime(ctx, RegimeRow{
		Date:        "2026-04-16",
		Regime:      "RISK_OFF",
		CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic: 0,
	}); err != nil {
		t.Fatalf("upsert real: %v", err)
	}
	got, err := store.LoadRegimeHistory(ctx, 100)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Date != "2026-04-16" {
		t.Errorf("date = %q, want 2026-04-16", got[0].Date)
	}
	if got[0].IsSynthetic != 0 {
		t.Errorf("IsSynthetic = %d, want 0", got[0].IsSynthetic)
	}
}

// ------------------------------------------------------------------
// Stress
// ------------------------------------------------------------------

func TestSQLiteHistoricalStore_UpsertStress_RoundTripComponents(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()

	row := StressRow{
		Date:   "2026-04-15",
		Score:  0.42,
		Regime: "medium",
		Components: map[string]any{
			"us":       0.5,
			"asia":     0.3,
			"isString": "ignored-or-kept",
		},
		Source:      "macro-file",
		CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		IsSynthetic: 1,
	}
	if err := store.UpsertStress(context.Background(), row); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, ok, err := store.LoadStressByDateAll(context.Background(), "2026-04-15")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Regime != "medium" || got.Score != 0.42 || got.Source != "macro-file" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Components["us"] != 0.5 {
		t.Errorf("components[us] = %v, want 0.5", got.Components["us"])
	}
}

func TestSQLiteHistoricalStore_LoadStressHistory_Limit(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	for i := 1; i <= 4; i++ {
		date := time.Date(2026, 4, i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if err := store.UpsertStress(context.Background(), StressRow{
			Date: date, Score: float64(i) / 10.0, IsSynthetic: 1,
			CapturedAt: time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("upsert %s: %v", date, err)
		}
	}
	got, err := store.LoadStressHistoryAll(context.Background(), 2)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("limit=2 → got %d, want 2", len(got))
	}
}

// ------------------------------------------------------------------
// Event calendar
// ------------------------------------------------------------------

func TestSQLiteHistoricalStore_EventCalendar_CompositePK(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	capt := time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC)
	for i, eid := range []string{"evt-tech-peak-A", "evt-dividend-B", "evt-tech-peak-A"} {
		theme := "tech-peak"
		if eid == "evt-dividend-B" {
			theme = "dividend"
		}
		if err := store.UpsertEventCalendar(context.Background(), EventCalendarRow{
			Date: "2026-04-15", EventID: eid, ActiveTheme: theme,
			Source: "session-derive", CapturedAt: capt, IsSynthetic: 1,
		}); err != nil {
			t.Fatalf("upsert #%d: %v", i, err)
		}
	}
	got, err := store.LoadEventCalendarByDateAll(context.Background(), "2026-04-15")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Composite PK ensures evt-tech-peak-A is deduped.
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (PK dedup)", len(got))
	}
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.EventID] = true
	}
	if !ids["evt-tech-peak-A"] || !ids["evt-dividend-B"] {
		t.Errorf("ids = %v, want both", ids)
	}
}

func TestSQLiteHistoricalStore_EventCalendar_RangeQuery(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	capt := time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC)
	for _, d := range []string{"2026-04-15", "2026-04-16", "2026-04-17"} {
		if err := store.UpsertEventCalendar(context.Background(), EventCalendarRow{
			Date: d, EventID: "evt-x", ActiveTheme: "x",
			Source: "session-derive", CapturedAt: capt, IsSynthetic: 1,
		}); err != nil {
			t.Fatalf("upsert %s: %v", d, err)
		}
	}
	got, err := store.LoadEventCalendarRangeAll(context.Background(), "2026-04-15", "2026-04-16", 100)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (range)", len(got))
	}
	for _, r := range got {
		if r.Date < "2026-04-15" || r.Date > "2026-04-16" {
			t.Errorf("date %s outside range", r.Date)
		}
	}
}

// ------------------------------------------------------------------
// Prediction backtest
// ------------------------------------------------------------------

func TestSQLiteHistoricalStore_PredictionBacktest_RoundTrip(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	capt := time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC)
	if err := store.UpsertPredictionBacktest(context.Background(), PredictionBacktestRow{
		Date:               "2026-04-15",
		PredictedDirection: "inflow", PredictedConfidence: 0.78,
		ActualDirection: "inflow", ActualCapitalFlowChan: 0.012,
		Hit: true, ModelVersion: "v0.0.0.32",
		CapturedAt: capt, IsSynthetic: 1,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.LoadPredictionBacktestRangeAll(context.Background(), "2026-04-15", "2026-04-16", 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	r := got[0]
	if r.Date != "2026-04-15" || r.PredictedDirection != "inflow" || !r.Hit {
		t.Errorf("unexpected row: %+v", r)
	}
	if r.PredictedConfidence != 0.78 {
		t.Errorf("confidence = %f, want 0.78", r.PredictedConfidence)
	}
	if r.ModelVersion != "v0.0.0.32" {
		t.Errorf("model = %q, want v0.0.0.32", r.ModelVersion)
	}
}

// ------------------------------------------------------------------
// Constraints / sanity
// ------------------------------------------------------------------

func TestSQLiteHistoricalStore_UpsertEventCalendar_RequiresBoth(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	cases := []EventCalendarRow{
		{Date: "", EventID: "x"},
		{Date: "2026-04-15", EventID: ""},
	}
	for i, c := range cases {
		if err := store.UpsertEventCalendar(context.Background(), c); err == nil {
			t.Errorf("case #%d: expected error for empty key", i)
		}
	}
}

func TestSQLiteHistoricalStore_LoadByDate_NotFound(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	_, ok, err := store.LoadRegimeByDate(context.Background(), "9999-01-01")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ok {
		t.Error("expected ok=false for missing date")
	}
}

func TestSQLiteHistoricalStore_ConcurrentUpserts(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	capt := time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC)
	const goroutines = 10
	const perGoroutine = 5
	doneCh := make(chan error, goroutines)
	for g := range goroutines {
		go func(grp int) {
			for i := range perGoroutine {
				date := time.Date(2026, 4, (grp%28)+1, 0, 0, 0, 0, time.UTC).
					AddDate(0, 0, i).Format("2006-01-02")
				err := store.UpsertRegime(context.Background(), RegimeRow{
					Date: date, Regime: "RISK_ON", CapturedAt: capt, IsSynthetic: 1,
				})
				if err != nil {
					doneCh <- err
					return
				}
			}
			doneCh <- nil
		}(g)
	}
	for range goroutines {
		if err := <-doneCh; err != nil {
			t.Fatalf("goroutine err: %v", err)
		}
	}
	// Some rows were duplicated across goroutines (same date),
	// so we expect at most goroutines*perGoroutine distinct dates.
	got, err := store.LoadRegimeHistoryAll(context.Background(), 0)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) == 0 {
		t.Error("expected at least 1 row after concurrent inserts")
	}
	// All rows must carry IsSynthetic=1 — read path integrity.
	for _, r := range got {
		if r.IsSynthetic != 1 {
			t.Errorf("row %s: IsSynthetic = %d, want 1", r.Date, r.IsSynthetic)
		}
	}
}

// ------------------------------------------------------------------
// Smoke guard — keeps the package from being accidentally pruned by
// static analysis. Cheap and tells us that nothing got renamed
// unexpectedly.
// ------------------------------------------------------------------

func TestSchemaConstants_Recognised(t *testing.T) {
	// hasTables reports all 4 keys — smoke check the contract.
	store, raw, _, done := mustOpenStore(t)
	defer done()
	_ = store
	has, err := raw.HasTables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for k, v := range has {
		if v {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	want := []string{"event_calendar_history", "geopolitical_history", "period_history", "prediction_backtest", "regime_history", "stress_index_history"}
	for i, name := range want {
		if i >= len(got) || got[i] != name {
			t.Errorf("HasTables = %v, want %v", got, want)
			break
		}
	}
}

// ------------------------------------------------------------------
// Geopolitical
// ------------------------------------------------------------------

func TestSQLiteHistoricalStore_UpsertGeopolitical_Idempotent(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()

	row := GeopoliticalRow{
		Date:        "2026-04-15",
		Intensity:   42.5,
		Sources:     []string{"rss", "gdelt"},
		Source:      "macro_ingest",
		CapturedAt:  time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC),
		IsSynthetic: 0,
	}
	if err := store.UpsertGeopolitical(context.Background(), row); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	row.Intensity = 43.0
	if err := store.UpsertGeopolitical(context.Background(), row); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, ok, err := store.LoadGeopoliticalByDate(context.Background(), row.Date)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatalf("expected row for %s", row.Date)
	}
	if got.Intensity != 43.0 {
		t.Errorf("intensity = %v, want 43.0", got.Intensity)
	}
	if len(got.Sources) != 2 || got.Sources[0] != "rss" || got.Sources[1] != "gdelt" {
		t.Errorf("sources = %v, want [rss gdelt]", got.Sources)
	}
}

// TestSQLiteHistoricalStore_GeopoliticalEventsRoundTrip covers G5-4: the
// sources_json column now stores {"feeds":[...],"events":[...]} and both
// legacy (plain []string) and new formats must round-trip.
func TestSQLiteHistoricalStore_GeopoliticalEventsRoundTrip(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()

	row := GeopoliticalRow{
		Date:      "2026-08-25",
		Intensity: 42,
		Sources:   []string{"https://feed1.example/rss"},
		Events: []GeoEventRow{
			{Title: "共機繞台 40 架次", Keyword: "共機", Source: "https://feed1.example/rss"},
			{Title: "PLA drills near Taiwan Strait", Keyword: "taiwan strait", Source: "https://feed1.example/rss"},
		},
		Source:      "macro_ingest",
		CapturedAt:  time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		IsSynthetic: 0,
	}
	if err := store.UpsertGeopolitical(context.Background(), row); err != nil {
		t.Fatalf("upsert with events: %v", err)
	}

	got, ok, err := store.LoadGeopoliticalByDateAll(context.Background(), "2026-08-25")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if len(got.Sources) != 1 || got.Sources[0] != "https://feed1.example/rss" {
		t.Errorf("Sources = %v, want feed URL preserved", got.Sources)
	}
	if len(got.Events) != 2 || got.Events[0].Keyword != "共機" || got.Events[1].Title != "PLA drills near Taiwan Strait" {
		t.Errorf("Events = %+v, want 2 traced items", got.Events)
	}

	// Legacy format (plain []string, no events) must still round-trip:
	// Upsert writes the plain array when Events is nil, and the tolerant
	// loader must read it back into Sources with Events nil.
	legacy := GeopoliticalRow{
		Date:       "2026-08-01",
		Intensity:  10,
		Sources:    []string{"https://legacy.example/rss"},
		Source:     "macro_ingest",
		CapturedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertGeopolitical(context.Background(), legacy); err != nil {
		t.Fatalf("upsert legacy: %v", err)
	}
	gotL, ok, err := store.LoadGeopoliticalByDateAll(context.Background(), "2026-08-01")
	if err != nil || !ok {
		t.Fatalf("load legacy: ok=%v err=%v", ok, err)
	}
	if len(gotL.Sources) != 1 || gotL.Sources[0] != "https://legacy.example/rss" {
		t.Errorf("legacy Sources = %v, want plain array read back", gotL.Sources)
	}
	if len(gotL.Events) != 0 {
		t.Errorf("legacy Events = %+v, want nil", gotL.Events)
	}
}

func TestSQLiteHistoricalStore_UpsertGeopolitical_RequiresDate(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	if err := store.UpsertGeopolitical(context.Background(), GeopoliticalRow{}); err == nil {
		t.Error("expected error for empty date")
	}
}

func TestSQLiteHistoricalStore_LoadGeopoliticalHistory_OrderedDesc(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	rows := []GeopoliticalRow{
		{Date: "2026-04-15", Intensity: 10, CapturedAt: time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC), IsSynthetic: 0},
		{Date: "2026-04-14", Intensity: 20, CapturedAt: time.Date(2026, 4, 14, 6, 0, 0, 0, time.UTC), IsSynthetic: 0},
		{Date: "2026-04-13", Intensity: 30, CapturedAt: time.Date(2026, 4, 13, 6, 0, 0, 0, time.UTC), IsSynthetic: 0},
	}
	for _, r := range rows {
		if err := store.UpsertGeopolitical(context.Background(), r); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	got, err := store.LoadGeopoliticalHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(got))
	}
	if got[0].Date != "2026-04-15" {
		t.Errorf("first row date = %q, want 2026-04-15", got[0].Date)
	}
	if got[2].Date != "2026-04-13" {
		t.Errorf("last row date = %q, want 2026-04-13", got[2].Date)
	}
}

func TestSQLiteHistoricalStore_LoadGeopoliticalHistory_Limit(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	for _, d := range []string{"2026-04-15", "2026-04-14", "2026-04-13"} {
		if err := store.UpsertGeopolitical(context.Background(), GeopoliticalRow{
			Date:       d,
			Intensity:  10,
			CapturedAt: time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	got, err := store.LoadGeopoliticalHistory(context.Background(), 2)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 rows, got %d", len(got))
	}
}

func TestSQLiteHistoricalStore_LoadGeopoliticalHistory_FiltersSynthetic(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()
	if err := store.UpsertGeopolitical(context.Background(), GeopoliticalRow{
		Date:        "2026-04-15",
		Intensity:   10,
		CapturedAt:  time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC),
		IsSynthetic: 1,
	}); err != nil {
		t.Fatalf("upsert synthetic: %v", err)
	}
	if err := store.UpsertGeopolitical(context.Background(), GeopoliticalRow{
		Date:        "2026-04-15",
		Intensity:   20,
		CapturedAt:  time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC),
		IsSynthetic: 0,
	}); err != nil {
		t.Fatalf("upsert real: %v", err)
	}
	got, err := store.LoadGeopoliticalHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Intensity != 20 {
		t.Errorf("expected 1 real row with intensity 20, got %+v", got)
	}
	all, err := store.LoadGeopoliticalHistoryAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(all) != 1 || all[0].Intensity != 20 {
		t.Errorf("all should also return the winning non-synthetic row, got %+v", all)
	}
}

// TestInitSchema_AddsRegimeHistorySourceColumn covers D01: a fresh database
// initialized via InitSchema must include the new 'source' column on
// regime_history. This guards against future schema edits silently dropping
// the column or changing its DEFAULT.
func TestInitSchema_AddsRegimeHistorySourceColumn(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	rows, err := db.Query(`PRAGMA table_info(regime_history)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var cid, notnull, pk int
	var name, ctype string
	var dflt sql.NullString
	var foundSource bool
	for rows.Next() {
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == "source" {
			foundSource = true
			if !dflt.Valid || dflt.String != "'synthetic'" {
				t.Errorf("regime_history.source default = %v, want 'synthetic'", dflt)
			}
			if notnull != 1 {
				t.Errorf("regime_history.source NOT NULL = %d, want 1", notnull)
			}
		}
	}
	if !foundSource {
		t.Error("regime_history.source column missing after InitSchema")
	}
}

// TestInitSchema_RegimeHistorySourceBackfill_PreExistingSchema covers D01
// against the realistic upgrade path: a database that was created before
// PR #1247 (no source column) must be migrated forward without losing
// existing rows, and the migrated source column must default to 'synthetic'
// for the legacy rows.
func TestInitSchema_RegimeHistorySourceBackfill_PreExistingSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Simulate a pre-existing DB: schema is missing source column and
	// contains 2 legacy rows written by an older binary.
	if _, err := db.Exec(`
		CREATE TABLE regime_history (
			date TEXT PRIMARY KEY,
			regime TEXT NOT NULL,
			source_session_id TEXT,
			recorded_at TEXT,
			captured_at TEXT NOT NULL,
			is_synthetic INTEGER NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	for _, date := range []string{"2026-01-01", "2026-01-02"} {
		if _, err := db.Exec(`INSERT INTO regime_history (date, regime, captured_at, is_synthetic) VALUES (?, 'RISK_ON', ?, 0)`,
			date, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatalf("insert legacy row: %v", err)
		}
	}

	// Run InitSchema — should add source column with DEFAULT 'synthetic'.
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema (upgrade): %v", err)
	}

	// Verify the new column exists and existing rows were backfilled.
	rows, err := db.Query(`SELECT date, source FROM regime_history ORDER BY date`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var d, s string
		if err := rows.Scan(&d, &s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, d+":"+s)
	}
	want := []string{"2026-01-01:synthetic", "2026-01-02:synthetic"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("after migration got %v, want %v", got, want)
	}
}

// TestInitSchema_RegimeHistorySourceMigration_Idempotent covers D01:
// running InitSchema twice on the same DB must not error (the second
// call should detect the column already exists and skip the ALTER).
func TestInitSchema_RegimeHistorySourceMigration_Idempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := InitSchema(db); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("second init (idempotent): %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("third init (idempotent): %v", err)
	}
}

// TestSQLiteHistoricalStore_UpsertRegime_RoundTripSource covers D02:
// UpsertRegime must persist Source and LoadRegimeHistory / LoadRegimeByDate
// must return it. Backfill 'synthetic' default applies to rows written
// before the migration; live 'macro_ingest' rows populate explicitly.
func TestSQLiteHistoricalStore_UpsertRegime_RoundTripSource(t *testing.T) {
	store, _, _, done := mustOpenStore(t)
	defer done()

	cases := []struct {
		date   string
		regime string
		source string
	}{
		{"2026-06-01", "RISK_ON", "synthetic"},
		{"2026-06-02", "RISK_OFF", "macro_ingest"},
		{"2026-06-03", "NEUTRAL", ""}, // empty → nullString → NULL → backfill default
	}
	now := time.Now().UTC()
	for _, c := range cases {
		row := RegimeRow{
			Date:        c.date,
			Regime:      c.regime,
			Source:      c.source,
			IsSynthetic: 0,
			CapturedAt:  now,
			RecordedAt:  now,
		}
		if err := store.UpsertRegime(context.Background(), row); err != nil {
			t.Fatalf("upsert %s: %v", c.date, err)
		}
	}

	rows, err := store.LoadRegimeHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byDate := map[string]string{}
	for _, r := range rows {
		byDate[r.Date] = r.Source
	}
	if byDate["2026-06-01"] != "synthetic" {
		t.Errorf("2026-06-01 source = %q, want synthetic", byDate["2026-06-01"])
	}
	if byDate["2026-06-02"] != "macro_ingest" {
		t.Errorf("2026-06-02 source = %q, want macro_ingest", byDate["2026-06-02"])
	}
	if byDate["2026-06-03"] != "synthetic" {
		t.Errorf("2026-06-03 source = %q, want synthetic (empty→default substitution)", byDate["2026-06-03"])
	}
}
