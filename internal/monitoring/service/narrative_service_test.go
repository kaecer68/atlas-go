package service

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

type mockMacroProvider2 struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (m *mockMacroProvider2) Name() string { return "mock" }

func (m *mockMacroProvider2) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	if m.err != nil {
		return marketdata.MacroDataSnapshot{}, m.err
	}
	return m.snap, nil
}

type mockGeoProvider2 struct {
	score geopolitical.GeopoliticalRiskScore
	err   error
}

func (m *mockGeoProvider2) Name() string { return "mock-geo" }

func (m *mockGeoProvider2) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	if m.err != nil {
		return geopolitical.GeopoliticalRiskScore{}, m.err
	}
	return m.score, nil
}

func TestNarrativeService_NewNarrativeService(t *testing.T) {
	ne := narrative.NewNarrativeEngine()
	rg := narrative.NewReportGenerator()
	svc := NewNarrativeService("/tmp/work", ne, rg)
	if svc == nil {
		t.Fatal("NewNarrativeService returned nil")
	}
	if svc.NarrativeEngine != ne {
		t.Error("NarrativeEngine not set")
	}
	if svc.ReportGenerator != rg {
		t.Error("ReportGenerator not set")
	}
}

func TestNarrativeService_SetMacroProvider(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	mp := &mockMacroProvider2{}
	svc.SetMacroProvider(mp)
	if svc.macroProvider != mp {
		t.Error("macroProvider not set")
	}
}

func TestNarrativeService_SetGeoProvider(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	gp := &mockGeoProvider2{}
	svc.SetGeoProvider(gp)
	if svc.geoProvider != gp {
		t.Error("geoProvider not set")
	}
}

func TestNarrativeService_BuildMarketNarrativeData_NilMacroProvider(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	_, err := svc.BuildMarketNarrativeData(context.Background())
	if err == nil {
		t.Error("expected error when macro provider is nil")
	}
}

func TestNarrativeService_BuildMarketNarrativeData_FetchError(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	svc.SetMacroProvider(&mockMacroProvider2{err: context.DeadlineExceeded})
	_, err := svc.BuildMarketNarrativeData(context.Background())
	if err == nil {
		t.Error("expected error from fetch failure")
	}
}

func TestNarrativeService_BuildMarketNarrativeData_OK(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	svc.SetMacroProvider(&mockMacroProvider2{
		snap: marketdata.MacroDataSnapshot{
			RecordedAt: time.Now().Unix(),
			DXY:        marketdata.MacroDataPoint{Symbol: "DXY", Value: 104.5, ChangePct: 0.2},
			US10Y:      marketdata.MacroDataPoint{Symbol: "US10Y", Value: 4.3, ChangePct: -0.1},
		},
	})
	data, err := svc.BuildMarketNarrativeData(context.Background())
	if err != nil {
		t.Fatalf("BuildMarketNarrativeData error = %v", err)
	}
	// No geo provider set → GeopoliticalGPR should be 0
	if data.GeopoliticalGPR != 0 {
		t.Errorf("GeopoliticalGPR = %v, want 0 (no geo provider)", data.GeopoliticalGPR)
	}
}

func TestNarrativeService_BuildMarketNarrativeData_WithGeoProvider(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	svc.SetMacroProvider(&mockMacroProvider2{
		snap: marketdata.MacroDataSnapshot{RecordedAt: time.Now().Unix()},
	})
	svc.SetGeoProvider(&mockGeoProvider2{
		score: geopolitical.GeopoliticalRiskScore{Intensity: 0.6},
	})
	data, err := svc.BuildMarketNarrativeData(context.Background())
	if err != nil {
		t.Fatalf("BuildMarketNarrativeData error = %v", err)
	}
	if data.GeopoliticalGPR != 0.6 {
		t.Errorf("GeopoliticalGPR = %v, want 0.6", data.GeopoliticalGPR)
	}
}

func TestNarrativeService_BuildMarketNarrativeData_GeoProviderError(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	svc.SetMacroProvider(&mockMacroProvider2{
		snap: marketdata.MacroDataSnapshot{RecordedAt: time.Now().Unix()},
	})
	svc.SetGeoProvider(&mockGeoProvider2{err: context.DeadlineExceeded})
	data, err := svc.BuildMarketNarrativeData(context.Background())
	if err != nil {
		t.Fatalf("BuildMarketNarrativeData error = %v", err)
	}
	if data.GeopoliticalGPR != 0 {
		t.Errorf("GeopoliticalGPR = %v, want 0 (graceful fallback on geo error)", data.GeopoliticalGPR)
	}
}

func TestNarrativeService_DetectEvents(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	events := svc.DetectEvents(narrative.MarketNarrativeData{})
	if events == nil {
		t.Fatal("DetectEvents returned nil")
	}
	if len(events) == 0 {
		t.Error("expected at least 1 event from DetectEvents (always emits default)")
	}
}

func TestNarrativeService_GetTemplates(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	templates := svc.GetTemplates()
	if templates == nil {
		t.Fatal("GetTemplates returned nil")
	}
	if len(templates) == 0 {
		t.Error("expected non-empty templates from NarrativeEngine")
	}
}

func TestNarrativeService_GenerateDailySummary(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	summary := svc.GenerateDailySummary("2026-06-15", nil, nil, nil)
	if summary == nil {
		t.Fatal("GenerateDailySummary returned nil")
	}
	if summary.Date != "2026-06-15" {
		t.Errorf("Date = %q, want 2026-06-15", summary.Date)
	}
}

func TestNarrativeService_GetStressIndexHistory(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	history := svc.GetStressIndexHistory(30)
	if history == nil {
		t.Fatal("GetStressIndexHistory returned nil")
	}
	if len(history) != 0 {
		t.Errorf("expected empty history for fresh engine, got %d entries", len(history))
	}
}

type mockStressHistoricalStore struct {
	rows []ledger.StressRow
}

func (m *mockStressHistoricalStore) UpsertRegime(_ context.Context, _ ledger.RegimeRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadRegimeByDate(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadRegimeByDateAll(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	return ledger.RegimeRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadRegimeHistory(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadRegimeHistoryAll(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) UpsertStress(_ context.Context, _ ledger.StressRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadStressByDate(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadStressByDateAll(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	return ledger.StressRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadStressHistory(_ context.Context, limit int) ([]ledger.StressRow, error) {
	if limit > len(m.rows) {
		limit = len(m.rows)
	}
	return m.rows[:limit], nil
}

func (m *mockStressHistoricalStore) LoadStressHistoryAll(_ context.Context, limit int) ([]ledger.StressRow, error) {
	if limit > len(m.rows) {
		limit = len(m.rows)
	}
	return m.rows[:limit], nil
}

func (m *mockStressHistoricalStore) UpsertEventCalendar(_ context.Context, _ ledger.EventCalendarRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadEventCalendarByDate(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadEventCalendarByDateAll(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadEventCalendarRange(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadEventCalendarRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) UpsertPredictionBacktest(_ context.Context, _ ledger.PredictionBacktestRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadPredictionBacktestRange(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadPredictionBacktestRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) UpsertGeopolitical(_ context.Context, _ ledger.GeopoliticalRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadGeopoliticalByDate(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadGeopoliticalByDateAll(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	return ledger.GeopoliticalRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadGeopoliticalHistory(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadGeopoliticalHistoryAll(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) UpsertPeriod(_ context.Context, _ ledger.PeriodRow) error {
	return nil
}

func (m *mockStressHistoricalStore) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockStressHistoricalStore) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockStressHistoricalStore) CountSynthetic(_ context.Context) (map[string]int64, error) {
	return nil, nil
}

func TestNarrativeService_GetStressIndexHistory_FromLedger(t *testing.T) {
	store := &mockStressHistoricalStore{
		rows: []ledger.StressRow{
			{Date: "2026-07-20", Score: 12.0, Regime: "low"},
			{Date: "2026-07-19", Score: 15.0, Regime: "low"},
			{Date: "2026-07-18", Score: 30.0, Regime: "alert"},
		},
	}
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator()).
		WithHistoricalStore(store)

	history := svc.GetStressIndexHistory(30)
	if len(history) != 3 {
		t.Fatalf("expected 3 entries from ledger, got %d", len(history))
	}
	if history[0].Score != 30.0 || history[2].Score != 12.0 {
		t.Errorf("expected ascending order [alert 30, low 15, low 12], got %v", history)
	}
}

func TestNarrativeService_GetStressIndexHistory_DaysClamping(t *testing.T) {
	store := &mockStressHistoricalStore{
		rows: []ledger.StressRow{
			{Date: "2026-07-20", Score: 12.0, Regime: "low"},
		},
	}
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator()).
		WithHistoricalStore(store)

	if got := svc.GetStressIndexHistory(0); len(got) != 1 {
		t.Errorf("days=0 should default to 30 and return 1 row, got %d", len(got))
	}
	if got := svc.GetStressIndexHistory(500); len(got) != 1 {
		t.Errorf("days>365 should clamp to 365 and return 1 row, got %d", len(got))
	}
	if got := svc.GetStressIndexHistory(-5); len(got) != 1 {
		t.Errorf("days<0 should default to 30 and return 1 row, got %d", len(got))
	}
}

func TestNarrativeService_GetStressIndexHistory_WithComponents(t *testing.T) {
	store := &mockStressHistoricalStore{
		rows: []ledger.StressRow{
			{
				Date:       "2026-07-20",
				Score:      25.0,
				Regime:     "alert",
				Components: map[string]interface{}{"dxy": 1.5, "vix": 20.0, "bad": "not-a-number"},
			},
		},
	}
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator()).
		WithHistoricalStore(store)

	history := svc.GetStressIndexHistory(30)
	if len(history) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(history))
	}
	idx := history[0]
	if idx.Components["dxy"] != 1.5 {
		t.Errorf("dxy component = %v, want 1.5", idx.Components["dxy"])
	}
	if idx.Components["vix"] != 20.0 {
		t.Errorf("vix component = %v, want 20.0", idx.Components["vix"])
	}
	if _, ok := idx.Components["bad"]; ok {
		t.Errorf("non-float component should be skipped")
	}
}

// mockGeoHistoricalStore is a focused stub of ledger.HistoricalStore that
// satisfies only LoadGeopoliticalHistory for the GetGeopoliticalHistory test.
// All other methods panic if invoked — they are intentionally unused.
type mockGeoHistoricalStore struct {
	rows []ledger.GeopoliticalRow
	err  error
}

func (m *mockGeoHistoricalStore) UpsertRegime(_ context.Context, _ ledger.RegimeRow) error {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadRegimeByDate(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadRegimeByDateAll(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadRegimeHistory(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadRegimeHistoryAll(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) UpsertStress(_ context.Context, _ ledger.StressRow) error {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadStressByDate(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadStressByDateAll(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) UpsertPeriod(_ context.Context, _ ledger.PeriodRow) error {
	return nil
}

func (m *mockGeoHistoricalStore) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockGeoHistoricalStore) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockGeoHistoricalStore) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockGeoHistoricalStore) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockGeoHistoricalStore) LoadStressHistory(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadStressHistoryAll(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) UpsertGeopolitical(_ context.Context, _ ledger.GeopoliticalRow) error {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadGeopoliticalByDate(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadGeopoliticalByDateAll(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadGeopoliticalHistory(_ context.Context, limit int) ([]ledger.GeopoliticalRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && len(m.rows) > limit {
		return m.rows[:limit], nil
	}
	return m.rows, nil
}

func (m *mockGeoHistoricalStore) LoadGeopoliticalHistoryAll(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) UpsertEventCalendar(_ context.Context, _ ledger.EventCalendarRow) error {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadEventCalendarByDate(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadEventCalendarByDateAll(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadEventCalendarRange(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadEventCalendarRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) UpsertPredictionBacktest(_ context.Context, _ ledger.PredictionBacktestRow) error {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadPredictionBacktestRange(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) LoadPredictionBacktestRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}

func (m *mockGeoHistoricalStore) CountSynthetic(_ context.Context) (map[string]int64, error) {
	panic("not implemented")
}

func TestNarrativeService_GetGeopoliticalHistory_NilStore(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	got := svc.GetGeopoliticalHistory(30)
	if got != nil {
		t.Errorf("expected nil without historicalStore, got %+v", got)
	}
}

func TestNarrativeService_GetGeopoliticalHistory_DaysBounds(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	store := &mockGeoHistoricalStore{rows: []ledger.GeopoliticalRow{}}
	svc.WithHistoricalStore(store)
	for _, days := range []int{0, -5, 9999} {
		got := svc.GetGeopoliticalHistory(days)
		if got != nil {
			t.Errorf("days=%d: expected nil (empty store), got %+v", days, got)
		}
	}
}

func TestNarrativeService_GetGeopoliticalHistory_FromStore(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	now := time.Now().UTC()
	captured := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
	date := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }
	store := &mockGeoHistoricalStore{rows: []ledger.GeopoliticalRow{
		{Date: date(0), Intensity: 42.5, Sources: []string{"rss", "gdelt"}, Source: "macro_ingest", CapturedAt: captured},
		{Date: date(-1), Intensity: 30.0, Source: "macro_ingest", CapturedAt: captured.AddDate(0, 0, -1)},
	}}
	svc.WithHistoricalStore(store)

	got := svc.GetGeopoliticalHistory(30)
	if len(got) != 2 {
		t.Fatalf("expected 2 points, got %d", len(got))
	}
	if got[0].Date != date(0) {
		t.Errorf("first date = %q, want %q", got[0].Date, date(0))
	}
	if got[0].Intensity != 42.5 {
		t.Errorf("first intensity = %v, want 42.5", got[0].Intensity)
	}
	if got[0].Source != "macro_ingest" {
		t.Errorf("first source = %q, want macro_ingest", got[0].Source)
	}
	if got[0].CapturedAt != captured.UTC().Format(time.RFC3339) {
		t.Errorf("first captured_at = %q, want %q", got[0].CapturedAt, captured.UTC().Format(time.RFC3339))
	}
	if len(got[0].Sources) != 2 || got[0].Sources[0] != "rss" || got[0].Sources[1] != "gdelt" {
		t.Errorf("first sources = %v, want [rss gdelt]", got[0].Sources)
	}
}

func TestNarrativeService_GetGeopoliticalHistory_StoreError(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	store := &mockGeoHistoricalStore{err: context.DeadlineExceeded}
	svc.WithHistoricalStore(store)
	got := svc.GetGeopoliticalHistory(30)
	if got != nil {
		t.Errorf("expected nil on store error, got %+v", got)
	}
}

func TestNarrativeService_WithHistoricalStore_ChainsReturnSameService(t *testing.T) {
	svc := NewNarrativeService("/tmp/work", narrative.NewNarrativeEngine(), narrative.NewReportGenerator())
	store := &mockGeoHistoricalStore{}
	out := svc.WithHistoricalStore(store)
	if out != svc {
		t.Errorf("WithHistoricalStore must return the same service for fluent chaining")
	}
	if svc.historicalStore == nil {
		t.Error("historicalStore not set after WithHistoricalStore")
	}
}

func TestNarrativeService_ListModels_ReturnsAll(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := NewNarrativeService(t.TempDir(), eng, nil)

	models := svc.ListModels()
	// Full library = all models, not just active ones.
	if len(models) != 21 {
		t.Fatalf("expected 21 models in full library, got %d", len(models))
	}
}

func TestNarrativeService_ModelInventory_Structure(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := NewNarrativeService(t.TempDir(), eng, nil)
	svc.SetMacroProvider(&mockMacroProvider2{snap: marketdata.MacroDataSnapshot{}})

	inv := svc.ModelInventory(context.Background())

	all, ok := inv["all_models"].([]narrative.InvestmentModel)
	if !ok || len(all) != 21 {
		t.Fatalf("expected all_models with 21 models, got %T len=%d", inv["all_models"], len(all))
	}
	if _, ok := inv["active_models"].([]narrative.InvestmentModel); !ok {
		t.Fatalf("expected active_models slice, got %T", inv["active_models"])
	}
	tm, ok := inv["theme_to_models"].(map[string][]string)
	if !ok {
		t.Fatalf("expected theme_to_models map, got %T", inv["theme_to_models"])
	}
	tt, ok := inv["theme_to_templates"].(map[string][]string)
	if !ok {
		t.Fatalf("expected theme_to_templates map, got %T", inv["theme_to_templates"])
	}
	// 表裡結構: AI_capex_surge maps to ai_supercycle_model AND to its template.
	if len(tm["AI_capex_surge"]) == 0 {
		t.Errorf("AI_capex_surge should map to ≥1 model, got %v", tm["AI_capex_surge"])
	}
	if len(tt["AI_capex_surge"]) == 0 {
		t.Errorf("AI_capex_surge should map to ≥1 template, got %v", tt["AI_capex_surge"])
	}
	if _, ok := inv["workflow"]; !ok {
		t.Error("expected workflow field in inventory")
	}
}
