package monitoring

import "testing"

// ============================================================================
// NewTreeBasedMapper
// ============================================================================

func TestNewTreeBasedMapper_Empty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tree ClassificationTreeAccessor
	}{
		{
			name: "empty level1",
			tree: newFakeTree(nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mapper := NewTreeBasedMapper(tc.tree)
			if mapper == nil {
				t.Fatal("NewTreeBasedMapper returned nil")
			}
			if len(mapper.symbolToIndustry) != 0 {
				t.Fatalf("expected empty map, got %d entries", len(mapper.symbolToIndustry))
			}
		})
	}
}

func TestNewTreeBasedMapper_Populated(t *testing.T) {
	t.Parallel()
	segments := sampleClassificationTree()
	tree := newFakeTree(segments)

	mapper := NewTreeBasedMapper(tree)
	if mapper == nil {
		t.Fatal("NewTreeBasedMapper returned nil")
	}

	// sampleClassificationTree has 3 segments: semiconductor (2330,2317),
	// tech (2454,2382), financial (2881,2882) → 6 total entries
	if len(mapper.symbolToIndustry) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(mapper.symbolToIndustry))
	}

	// Spot-check a few known symbols
	expected := map[string]string{
		"2330": "semiconductor",
		"2317": "semiconductor",
		"2454": "tech",
		"2881": "financial",
	}
	for sym, industry := range expected {
		got, ok := mapper.symbolToIndustry[sym]
		if !ok {
			t.Errorf("symbol %s not found in map", sym)
			continue
		}
		if got != industry {
			t.Errorf("symbol %s: got industry %q, want %q", sym, got, industry)
		}
	}
}

// ============================================================================
// GetClassification
// ============================================================================

func TestGetClassification(t *testing.T) {
	t.Parallel()
	segments := sampleClassificationTree()
	tree := newFakeTree(segments)
	mapper := NewTreeBasedMapper(tree)

	tests := []struct {
		name      string
		symbol    string
		wantOK    bool
		wantID    string
		wantEmpty bool // true → expect nil, false classification
	}{
		{
			name:   "known TWSE semiconductor",
			symbol: "2330",
			wantOK: true,
			wantID: "semiconductor",
		},
		{
			name:   "known TWSE tech",
			symbol: "2317",
			wantOK: true,
			wantID: "semiconductor",
		},
		{
			name:   "known tech symbol",
			symbol: "2454",
			wantOK: true,
			wantID: "tech",
		},
		{
			name:   "TW suffix normalization",
			symbol: "2330.TW",
			wantOK: true,
			wantID: "semiconductor",
		},
		{
			name:      "unknown symbol",
			symbol:    "9999",
			wantOK:    false,
			wantEmpty: true,
		},
		{
			name:      "empty string",
			symbol:    "",
			wantOK:    false,
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := mapper.GetClassification(tc.symbol)
			if ok != tc.wantOK {
				t.Errorf("GetClassification(%q) ok = %v, want %v", tc.symbol, ok, tc.wantOK)
				return
			}
			if tc.wantEmpty {
				if got != nil {
					t.Errorf("GetClassification(%q) = %v, want nil", tc.symbol, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetClassification(%q) returned nil, want non-nil", tc.symbol)
			}
			if got.Level1.ID != tc.wantID {
				t.Errorf("GetClassification(%q).Level1.ID = %q, want %q", tc.symbol, got.Level1.ID, tc.wantID)
			}
			// Symbol field should be the normalized form
			norm := normalizeSymbol(tc.symbol)
			if got.Symbol != norm {
				t.Errorf("GetClassification(%q).Symbol = %q, want normalized %q", tc.symbol, got.Symbol, norm)
			}
		})
	}
}

// ============================================================================
// GetSymbolsByIndustry
// ============================================================================

func TestGetSymbolsByIndustry(t *testing.T) {
	t.Parallel()
	segments := sampleClassificationTree()
	tree := newFakeTree(segments)
	mapper := NewTreeBasedMapper(tree)

	tests := []struct {
		name      string
		industry  string
		wantNil   bool
		wantCount int
	}{
		{
			name:      "known industry semiconductor",
			industry:  "semiconductor",
			wantNil:   false,
			wantCount: 2, // 2330, 2317
		},
		{
			name:      "known industry tech",
			industry:  "tech",
			wantNil:   false,
			wantCount: 2, // 2454, 2382
		},
		{
			name:      "unknown industry",
			industry:  "nonexistent",
			wantNil:   true,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapper.GetSymbolsByIndustry(tc.industry)
			if tc.wantNil {
				if got != nil {
					t.Errorf("GetSymbolsByIndustry(%q) = %v, want nil", tc.industry, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetSymbolsByIndustry(%q) returned nil, want %d symbols", tc.industry, tc.wantCount)
			}
			if len(got) != tc.wantCount {
				t.Errorf("GetSymbolsByIndustry(%q) returned %d symbols, want %d", tc.industry, len(got), tc.wantCount)
			}
		})
	}
}

func TestGetSymbolsByIndustry_ZeroSymbols(t *testing.T) {
	t.Parallel()
	// Build a tree where one segment has no representative stocks
	segments := []IndustrySegment{
		{
			ID:                   "empty",
			Name:                 "Empty",
			Level:                1,
			RepresentativeStocks: nil, // zero symbols
		},
	}
	tree := newFakeTree(segments)
	mapper := NewTreeBasedMapper(tree)

	got := mapper.GetSymbolsByIndustry("empty")
	// A segment that exists but has nil RepresentativeStocks returns nil (the segment has no stocks)
	if got != nil {
		t.Errorf("GetSymbolsByIndustry(%q) = %v, want nil for empty segment", "empty", got)
	}
}
