package marketdata

import (
	"testing"
	"time"
)

func TestCanonicalizeSegmentID(t *testing.T) {
	cases := []struct {
		segment   string
		wantID    string
		wantL1    string
		wantEmpty bool
	}{
		{"semiconductor", "semiconductor", "semiconductor", false},
		{"foundry", "foundry", "semiconductor", false},
		{"ai_supply_chain", "ai_supply_chain", "electronics", false},
		{"robotics", "robotics", "machinery", false},
		{"leo_satellite", "leo_satellite", "telecom", false},
		{"cooling", "cooling", "semiconductor", false},
		{"defensive", "", "", true},
		{"tech", "", "", true},
		{"unknown_segment_id", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		id, l1, reason := CanonicalizeSegmentID(c.segment)
		if id != c.wantID || l1 != c.wantL1 {
			t.Errorf("CanonicalizeSegmentID(%q) = (%q, %q), want (%q, %q)", c.segment, id, l1, c.wantID, c.wantL1)
		}
		if c.wantEmpty && reason == "" {
			t.Errorf("CanonicalizeSegmentID(%q) must explain why it has no canonical L1 sector", c.segment)
		}
		if !c.wantEmpty && reason != "" {
			t.Errorf("CanonicalizeSegmentID(%q) reason = %q, want empty", c.segment, reason)
		}
	}
}

// stubTree implements ClassificationTreeAccessor for mapper tests.
type stubTree struct {
	segments map[string]*MapperIndustrySegment
	children map[string][]*MapperIndustrySegment
}

func (s *stubTree) GetSegment(id string) (*MapperIndustrySegment, bool) {
	seg, ok := s.segments[id]
	return seg, ok
}

func (s *stubTree) GetChildren(id string) []*MapperIndustrySegment { return s.children[id] }

func (s *stubTree) GetLevel1() []*MapperIndustrySegment {
	out := []*MapperIndustrySegment{}
	for _, seg := range s.segments {
		if seg.Level == int(MapperLevel1) {
			out = append(out, seg)
		}
	}
	return out
}

func (s *stubTree) GetPath(id string) []*MapperIndustrySegment {
	path := []*MapperIndustrySegment{}
	for id != "" {
		seg, ok := s.segments[id]
		if !ok {
			break
		}
		path = append([]*MapperIndustrySegment{seg}, path...)
		id = seg.ParentID
	}
	return path
}

func (s *stubTree) GetAllSegments() []*MapperIndustrySegment {
	out := make([]*MapperIndustrySegment, 0, len(s.segments))
	for _, seg := range s.segments {
		out = append(out, seg)
	}
	return out
}

func TestSymbolIndustryMapper_SetsCanonicalFields(t *testing.T) {
	tree := &stubTree{segments: map[string]*MapperIndustrySegment{}}
	tree.segments["ai_supply_chain"] = &MapperIndustrySegment{
		ID: "ai_supply_chain", Name: "AI供應鏈", NameEN: "AI Supply Chain", Level: int(MapperLevel1),
	}
	tree.segments["server_assembly"] = &MapperIndustrySegment{
		ID: "server_assembly", Name: "伺服器組裝", NameEN: "Server Assembly", Level: int(MapperLevel2),
		ParentID: "ai_supply_chain",
	}
	tree.segments["defensive"] = &MapperIndustrySegment{
		ID: "defensive", Name: "防禦性資產", NameEN: "Defensive", Level: int(MapperLevel1),
	}

	m := &SymbolIndustryMapper{tree: tree, cache: map[string]MapperIndustryClassification{}}

	got := m.classifyForSymbol("2317", tree.segments["server_assembly"], time.Now())
	if got.CanonicalSectorID != "server_assembly" {
		t.Errorf("CanonicalSectorID = %q, want server_assembly", got.CanonicalSectorID)
	}
	_ = got
	// server_assembly's declared parent is semiconductor (the authored tree
	// nests it under semiconductor); the table is required to agree.
	if got.CanonicalL1 != "semiconductor" {
		t.Errorf("CanonicalL1 = %q, want semiconductor (declared parent table)", got.CanonicalL1)
	}

	// A strategy bucket has no canonical L1 sector and must say so instead of
	// silently producing an empty ID.
	got = m.classifyForSymbol("1101", tree.segments["defensive"], time.Now())
	if got.CanonicalL1 != "" {
		t.Errorf("defensive CanonicalL1 = %q, want empty", got.CanonicalL1)
	}
	if got.CanonicalReason == "" {
		t.Error("defensive must carry a CanonicalReason")
	}
}

func TestSymbolIndustryMapper_TreeStructuralParentWins(t *testing.T) {
	// server_assembly sits under semiconductor in the production tree, while the
	// declared L2 table says electronics. The structural parent wins, matching
	// industry.SymbolL1Mapper.
	tree := &stubTree{segments: map[string]*MapperIndustrySegment{}}
	tree.segments["semiconductor"] = &MapperIndustrySegment{
		ID: "semiconductor", Name: "半導體", Level: int(MapperLevel1),
	}
	tree.segments["server_assembly"] = &MapperIndustrySegment{
		ID: "server_assembly", Name: "伺服器組裝", Level: int(MapperLevel2), ParentID: "semiconductor",
	}
	m := &SymbolIndustryMapper{tree: tree, cache: map[string]MapperIndustryClassification{}}

	got := m.classifyForSymbol("2324", tree.segments["server_assembly"], time.Now())
	if got.CanonicalL1 != "semiconductor" {
		t.Errorf("CanonicalL1 = %q, want semiconductor (tree structure is authoritative)", got.CanonicalL1)
	}
}
