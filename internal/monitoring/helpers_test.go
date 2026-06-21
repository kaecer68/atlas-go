package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// Mocks
// ============================================================================

// mockFeedFetcher is a closure-based FeedFetcher impl with per-channel
// responses and errors. Used by narrative_bridge_test.go.
type mockFeedFetcher struct {
	data map[string]*FeedData
	errs map[string]error
}

func newMockFeedFetcher() *mockFeedFetcher {
	return &mockFeedFetcher{
		data: make(map[string]*FeedData),
		errs: make(map[string]error),
	}
}

func (m *mockFeedFetcher) Fetch(ctx context.Context, channelID string) (*FeedData, error) {
	if err, ok := m.errs[channelID]; ok {
		return nil, err
	}
	if d, ok := m.data[channelID]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("no mock data for channel %s", channelID)
}

// fakeTree is a richer ClassificationTreeAccessor with configurable
// industry segments. Unlike mockTree (which returns all nil), this
// supports classification lookups.
type fakeTree struct {
	level1   []IndustrySegment
	byID     map[string]IndustrySegment
	children map[string][]IndustrySegment
	paths    map[string][]IndustrySegment
}

var _ ClassificationTreeAccessor = (*fakeTree)(nil)

func newFakeTree(segments []IndustrySegment) *fakeTree {
	ft := &fakeTree{
		level1:   make([]IndustrySegment, len(segments)),
		byID:     make(map[string]IndustrySegment),
		children: make(map[string][]IndustrySegment),
		paths:    make(map[string][]IndustrySegment),
	}
	for i, seg := range segments {
		ft.level1[i] = seg
		ft.byID[seg.ID] = seg
	}
	return ft
}

func (ft *fakeTree) GetSegment(id string) (IndustrySegment, bool) {
	seg, ok := ft.byID[id]
	return seg, ok
}

func (ft *fakeTree) GetChildren(id string) []IndustrySegment {
	return ft.children[id]
}

func (ft *fakeTree) GetLevel1() []IndustrySegment {
	return ft.level1
}

func (ft *fakeTree) GetPath(id string) []IndustrySegment {
	return ft.paths[id]
}

// ============================================================================
// Helpers
// ============================================================================

// tempDir returns t.TempDir() wrapped for clarity.
func tempDir(t *testing.T) string {
	return t.TempDir()
}

// sampleRankedSymbols returns n ranked symbols with deterministic scores.
func sampleRankedSymbols(n int) []RankedSymbol {
	symbols := []RankedSymbol{
		{Symbol: "2330", Score: 0.85, Industry: "semiconductor", FactorBreakdown: map[string]float64{"pe": 0.9, "volume": 0.8}, ScoreFresh: true},
		{Symbol: "2317", Score: 0.72, Industry: "tech", FactorBreakdown: map[string]float64{"pe": 0.7, "volume": 0.75}, ScoreFresh: true},
		{Symbol: "2454", Score: 0.68, Industry: "tech", FactorBreakdown: map[string]float64{"pe": 0.65, "volume": 0.7}, ScoreFresh: false},
		{Symbol: "2881", Score: 0.55, Industry: "financial", FactorBreakdown: map[string]float64{"pe": 0.5, "volume": 0.6}, ScoreFresh: true},
		{Symbol: "1101", Score: 0.45, Industry: "cement", FactorBreakdown: map[string]float64{"pe": 0.4, "volume": 0.5}, ScoreFresh: false},
	}
	if n <= 0 {
		return nil
	}
	if n >= len(symbols) {
		out := make([]RankedSymbol, len(symbols))
		copy(out, symbols)
		return out
	}
	out := make([]RankedSymbol, n)
	copy(out, symbols[:n])
	return out
}

// sampleUniverseBuildResult returns a representative UniverseBuildResult.
func sampleUniverseBuildResult() *UniverseBuildResult {
	return &UniverseBuildResult{
		SymbolsBuilt:    500,
		SymbolsFiltered: 320,
		SymbolsRanked:   50,
		SymbolsExcluded: 5,
		FullRebuild:     false,
		Timestamp:       time.Now(),
	}
}

// sampleWatchlist returns a valid watchlist JSON content for testing
// CheckD6Expiry.
func sampleWatchlist(symbols []string, ageDays int) []byte {
	now := time.Now()
	entries := make([]D6WatchlistEntry, len(symbols))
	for i, sym := range symbols {
		firstDate := now.AddDate(0, 0, -ageDays).Format("2006-01-02")
		entries[i] = D6WatchlistEntry{
			Symbol:              sym,
			Industry:            "semiconductor",
			ConsecutiveFailures: ageDays,
			FirstFailureDate:    firstDate,
			LastCheckDate:       now.Format("2006-01-02"),
		}
	}
	wl := Watchlist{
		Version:   "1",
		Symbols:   entries,
		UpdatedAt: now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(wl)
	return data
}

// sampleClassificationTree returns a small 3-industry classification tree
// for use with fakeTree.
func sampleClassificationTree() []IndustrySegment {
	return []IndustrySegment{
		{
			ID:                   "semiconductor",
			Name:                 "半導體",
			NameEN:               "Semiconductor",
			Level:                1,
			Weight:               0.35,
			Cyclicality:          "high",
			RepresentativeStocks: []string{"2330", "2317"},
		},
		{
			ID:                   "tech",
			Name:                 "科技",
			NameEN:               "Technology",
			Level:                1,
			Weight:               0.25,
			Cyclicality:          "high",
			RepresentativeStocks: []string{"2454", "2382"},
		},
		{
			ID:                   "financial",
			Name:                 "金融",
			NameEN:               "Financial",
			Level:                1,
			Weight:               0.15,
			Cyclicality:          "medium",
			RepresentativeStocks: []string{"2881", "2882"},
		},
	}
}

// TestHelpersSmoke is a compile-time smoke test that verifies all helpers
// are well-formed. It is not a functional test; it merely forces staticcheck
// to recognise that helpers are referenced.
func TestHelpersSmoke(t *testing.T) {
	// FeedFetcher mock
	fetcher := newMockFeedFetcher()
	fetcher.data["ch"] = &FeedData{Data: []byte("<rss/>")}
	fetcher.errs["err_ch"] = fmt.Errorf("mock error")
	ctx := context.Background()
	if _, err := fetcher.Fetch(ctx, "err_ch"); err == nil {
		t.Fatal("expected error for err_ch")
	}
	if d, err := fetcher.Fetch(ctx, "ch"); err != nil || d == nil {
		t.Fatalf("unexpected failure for ch: err=%v, d=%v", err, d)
	}

	// fakeTree
	tree := newFakeTree(sampleClassificationTree())
	if len(tree.GetLevel1()) != 3 {
		t.Fatalf("expected 3 level1 segments, got %d", len(tree.GetLevel1()))
	}
	if _, ok := tree.GetSegment("nonexistent"); ok {
		t.Fatal("GetSegment should return false for nonexistent id")
	}

	// Helpers
	if d := tempDir(t); d == "" {
		t.Fatal("tempDir returned empty string")
	}
	if n := len(sampleRankedSymbols(3)); n != 3 {
		t.Fatalf("sampleRankedSymbols(3) returned %d, want 3", n)
	}
	if r := sampleUniverseBuildResult(); r == nil {
		t.Fatal("sampleUniverseBuildResult returned nil")
	}
	if w := sampleWatchlist([]string{"A", "B"}, 5); len(w) == 0 {
		t.Fatal("sampleWatchlist returned empty")
	}
	if s := sampleClassificationTree(); len(s) != 3 {
		t.Fatalf("sampleClassificationTree returned %d segments, want 3", len(s))
	}
}
