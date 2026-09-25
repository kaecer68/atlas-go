package monitoring

import (
	"context"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// ─── issue #1943: per-stock industry substrate as the universe population ───

// fakeSubstrate is an in-memory industry.SymbolIndustrySubstrate.
type fakeSubstrate struct {
	bySymbol map[string]industry.SectorID
	symbols  []string
	lookups  int
}

func newFakeSubstrate(entries map[string]industry.SectorID) *fakeSubstrate {
	syms := make([]string, 0, len(entries))
	for s := range entries {
		syms = append(syms, s)
	}
	slices.Sort(syms)
	return &fakeSubstrate{bySymbol: entries, symbols: syms}
}

func (f *fakeSubstrate) ResolveL1(symbol string) (industry.SectorID, bool) {
	f.lookups++
	id, ok := f.bySymbol[symbol]
	return id, ok
}

func (f *fakeSubstrate) Symbols() []string { return slices.Clone(f.symbols) }

// TestSubstratePopulation_NormalizesDedupesSorts pins the population helper:
// the substrate's own list is the answer, but it must never be trusted to be
// already normalized, and an empty substrate must yield nil (so the caller can
// fall back to the tree instead of building an empty universe).
func TestSubstratePopulation_NormalizesDedupesSorts(t *testing.T) {
	t.Run("nil_substrate_is_nil", func(t *testing.T) {
		if got := substratePopulation(nil); got != nil {
			t.Fatalf("substratePopulation(nil) = %v, want nil", got)
		}
	})

	t.Run("empty_substrate_is_nil", func(t *testing.T) {
		if got := substratePopulation(newFakeSubstrate(nil)); got != nil {
			t.Fatalf("substratePopulation(empty) = %v, want nil", got)
		}
	})

	t.Run("normalized_deduped_sorted", func(t *testing.T) {
		sub := &fakeSubstrate{symbols: []string{" 2603 ", "2330.TW", "2330", "", "1101"}}
		got := substratePopulation(sub)
		want := []string{"1101", "2330", "2603"}
		if !slices.Equal(got, want) {
			t.Fatalf("substratePopulation = %v, want %v", got, want)
		}
	})
}

// TestGatherAllSymbols_UsesSubstrate covers the population switch itself: with a
// substrate the whole listed market is the population, without one the tree's
// representative stocks answer exactly as before.
func TestGatherAllSymbols_UsesSubstrate(t *testing.T) {
	tree := newFakeTree([]IndustrySegment{
		{ID: "semiconductor", Level: 1, RepresentativeStocks: []string{"2330"}},
		{ID: "tech", Level: 1, RepresentativeStocks: []string{"2317"}},
	})
	mapper := NewTreeBasedMapper(tree)

	without := gatherAllSymbols(tree, mapper, nil)
	if !slices.Equal(without, []string{"2330", "2317"}) {
		t.Fatalf("gate-off population = %v, want the tree's representative stocks", without)
	}

	sub := newFakeSubstrate(map[string]industry.SectorID{
		"2330": "semiconductor",
		"2317": "electronics",
		"2603": "shipping",
		"2881": "financials",
	})
	with := gatherAllSymbols(tree, mapper, sub)
	want := []string{"2317", "2330", "2603", "2881"}
	if !slices.Equal(with, want) {
		t.Fatalf("gate-on population = %v, want %v", with, want)
	}
}

// TestSubstrateIndustryMapper_CanonicalFirstThenTree pins the resolution order:
// the substrate answers with its CANONICAL L1 id, symbols it does not know fall
// through to the wrapped mapper, and a nil substrate leaves the inner mapper
// untouched (the gate-off identity).
func TestSubstrateIndustryMapper_CanonicalFirstThenTree(t *testing.T) {
	inner := &mockMapper{
		classifications: map[string]*IndustryClassification{
			"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor", Name: "半導體", Cyclicality: "high"}},
		},
	}
	tree := newFakeTree([]IndustrySegment{
		{ID: "semiconductor", Name: "半導體", NameEN: "Semiconductor", Level: 1, Cyclicality: "high"},
	})

	t.Run("nil_substrate_is_identity", func(t *testing.T) {
		got := NewSubstrateIndustryMapper(inner, nil, tree)
		if got != SymbolIndustryMapper(inner) {
			t.Fatal("nil substrate must return the inner mapper unchanged")
		}
	})

	t.Run("substrate_answers_with_canonical_l1", func(t *testing.T) {
		sub := newFakeSubstrate(map[string]industry.SectorID{"2603": "shipping"})
		m := NewSubstrateIndustryMapper(inner, sub, tree)
		cls, ok := m.GetClassification("2603.TW")
		if !ok {
			t.Fatal("expected the substrate to classify 2603")
		}
		if cls.Level1.ID != "shipping" {
			t.Fatalf("Level1.ID = %q, want shipping", cls.Level1.ID)
		}
		if cls.Symbol != "2603" {
			t.Fatalf("Symbol = %q, want the normalized 2603", cls.Symbol)
		}
	})

	t.Run("tree_metadata_copied_when_declared", func(t *testing.T) {
		sub := newFakeSubstrate(map[string]industry.SectorID{"2330": "semiconductor"})
		m := NewSubstrateIndustryMapper(inner, sub, tree)
		cls, ok := m.GetClassification("2330")
		if !ok {
			t.Fatal("expected 2330 to classify")
		}
		if cls.Level1.Name != "半導體" || cls.Level1.Cyclicality != "high" {
			t.Fatalf("tree metadata not carried: %+v", cls.Level1)
		}
	})

	t.Run("unknown_symbol_falls_back_to_inner", func(t *testing.T) {
		sub := newFakeSubstrate(map[string]industry.SectorID{"2603": "shipping"})
		m := NewSubstrateIndustryMapper(inner, sub, tree)
		cls, ok := m.GetClassification("2330")
		if !ok || cls.Level1.ID != "semiconductor" {
			t.Fatalf("fallback failed: ok=%v cls=%+v", ok, cls)
		}
		if _, ok := m.GetClassification("9999"); ok {
			t.Fatal("unknown symbol must stay unknown")
		}
	})

	t.Run("missing_tree_metadata_keeps_id_only", func(t *testing.T) {
		sub := newFakeSubstrate(map[string]industry.SectorID{"2603": "shipping"})
		m := NewSubstrateIndustryMapper(inner, sub, nil)
		cls, ok := m.GetClassification("2603")
		if !ok {
			t.Fatal("expected classification without a tree")
		}
		if cls.Level1.ID != "shipping" || cls.Level1.Name != "" {
			t.Fatalf("expected id-only segment, got %+v", cls.Level1)
		}
	})
}

// TestBuildUniverse_SubstrateGrowsPopulation is the end-to-end consumption
// evidence for issue #1943: the SAME pipeline, with the substrate installed,
// builds a strictly larger population, and the substrate is actually consulted
// (a substrate that is installed but never asked anything is the #1944
// write-with-no-reader failure shape).
func TestBuildUniverse_SubstrateGrowsPopulation(t *testing.T) {
	ctx := context.Background()

	baseline := buildDepsFixture(t, tempDir(t))
	baseResult, _, err := BuildUniverse(ctx, baseline, false)
	if err != nil {
		t.Fatalf("baseline BuildUniverse: %v", err)
	}

	deps := buildDepsFixture(t, tempDir(t))
	sub := newFakeSubstrate(map[string]industry.SectorID{
		"2330": "semiconductor",
		"2317": "electronics",
		"2454": "semiconductor",
		"2603": "shipping",
		"2881": "financials",
	})
	deps.Substrate = sub
	deps.Mapper = NewSubstrateIndustryMapper(deps.Mapper, sub, deps.Tree)
	deps.Quotes = &mockQuoteProv{quotes: map[string]domain.Quote{
		"2330": {Symbol: "2330", Last: 500, Volume: 50_000_000},
		"2317": {Symbol: "2317", Last: 100, Volume: 30_000_000},
		"2454": {Symbol: "2454", Last: 900, Volume: 20_000_000},
		"2603": {Symbol: "2603", Last: 180, Volume: 40_000_000},
		"2881": {Symbol: "2881", Last: 60, Volume: 25_000_000},
	}}

	result, _, err := BuildUniverse(ctx, deps, false)
	if err != nil {
		t.Fatalf("substrate BuildUniverse: %v", err)
	}

	if result.SymbolsBuilt != 5 {
		t.Fatalf("symbols_built = %d, want the substrate population (5)", result.SymbolsBuilt)
	}
	if result.SymbolsBuilt <= baseResult.SymbolsBuilt {
		t.Fatalf("substrate population (%d) must exceed the tree-only population (%d)",
			result.SymbolsBuilt, baseResult.SymbolsBuilt)
	}
	if sub.lookups == 0 {
		t.Fatal("substrate was installed but never consulted (#1944 inert write)")
	}
}
