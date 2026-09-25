package industry

import "sort"

// SymbolSectorIndex is a deterministic symbol → segment resolution derived from
// the representative stocks declared by the classification tree.
//
// Why this type exists (#1944 Batch 4, item I27): three monitoring call sites
// each re-implemented "loop over GetAllSegments() and write map[symbol] = seg.ID".
// GetAllSegments() returns segments in Go map order, so a symbol declared by two
// segments (the shipped tree has several) got a label that changed between
// process runs — and every one of the three copies could disagree with the
// others. This is the single authority they now share.
type SymbolSectorIndex struct {
	// BySymbol maps a representative-stock symbol to the segment ID chosen for
	// it. Deterministic rule: the most specific (deepest) claiming segment
	// wins; equal depth breaks on the lexicographically smallest segment ID.
	BySymbol map[string]string
	// MultiAssigned lists the symbols claimed by more than one segment together
	// with every claiming segment ID (sorted). It is the observable half of the
	// ambiguity: callers that only need a label can ignore it, audits can report
	// it instead of re-deriving the ambiguity.
	MultiAssigned map[string][]string
}

// SectorFor returns the chosen segment ID for symbol.
func (i SymbolSectorIndex) SectorFor(symbol string) (string, bool) {
	if i.BySymbol == nil {
		return "", false
	}
	id, ok := i.BySymbol[symbol]
	return id, ok
}

// BuildSymbolSectorIndex builds the index for tree. A nil tree yields an empty
// index (no panic), which is what the callers rely on for their "no classifier"
// path.
func BuildSymbolSectorIndex(tree *ClassificationTree) SymbolSectorIndex {
	idx := SymbolSectorIndex{
		BySymbol:      map[string]string{},
		MultiAssigned: map[string][]string{},
	}
	if tree == nil {
		return idx
	}

	claims := map[string][]string{}
	depth := map[string]int{}
	for _, seg := range tree.GetAllSegments() {
		if seg == nil {
			continue
		}
		if _, ok := depth[seg.ID]; !ok {
			depth[seg.ID] = len(tree.GetPath(seg.ID))
		}
		for _, sym := range seg.RepresentativeStocks {
			if sym == "" {
				continue
			}
			claims[sym] = append(claims[sym], seg.ID)
		}
	}

	for sym, ids := range claims {
		sort.Strings(ids)
		ids = dedupeStrings(ids)
		best := ids[0]
		for _, id := range ids[1:] {
			switch {
			case depth[id] > depth[best]:
				best = id
			case depth[id] == depth[best] && id < best:
				best = id
			}
		}
		idx.BySymbol[sym] = best
		if len(ids) > 1 {
			idx.MultiAssigned[sym] = ids
		}
	}
	return idx
}

// dedupeStrings removes duplicates from a sorted slice in place.
func dedupeStrings(sorted []string) []string {
	if len(sorted) < 2 {
		return sorted
	}
	out := sorted[:1]
	for _, s := range sorted[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
