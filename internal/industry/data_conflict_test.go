package industry_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// knownSymbolConflicts pins the symbol-level disagreements between the two
// authored classification sources:
//
//   - the classification tree (configs/parameters/industry.json), which is the
//     production path, resolved with the declared L2→L1 parent table
//     (rank 0 = tree structure, rank 1 = declared table), and
//   - the hard-coded fallback table industry.DefaultRepresentativeStocks().
//
// Issue #1943 unifies the *key spaces*; it cannot decide which source is right
// about a symbol. Pinning the set means any change — a fix or a regression —
// shows up here instead of silently moving a symbol between industries.
// Resolving them needs a per-symbol industry source of truth (the spec's "known
// gaps": the database has no per-stock industry column).
var knownSymbolConflicts = []struct {
	Symbol     string
	Tree       industry.SectorID
	FallbackL1 industry.SectorID
	Why        string
}{
	{"1707", industry.SectorSteel, industry.SectorChemicals,
		"tree places it under mining (→ steel); the fallback table calls it 化學"},
	{"2324", industry.SectorSemiconductor, industry.SectorElectronics,
		"tree nests server_assembly under semiconductor; the fallback table follows the TWSE 電腦及週邊設備 grouping"},
	{"2356", industry.SectorSemiconductor, industry.SectorElectronics,
		"same as 2324; also the one symbol claimed by two tree segments (server_assembly vs robotics)"},
	{"3661", industry.SectorElectronics, industry.SectorSemiconductor,
		"tree places it under ai_supply_chain (→ electronics); the fallback table calls it 半導體"},
}

func TestKnownSymbolClassificationConflicts(t *testing.T) {
	loadRepoParameters(t)

	tree := industry.DefaultClassification()
	mapper, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}

	fallback := map[string]industry.SectorID{}
	for id, symbols := range industry.DefaultRepresentativeStocks() {
		for _, sym := range symbols {
			fallback[sym] = id
		}
	}

	type conflict struct {
		Symbol     string
		Tree       industry.SectorID
		FallbackL1 industry.SectorID
	}
	observed := []conflict{}
	for _, sym := range mapper.Symbols() {
		treeL1, ok := mapper.ResolveL1(sym)
		if !ok {
			continue
		}
		fallbackL1, known := fallback[sym]
		if !known || fallbackL1 == treeL1 {
			continue
		}
		observed = append(observed, conflict{sym, treeL1, fallbackL1})
	}

	want := make([]conflict, 0, len(knownSymbolConflicts))
	for _, k := range knownSymbolConflicts {
		want = append(want, conflict{k.Symbol, k.Tree, k.FallbackL1})
	}
	slices.SortFunc(want, func(a, b conflict) int { return strings.Compare(a.Symbol, b.Symbol) })
	slices.SortFunc(observed, func(a, b conflict) int { return strings.Compare(a.Symbol, b.Symbol) })

	if !slices.Equal(observed, want) {
		t.Errorf("symbol-level tree-vs-fallback conflicts changed.\n got: %+v\nwant: %+v\n"+
			"If you fixed one, remove it from knownSymbolConflicts and update the spec; "+
			"if you added one, the classification data regressed.", observed, want)
	}
	for _, c := range observed {
		t.Logf("KNOWN CONFLICT: %s tree=%s fallback=%s", c.Symbol, c.Tree, c.FallbackL1)
	}
}
