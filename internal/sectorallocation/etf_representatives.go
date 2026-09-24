package sectorallocation

import (
	"slices"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// ETFRepresentative is the typed view of one declared ETF → canonical L1
// exposure row. The authoritative table lives in internal/sectormap
// (NamespaceETFRepresentatives) because that package is a leaf: the audit CLI
// and the drift tests can read the table without importing sectorallocation.
//
// This file adds nothing to the data. It only types it (industry.SectorID keys
// instead of strings) so sector-allocation call sites cannot hand a typo through
// to a weight vector.
type ETFRepresentative struct {
	Symbol    string
	Name      string
	Benchmark string
	Issuer    string
	// AsOf is the data date of the issuer's holdings page the row was derived
	// from, not the date this code was written.
	AsOf string
	// SourceURL is the first-party page the holdings were read from.
	SourceURL string
	// Holdings is the number of positions the issuer published.
	Holdings int
	// L1Weights is the canonical L1 exposure implied by those holdings. It sums
	// to 1 (the uncovered remainder is reported by sectormap, not hidden here).
	L1Weights map[industry.SectorID]float64
}

// ETFRepresentatives returns every declared ETF row, ordered by symbol. The
// returned slice and its maps are deep copies.
func ETFRepresentatives() []ETFRepresentative {
	src := sectormap.ETFRepresentatives()
	out := make([]ETFRepresentative, 0, len(src))
	for _, r := range src {
		out = append(out, ETFRepresentative{
			Symbol:    r.Symbol,
			Name:      r.Name,
			Benchmark: r.Benchmark,
			Issuer:    r.Issuer,
			AsOf:      r.AsOf,
			SourceURL: r.SourceURL,
			Holdings:  r.Holdings,
			L1Weights: typedWeights(r.L1Targets),
		})
	}
	return out
}

// ETFRepresentativeSymbols returns the declared ETF symbols, sorted.
func ETFRepresentativeSymbols() []string { return sectormap.ETFRepresentativeSymbols() }

// ETFRepresentativeLookup returns the L1 weight vector of one ETF symbol, and
// whether the symbol is declared. The returned map is a copy.
func ETFRepresentativeLookup(symbol string) (map[industry.SectorID]float64, bool) {
	m := sectormap.Resolve(sectormap.NamespaceETFRepresentatives, symbol)
	if !m.IsMapped() {
		return nil, false
	}
	return typedWeights(m.Targets), true
}

// ETFL1Coverage returns the canonical L1 sectors reachable from any declared
// ETF, sorted. This is the metric the industry-namespace audit publishes as
// "ETF L1 coverage"; the acceptance floor (>= 12) is pinned by
// TestETFL1Coverage_MeetsFloorIsBackedByHoldings.
func ETFL1Coverage() []industry.SectorID {
	ids := sectormap.ETFL1Coverage()
	out := make([]industry.SectorID, 0, len(ids))
	for _, id := range ids {
		out = append(out, industry.SectorID(id))
	}
	return out
}

// ETFL1CoverageCount is the cardinality of ETFL1Coverage().
func ETFL1CoverageCount() int { return sectormap.ETFL1CoverageCount() }

// typedWeights converts a string-keyed weight map to industry.SectorID keys.
func typedWeights(m map[string]float64) map[industry.SectorID]float64 {
	out := make(map[industry.SectorID]float64, len(m))
	for id, w := range m {
		out[industry.SectorID(id)] = w
	}
	return out
}

// ETFRepresentativeL1Set returns the L1 sectors one ETF touches, sorted. Useful
// for reporting "which ETF can I use for sector X" without re-deriving it.
func ETFRepresentativeL1Set(symbol string) []industry.SectorID {
	weights, ok := ETFRepresentativeLookup(symbol)
	if !ok {
		return nil
	}
	out := make([]industry.SectorID, 0, len(weights))
	for id := range weights {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
