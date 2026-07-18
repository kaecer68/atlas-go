package industry_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func TestSymbolL1Mapper_NewSymbolL1Mapper_NormalizesTWSuffix(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330", "2330.TW"},
	})
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "foundry",
		Name:                 "晶圓代工",
		Level:                industry.Level2,
		ParentID:             "semiconductor",
		RepresentativeStocks: []string{"2330"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	id, ok := m.ResolveL1("2330")
	if !ok {
		t.Fatal("2330 should resolve to semiconductor")
	}
	if id != industry.SectorSemiconductor {
		t.Fatalf("got %q, want semiconductor", id)
	}
	// .TW suffix should normalize to same L1
	id2, ok2 := m.ResolveL1("2330.TW")
	if !ok2 || id2 != industry.SectorSemiconductor {
		t.Fatalf("2330.TW should resolve to semiconductor, got %q ok=%v", id2, ok2)
	}
}

func TestSymbolL1Mapper_L2StockResolvesToL1Ancestor(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"},
	})
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "foundry",
		Name:                 "晶圓代工",
		Level:                industry.Level2,
		ParentID:             "semiconductor",
		RepresentativeStocks: []string{"5347"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	id, ok := m.ResolveL1("5347")
	if !ok {
		t.Fatal("5347 should resolve to semiconductor via L2->L1 ancestor")
	}
	if id != industry.SectorSemiconductor {
		t.Fatalf("got %q, want semiconductor", id)
	}
}

func TestSymbolL1Mapper_RejectsNonL1Root(t *testing.T) {
	tree := industry.NewClassificationTree()
	// only L2 segment without L1 parent
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "foundry",
		Name:                 "晶圓代工",
		Level:                industry.Level2,
		RepresentativeStocks: []string{"5347"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper should tolerate or skip non-L1 segments: %v", err)
	}
	_, ok := m.ResolveL1("5347")
	if ok {
		t.Fatal("L2 segment with no L1 ancestor should NOT resolve")
	}
}

func TestSymbolL1Mapper_RejectsCrossL1Duplicate(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"},
	})
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "electronics",
		Name:                 "電子零組件",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"}, // duplicate!
	})

	_, err := industry.NewSymbolL1Mapper(tree)
	if err == nil {
		t.Fatal("duplicate symbol 2330 across two L1 sectors must be rejected")
	}
}

func TestSymbolL1Mapper_DoesNotFuzzyMap(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	_, ok := m.ResolveL1("2330 TW") // space instead of dot
	if ok {
		t.Fatal("fuzzy match must not resolve")
	}
	_, ok = m.ResolveL1("2331") // close but wrong
	if ok {
		t.Fatal("near-match must not resolve")
	}
}

func TestSymbolL1Mapper_UnknownSymbolReturnsFalse(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		Level:                industry.Level1,
		RepresentativeStocks: []string{"2330"},
	})

	m, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	_, ok := m.ResolveL1("9999")
	if ok {
		t.Fatal("unknown symbol must return false")
	}
	_, ok = m.ResolveL1("")
	if ok {
		t.Fatal("empty symbol must return false")
	}
}

func TestSymbolL1Mapper_NilTreeReturnsError(t *testing.T) {
	_, err := industry.NewSymbolL1Mapper(nil)
	if err == nil {
		t.Fatal("nil tree must produce error")
	}
}
