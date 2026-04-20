package narrative

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestMacroIngestorDetectsUSRatesEvent(t *testing.T) {
	dir := t.TempDir()
	mock := &marketdata.MockMacroProvider{
		Snapshot: marketdata.MacroDataSnapshot{
			US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 150, ChangePct: 2.0},
			DXY:   marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 105, ChangePct: 1.8},
		},
	}
	ingestor := NewMacroIngestor(mock, dir)
	events, _, err := ingestor.Ingest(context.Background())
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	found := false
	for _, e := range events {
		if e.Theme == "US_rates_up" {
			found = true
			if e.Region != "US" {
				t.Fatalf("expected region US, got %s", e.Region)
			}
		}
	}
	if !found {
		t.Fatalf("expected US_rates_up event")
	}

	// Verify snapshot persisted.
	latest := filepath.Join(dir, "latest.json")
	if _, err := os.Stat(latest); os.IsNotExist(err) {
		t.Fatalf("expected latest snapshot to be saved")
	}
}

func TestMacroIngestorDetectsJPYCarryUnwind(t *testing.T) {
	dir := t.TempDir()
	mock := &marketdata.MockMacroProvider{
		Snapshot: marketdata.MacroDataSnapshot{
			JPY: marketdata.MacroDataPoint{Symbol: "JPY=X", Value: 145, ChangePct: 3.5},
			VIX: marketdata.MacroDataPoint{Symbol: "^VIX", Value: 30, ChangePct: 10},
		},
	}
	ingestor := NewMacroIngestor(mock, dir)
	events, _, err := ingestor.Ingest(context.Background())
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	found := false
	for _, e := range events {
		if e.Theme == "JPY_carry_unwind" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected JPY_carry_unwind event")
	}
}

func TestMacroIngestorNoTriggerOnCalmData(t *testing.T) {
	dir := t.TempDir()
	mock := &marketdata.MockMacroProvider{
		Snapshot: marketdata.MacroDataSnapshot{
			US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 5, ChangePct: 0.2},
			JPY:   marketdata.MacroDataPoint{Symbol: "JPY=X", Value: 1.0, ChangePct: 0.1},
			Gold:  marketdata.MacroDataPoint{Symbol: "GC=F", Value: 1.0, ChangePct: 0.5},
			Oil:   marketdata.MacroDataPoint{Symbol: "CL=F", Value: 2.0, ChangePct: 1.0},
			VIX:   marketdata.MacroDataPoint{Symbol: "^VIX", Value: 15, ChangePct: 0},
		},
	}
	ingestor := NewMacroIngestor(mock, dir)
	events, _, err := ingestor.Ingest(context.Background())
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events on calm data, got %d", len(events))
	}
}

func TestDetectRetailFrenzyEvent(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		RetailSentiment: marketdata.MacroDataPoint{Symbol: "RETAIL_SENTIMENT", Value: 0.85},
	}

	event := detectRetailFrenzyEvent(snap)
	if event == nil {
		t.Fatal("expected event for high sentiment")
	}
	if event.Theme != "retail_frenzy" {
		t.Errorf("expected theme retail_frenzy, got %s", event.Theme)
	}
}

func TestDetectRetailFearEvent(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		RetailSentiment: marketdata.MacroDataPoint{Symbol: "RETAIL_SENTIMENT", Value: -0.85},
	}

	event := detectRetailFearEvent(snap)
	if event == nil {
		t.Fatal("expected event for low sentiment")
	}
	if event.Theme != "retail_fear" {
		t.Errorf("expected theme retail_fear, got %s", event.Theme)
	}
}

func TestDetectRetailEvents_Neutral(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		RetailSentiment: marketdata.MacroDataPoint{Symbol: "RETAIL_SENTIMENT", Value: 0.0},
	}

	if detectRetailFrenzyEvent(snap) != nil {
		t.Error("expected nil for neutral sentiment")
	}
	if detectRetailFearEvent(snap) != nil {
		t.Error("expected nil for neutral sentiment")
	}
}
