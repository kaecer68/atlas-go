package service

import (
	"context"
	"testing"
	"time"

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
