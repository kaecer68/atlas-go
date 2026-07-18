package industry

import (
	"fmt"
	"strings"
)

// SymbolL1Mapper maps a stock symbol (e.g. "2330", "2330.TW") to its
// canonical L1 SectorID by walking the ClassificationTree. The mapper
// is built once from the canonical taxonomy and supports batch lookups
// via ResolveL1.
type SymbolL1Mapper struct {
	bySymbol map[string]SectorID
}

// NewSymbolL1Mapper builds a SymbolL1Mapper from the given
// ClassificationTree. For each segment in the tree, every representative
// stock is resolved to its L1 ancestor via GetPath. The symbol is
// normalized (trimmed, .TW suffix stripped) before insertion.
//
// Returns an error when a single symbol maps to two different L1 sectors.
// Returns an error when tree is nil. Segments without a L1 ancestor
// are silently skipped.
func NewSymbolL1Mapper(tree *ClassificationTree) (*SymbolL1Mapper, error) {
	if tree == nil {
		return nil, fmt.Errorf("symbol_l1_mapper: tree must not be nil")
	}

	m := &SymbolL1Mapper{
		bySymbol: make(map[string]SectorID, 200),
	}
	seen := make(map[string]SectorID)

	for _, seg := range tree.GetAllSegments() {
		// Resolve the L1 ancestor via GetPath.
		// The first segment in the path with Level == Level1 is the L1 root.
		path := tree.GetPath(seg.ID)
		var l1 SectorID
		for _, s := range path {
			if id := SectorID(s.ID); IsL1(id) {
				l1 = id
				break
			}
		}
		if l1 == "" {
			continue // no L1 ancestor, skip (e.g. orphan L2)
		}

		for _, sym := range seg.RepresentativeStocks {
			key := normalizeSymbol(sym)
			if key == "" {
				continue
			}
			if existing, ok := seen[key]; ok && existing != l1 {
				return nil, fmt.Errorf("symbol_l1_mapper: duplicate symbol %q maps to both %s and %s", key, existing, l1)
			}
			seen[key] = l1
			m.bySymbol[key] = l1
		}
	}

	return m, nil
}

// ResolveL1 returns the L1 SectorID for the given stock symbol.
// The symbol is normalized before lookup. Returns (SectorID, true)
// on match, or ("", false) when unknown.
func (m *SymbolL1Mapper) ResolveL1(symbol string) (SectorID, bool) {
	key := normalizeSymbol(symbol)
	id, ok := m.bySymbol[key]
	return id, ok
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
