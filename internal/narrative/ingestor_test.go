package narrative

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type trackEventBus struct {
	mu         sync.Mutex
	published  []string
}

func (t *trackEventBus) PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.published = append(t.published, theme)
	return nil
}

func TestMacroIngestorLifecycleGatesDuplicateTheme(t *testing.T) {
	dir := t.TempDir()
	bus := &trackEventBus{}

	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "existing-1", Theme: "US_rates_up", Status: "active", Timestamp: now,
		Duration: 7 * 24 * time.Hour,
	})

	mock := &marketdata.MockMacroProvider{
		Snapshot: marketdata.MacroDataSnapshot{
			US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 150, ChangePct: 2.0},
			DXY:   marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 105, ChangePct: 1.8},
		},
	}
	ingestor := NewMacroIngestor(mock, dir)
	ingestor.SetLifecycleManager(lm)
	ingestor.SetEventBus(bus)

	events, _, err := ingestor.Ingest(context.Background())
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	foundUSRates := false
	for _, e := range events {
		if e.Theme == "US_rates_up" {
			foundUSRates = true
		}
	}
	if !foundUSRates {
		t.Fatal("expected US_rates_up in returned events (detection should still work)")
	}

	bus.mu.Lock()
	published := make([]string, len(bus.published))
	copy(published, bus.published)
	bus.mu.Unlock()

	for _, theme := range published {
		if theme == "US_rates_up" {
			t.Fatal("lifecycle gate FAILED: US_rates_up was published even though it's already active")
		}
	}
}

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
