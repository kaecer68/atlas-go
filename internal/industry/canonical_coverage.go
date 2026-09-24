package industry

import (
	"slices"
)

// CanonicalCoverage measures how much of a symbol universe resolves to a
// canonical L1 sector. Issue #1943 item 4: consumers had no way to answer
// "how many symbols can this system actually place in an industry?".
//
// Two distinct numbers matter and must not be conflated:
//
//   - Mapping coverage: of the symbols the system declares (tree representative
//     stocks), how many map to a canonical L1 sector? Before #1943 the
//     production tree mapped 18 of 44 declared symbols; it now maps 44 of 44.
//   - Universe coverage: of the tradable universe (e.g. TWSE listed symbols),
//     how many map to a canonical L1 sector? This is a data-coverage problem,
//     not a mapping problem, and it is what limits industry-level statistics.
type CanonicalCoverage struct {
	// Universe is the number of distinct symbols supplied.
	Universe int `json:"universe"`
	// Mapped is the number of distinct symbols with a canonical L1 sector.
	Mapped int `json:"mapped"`
	// Ratio is Mapped / Universe (0 when the universe is empty).
	Ratio float64 `json:"ratio"`
	// ByL1 is the number of mapped symbols per canonical L1 sector.
	ByL1 map[SectorID]int `json:"by_l1"`
	// UnmappedSymbols lists (sorted) the symbols with no canonical L1 sector.
	UnmappedSymbols []string `json:"unmapped_symbols"`
	// L1SectorsCovered is the number of canonical L1 sectors that receive at
	// least one symbol.
	L1SectorsCovered int `json:"l1_sectors_covered"`
}

// ComputeCanonicalCoverage computes the canonical L1 coverage of symbols using
// the given mapper. Input symbols are normalized and de-duplicated. A nil mapper
// yields an all-unmapped result rather than a panic.
func ComputeCanonicalCoverage(symbols []string, m *SymbolL1Mapper) CanonicalCoverage {
	cov := CanonicalCoverage{
		ByL1:            map[SectorID]int{},
		UnmappedSymbols: []string{},
	}
	seen := make(map[string]struct{}, len(symbols))
	unique := make([]string, 0, len(symbols))
	for _, raw := range symbols {
		key := normalizeSymbol(raw)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	cov.Universe = len(unique)

	for _, sym := range unique {
		var id SectorID
		var ok bool
		if m != nil {
			id, ok = m.ResolveL1(sym)
		}
		if !ok {
			cov.UnmappedSymbols = append(cov.UnmappedSymbols, sym)
			continue
		}
		cov.Mapped++
		cov.ByL1[id]++
	}
	slices.Sort(cov.UnmappedSymbols)
	cov.L1SectorsCovered = len(cov.ByL1)
	if cov.Universe > 0 {
		cov.Ratio = float64(cov.Mapped) / float64(cov.Universe)
	}
	return cov
}

// DeclaredRepresentativeUniverse returns every distinct representative stock
// declared by the tree at the given level (use Level1 for the tree's Level-1
// segments). Symbols are normalized; "" entries are dropped.
func DeclaredRepresentativeUniverse(tree *ClassificationTree, level IndustryLevel) []string {
	if tree == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, seg := range tree.GetAllSegments() {
		if seg.Level != level {
			continue
		}
		for _, raw := range seg.RepresentativeStocks {
			key := normalizeSymbol(raw)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}
