package industry_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// multiClaimTree builds a tree where one symbol is claimed by three segments at
// three different depths, so the deterministic rule can be pinned.
func multiClaimTree() *industry.ClassificationTree {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID: "l1_broad", Name: "Broad", Level: industry.Level1,
		RepresentativeStocks: []string{"2330", "2317"},
	})
	tree.AddSegment(&industry.IndustrySegment{
		ID: "l2_foundry", Name: "Foundry", Level: industry.Level2, ParentID: "l1_broad",
		RepresentativeStocks: []string{"2330", "2303"},
	})
	tree.AddSegment(&industry.IndustrySegment{
		ID: "l3_advanced", Name: "Advanced", Level: industry.Level3, ParentID: "l2_foundry",
		RepresentativeStocks: []string{"2330"},
	})
	return tree
}

// TestBuildSymbolSectorIndex_DeterministicAcrossRuns is the I27 regression
// proof: the three monitoring call sites used to iterate GetAllSegments() in Go
// map order, so a multi-assigned symbol could resolve differently per process.
func TestBuildSymbolSectorIndex_DeterministicAcrossRuns(t *testing.T) {
	tree := multiClaimTree()
	first := industry.BuildSymbolSectorIndex(tree).BySymbol
	for run := 0; run < 50; run++ {
		got := industry.BuildSymbolSectorIndex(tree).BySymbol
		for sym, id := range first {
			if got[sym] != id {
				t.Fatalf("run %d: %s = %q, want %q (non-deterministic resolution)", run, sym, got[sym], id)
			}
		}
		if len(got) != len(first) {
			t.Fatalf("run %d: %d symbols, want %d", run, len(got), len(first))
		}
	}
}

// TestBuildSymbolSectorIndex_MostSpecificSegmentWins pins the documented rule.
func TestBuildSymbolSectorIndex_MostSpecificSegmentWins(t *testing.T) {
	idx := industry.BuildSymbolSectorIndex(multiClaimTree())
	cases := map[string]string{
		"2330": "l3_advanced", // claimed by L1+L2+L3 → deepest wins
		"2303": "l2_foundry",  // claimed by L2 only
		"2317": "l1_broad",    // claimed by L1 only
	}
	for sym, want := range cases {
		got, ok := idx.SectorFor(sym)
		if !ok {
			t.Fatalf("SectorFor(%s): not found", sym)
		}
		if got != want {
			t.Errorf("SectorFor(%s) = %q, want %q", sym, got, want)
		}
	}
}

// TestBuildSymbolSectorIndex_ReportsMultiAssigned keeps the ambiguity
// observable instead of silently picking a winner.
func TestBuildSymbolSectorIndex_ReportsMultiAssigned(t *testing.T) {
	idx := industry.BuildSymbolSectorIndex(multiClaimTree())
	if _, ok := idx.MultiAssigned["2317"]; ok {
		t.Error("2317 is claimed by a single segment, must not be reported as multi-assigned")
	}
	got, ok := idx.MultiAssigned["2330"]
	if !ok {
		t.Fatal("2330 is claimed by three segments, must be reported as multi-assigned")
	}
	want := []string{"l1_broad", "l2_foundry", "l3_advanced"}
	if len(got) != len(want) {
		t.Fatalf("MultiAssigned[2330] = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MultiAssigned[2330] = %v, want sorted %v", got, want)
		}
	}
}

// TestBuildSymbolSectorIndex_NilAndEmptyTrees pins the callers' nil path.
func TestBuildSymbolSectorIndex_NilAndEmptyTrees(t *testing.T) {
	nilIdx := industry.BuildSymbolSectorIndex(nil)
	if len(nilIdx.BySymbol) != 0 {
		t.Errorf("nil tree: %d symbols, want 0", len(nilIdx.BySymbol))
	}
	if _, ok := nilIdx.SectorFor("2330"); ok {
		t.Error("nil tree: SectorFor must report not-found")
	}

	empty := industry.BuildSymbolSectorIndex(industry.NewClassificationTree())
	if len(empty.BySymbol) != 0 {
		t.Errorf("empty tree: %d symbols, want 0", len(empty.BySymbol))
	}
}

var _ = domain.Quote{} // keep domain import honest if the fixture changes
