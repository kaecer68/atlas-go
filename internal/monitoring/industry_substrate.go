package monitoring

// industry_substrate.go — the per-stock industry field as the SmartUniverse
// population source (issue #1943).
//
// Before this file the universe was populated by walking the classification
// tree's representative_stocks (TreeBasedMapper), which the production config
// declares for ~27 symbols: universe_snapshot.json recorded
// symbols_built=27 out of ~1988 listed companies (~1.4 %), so every
// industry-level statistic derived from the ranking rested on a population far
// too small to be significant (#1943: "生產母體僅 27 支").
//
// The first-party `symbol_industry` channel supplies the whole listed market
// (TWSE 上市 + TPEx 上櫃) mapped to canonical L1 through the declared namespace
// table (#1958). When wiring installs it — only when
// configs/parameters.json -> industry.substrate_from_symbol_industry_enabled is
// true, default false — the pipeline uses:
//
//	population : substrate.Symbols()                 (gatherAllSymbols)
//	industry   : substrate.ResolveL1()               (SubstrateIndustryMapper)
//
// With no substrate installed both functions are the pre-#1943 code paths
// unchanged, which is what makes "gate off" a byte-identical run rather than a
// similar-looking one.

import (
	"slices"
	"strings"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// SubstrateIndustryMapper decorates a SymbolIndustryMapper with the per-stock
// industry substrate. The substrate answers first (it covers the whole listed
// market); the wrapped mapper — the classification tree — remains the fallback,
// so the mapped population can only grow.
//
// GetSymbolsByIndustry is deliberately NOT overridden: its callers
// (IndustryFilter's semiconductor supply-chain expansion, the D6 check) ask for
// a segment's members and the tree is the owner of that structure. Forwarding
// keeps the expansion identical to the pre-substrate behavior.
type SubstrateIndustryMapper struct {
	inner     SymbolIndustryMapper
	substrate industry.SymbolIndustrySubstrate
	tree      ClassificationTreeAccessor
}

// NewSubstrateIndustryMapper wraps inner. A nil substrate returns inner
// unchanged, so the caller does not need a branch.
func NewSubstrateIndustryMapper(inner SymbolIndustryMapper, substrate industry.SymbolIndustrySubstrate, tree ClassificationTreeAccessor) SymbolIndustryMapper {
	if substrate == nil || inner == nil {
		return inner
	}
	return &SubstrateIndustryMapper{inner: inner, substrate: substrate, tree: tree}
}

// GetClassification resolves the symbol through the substrate first.
//
// The returned Level1 segment carries the CANONICAL L1 id (the substrate's
// vocabulary), which is what downstream module joins must use. Segment
// metadata (name / cyclicality) is copied from the tree when the tree declares
// the same id; otherwise only the id is set, and consumers that need names must
// look them up in the canonical taxonomy instead of assuming they are present.
func (m *SubstrateIndustryMapper) GetClassification(symbol string) (*IndustryClassification, bool) {
	norm := normalizeSymbol(symbol)
	if norm == "" {
		return m.inner.GetClassification(symbol)
	}
	sector, ok := m.substrate.ResolveL1(norm)
	if !ok || sector == "" {
		return m.inner.GetClassification(symbol)
	}
	l1 := string(sector)
	seg := IndustrySegment{ID: l1, Level: 1}
	if m.tree != nil {
		if fromTree, found := m.tree.GetSegment(l1); found {
			seg = fromTree
			seg.ID = l1
			seg.Level = 1
		}
	}
	return &IndustryClassification{Symbol: norm, Level1: seg}, true
}

// GetSymbolsByIndustry forwards to the wrapped mapper (see the type comment).
func (m *SubstrateIndustryMapper) GetSymbolsByIndustry(industryID string) []string {
	return m.inner.GetSymbolsByIndustry(industryID)
}

// substratePopulation returns the substrate's symbol population, normalized,
// de-duplicated and sorted. It returns nil when no substrate is installed or
// the substrate is empty, so callers can fall back to the tree without having
// to distinguish the two cases (an empty substrate must never silently shrink
// the universe to zero).
func substratePopulation(substrate industry.SymbolIndustrySubstrate) []string {
	if substrate == nil {
		return nil
	}
	raw := substrate.Symbols()
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		key := normalizeSymbol(strings.TrimSpace(s))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
