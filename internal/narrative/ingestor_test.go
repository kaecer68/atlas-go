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

func (t *trackEventBus) PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow, explanation, sentimentExplanation string) {
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

func TestMacroIngestor_LoadPreviousSnapshot(t *testing.T) {
	dir := t.TempDir()
	ingestor := NewMacroIngestor(nil, dir) // nil provider — only testing file I/O

	may1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Unix()
	may2 := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC).Unix()
	may3 := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC).Unix()

	// Write May 1 snapshot
	snap1 := marketdata.MacroDataSnapshot{
		RecordedAt: may1,
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.5},
	}
	data1, _ := json.Marshal(snap1)
	os.WriteFile(filepath.Join(dir, "2026-05-01.json"), data1, 0o644)

	// Write May 2 snapshot
	snap2 := marketdata.MacroDataSnapshot{
		RecordedAt: may2,
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.7},
	}
	data2, _ := json.Marshal(snap2)
	os.WriteFile(filepath.Join(dir, "2026-05-02.json"), data2, 0o644)

	// Write latest.json (should be skipped by loadPreviousSnapshot)
	latestData, _ := json.Marshal(marketdata.MacroDataSnapshot{RecordedAt: may2})
	os.WriteFile(filepath.Join(dir, "latest.json"), latestData, 0o644)

	// curr has May 3 timestamp — should return May 2 (latest with RecordedAt < curr)
	curr := marketdata.MacroDataSnapshot{RecordedAt: may3}

	prev, err := ingestor.loadPreviousSnapshot(curr)
	if err != nil {
		t.Fatalf("loadPreviousSnapshot: %v", err)
	}
	if prev.RecordedAt != may2 {
		t.Fatalf("expected May 2 snapshot (RecordedAt=%d), got RecordedAt=%d", may2, prev.RecordedAt)
	}
	if prev.US10Y.Value != 4.7 {
		t.Fatalf("expected US10Y.Value=4.7, got %f", prev.US10Y.Value)
	}

	// Empty directory: should return error
	emptyIngestor := NewMacroIngestor(nil, t.TempDir())
	_, err = emptyIngestor.loadPreviousSnapshot(curr)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}

	// No snapshots with RecordedAt < curr: should return error
	noMatchDir := t.TempDir()
	noMatchIngestor := NewMacroIngestor(nil, noMatchDir)
	futureSnap := marketdata.MacroDataSnapshot{RecordedAt: may3 + 86400} // May 4
	futureData, _ := json.Marshal(futureSnap)
	os.WriteFile(filepath.Join(noMatchDir, "2026-05-04.json"), futureData, 0o644)
	_, err = noMatchIngestor.loadPreviousSnapshot(curr)
	if err == nil {
		t.Fatal("expected error when no previous snapshot found (all candidates RecordedAt >= curr)")
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

func TestMergeWithPrev_MarginMaintenanceRatio(t *testing.T) {
	// Test 1: prev MarginMaintenanceRatio propagates when curr empty
	prev := marketdata.MacroDataSnapshot{
		MarginMaintenanceRatio: marketdata.MacroDataPoint{Symbol: "TSE_MARGIN_MAINT", Value: 165.5},
	}
	curr := marketdata.MacroDataSnapshot{}
	result := mergeWithPrev(curr, prev)
	if result.MarginMaintenanceRatio.Symbol != "TSE_MARGIN_MAINT" || result.MarginMaintenanceRatio.Value != 165.5 {
		t.Errorf("mergeWithPrev: expected MarginMaintenanceRatio 165.5 from prev, got %+v", result.MarginMaintenanceRatio)
	}

	// Test 2: curr MarginMaintenanceRatio takes precedence over prev
	curr.MarginMaintenanceRatio = marketdata.MacroDataPoint{Symbol: "TSE_MARGIN_MAINT", Value: 170.0}
	result = mergeWithPrev(curr, prev)
	if result.MarginMaintenanceRatio.Value != 170.0 {
		t.Errorf("mergeWithPrev: expected curr MarginMaintenanceRatio 170.0 to override, got %+v", result.MarginMaintenanceRatio)
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

func TestComputeAICapexSentiment(t *testing.T) {
	config.ResetParametersConfig()

	tests := []struct {
		name              string
		tsmcYoYChangePct  float64
		expectedSentiment float64
	}{
		{
			name:              "below positive threshold (default 0.0) returns fallback -0.3",
			tsmcYoYChangePct:  -5.0,
			expectedSentiment: -0.3,
		},
		{
			name:              "at positive threshold (0.0) returns 0.5 (ratio=0, start of linear zone)",
			tsmcYoYChangePct:  0.0,
			expectedSentiment: 0.5,
		},
		{
			name:              "between thresholds (5.0) returns 0.65 (linear interpolation 0.5→0.8)",
			tsmcYoYChangePct:  5.0,
			expectedSentiment: 0.65,
		},
		{
			name:              "at YoY threshold (10.0) returns 0.8 (extra=0, start of upper zone)",
			tsmcYoYChangePct:  10.0,
			expectedSentiment: 0.8,
		},
		{
			name:              "above YoY threshold (35.0) returns 1.0 (extra=1.0, capped)",
			tsmcYoYChangePct:  35.0,
			expectedSentiment: 1.0,
		},
		{
			name:              "far above YoY threshold (100.0) returns 1.0 (extra capped at 1.0)",
			tsmcYoYChangePct:  100.0,
			expectedSentiment: 1.0,
		},
		{
			name:              "1.5x YoY threshold (15.0) returns 0.9 (extra=0.5)",
			tsmcYoYChangePct:  15.0,
			expectedSentiment: 0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeAICapexSentiment(tt.tsmcYoYChangePct)
			if got != tt.expectedSentiment {
				t.Fatalf("computeAICapexSentiment(%f) = %f, want %f", tt.tsmcYoYChangePct, got, tt.expectedSentiment)
			}
		})
	}
}

func TestComputeAICapexSentiment_ContinuousInterpolation(t *testing.T) {
	config.ResetParametersConfig()
	params := config.GetParametersConfig()
	params.Narrative.TSMCRevenuePositiveThreshold.Value = 10.0
	params.Narrative.TSMCRevenueYoYThreshold.Value = 30.0
	params.Narrative.AICapexFallbackSentiment.Value = -0.3

	tests := []struct {
		name              string
		tsmcYoYChangePct  float64
		expectedSentiment float64
	}{
		{
			name:              "0 returns fallback -0.3 (below positive threshold 10)",
			tsmcYoYChangePct:  0.0,
			expectedSentiment: -0.3,
		},
		{
			name:              "10 returns 0.5 (at lower bound, ratio=0)",
			tsmcYoYChangePct:  10.0,
			expectedSentiment: 0.5,
		},
		{
			name:              "20 returns 0.65 (midpoint: ratio=0.5, 0.5+0.3*0.5=0.65)",
			tsmcYoYChangePct:  20.0,
			expectedSentiment: 0.65,
		},
		{
			name:              "30 returns 0.8 (at YoY threshold, extra=0)",
			tsmcYoYChangePct:  30.0,
			expectedSentiment: 0.8,
		},
		{
			name:              "60 returns 1.0 (2x YoY, extra=1.0, capped)",
			tsmcYoYChangePct:  60.0,
			expectedSentiment: 1.0,
		},
		{
			name:              "45 returns 0.9 (1.5x YoY, extra=0.5)",
			tsmcYoYChangePct:  45.0,
			expectedSentiment: 0.9,
		},
		{
			name:              "90 returns 1.0 (3x YoY, extra=2.0 capped at 1.0)",
			tsmcYoYChangePct:  90.0,
			expectedSentiment: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeAICapexSentiment(tt.tsmcYoYChangePct)
			if got != tt.expectedSentiment {
				t.Fatalf("computeAICapexSentiment(%f) = %f, want %f", tt.tsmcYoYChangePct, got, tt.expectedSentiment)
			}
		})
	}
}

func TestDetectAICapexEventFromSnapshot(t *testing.T) {
	config.ResetParametersConfig()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	// Default config values:
	//   AICapexSentimentThreshold = 0.5
	//   ConfidenceBaseAICapex     = 0.70
	//   ConfidenceDeviationCeiling = 0.95
	//
	// computeDeviationConfidence(0.8, 0.5, 0.70, 0.95):
	//   ratio = 0.8/0.5 = 1.6 → 0.70 + (0.6)*(0.25) = 0.85
	// With prevTSMC positive ChangePct: 0.85 + 0.05 = 0.90

	prevWithPositiveChg := marketdata.MacroDataPoint{
		Symbol:    "TSMC",
		Value:     600.0,
		ChangePct: 5.0,
	}
	prevWithNegativeChg := marketdata.MacroDataPoint{
		Symbol:    "TSMC",
		Value:     600.0,
		ChangePct: -2.0,
	}
	prevEmpty := marketdata.MacroDataPoint{}

	tests := []struct {
		name               string
		sentiment          float64
		prevTSMC           marketdata.MacroDataPoint
		wantNil            bool
		expectedConfidence float64
	}{
		{
			name:               "sentiment above threshold (0.8) with positive prevTSMC boosts confidence",
			sentiment:          0.8,
			prevTSMC:           prevWithPositiveChg,
			wantNil:            false,
			expectedConfidence: 0.90,
		},
		{
			name:               "sentiment above threshold (0.8) without prevTSMC returns base confidence",
			sentiment:          0.8,
			prevTSMC:           prevEmpty,
			wantNil:            false,
			expectedConfidence: 0.85,
		},
		{
			name:               "sentiment above threshold (0.8) with negative prevTSMC no boost",
			sentiment:          0.8,
			prevTSMC:           prevWithNegativeChg,
			wantNil:            false,
			expectedConfidence: 0.85,
		},
		{
			name:      "sentiment at threshold (0.5) returns nil",
			sentiment: 0.5,
			prevTSMC:  prevWithPositiveChg,
			wantNil:   true,
		},
		{
			name:      "sentiment below threshold (0.3) returns nil",
			sentiment: 0.3,
			prevTSMC:  prevWithPositiveChg,
			wantNil:   true,
		},
		{
			name:      "sentiment strongly negative (-0.5) returns nil",
			sentiment: -0.5,
			prevTSMC:  prevWithPositiveChg,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := detectAICapexEventFromSnapshot(tt.sentiment, tt.prevTSMC, now)

			if tt.wantNil {
				if event != nil {
					t.Fatalf("expected nil event, got %+v", event)
				}
				return
			}

			if event == nil {
				t.Fatal("expected non-nil event")
			}

			if event.Theme != "AI_capex_surge" {
				t.Fatalf("expected theme AI_capex_surge, got %s", event.Theme)
			}
			if event.Region != "US" {
				t.Fatalf("expected region US, got %s", event.Region)
			}
			if event.Sentiment != 0.8 {
				t.Fatalf("expected sentiment 0.8, got %f", event.Sentiment)
			}
			if event.Confidence != tt.expectedConfidence {
				t.Fatalf("expected confidence %f, got %f", tt.expectedConfidence, event.Confidence)
			}
			if event.ConfidenceSource != "deviation_based_v1" {
				t.Fatalf("expected confidence_source deviation_based_v1, got %s", event.ConfidenceSource)
			}
			if event.CapitalFlow != "tech_capex_inflow" {
				t.Fatalf("expected capital_flow tech_capex_inflow, got %s", event.CapitalFlow)
			}
			if event.TimeWindow != "1_month" {
				t.Fatalf("expected time_window 1_month, got %s", event.TimeWindow)
			}
			if event.SourceData == nil {
				t.Fatal("expected non-nil SourceData")
			}
			if sent, ok := event.SourceData["ai_capex_sentiment"]; !ok || sent != tt.sentiment {
				t.Fatalf("expected SourceData[ai_capex_sentiment] = %f, got %v", tt.sentiment, event.SourceData["ai_capex_sentiment"])
			}
		})
	}
}

func writeMarginHistory(dir string, entries []MarginHistoryEntry) error {
	for _, e := range entries {
		file := marginHistoryFile(e)
		data, err := json.Marshal(file)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Date+"_margin.json"), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func makeRisingHistory(n int, startBalance, endBalance float64) []MarginHistoryEntry {
	entries := make([]MarginHistoryEntry, n)
	step := (endBalance - startBalance) / float64(n-1)
	for i := 0; i < n; i++ {
		entries[i] = MarginHistoryEntry{
			Date:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("20060102"),
			MarginBalance: startBalance + step*float64(i),
			ChangePct:     1.0,
		}
	}
	return entries
}

func TestDetectRetailFrenzyEventFromSnapshot(t *testing.T) {
	config.ResetParametersConfig()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("triggers frenzy when margin at high percentile with rising acceleration", func(t *testing.T) {
		dir := t.TempDir()
		history := makeRisingHistory(61, 500_000_000, 800_000_000)
		if err := writeMarginHistory(dir, history); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}

		currentValue := 810_000_000.0
		marginPt := marketdata.MacroDataPoint{Symbol: "TWSEMARGIN", Value: currentValue}
		event := detectRetailFrenzyEventFromSnapshot(marginPt, dir, now)

		if event == nil {
			t.Fatal("expected non-nil frenzy event")
		}
		if event.Theme != "retail_frenzy" {
			t.Fatalf("expected retail_frenzy theme, got %s", event.Theme)
		}
		if event.Region != "TW" {
			t.Fatalf("expected TW region, got %s", event.Region)
		}
		if event.Sentiment != 1.0 {
			t.Fatalf("expected sentiment 1.0, got %f", event.Sentiment)
		}
		if event.Confidence < 0.45 || event.Confidence > 0.9 {
			t.Fatalf("expected confidence in [0.45, 0.9], got %f", event.Confidence)
		}
		if event.ConfidenceSource != "margin_history_percentile" {
			t.Fatalf("expected confidence_source margin_history_percentile, got %s", event.ConfidenceSource)
		}
		if event.SourceData == nil {
			t.Fatal("expected non-nil SourceData")
		}
	})

	t.Run("returns nil when margin balance is empty symbol", func(t *testing.T) {
		marginPt := marketdata.MacroDataPoint{Symbol: "", Value: 1_000_000_000}
		event := detectRetailFrenzyEventFromSnapshot(marginPt, "", now)
		if event != nil {
			t.Fatal("expected nil event for empty symbol")
		}
	})

	t.Run("returns nil when margin history is insufficient", func(t *testing.T) {
		dir := t.TempDir()
		history := makeRisingHistory(20, 500_000_000, 550_000_000)
		if err := writeMarginHistory(dir, history); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}

		marginPt := marketdata.MacroDataPoint{Symbol: "TWSEMARGIN", Value: 560_000_000}
		event := detectRetailFrenzyEventFromSnapshot(marginPt, dir, now)
		if event != nil {
			t.Fatal("expected nil event when history < 30 entries")
		}
	})

	t.Run("returns nil when margin history directory does not exist", func(t *testing.T) {
		marginPt := marketdata.MacroDataPoint{Symbol: "TWSEMARGIN", Value: 1_000_000_000}
		event := detectRetailFrenzyEventFromSnapshot(marginPt, "/nonexistent/dir", now)
		if event != nil {
			t.Fatal("expected nil event when history dir doesn't exist")
		}
	})
}

func TestDetectRetailFearEventFromSnapshot(t *testing.T) {
	config.ResetParametersConfig()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("triggers fear when margin at low percentile with falling acceleration", func(t *testing.T) {
		dir := t.TempDir()
		history := makeRisingHistory(61, 500_000_000, 800_000_000)
		if err := writeMarginHistory(dir, history); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}

		currentValue := 490_000_000.0
		marginPt := marketdata.MacroDataPoint{Symbol: "TWSEMARGIN", Value: currentValue}
		event := detectRetailFearEventFromSnapshot(marginPt, dir, now)

		if event == nil {
			t.Fatal("expected non-nil fear event")
		}
		if event.Theme != "retail_fear" {
			t.Fatalf("expected retail_fear theme, got %s", event.Theme)
		}
		if event.Region != "TW" {
			t.Fatalf("expected TW region, got %s", event.Region)
		}
		if event.Sentiment != -1.0 {
			t.Fatalf("expected sentiment -1.0, got %f", event.Sentiment)
		}
		if event.ConfidenceSource != "margin_history_percentile" {
			t.Fatalf("expected confidence_source margin_history_percentile, got %s", event.ConfidenceSource)
		}
	})

	t.Run("returns nil when margin balance is empty symbol", func(t *testing.T) {
		marginPt := marketdata.MacroDataPoint{Symbol: "", Value: 100_000_000}
		event := detectRetailFearEventFromSnapshot(marginPt, "", now)
		if event != nil {
			t.Fatal("expected nil event for empty symbol")
		}
	})

	t.Run("returns nil when margin history is insufficient", func(t *testing.T) {
		dir := t.TempDir()
		history := makeRisingHistory(20, 500_000_000, 550_000_000)
		if err := writeMarginHistory(dir, history); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}

		marginPt := marketdata.MacroDataPoint{Symbol: "TWSEMARGIN", Value: 300_000_000}
		event := detectRetailFearEventFromSnapshot(marginPt, dir, now)
		if event != nil {
			t.Fatal("expected nil event when history < 30 entries")
		}
	})
}
