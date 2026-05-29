package narrative

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type trackEventBus struct {
	mu        sync.Mutex
	published []string
}

func (t *trackEventBus) PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.published = append(t.published, theme)
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
	config.ResetParametersConfig()
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

func TestHasValidYahooData(t *testing.T) {
	empty := marketdata.MacroDataSnapshot{}
	if hasValidYahooData(empty) {
		t.Fatal("expected false for empty snapshot")
	}

	onlyUS10Y := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX"},
	}
	if !hasValidYahooData(onlyUS10Y) {
		t.Fatal("expected true when US10Y has symbol")
	}

	onlyDXY := marketdata.MacroDataSnapshot{
		DXY: marketdata.MacroDataPoint{Symbol: "DX-Y.NYB"},
	}
	if !hasValidYahooData(onlyDXY) {
		t.Fatal("expected true when DXY has symbol")
	}

	onlyVIX := marketdata.MacroDataSnapshot{
		VIX: marketdata.MacroDataPoint{Symbol: "^VIX"},
	}
	if !hasValidYahooData(onlyVIX) {
		t.Fatal("expected true when VIX has symbol")
	}
}

func TestLoadFallbackDatedSnapshotNoFiles(t *testing.T) {
	dir := t.TempDir()
	ingestor := NewMacroIngestor(&marketdata.MockMacroProvider{}, dir)

	_, err := ingestor.loadFallbackDatedSnapshot()
	if err == nil {
		t.Fatal("expected error when no dated snapshots exist")
	}
}

func TestLoadFallbackDatedSnapshotSkipsLatest(t *testing.T) {
	dir := t.TempDir()

	invalidLatest := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "", Value: 0},
	}
	data, _ := json.Marshal(invalidLatest)
	os.WriteFile(filepath.Join(dir, "latest.json"), data, 0o644)

	validDated := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.5},
		DXY:   marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 105},
	}
	datedData, _ := json.Marshal(validDated)
	os.WriteFile(filepath.Join(dir, "2026-05-11.json"), datedData, 0o644)

	ingestor := NewMacroIngestor(&marketdata.MockMacroProvider{}, dir)
	snap, err := ingestor.loadFallbackDatedSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.US10Y.Symbol != "^TNX" {
		t.Fatalf("expected US10Y symbol ^TNX from dated fallback, got %s", snap.US10Y.Symbol)
	}
}

func TestLoadFallbackDatedSnapshotPicksNewest(t *testing.T) {
	dir := t.TempDir()

	older := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.5},
	}
	olderData, _ := json.Marshal(older)
	olderPath := filepath.Join(dir, "2026-05-10.json")
	os.WriteFile(olderPath, olderData, 0o644)
	os.Chtimes(olderPath, time.Now(), time.Now().Add(-2*time.Hour))

	newer := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 5.0},
	}
	newerData, _ := json.Marshal(newer)
	newerPath := filepath.Join(dir, "2026-05-11.json")
	os.WriteFile(newerPath, newerData, 0o644)

	ingestor := NewMacroIngestor(&marketdata.MockMacroProvider{}, dir)
	snap, err := ingestor.loadFallbackDatedSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.US10Y.Value != 5.0 {
		t.Fatalf("expected newest snapshot (5.0), got %f", snap.US10Y.Value)
	}
}

func TestMergeWithPrev_BDI(t *testing.T) {
	// Test 1: prev BDI propagates when curr empty
	prev := marketdata.MacroDataSnapshot{
		Bdi: marketdata.MacroDataPoint{Symbol: ".BADI", Value: 1500.0},
	}
	curr := marketdata.MacroDataSnapshot{}
	result := mergeWithPrev(curr, prev)
	if result.Bdi.Symbol != ".BADI" || result.Bdi.Value != 1500.0 {
		t.Errorf("mergeWithPrev: expected BDI 1500 from prev, got %+v", result.Bdi)
	}

	// Test 2: curr BDI takes precedence over prev
	curr.Bdi = marketdata.MacroDataPoint{Symbol: ".BADI", Value: 1600.0}
	result = mergeWithPrev(curr, prev)
	if result.Bdi.Value != 1600.0 {
		t.Errorf("mergeWithPrev: expected curr BDI 1600 to override, got %+v", result.Bdi)
	}
}

func TestSnapshotDirAccessor(t *testing.T) {
	dir := t.TempDir()
	ingestor := NewMacroIngestor(&marketdata.MockMacroProvider{}, dir)
	if ingestor.SnapshotDir() != dir {
		t.Fatalf("expected %s, got %s", dir, ingestor.SnapshotDir())
	}
}

func TestHasValidYahooDataAndIngestChain(t *testing.T) {
	dir := t.TempDir()

	validDated := marketdata.MacroDataSnapshot{
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.5, ChangePct: 0.1},
		VIX:   marketdata.MacroDataPoint{Symbol: "^VIX", Value: 15, ChangePct: 0},
	}
	data, _ := json.Marshal(validDated)
	os.WriteFile(filepath.Join(dir, "2026-05-11.json"), data, 0o644)

	invalidLatest := marketdata.MacroDataSnapshot{}
	invalidData, _ := json.Marshal(invalidLatest)
	os.WriteFile(filepath.Join(dir, "latest.json"), invalidData, 0o644)

	mock := &marketdata.MockMacroProvider{
		Snapshot: marketdata.MacroDataSnapshot{},
	}
	ingestor := NewMacroIngestor(mock, dir)
	snap, err := ingestor.loadLatestSnapshot()
	if err != nil {
		t.Fatalf("loadLatestSnapshot should fallback to dated: %v", err)
	}
	if snap.US10Y.Symbol != "^TNX" {
		t.Fatalf("expected fallback US10Y ^TNX, got %s", snap.US10Y.Symbol)
	}
}
