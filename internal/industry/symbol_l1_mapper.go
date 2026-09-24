package industry

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// SymbolL1Mapper maps a stock symbol (e.g. "2330", "2330.TW") to its canonical
// L1 SectorID by walking the ClassificationTree. The mapper is built once from
// the canonical taxonomy and supports batch lookups via ResolveL1.
//
// Issue #1943: the tree's Level-1 segments are not all canonical L1 sectors.
// Seven of them (ai_supply_chain, consumer, etf_rotation, industrial,
// leo_satellite, mining, robotics) are canonical *L2* sub-industries and four
// (defensive, high_dividend, small_cap, tech) are strategy buckets. The previous
// implementation only accepted a segment as a Level-1 root when its ID was a
// canonical L1 ID, so those eleven segments — and every symbol underneath them —
// were silently skipped (the production config lost 19 of 29 segments).
//
// Resolution is now explicit and two-tier:
//
//	rank 0 (structural): the nearest ancestor in the tree path (root→segment,
//	  segment included) whose ID is a canonical L1 sector. The tree's own
//	  nesting wins because it is the authored structure.
//	rank 1 (declared): no canonical L1 ancestor exists, so the segment (then,
//	  walking outward, its ancestors) is translated through the declared
//	  namespace table in internal/sectormap — e.g. robotics → machinery,
//	  ai_supply_chain → electronics.
//
// Segments that translate to nothing are recorded by UnmappedSegments() instead
// of being dropped in silence. With the production config that is exactly the
// five segments whose IDs are size/style/asset-class buckets: defensive,
// etf_rotation, high_dividend, small_cap, tech. (pcb and thermal sit under the
// canonical L1 segment electronics, so rank 0 resolves them even though there is
// no canonical node named pcb/thermal of their own — the key is not canonical,
// the segment is usable.)
type SymbolL1Mapper struct {
	bySymbol  map[string]SectorID
	unmapped  map[string]string
	conflicts []SymbolL1Conflict
}

// SymbolL1Claim is one segment's claim on a symbol.
type SymbolL1Claim struct {
	Segment string
	Level   IndustryLevel
	Depth   int
	Rank    int
	Target  SectorID
}

// SymbolL1Conflict records a symbol claimed by two segments that resolve to
// different canonical L1 sectors. The conflict is resolved deterministically
// (lower rank wins, then the deeper segment, then the alphabetically first
// segment ID) and reported so operators can fix the classification data.
type SymbolL1Conflict struct {
	Symbol   string
	Chosen   SymbolL1Claim
	Rejected []SymbolL1Claim
}

// NewSymbolL1Mapper builds a SymbolL1Mapper from the given ClassificationTree
// using the config classification-tree namespace for rank-1 translation.
//
// It returns an error when tree is nil, and when one symbol is declared as a
// representative stock of two different canonical L1 segments — that is an
// authoring error in the tree itself and cannot be resolved by policy.
func NewSymbolL1Mapper(tree *ClassificationTree) (*SymbolL1Mapper, error) {
	return NewSymbolL1MapperWithNamespace(tree, sectormap.NamespaceClassificationTree)
}

// NewSymbolL1MapperWithNamespace is NewSymbolL1Mapper with an explicit foreign
// namespace for rank-1 translation. Use it when the tree carries a vocabulary
// other than the config classification tree.
func NewSymbolL1MapperWithNamespace(tree *ClassificationTree, ns ForeignNamespace) (*SymbolL1Mapper, error) {
	if tree == nil {
		return nil, fmt.Errorf("symbol_l1_mapper: tree must not be nil")
	}

	m := &SymbolL1Mapper{
		bySymbol: make(map[string]SectorID, 200),
		unmapped: make(map[string]string),
	}

	type claim struct {
		segment string
		level   IndustryLevel
		depth   int
		rank    int
		target  SectorID
		isL1Seg bool // the segment's own ID is a canonical L1 sector
	}
	claims := make(map[string][]claim, 200)

	for _, seg := range tree.GetAllSegments() {
		path := tree.GetPath(seg.ID)
		target, rank, reason := resolveSegmentL1(path, ns)
		if target == "" {
			// Report every unresolvable segment, with or without representative
			// stocks: "this segment has no canonical L1 target" is a taxonomy
			// gap that must be visible in the audit.
			m.unmapped[seg.ID] = reason
			continue
		}
		if len(seg.RepresentativeStocks) == 0 {
			continue
		}
		own := SectorID(seg.ID)
		for _, sym := range seg.RepresentativeStocks {
			key := normalizeSymbol(sym)
			if key == "" {
				continue
			}
			claims[key] = append(claims[key], claim{
				segment: seg.ID,
				level:   seg.Level,
				depth:   len(path),
				rank:    rank,
				target:  target,
				isL1Seg: own.IsL1(),
			})
		}
	}

	for sym, cs := range claims {
		// Authoring error: two distinct canonical L1 segments claim the same
		// symbol (the symbol is listed twice inside one segment is fine).
		roots := make(map[string]struct{}, 2)
		for _, c := range cs {
			if c.isL1Seg {
				roots[c.segment] = struct{}{}
			}
		}
		if len(roots) > 1 {
			return nil, fmt.Errorf("symbol_l1_mapper: duplicate symbol %q declared by %d canonical L1 segments", sym, len(roots))
		}

		slices.SortFunc(cs, func(a, b claim) int {
			if a.rank != b.rank {
				return a.rank - b.rank
			}
			if a.depth != b.depth {
				return b.depth - a.depth // deeper (more specific) segment first
			}
			return strings.Compare(a.segment, b.segment)
		})

		best := cs[0]
		m.bySymbol[sym] = best.target

		var rejected []SymbolL1Claim
		for _, c := range cs[1:] {
			if c.target == best.target {
				continue // same canonical L1: structural overlap, not a conflict
			}
			rejected = append(rejected, SymbolL1Claim{
				Segment: c.segment,
				Level:   c.level,
				Depth:   c.depth,
				Rank:    c.rank,
				Target:  c.target,
			})
		}
		if len(rejected) > 0 {
			m.conflicts = append(m.conflicts, SymbolL1Conflict{
				Symbol: sym,
				Chosen: SymbolL1Claim{
					Segment: best.segment,
					Level:   best.level,
					Depth:   best.depth,
					Rank:    best.rank,
					Target:  best.target,
				},
				Rejected: rejected,
			})
		}
	}

	slices.SortFunc(m.conflicts, func(a, b SymbolL1Conflict) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})
	return m, nil
}

// resolveSegmentL1 resolves one tree path to a canonical L1 sector.
// It returns the target, the resolution rank (0 structural, 1 declared) and an
// explanatory reason when no target exists.
func resolveSegmentL1(path []*IndustrySegment, ns ForeignNamespace) (SectorID, int, string) {
	// A nil/empty path means the segment is not in the tree (orphan).
	if len(path) == 0 {
		return "", 1, "segment is not registered in the tree (orphan); no canonical L1 target"
	}
	for _, s := range path {
		if id := SectorID(s.ID); id.IsL1() {
			return id, 0, ""
		}
	}
	// Nearest-to-farthest declared translation.
	for i := len(path) - 1; i >= 0; i-- {
		if l1, ok := CanonicalL1FromForeign(ns, path[i].ID); ok {
			return l1, 1, ""
		}
	}
	reason := ResolveForeign(ns, path[len(path)-1].ID).Reason
	if reason == "" {
		reason = "no canonical L1 target declared for this segment"
	}
	return "", 1, reason
}

// ResolveL1 returns the L1 SectorID for the given stock symbol.
// The symbol is normalized before lookup. Returns (SectorID, true)
// on match, or ("", false) when unknown.
func (m *SymbolL1Mapper) ResolveL1(symbol string) (SectorID, bool) {
	key := normalizeSymbol(symbol)
	id, ok := m.bySymbol[key]
	return id, ok
}

// Len returns the number of symbols in the mapping.
func (m *SymbolL1Mapper) Len() int { return len(m.bySymbol) }

// Symbols returns every mapped symbol, sorted.
func (m *SymbolL1Mapper) Symbols() []string {
	out := make([]string, 0, len(m.bySymbol))
	for sym := range m.bySymbol {
		out = append(out, sym)
	}
	slices.Sort(out)
	return out
}

// L1Counts returns the number of mapped symbols per canonical L1 sector.
func (m *SymbolL1Mapper) L1Counts() map[SectorID]int {
	out := make(map[SectorID]int, 20)
	for _, id := range m.bySymbol {
		out[id]++
	}
	return out
}

// UnmappedSegments returns the segments that could not be resolved to a
// canonical L1 sector, keyed by segment ID, with the reason. These segments'
// representative stocks are NOT in the mapping — callers that need full coverage
// must treat this as a gap to report, not as an empty result.
func (m *SymbolL1Mapper) UnmappedSegments() map[string]string {
	out := make(map[string]string, len(m.unmapped))
	for k, v := range m.unmapped {
		out[k] = v
	}
	return out
}

// Conflicts returns the symbol-level conflicts resolved during construction.
func (m *SymbolL1Mapper) Conflicts() []SymbolL1Conflict {
	out := make([]SymbolL1Conflict, len(m.conflicts))
	copy(out, m.conflicts)
	return out
}

// normalizeSymbol trims whitespace and strips a trailing ".TW" suffix
// (case-insensitive). No fuzzy matching is performed.
func normalizeSymbol(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 3 && strings.EqualFold(s[len(s)-3:], ".TW") {
		s = s[:len(s)-3]
	}
	return s
}
