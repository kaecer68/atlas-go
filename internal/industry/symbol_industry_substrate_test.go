package industry_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// ─── issue #1943: per-stock industry substrate ──────────────────────────────

// stubSubstrate is an in-memory industry.SymbolIndustrySubstrate.
type stubSubstrate struct {
	bySymbol map[string]industry.SectorID
	symbols  []string
}

func (s *stubSubstrate) ResolveL1(symbol string) (industry.SectorID, bool) {
	id, ok := s.bySymbol[symbol]
	return id, ok
}

func (s *stubSubstrate) Symbols() []string { return append([]string(nil), s.symbols...) }

// TestSymbolL1Mapper_SubstratePrecedence pins the resolution order of the
// issue #1943 substrate: the per-stock field answers first (it covers the whole
// listed market), the representative-stock table stays as the fallback, and an
// uninstalled substrate leaves the mapper byte-identical with the pre-#1943
// behavior.
func TestSymbolL1Mapper_SubstratePrecedence(t *testing.T) {
	tree := industry.DefaultClassification()
	mapper, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}

	// 2330 is a declared representative stock of the canonical semiconductor
	// L1 segment; 6116 is not declared anywhere in the tree.
	if id, ok := mapper.ResolveL1("2330"); !ok || id != industry.SectorSemiconductor {
		t.Fatalf("baseline ResolveL1(2330) = %q, %v; want semiconductor", id, ok)
	}
	if _, ok := mapper.ResolveL1("6116"); ok {
		t.Fatal("6116 must be unresolved while no substrate is installed")
	}

	sub := &stubSubstrate{
		bySymbol: map[string]industry.SectorID{
			"2330": "semiconductor",
			"6116": "optoelectronics", // touch panel: outside the representative table
		},
		symbols: []string{"2330", "6116"},
	}
	mapper.WithSymbolIndustrySubstrate(sub)

	id, source, ok := mapper.ResolveL1WithSource("6116")
	if !ok || id != industry.SectorOptoelectronics {
		t.Fatalf("substrate ResolveL1(6116) = %q, %v; want optoelectronics", id, ok)
	}
	if source != industry.L1SourceSymbolIndustry {
		t.Fatalf("source = %q, want %q", source, industry.L1SourceSymbolIndustry)
	}

	// A symbol the substrate does not know still resolves through the
	// representative-stock table (fallback, not a coverage regression).
	id, source, ok = mapper.ResolveL1WithSource("2317")
	if !ok {
		t.Fatal("2317 must still resolve through the representative-stock fallback")
	}
	if source != industry.L1SourceRepresentativeStocks {
		t.Fatalf("source = %q, want %q", source, industry.L1SourceRepresentativeStocks)
	}
	if id != industry.SectorElectronics {
		t.Fatalf("2317 resolved to %q, want electronics", id)
	}

	// Removing the substrate restores the pre-#1943 behavior exactly.
	mapper.WithSymbolIndustrySubstrate(nil)
	if mapper.SymbolIndustrySubstrate() != nil {
		t.Fatal("WithSymbolIndustrySubstrate(nil) must clear the substrate")
	}
	if _, ok := mapper.ResolveL1("6116"); ok {
		t.Fatal("6116 must be unresolved again after clearing the substrate")
	}
}

// TestSymbolIndustryConsumerRegistry pins the #1944 consumption-evidence
// registry: wiring announces its consumers, duplicates are idempotent, and the
// registry can be reset so a removed consumer stops being reported.
func TestSymbolIndustryConsumerRegistry(t *testing.T) {
	industry.ResetSymbolIndustryConsumers()
	t.Cleanup(industry.ResetSymbolIndustryConsumers)

	if industry.SymbolIndustryConsumerWired() {
		t.Fatal("registry must start empty")
	}
	industry.RegisterSymbolIndustryConsumer("")
	if industry.SymbolIndustryConsumerWired() {
		t.Fatal("empty labels must be ignored")
	}

	industry.RegisterSymbolIndustryConsumer("cmd/atlas.universe_builder")
	industry.RegisterSymbolIndustryConsumer("cmd/atlas.universe_builder")
	industry.RegisterSymbolIndustryConsumer("cmd/atlas.sector_exposure")

	got := industry.RegisteredSymbolIndustryConsumers()
	want := []string{"cmd/atlas.sector_exposure", "cmd/atlas.universe_builder"} // sorted
	if len(got) != len(want) {
		t.Fatalf("consumers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("consumers = %v, want %v", got, want)
		}
	}
	if !industry.SymbolIndustryConsumerWired() {
		t.Fatal("SymbolIndustryConsumerWired must be true after registration")
	}

	industry.ResetSymbolIndustryConsumers()
	if industry.SymbolIndustryConsumerWired() {
		t.Fatal("reset must clear the registry")
	}
}
