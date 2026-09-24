package industry

import (
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// ForeignNamespace names a non-canonical sector vocabulary. Internal/industry
// re-exports sectormap's namespaces so callers inside the industry domain do not
// need to import the leaf package directly.
//
// See internal/sectormap/doc.go for the design rules (explicit tables only, no
// fuzzy matching, unmapped keys are reported rather than dropped).
type ForeignNamespace = sectormap.Namespace

// Declared foreign namespaces. Issue #1943.
const (
	NamespaceCanonical                = sectormap.NamespaceCanonical
	NamespaceRepresentativeStocks     = sectormap.NamespaceRepresentativeStocks
	NamespaceClassificationTree       = sectormap.NamespaceClassificationTree
	NamespaceSectorSymbols            = sectormap.NamespaceSectorSymbols
	NamespaceGICSBaseWeights          = sectormap.NamespaceGICSBaseWeights
	NamespaceDefaultMetrics           = sectormap.NamespaceDefaultMetrics
	NamespaceCycleThresholds          = sectormap.NamespaceCycleThresholds
	NamespaceTWSESectorIndex          = sectormap.NamespaceTWSESectorIndex
	NamespaceTWSESectorIndexLegacy    = sectormap.NamespaceTWSESectorIndexLegacy
	NamespaceSectorIndexReader        = sectormap.NamespaceSectorIndexReader
	NamespaceStrategyTechniqueSectors = sectormap.NamespaceStrategyTechniqueSectors
)

// ForeignMapping is the typed view of one foreign key's disposition.
type ForeignMapping struct {
	Namespace  ForeignNamespace
	Key        string
	Targets    map[SectorID]float64
	L1         SectorID
	Candidates []SectorID
	Status     sectormap.Status
	Reason     string
}

// Materialized reports whether the key has at least one canonical target.
func (m ForeignMapping) Materialized() bool { return len(m.Targets) > 0 }

// ResolveForeign returns the declared disposition of key inside namespace ns,
// with canonical IDs typed as SectorID. Unmapped and unknown keys are returned
// with an explicit status and reason — never as a silent zero value.
func ResolveForeign(ns ForeignNamespace, key string) ForeignMapping {
	raw := sectormap.Resolve(ns, key)
	out := ForeignMapping{
		Namespace: raw.Namespace,
		Key:       raw.Key,
		Status:    raw.Status,
		Reason:    raw.Reason,
	}
	if len(raw.Targets) > 0 {
		out.Targets = make(map[SectorID]float64, len(raw.Targets))
		for id, w := range raw.Targets {
			out.Targets[SectorID(id)] = w
		}
	}
	for _, c := range raw.Candidates {
		out.Candidates = append(out.Candidates, SectorID(c))
	}
	if l1, ok := sectormap.ResolveL1(ns, key); ok {
		out.L1 = SectorID(l1)
	}
	return out
}

// CanonicalFromForeign maps a foreign key to its primary canonical ID
// (L1 or L2). It returns ("", false) when the key is unmapped or unknown.
func CanonicalFromForeign(ns ForeignNamespace, key string) (SectorID, bool) {
	id, ok := sectormap.Resolve(ns, key).Primary()
	if !ok {
		return "", false
	}
	sid := SectorID(id)
	if !sid.IsValid() {
		// Defensive: the tables are drift-tested to only contain canonical IDs.
		return "", false
	}
	return sid, true
}

// CanonicalL1FromForeign maps a foreign key to a canonical *L1* sector,
// rolling canonical L2 targets up through the declared L2→L1 parent table.
// It returns ("", false) for unmapped keys and for L2 keys without an L1 parent
// (currently etf_rotation).
func CanonicalL1FromForeign(ns ForeignNamespace, key string) (SectorID, bool) {
	id, ok := sectormap.ResolveL1(ns, key)
	if !ok {
		return "", false
	}
	sid := SectorID(id)
	if !sid.IsL1() {
		return "", false
	}
	return sid, true
}

// ParentL1 resolves a canonical SectorID to its canonical L1 sector. Canonical
// L1 IDs are their own parent. It returns ("", false) for unknown IDs and for
// the L2 ID etf_rotation, which is an asset-class bucket rather than an equity
// industry.
func ParentL1(id SectorID) (SectorID, bool) {
	parent, ok := sectormap.ParentL1Of(string(id))
	if !ok {
		return "", false
	}
	return SectorID(parent), true
}

// NamespaceAudit is the per-namespace disposition summary used by the
// namespace audit CLI and by the tests guarding the canonical lists.
type NamespaceAudit struct {
	Namespace    ForeignNamespace `json:"namespace"`
	DeclaredKeys int              `json:"declared_keys"`
	CanonicalKey int              `json:"canonical_keys"`
	MappedKeys   int              `json:"mapped_keys"`
	UnmappedKeys int              `json:"unmapped_keys"`
	UnmappedList []string         `json:"unmapped_key_list"`
	CoveredL1    []SectorID       `json:"covered_canonical_l1"`
}

// AuditedNamespaces returns the declared namespaces, sorted.
func AuditedNamespaces() []ForeignNamespace { return sectormap.Namespaces() }

// AuditNamespace summarises one namespace.
func AuditNamespace(ns ForeignNamespace) NamespaceAudit {
	rep := sectormap.Report(ns)
	out := NamespaceAudit{
		Namespace:    rep.Namespace,
		DeclaredKeys: rep.DeclaredKeys,
		CanonicalKey: rep.Canonical,
		MappedKeys:   rep.Mapped,
		UnmappedKeys: rep.Unmapped,
		UnmappedList: rep.UnmappedKeys,
	}
	for _, id := range rep.CoveredL1 {
		out.CoveredL1 = append(out.CoveredL1, SectorID(id))
	}
	return out
}

// AuditAllNamespaces summarises every declared namespace.
func AuditAllNamespaces() []NamespaceAudit {
	out := make([]NamespaceAudit, 0, len(sectormap.Namespaces()))
	for _, ns := range sectormap.Namespaces() {
		out = append(out, AuditNamespace(ns))
	}
	return out
}
