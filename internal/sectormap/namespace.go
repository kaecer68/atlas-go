package sectormap

import (
	"maps"
	"slices"
)

// Namespace identifies one sector vocabulary. Every vocabulary that speaks about
// "industry" / "sector" / "theme" keys is listed here, including the ones that
// are not equity industries at all (strategy buckets, asset classes) — those are
// precisely the ones that must NOT leak into an L1 weight vector, so they are
// declared and reported instead of being ignored.
type Namespace string

const (
	// NamespaceCanonical is the canonical taxonomy itself (20 L1 + 18 L2),
	// owned by internal/industry/sector.go. Keys are identity-mapped.
	NamespaceCanonical Namespace = "canonical_l1_l2"

	// NamespaceRepresentativeStocks is the hard-coded
	// industry.DefaultRepresentativeStocks() map (20 canonical L1 / ~96 symbols).
	NamespaceRepresentativeStocks Namespace = "industry_representative_stocks"

	// NamespaceClassificationTree is configs/parameters/industry.json →
	// classification_tree (16 L1 + 13 L2 segments, 50 representative stocks).
	NamespaceClassificationTree Namespace = "config_classification_tree"

	// NamespaceSectorSymbols is configs/sector_symbols.json (22 keys; consumed by
	// the narrative model, the correlation loader and calibrate-seasonal).
	NamespaceSectorSymbols Namespace = "config_sector_symbols"

	// NamespaceGICSBaseWeights is configs/parameters/sector_allocation.json →
	// base_weights (GICS-style 11 sectors + _cash_reserve).
	NamespaceGICSBaseWeights Namespace = "config_sector_allocation_base_weights"

	// NamespaceDefaultMetrics is configs/parameters/industry.json →
	// default_metrics, the CycleTracker seed map (23 keys).
	NamespaceDefaultMetrics Namespace = "config_industry_default_metrics"

	// NamespaceCycleThresholds is configs/parameters/industry.json →
	// cycle_thresholds (10 keys).
	NamespaceCycleThresholds Namespace = "config_industry_cycle_thresholds"

	// NamespaceTWSESectorIndex is the TWSE OpenAPI v1 industry-name vocabulary
	// (Chinese, 22 names) used by marketdata.TWSESectorIndexProvider.
	NamespaceTWSESectorIndex Namespace = "twse_sector_index_name"

	// NamespaceTWSESectorIndexLegacy is the deprecated 8-name subset that
	// marketdata.TWSESectorIndexProvider.mapIndustryName used to emit with two
	// non-canonical IDs (ai_supply_chain, robotics).
	NamespaceTWSESectorIndexLegacy Namespace = "twse_sector_index_name_legacy"

	// NamespaceSectorIndexReader is the sector-ID vocabulary accepted by
	// marketdata.SectorIndexReader when reading sector_index files: the 20
	// canonical L1 IDs plus the two legacy IDs (ai_supply_chain, robotics).
	NamespaceSectorIndexReader Namespace = "marketdata_sector_index_reader_ids"

	// NamespaceStrategyTechniqueSectors is the free-text `sectors` tag vocabulary
	// in data/seeds/strategy_techniques.json. It mixes equity sectors with size,
	// style and asset-class buckets.
	NamespaceStrategyTechniqueSectors Namespace = "strategy_technique_sectors"

	// NamespaceFinMindSectorSeries is the FinMind "twse" sector index series
	// vocabulary consumed by marketdata.FinMindSectorIndexProvider. It covers 18
	// of the 20 canonical L1 sectors; chemicals and tourism are documented gaps.
	NamespaceFinMindSectorSeries Namespace = "finmind_sector_series"

	// NamespaceTWSESIndustryCode is the TWSE/TPEx listed-company industry code
	// vocabulary: the `產業別` field of TWSE OpenAPI t187ap03_L (上市) and the
	// `SecuritiesIndustryCode` field of TPEx OpenAPI mopsfin_t187ap03_O (上櫃).
	// It is the per-symbol industry source that makes an ETF's published
	// holdings resolvable to canonical L1 sectors without guessing.
	NamespaceTWSESIndustryCode Namespace = "twse_industry_code"

	// NamespaceETFRepresentatives is the delivery-vehicle vocabulary: each
	// Taiwan-listed ETF that sector allocation can trade, mapped to the
	// canonical L1 exposure its published holdings imply. Keys come from
	// configs/etf_metadata.json; the disposition is derived from the issuer's
	// holdings page (see internal/sectormap/etf_representatives.go) and is
	// reported here so the ETF coverage metric is auditable.
	NamespaceETFRepresentatives Namespace = "sectorallocation_etf_representatives"
)

// Status classifies the disposition of a single foreign key.
type Status string

const (
	// StatusCanonical: the foreign key already IS a canonical ID (identity).
	StatusCanonical Status = "canonical"
	// StatusMapped: an explicit, reviewed non-identity mapping exists.
	StatusMapped Status = "mapped"
	// StatusUnmapped: the key is declared but has no canonical target. Callers
	// must report it; dropping it silently is a bug.
	StatusUnmapped Status = "unmapped"
	// StatusUnknown: the key is not declared in this namespace at all. That is
	// drift (upstream changed without updating the table), not "unmapped".
	StatusUnknown Status = "unknown"
)

// Mapping is the resolved disposition of one foreign key.
type Mapping struct {
	Namespace Namespace `json:"namespace"`
	Key       string    `json:"key"`
	// Targets maps canonical IDs to their share of the foreign key. A 1:1
	// mapping has exactly one entry with weight 1. Empty when unmapped.
	Targets map[string]float64 `json:"targets,omitempty"`
	// Candidates lists canonical IDs an operator may pick from when the key is
	// unmapped. These are documentation, not an automatic mapping.
	Candidates []string `json:"candidates,omitempty"`
	Status     Status   `json:"status"`
	Reason     string   `json:"reason,omitempty"`
}

// Primary returns the highest-weighted canonical target (ties broken by ID) and
// whether the mapping has any target at all.
func (m Mapping) Primary() (string, bool) {
	if len(m.Targets) == 0 {
		return "", false
	}
	best := ""
	bestW := -1.0
	for _, id := range slices.Sorted(maps.Keys(m.Targets)) {
		if w := m.Targets[id]; w > bestW {
			best, bestW = id, w
		}
	}
	return best, true
}

// IsMapped reports whether the key has at least one canonical target.
func (m Mapping) IsMapped() bool { return len(m.Targets) > 0 }

// Resolve returns the declared disposition of key inside namespace ns. The
// returned Mapping owns its maps; callers may mutate them.
func Resolve(ns Namespace, key string) Mapping {
	table, ok := tables[ns]
	if !ok {
		return Mapping{
			Namespace: ns,
			Key:       key,
			Status:    StatusUnknown,
			Reason:    "unknown namespace: not declared in internal/sectormap",
		}
	}
	d, ok := table[key]
	if !ok {
		return Mapping{
			Namespace: ns,
			Key:       key,
			Status:    StatusUnknown,
			Reason: "key not declared for namespace " + string(ns) +
				": upstream vocabulary changed without updating internal/sectormap (drift)",
		}
	}
	m := Mapping{
		Namespace:  ns,
		Key:        key,
		Candidates: slices.Clone(d.candidates),
		Reason:     d.reason,
	}
	if d.self {
		m.Targets = map[string]float64{key: 1.0}
		m.Status = StatusCanonical
		if m.Reason == "" {
			m.Reason = "key is already a canonical sector ID"
		}
		return m
	}
	if len(d.targets) == 0 {
		m.Status = StatusUnmapped
		return m
	}
	m.Targets = maps.Clone(d.targets)
	m.Status = StatusMapped
	return m
}

// ResolveL1 resolves key to its canonical L1 sector: a direct L1 target wins,
// otherwise an L2 target is rolled up through the declared parent table. It
// returns ("", false) for unmapped and unknown keys — callers must handle that
// explicitly rather than defaulting to a bucket.
func ResolveL1(ns Namespace, key string) (string, bool) {
	m := Resolve(ns, key)
	if !m.IsMapped() {
		return "", false
	}
	if len(m.Targets) == 1 {
		if id, ok := m.Primary(); ok {
			return ParentL1Of(id)
		}
	}
	// Multi-target mapping: return the L1 parent of the primary target.
	if id, ok := m.Primary(); ok {
		return ParentL1Of(id)
	}
	return "", false
}

// Namespaces returns every declared namespace, sorted.
func Namespaces() []Namespace {
	out := slices.Sorted(maps.Keys(tables))
	return out
}

// Keys returns the declared keys of a namespace, sorted. Unknown namespaces
// return nil.
func Keys(ns Namespace) []string {
	table, ok := tables[ns]
	if !ok {
		return nil
	}
	return slices.Sorted(maps.Keys(table))
}

// Mappings returns the full resolved table for a namespace, sorted by key.
func Mappings(ns Namespace) []Mapping {
	out := make([]Mapping, 0, len(tables[ns]))
	for _, k := range Keys(ns) {
		out = append(out, Resolve(ns, k))
	}
	return out
}

// NamespaceReport is a machine-readable summary of one namespace.
type NamespaceReport struct {
	Namespace    Namespace `json:"namespace"`
	DeclaredKeys int       `json:"declared_keys"`
	Canonical    int       `json:"canonical_keys"`
	Mapped       int       `json:"mapped_keys"`
	Unmapped     int       `json:"unmapped_keys"`
	UnmappedKeys []string  `json:"unmapped_key_list"`
	Candidates   []string  `json:"unmapped_with_candidates,omitempty"`
	CoveredL1    []string  `json:"covered_canonical_l1"`
	CoveredL2    []string  `json:"covered_canonical_l2"`
}

// Report summarizes one namespace.
func Report(ns Namespace) NamespaceReport {
	rep := NamespaceReport{
		Namespace:    ns,
		UnmappedKeys: []string{},
		CoveredL1:    []string{},
		CoveredL2:    []string{},
	}
	for _, m := range Mappings(ns) {
		rep.DeclaredKeys++
		switch m.Status {
		case StatusCanonical:
			rep.Canonical++
		case StatusMapped:
			rep.Mapped++
		case StatusUnmapped:
			rep.Unmapped++
			rep.UnmappedKeys = append(rep.UnmappedKeys, m.Key)
			if len(m.Candidates) > 0 {
				rep.Candidates = append(rep.Candidates, m.Key)
			}
		case StatusUnknown:
			rep.UnmappedKeys = append(rep.UnmappedKeys, m.Key)
		}
	}
	rep.CoveredL1 = CoveredL1(ns, true)
	rep.CoveredL2 = CoveredL2(ns)
	return rep
}

// Reports summarizes every declared namespace.
func Reports() []NamespaceReport {
	out := make([]NamespaceReport, 0, len(tables))
	for _, ns := range Namespaces() {
		out = append(out, Report(ns))
	}
	return out
}

// CoveredL1 returns the canonical L1 IDs reachable from a namespace, sorted.
// When includeL2Parents is true, L2 targets also count for their declared L1
// parent.
func CoveredL1(ns Namespace, includeL2Parents bool) []string {
	seen := map[string]struct{}{}
	for _, m := range Mappings(ns) {
		for id := range m.Targets {
			if IsCanonicalL1(id) {
				seen[id] = struct{}{}
				continue
			}
			if includeL2Parents {
				if parent, ok := ParentL1Of(id); ok {
					seen[parent] = struct{}{}
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// CoveredL2 returns the canonical L2 IDs reachable from a namespace, sorted.
func CoveredL2(ns Namespace) []string {
	seen := map[string]struct{}{}
	for _, m := range Mappings(ns) {
		for id := range m.Targets {
			if IsCanonicalL2(id) {
				seen[id] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}
