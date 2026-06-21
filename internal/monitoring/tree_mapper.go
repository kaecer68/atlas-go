package monitoring

// TreeBasedMapper implements SymbolIndustryMapper by deriving symbol→industry
// mappings from a ClassificationTreeAccessor's per-segment RepresentativeStocks.
//
// It is designed for the SmartUniverse pipeline where a full FinMind-based
// mapper is unavailable — the classification tree carries enough representative
// stocks per Level-1 industry to bootstrap the pipeline.
type TreeBasedMapper struct {
	tree             ClassificationTreeAccessor
	symbolToIndustry map[string]string // normalized symbol → industry ID
}

// NewTreeBasedMapper builds a TreeBasedMapper by walking every Level-1 segment
// returned by tree.GetLevel1() and indexing each segment's RepresentativeStocks
// into the reverse lookup table. Symbols are normalized via normalizeSymbol()
// before insertion (".TW" suffix stripped).
func NewTreeBasedMapper(tree ClassificationTreeAccessor) *TreeBasedMapper {
	m := &TreeBasedMapper{
		tree:             tree,
		symbolToIndustry: make(map[string]string),
	}
	for _, seg := range tree.GetLevel1() {
		for _, sym := range seg.RepresentativeStocks {
			norm := normalizeSymbol(sym)
			if norm == "" {
				continue
			}
			m.symbolToIndustry[norm] = seg.ID
		}
	}
	return m
}

// GetClassification returns the IndustryClassification for a symbol by looking
// up its normalized form in the reverse index, then resolving the Level-1
// segment from the tree. Returns (nil, false) when the symbol is unknown.
func (m *TreeBasedMapper) GetClassification(symbol string) (*IndustryClassification, bool) {
	norm := normalizeSymbol(symbol)
	industryID, ok := m.symbolToIndustry[norm]
	if !ok {
		return nil, false
	}
	seg, ok := m.tree.GetSegment(industryID)
	if !ok {
		return nil, false
	}
	return &IndustryClassification{
		Symbol: norm,
		Level1: seg,
	}, true
}

// GetSymbolsByIndustry returns the RepresentativeStocks for the Level-1
// industry whose ID matches industryID. Returns nil when no such segment
// exists in the tree.
func (m *TreeBasedMapper) GetSymbolsByIndustry(industryID string) []string {
	l1 := m.tree.GetLevel1()
	for _, seg := range l1 {
		if seg.ID == industryID {
			return seg.RepresentativeStocks
		}
	}
	return nil
}
