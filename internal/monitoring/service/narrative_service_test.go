package service

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
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
	score narrative.GeopoliticalRiskScore
	err   error
}

func (m *mockGeoProvider2) Name() string { return "mock-geo" }

func (m *mockGeoProvider2) FetchScore(ctx context.Context) (narrative.GeopoliticalRiskScore, error) {
	if m.err != nil {
		return narrative.GeopoliticalRiskScore{}, m.err
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
		score: narrative.GeopoliticalRiskScore{Intensity: 0.6},
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
