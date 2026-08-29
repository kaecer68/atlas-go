package monitoring

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// rssXML returns a minimal RSS feed XML with the given title and description.
func rssXML(title, description string) []byte {
	return []byte(fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><item><title>%s</title><description>%s</description></item></channel></rss>`,
		title, description,
	))
}

// TestNarrativeEventBridge_Scrape covers the Scrape function: nil fetcher,
// successful parse, stale feed passthrough, per-channel error skip, and
// empty channel list.
func TestNarrativeEventBridge_Scrape(t *testing.T) {
	ctx := context.Background()

	t.Run("nil fetcher returns error", func(t *testing.T) {
		b := NewNarrativeEventBridgeWithFetcher("", nil)
		events, err := b.Scrape(ctx)
		if err == nil {
			t.Fatal("expected error for nil fetcher, got nil")
		}
		if !strings.Contains(err.Error(), "fetcher not set") {
			t.Errorf("expected 'fetcher not set' in error, got: %v", err)
		}
		if events != nil {
			t.Errorf("expected nil events on error, got %d", len(events))
		}
	})

	t.Run("fetcher returns data, events parsed correctly", func(t *testing.T) {
		mock := newMockFeedFetcher()
		mock.data["geopolitical_taiwan"] = &FeedData{
			Data: rssXML("CoWoS 產能擴充", "NVIDIA GB300 訂單大增"),
		}
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return mock.Fetch(ctx, channelID)
		})
		// Explicitly ensure channelIDs match mock key.
		b.channelIDs = []string{"geopolitical_taiwan"}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("expected at least 1 event, got 0 — keywords may not match RSS content")
		}
		// "CoWoS" should map to packaging_testing with bias 0.25.
		foundCoWoS := false
		foundGB300 := false
		for _, e := range events {
			if e.Keyword == "CoWoS" && e.IndustryID == "packaging_testing" {
				foundCoWoS = true
				if math.Abs(e.Bias-0.25) > 1e-9 {
					t.Errorf("CoWoS bias = %v, want 0.25", e.Bias)
				}
			}
			if e.Keyword == "GB300" && e.IndustryID == "ai_server" {
				foundGB300 = true
				if math.Abs(e.Bias-0.30) > 1e-9 {
					t.Errorf("GB300 bias = %v, want 0.30", e.Bias)
				}
			}
		}
		if !foundCoWoS {
			t.Error("expected CoWoS keyword event, not found")
		}
		if !foundGB300 {
			t.Error("expected GB300 keyword event, not found")
		}
	})

	t.Run("stale feed passes through (no filter on Stale flag)", func(t *testing.T) {
		// Note: Scrape does NOT filter stale feeds — it only logs a warning.
		// This test verifies current behaviour.
		mock := newMockFeedFetcher()
		mock.data["geopolitical_taiwan"] = &FeedData{
			Data:      rssXML("DRAM 價格上漲", "記憶體需求回升"),
			Stale:     true,
			LastError: "connection timeout during refresh",
		}
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return mock.Fetch(ctx, channelID)
		})
		b.channelIDs = []string{"geopolitical_taiwan"}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "DRAM" keyword should match "memory" industry.
		found := false
		for _, e := range events {
			if e.Keyword == "DRAM" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected DRAM event from stale feed, got none — stale flag does not block parsing")
		}
	})

	t.Run("fetch error per channel, others succeed", func(t *testing.T) {
		mock := newMockFeedFetcher()
		mock.data["good"] = &FeedData{
			Data: rssXML("HBM 供不應求", "高頻寬記憶體短缺"),
		}
		mock.errs["bad"] = fmt.Errorf("simulated network error")
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return mock.Fetch(ctx, channelID)
		})
		// Override channels: good channel + failing channel.
		b.channelIDs = []string{"good", "bad"}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		foundHBM := false
		for _, e := range events {
			if e.Keyword == "HBM" {
				foundHBM = true
				break
			}
		}
		if !foundHBM {
			t.Error("expected HBM event from 'good' channel after 'bad' channel failed")
		}
	})

	t.Run("empty channelIDs returns empty list", func(t *testing.T) {
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return &FeedData{Data: rssXML("CPI 數據公布", "通膨趨緩")}, nil
		})
		b.channelIDs = []string{}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected 0 events for empty channelIDs, got %d", len(events))
		}
	})

	t.Run("xml parse failure skipped with warning", func(t *testing.T) {
		mock := newMockFeedFetcher()
		mock.data["geopolitical_taiwan"] = &FeedData{
			Data: []byte("this is not valid xml <<< >"),
		}
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return mock.Fetch(ctx, channelID)
		})
		b.channelIDs = []string{"geopolitical_taiwan"}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected 0 events for invalid XML, got %d", len(events))
		}
	})

	t.Run("duplicate keyword+industry deduplicated", func(t *testing.T) {
		// Two RSS items both containing "CoWoS" → dedup should merge to one.
		mock := newMockFeedFetcher()
		mock.data["geopolitical_taiwan"] = &FeedData{
			Data: []byte(`<?xml version="1.0"?><rss><channel>
<item><title>CoWoS 產能滿載</title><description>先進封裝需求強勁</description></item>
<item><title>CoWoS 擴產計畫</title><description>台積電加速擴產</description></item>
</channel></rss>`),
		}
		b := newNarrativeEventBridge("")
		b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
			return mock.Fetch(ctx, channelID)
		})
		b.channelIDs = []string{"geopolitical_taiwan"}

		events, err := b.Scrape(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cowosCount := 0
		for _, e := range events {
			if e.Keyword == "CoWoS" {
				cowosCount++
			}
		}
		if cowosCount != 1 {
			t.Errorf("expected 1 CoWoS event after dedup, got %d", cowosCount)
		}
	})
}

// TestNarrativeEventBridge_SaveLoadCache covers the SaveCache + LoadCache
// roundtrip, including missing directory creation, corrupt cache handling,
// and missing file fallback.
func TestNarrativeEventBridge_SaveLoadCache(t *testing.T) {
	sampleEvents := []NarrativeEvent{
		{
			Keyword:    "HBM",
			IndustryID: "memory",
			EventType:  "order_news",
			HitCount:   3,
			Sources:    []string{"ch1"},
			DetectedAt: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
			Bias:       0.20,
			Confidence: 0.85,
		},
		{
			Keyword:    "核能",
			IndustryID: "energy",
			EventType:  "policy",
			HitCount:   5,
			Sources:    []string{"ch2"},
			DetectedAt: time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC),
			Bias:       0.15,
			Confidence: 0.60,
		},
	}

	t.Run("happy roundtrip", func(t *testing.T) {
		dir := tempDir(t)
		cachePath := filepath.Join(dir, "narrative_cache.json")
		b := NewNarrativeEventBridgeWithFetcher(cachePath, nil)

		if err := b.SaveCache(sampleEvents); err != nil {
			t.Fatalf("SaveCache failed: %v", err)
		}

		loaded, err := b.LoadCache()
		if err != nil {
			t.Fatalf("LoadCache failed: %v", err)
		}
		if len(loaded) != len(sampleEvents) {
			t.Fatalf("roundtrip length mismatch: wrote %d, read %d", len(sampleEvents), len(loaded))
		}
		for i := range sampleEvents {
			if loaded[i].Keyword != sampleEvents[i].Keyword {
				t.Errorf("event[%d] Keyword: got %q, want %q", i, loaded[i].Keyword, sampleEvents[i].Keyword)
			}
			if loaded[i].IndustryID != sampleEvents[i].IndustryID {
				t.Errorf("event[%d] IndustryID: got %q, want %q", i, loaded[i].IndustryID, sampleEvents[i].IndustryID)
			}
			if math.Abs(loaded[i].Bias-sampleEvents[i].Bias) > 1e-9 {
				t.Errorf("event[%d] Bias: got %v, want %v", i, loaded[i].Bias, sampleEvents[i].Bias)
			}
		}
	})

	t.Run("missing dir creation", func(t *testing.T) {
		dir := tempDir(t)
		// Use a nested path whose parent does NOT exist yet.
		cachePath := filepath.Join(dir, "deep", "nested", "cache.json")
		b := NewNarrativeEventBridgeWithFetcher(cachePath, nil)

		if err := b.SaveCache(sampleEvents); err != nil {
			t.Fatalf("SaveCache with missing dirs failed: %v", err)
		}
		// Verify the file exists on disk.
		if _, err := os.Stat(cachePath); err != nil {
			t.Errorf("cache file not created at %q: %v", cachePath, err)
		}
	})

	t.Run("corrupt cache parse error", func(t *testing.T) {
		dir := tempDir(t)
		cachePath := filepath.Join(dir, "bad.json")
		// Write invalid JSON directly (bypass SaveCache).
		if err := os.WriteFile(cachePath, []byte("this is not json{{{{"), 0o640); err != nil {
			t.Fatalf("write corrupt file: %v", err)
		}
		b := NewNarrativeEventBridgeWithFetcher(cachePath, nil)
		_, err := b.LoadCache()
		if err == nil {
			t.Fatal("expected parse error for corrupt cache, got nil")
		}
	})

	t.Run("missing file returns empty slice", func(t *testing.T) {
		dir := tempDir(t)
		cachePath := filepath.Join(dir, "does_not_exist.json")
		b := NewNarrativeEventBridgeWithFetcher(cachePath, nil)
		events, err := b.LoadCache()
		if err != nil {
			t.Fatalf("LoadCache for missing file should not error: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected empty slice for missing file, got %d events", len(events))
		}
	})
}

// TestNarrativeEventBridge_MapIndustries tests keyword-to-industry mapping with
// dedup, empty input, and unknown keyword fallback.
func TestNarrativeEventBridge_MapIndustries(t *testing.T) {
	b := newNarrativeEventBridge("")

	t.Run("dedup and sort by industry then keyword", func(t *testing.T) {
		// "CoWoS" maps to packaging_testing; "GB300" to ai_server.
		// Order should be ai_server < packaging_testing alphabetically.
		result := b.MapIndustries([]string{"CoWoS", "GB300", "CoWoS"})
		if len(result) != 2 {
			t.Fatalf("expected 2 deduplicated entries, got %d", len(result))
		}
		// Verify sort: ai_server < packaging_testing.
		if result[0].IndustryID != "ai_server" {
			t.Errorf("first entry should be ai_server, got %s", result[0].IndustryID)
		}
		if result[1].IndustryID != "packaging_testing" {
			t.Errorf("second entry should be packaging_testing, got %s", result[1].IndustryID)
		}
	})

	t.Run("empty keywords returns empty slice", func(t *testing.T) {
		result := b.MapIndustries([]string{})
		if len(result) != 0 {
			t.Errorf("expected empty result for empty input, got %d", len(result))
		}
	})

	t.Run("unknown keyword graceful fallback", func(t *testing.T) {
		result := b.MapIndustries([]string{"ZZZ_NOT_A_KEYWORD"})
		if len(result) != 0 {
			t.Errorf("expected empty result for unknown keyword, got %d", len(result))
		}
	})

	t.Run("same industry secondary sort by keyword", func(t *testing.T) {
		// "航運" and "SCFI" both map to "shipping" — secondary sort is by keyword.
		result := b.MapIndustries([]string{"航運", "SCFI"})
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(result))
		}
		if result[0].IndustryID != "shipping" || result[1].IndustryID != "shipping" {
			t.Error("both entries should have IndustryID=shipping")
		}
		// SCFI < 航運 lexicographically (ASCII: 'S' < '航' in UTF-8 but Go bytes compare).
		// Verify they are sorted.
		if result[0].Keyword >= result[1].Keyword {
			t.Errorf("expected sorted keywords, got %q before %q", result[0].Keyword, result[1].Keyword)
		}
	})
}

// TestNarrativeEventBridge_SetFetcher verifies that SetFetcher correctly
// swaps the fetcher so subsequent Scrape calls use the new implementation.
func TestNarrativeEventBridge_SetFetcher(t *testing.T) {
	b := newNarrativeEventBridge("")
	b.channelIDs = []string{"ch"}
	ctx := context.Background()

	// Initially nil fetcher → Scrape errors.
	_, err := b.Scrape(ctx)
	if err == nil {
		t.Fatal("expected error with nil fetcher")
	}

	// Set a valid fetcher → Scrape succeeds.
	b.SetFetcher(FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
		return &FeedData{Data: rssXML("HBM", "memory")}, nil
	}))
	events, err := b.Scrape(ctx)
	if err != nil {
		t.Fatalf("unexpected error after SetFetcher: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected events after setting valid fetcher")
	}
}

// TestNarrativeEventBridge_ApplyDecay tests the exponential decay function
// with zero elapsed, large elapsed, lambda=0 boundary, unknown event type,
// and future-detected event clamping.
func TestNarrativeEventBridge_ApplyDecay(t *testing.T) {
	b := newNarrativeEventBridge("")
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("zero elapsed time retains full bias", func(t *testing.T) {
		event := NarrativeEvent{
			EventType:  "order_news",
			Bias:       0.25,
			DetectedAt: now, // same as now
		}
		decayed := b.ApplyDecay(event, now)
		if math.Abs(decayed-0.25) > 1e-9 {
			t.Errorf("expected full bias 0.25 for zero elapsed, got %v", decayed)
		}
	})

	t.Run("large elapsed time approaches zero", func(t *testing.T) {
		// Use 7 days elapsed with order_news lambda (1/43200).
		event := NarrativeEvent{
			EventType:  "order_news",
			Bias:       0.25,
			DetectedAt: now.Add(-7 * 24 * time.Hour),
		}
		decayed := b.ApplyDecay(event, now)
		// After 7 days, exp(-lambda * 604800) ≈ exp(-14) ≈ 8.3e-7.
		if decayed > 1e-6 {
			t.Errorf("expected near-zero decay for 7-day-old event, got %v", decayed)
		}
	})

	t.Run("lambda zero boundary retains full bias", func(t *testing.T) {
		// Override decayLambda to test lambda=0 (no decay).
		b.decayLambda = map[string]float64{"macro": 0.0}
		event := NarrativeEvent{
			EventType:  "macro",
			Bias:       0.30,
			DetectedAt: now.Add(-100 * 24 * time.Hour), // 100 days ago
		}
		decayed := b.ApplyDecay(event, now)
		if math.Abs(decayed-0.30) > 1e-9 {
			t.Errorf("lambda=0 should retain full bias 0.30, got %v", decayed)
		}
	})

	t.Run("future event elapsed clamped to zero", func(t *testing.T) {
		b.decayLambda = defaultDecayLambdas
		// Event detected in the future relative to "now".
		event := NarrativeEvent{
			EventType:  "order_news",
			Bias:       0.25,
			DetectedAt: now.Add(1 * time.Hour),
		}
		decayed := b.ApplyDecay(event, now)
		if math.Abs(decayed-0.25) > 1e-9 {
			t.Errorf("future event should be clamped to elapsed=0, got %v, want 0.25", decayed)
		}
	})

	t.Run("unknown event type defaults to macro lambda", func(t *testing.T) {
		// Restore default lambdas and use an event type not in the map.
		b.decayLambda = defaultDecayLambdas
		event := NarrativeEvent{
			EventType:  "unknown_type",
			Bias:       0.20,
			DetectedAt: now.Add(-24 * time.Hour),
		}
		decayed := b.ApplyDecay(event, now)
		// With macro lambda (1/86400) and 86400 seconds:
		// exp(-(1/86400)*86400) = exp(-1) ≈ 0.3679.
		expected := 0.20 * math.Exp(-1)
		if math.Abs(decayed-expected) > 1e-9 {
			t.Errorf("unknown type should use macro lambda: got %v, want %v", decayed, expected)
		}
	})
}

// TestNarrativeEventBridge_ComputeConfidence tests the confidence ratio
// calculation with threshold=0, below-threshold, and at-threshold cases.
func TestNarrativeEventBridge_ComputeConfidence(t *testing.T) {
	b := newNarrativeEventBridge("")

	t.Run("threshold zero returns 1.0 saturated", func(t *testing.T) {
		// Override threshold to 0 via Configure.
		cfg := config.SmartUniverseConfig{
			ConfidenceThreshold: config.ParameterMetadata[int]{Value: 0},
		}
		b.Configure(cfg)
		if got := b.ComputeConfidence(0); math.Abs(got-1.0) > 1e-9 {
			t.Errorf("threshold=0, hitCount=0: expected 1.0, got %v", got)
		}
		if got := b.ComputeConfidence(100); math.Abs(got-1.0) > 1e-9 {
			t.Errorf("threshold=0, hitCount=100: expected 1.0, got %v", got)
		}
	})

	t.Run("hitCount below threshold returns ratio", func(t *testing.T) {
		cfg := config.SmartUniverseConfig{
			ConfidenceThreshold: config.ParameterMetadata[int]{Value: 10},
		}
		b.Configure(cfg)
		got := b.ComputeConfidence(3)
		expected := 3.0 / 10.0
		if math.Abs(got-expected) > 1e-9 {
			t.Errorf("hitCount=3, threshold=10: expected %v, got %v", expected, got)
		}
	})

	t.Run("hitCount at or above threshold returns 1.0", func(t *testing.T) {
		cfg := config.SmartUniverseConfig{
			ConfidenceThreshold: config.ParameterMetadata[int]{Value: 10},
		}
		b.Configure(cfg)
		if got := b.ComputeConfidence(10); math.Abs(got-1.0) > 1e-9 {
			t.Errorf("hitCount=10 == threshold=10: expected 1.0, got %v", got)
		}
		if got := b.ComputeConfidence(15); math.Abs(got-1.0) > 1e-9 {
			t.Errorf("hitCount=15 > threshold=10: expected 1.0, got %v", got)
		}
	})
}

// TestNarrativeEventBridge_StockWeightBias tests proportional stock weight
// allocation with zero-total guard and normal case.
func TestNarrativeEventBridge_StockWeightBias(t *testing.T) {
	b := newNarrativeEventBridge("")

	t.Run("zero total industry score returns zero", func(t *testing.T) {
		got := b.StockWeightBias(0.25, 0.5, 0.0)
		if math.Abs(got-0.0) > 1e-9 {
			t.Errorf("total=0 should return 0, got %v", got)
		}
	})

	t.Run("proportional allocation with non-zero total", func(t *testing.T) {
		// industryBias=0.30, stockScore=0.25, totalIndustryScore=0.75
		// => 0.30 * (0.25/0.75) = 0.30 * 0.333... = 0.10
		got := b.StockWeightBias(0.30, 0.25, 0.75)
		expected := 0.30 * (0.25 / 0.75)
		if math.Abs(got-expected) > 1e-9 {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("stock score zero, total non-zero yields zero bias", func(t *testing.T) {
		got := b.StockWeightBias(0.30, 0.0, 0.75)
		if math.Abs(got-0.0) > 1e-9 {
			t.Errorf("stockScore=0 should yield 0, got %v", got)
		}
	})
}

// TestNarrativeEventBridge_BuildAdjustment tests industry-level adjustment
// aggregation for single and multiple events.
func TestNarrativeEventBridge_BuildAdjustment(t *testing.T) {
	b := newNarrativeEventBridge("")
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("single event produces simple adjustment", func(t *testing.T) {
		events := []NarrativeEvent{
			{
				IndustryID: "memory",
				Keyword:    "HBM",
				EventType:  "order_news",
				HitCount:   5,
				Bias:       0.20,
				DetectedAt: now,
			},
		}
		adj := b.BuildAdjustment("memory", events, now)
		if adj.ActiveTheme != "HBM" {
			t.Errorf("ActiveTheme: got %q, want %q", adj.ActiveTheme, "HBM")
		}
		if adj.Confidence <= 0 {
			t.Errorf("Confidence should be > 0 for hitCount=5, got %v", adj.Confidence)
		}
		if math.Abs(adj.ProfitBias-adj.RevenueBias*defaultProfitBiasRatio) > 1e-9 {
			t.Errorf("ProfitBias (%v) should equal RevenueBias (%v) * defaultProfitBiasRatio (%v)",
				adj.ProfitBias, adj.RevenueBias, defaultProfitBiasRatio)
		}
	})

	t.Run("multi-event aggregation with sum/avg semantics", func(t *testing.T) {
		events := []NarrativeEvent{
			{
				IndustryID: "memory",
				Keyword:    "HBM",
				EventType:  "order_news",
				HitCount:   3,
				Bias:       0.20,
				DetectedAt: now,
			},
			{
				IndustryID: "memory",
				Keyword:    "DRAM",
				EventType:  "order_news",
				HitCount:   2,
				Bias:       0.15,
				DetectedAt: now,
			},
			// This event is for a different industry — should be excluded.
			{
				IndustryID: "energy",
				Keyword:    "核能",
				EventType:  "policy",
				HitCount:   4,
				Bias:       0.30,
				DetectedAt: now,
			},
		}
		adj := b.BuildAdjustment("memory", events, now)

		// RevenueBias should be sum of weighted biases (decayed * confidence).
		// With zero elapsed, decayed == bias. Threshold=3 (default):
		//   HBM: 0.20 * 1.0 = 0.20  (hitCount=3 → 3/3 → 1.0)
		//   DRAM: 0.15 * (2/3) = 0.10
		//   total ≈ 0.30
		expectedRevenue := 0.20*1.0 + 0.15*(2.0/3.0)
		if math.Abs(adj.RevenueBias-expectedRevenue) > 1e-6 {
			t.Errorf("RevenueBias: got %v, want %v", adj.RevenueBias, expectedRevenue)
		}
		// ActiveTheme should be the keyword with highest confidence (HBM, confidence=1.0).
		if adj.ActiveTheme != "HBM" {
			t.Errorf("ActiveTheme: got %q, want %q", adj.ActiveTheme, "HBM")
		}
	})

	t.Run("no matching industry returns zero adjustment", func(t *testing.T) {
		events := []NarrativeEvent{
			{
				IndustryID: "energy",
				Keyword:    "核能",
				EventType:  "policy",
				HitCount:   5,
				Bias:       0.30,
				DetectedAt: now,
			},
		}
		adj := b.BuildAdjustment("memory", events, now)
		if adj != (industry.NarrativeAdjustment{}) {
			t.Errorf("expected zero adjustment for no-match, got %+v", adj)
		}
	})
}

// TestNarrativeEventBridge_Scrape_Configure_Race verifies that concurrent
// Scrape (reader) and Configure (writer) calls do not race. It uses 10
// goroutines — 5 calling Scrape and 5 calling Configure — and is designed
// to be run with -race.
func TestNarrativeEventBridge_Scrape_Configure_Race(t *testing.T) {
	mock := newMockFeedFetcher()
	mock.data["geopolitical_taiwan"] = &FeedData{
		Data: rssXML("CPI 數據", "通膨數據公布"),
	}
	b := newNarrativeEventBridge("")
	b.feedFetcher = FeedFetcher(func(ctx context.Context, channelID string) (*FeedData, error) {
		return mock.Fetch(ctx, channelID)
	})
	b.channelIDs = []string{"geopolitical_taiwan"}

	cfg := config.SmartUniverseConfig{
		ConfidenceThreshold: config.ParameterMetadata[int]{Value: 5},
	}

	var wg sync.WaitGroup
	const goroutines = 10
	for i := range goroutines {
		wg.Add(1)
		if i%2 == 0 {
			// Scrape goroutines.
			go func() {
				defer wg.Done()
				_, _ = b.Scrape(context.Background())
			}()
		} else {
			// Configure goroutines.
			go func() {
				defer wg.Done()
				b.Configure(cfg)
			}()
		}
	}
	wg.Wait()
}
