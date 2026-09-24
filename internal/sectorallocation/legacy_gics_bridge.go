package sectorallocation

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// ErrLegacyGICSUnmapped is returned by ProjectLegacyGICSWeights when the GICS-style
// weight vector contains keys that have no canonical L1 target and carry a
// non-zero weight. Callers must resolve those keys (or explicitly zero them);
// silently dropping them would fabricate an L1 vector that does not sum to the
// intended total.
var ErrLegacyGICSUnmapped = errors.New("sectorallocation: legacy GICS weights contain unmapped keys")

// LegacyGICSProjection is the result of projecting the GICS-style
// `sector_allocation.base_weights` vocabulary onto canonical L1 sectors.
//
// Issue #1943: the legacy WeightEngine.ComputeWeights iterates
// cfg.BaseWeights and therefore emits GICS keys (healthcare, industrials,
// materials, real_estate, utilities, _cash_reserve). Those keys can never pass
// ValidateL1FinalTarget, which requires exactly the 20 canonical L1 IDs — the
// legacy output was structurally unconsumable, not merely mislabelled. This
// projection makes the translation explicit and reports what cannot be
// translated instead of dropping it.
type LegacyGICSProjection struct {
	// Weights is the projected canonical L1 weight vector. It is NOT
	// renormalized: it only contains weight that could be translated.
	Weights map[industry.SectorID]float64
	// BlockedWeight is the total weight of keys that could not be translated.
	BlockedWeight float64
	// MappedKeys / UnmappedKeys list the input keys by disposition, sorted.
	MappedKeys   []string
	UnmappedKeys []string
	// Candidates maps each unmapped key to the canonical L1 sectors an operator
	// may choose from. Documentation only — nothing is applied automatically.
	Candidates map[string][]industry.SectorID
}

// ProjectLegacyGICSWeights translates a GICS-style weight map into canonical L1
// weights using the declared namespace table in internal/sectormap.
//
// It returns ErrLegacyGICSUnmapped when any key with a non-zero weight could not
// be translated. The projection is still returned so the caller can report the
// blocked weight and the candidate targets.
func ProjectLegacyGICSWeights(base map[string]float64) (LegacyGICSProjection, error) {
	proj := LegacyGICSProjection{
		Weights:      make(map[industry.SectorID]float64, len(base)),
		MappedKeys:   []string{},
		UnmappedKeys: []string{},
		Candidates:   map[string][]industry.SectorID{},
	}
	for key, weight := range base {
		m := industry.ResolveForeign(industry.NamespaceGICSBaseWeights, key)
		contributed := false
		for id, share := range m.Targets {
			l1, ok := industry.ParentL1(id)
			if !ok {
				continue
			}
			proj.Weights[l1] += weight * share
			contributed = true
		}
		if contributed {
			proj.MappedKeys = append(proj.MappedKeys, key)
			continue
		}
		proj.UnmappedKeys = append(proj.UnmappedKeys, key)
		if len(m.Candidates) > 0 {
			proj.Candidates[key] = m.Candidates
		}
		if weight > 0 {
			proj.BlockedWeight += weight
		}
	}
	slices.Sort(proj.MappedKeys)
	slices.Sort(proj.UnmappedKeys)
	if len(proj.UnmappedKeys) > 0 && proj.BlockedWeight > 0 {
		return proj, fmt.Errorf("%w: %s (blocked weight %.4f)",
			ErrLegacyGICSUnmapped, strings.Join(proj.UnmappedKeys, ", "), proj.BlockedWeight)
	}
	return proj, nil
}

// FilterL1KeysReport is FilterL1Keys with the dropped keys reported instead of
// discarded in silence. Every dropped key is returned sorted, so a caller can
// log or fail on it rather than discovering months later that 60% of the input
// (all the GICS keys) never reached the L1 vector.
func FilterL1KeysReport(in map[string]float64) (kept map[string]float64, dropped []string) {
	kept = make(map[string]float64, len(in))
	dropped = []string{}
	for k, v := range in {
		if isL1KeyForFilter(k) {
			kept[k] = v
			continue
		}
		dropped = append(dropped, k)
	}
	slices.Sort(dropped)
	return kept, dropped
}

// L1KeysOnlyReport is LegacyCompatReader.L1KeysOnly with the dropped keys
// reported. The returned map is a copy.
func (r *LegacyCompatReader) L1KeysOnlyReport() (map[string]float64, []string) {
	if r == nil {
		return map[string]float64{}, nil
	}
	return FilterL1KeysReport(r.Read())
}

// ProjectedL1Keys returns the canonical L1 keys reachable from a GICS-style
// weight map, sorted. Unmapped keys are excluded and reported separately by
// ProjectLegacyGICSWeights.
func ProjectedL1Keys(base map[string]float64) []industry.SectorID {
	proj, _ := ProjectLegacyGICSWeights(base)
	return slices.Sorted(maps.Keys(proj.Weights))
}
