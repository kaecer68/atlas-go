package sectormap

import "slices"

// canonicalL1 is the canonical Level-1 equity sector taxonomy: 20 sectors,
// mirroring industry.DisplayZHTw (internal/industry/sector.go).
var canonicalL1 = []string{
	"auto",
	"biotech",
	"cement",
	"chemicals",
	"construction",
	"electronics",
	"energy",
	"financials",
	"food",
	"machinery",
	"optoelectronics",
	"other_electronics",
	"plastics",
	"retail",
	"semiconductor",
	"shipping",
	"steel",
	"telecom",
	"textiles",
	"tourism",
}

// canonicalL2 is the canonical Level-2 research/subsector taxonomy: 18
// sub-industries, mirroring industry.SubIndustryDisplayZHTw.
var canonicalL2 = []string{
	"ai_supply_chain",
	"consumer",
	"cooling",
	"copper_industry",
	"etf_rotation",
	"foundry",
	"ground_equipment",
	"industrial",
	"laser_communication",
	"leo_satellite",
	"metal_processing",
	"mining",
	"precious_metals_recycling",
	"rare_earth_specialty",
	"robotics",
	"satellite_pcb",
	"satellite_rf_components",
	"server_assembly",
}

// l2ParentL1 declares, for each canonical L2 sub-industry, the canonical L1
// sector it rolls up into. The canonical taxonomy in internal/industry/sector.go
// declares only the ID set and the layer, not the parent relation, so consumers
// that must answer "which L1 does this symbol belong to?" had no way to resolve
// an L2-only segment. The relation is declared here, explicitly, so it can be
// reviewed and tested instead of guessed:
//
//   - foundry / server_assembly / cooling / satellite_pcb / ai_supply_chain are
//     Taiwan electronics supply-chain subsectors → electronics (the pre-#1943
//     marketdata legacy map agreed for ai_supply_chain).
//   - robotics / industrial / metal_processing are capital-goods subsectors →
//     machinery (the pre-#1943 marketdata legacy map agreed for robotics).
//   - satellite_rf_components is a components subsector → other_electronics.
//   - leo_satellite / ground_equipment are communications-infrastructure
//     subsectors → telecom.
//   - laser_communication is an optical components subsector → optoelectronics.
//   - mining / copper_industry / precious_metals_recycling are metals →
//     steel (TWSE 鋼鐵類 covers mining/metal producers).
//   - rare_earth_specialty is a specialty-materials subsector → chemicals.
//   - consumer is a broad demand bucket; TWSE maps 消費 to 百貨零售 → retail.
//
// etf_rotation is deliberately absent: it is an asset-class rotation bucket,
// not an equity industry, so it has no canonical L1 parent. ParentL1Of reports
// false for it.
var l2ParentL1 = map[string]string{
	"ai_supply_chain":           "electronics",
	"consumer":                  "retail",
	"cooling":                   "electronics",
	"copper_industry":           "steel",
	"foundry":                   "semiconductor",
	"ground_equipment":          "telecom",
	"industrial":                "machinery",
	"laser_communication":       "optoelectronics",
	"leo_satellite":             "telecom",
	"metal_processing":          "machinery",
	"mining":                    "steel",
	"precious_metals_recycling": "steel",
	"rare_earth_specialty":      "chemicals",
	"robotics":                  "machinery",
	"satellite_pcb":             "electronics",
	"satellite_rf_components":   "other_electronics",
	"server_assembly":           "electronics",
}

// CanonicalL1IDs returns the 20 canonical L1 sector IDs in sorted order.
func CanonicalL1IDs() []string {
	out := slices.Clone(canonicalL1)
	slices.Sort(out)
	return out
}

// CanonicalL2IDs returns the 18 canonical L2 sub-industry IDs in sorted order.
func CanonicalL2IDs() []string {
	out := slices.Clone(canonicalL2)
	slices.Sort(out)
	return out
}

// CanonicalIDs returns the 38 canonical IDs (L1 + L2) in sorted order.
func CanonicalIDs() []string {
	out := make([]string, 0, len(canonicalL1)+len(canonicalL2))
	out = append(out, canonicalL1...)
	out = append(out, canonicalL2...)
	slices.Sort(out)
	return out
}

// IsCanonicalL1 reports whether id is one of the 20 canonical L1 sectors.
func IsCanonicalL1(id string) bool { return slices.Contains(canonicalL1, id) }

// IsCanonicalL2 reports whether id is one of the 18 canonical L2 sub-industries.
func IsCanonicalL2(id string) bool { return slices.Contains(canonicalL2, id) }

// IsCanonical reports whether id is a canonical L1 or L2 ID.
func IsCanonical(id string) bool { return IsCanonicalL1(id) || IsCanonicalL2(id) }

// ParentL1Of resolves a canonical ID to its canonical L1 sector. A canonical L1
// ID is its own parent. An L2 ID returns its declared parent. It returns
// ("", false) for unknown IDs and for L2 IDs that have no L1 parent
// (currently etf_rotation).
func ParentL1Of(id string) (string, bool) {
	if IsCanonicalL1(id) {
		return id, true
	}
	if parent, ok := l2ParentL1[id]; ok {
		return parent, true
	}
	return "", false
}

// L2ParentL1Table returns a copy of the canonical L2 → L1 parent table.
func L2ParentL1Table() map[string]string {
	out := make(map[string]string, len(l2ParentL1))
	for k, v := range l2ParentL1 {
		out[k] = v
	}
	return out
}
