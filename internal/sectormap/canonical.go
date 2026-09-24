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
// reviewed and tested instead of guessed.
//
// Rule (issue #1943): wherever the authored classification tree
// (configs/parameters/industry.json → classification_tree.parent_id) declares a
// parent chain for an L2 segment, this table MUST agree with it. Otherwise the
// same key would resolve to two different L1 sectors depending on whether it
// came in as a tree segment or as a raw key — exactly the class of defect this
// package removes. TestDrift_L2ParentL1AgreesWithAuthoredTree enforces the rule.
// For L2 IDs that are tree Level-1 roots (ai_supply_chain, consumer, industrial,
// leo_satellite, mining, robotics) the table is the only declaration and the
// rationale is written per entry.
//
// Rationale per entry:
//
//   - ai_supply_chain → electronics: 台灣電子供應鏈, no dedicated canonical L1.
//     The pre-#1943 marketdata legacy map agreed.
//   - consumer → retail: broad demand bucket; TWSE maps 消費 to 百貨零售. The
//     linked sector_symbols.json representative list leans plastics/food instead
//     — that is a data conflict recorded in the spec, not a mapping decision.
//   - cooling → semiconductor, server_assembly → semiconductor,
//     satellite_pcb → telecom, satellite_rf_components → telecom,
//     laser_communication → telecom: the authored tree parents
//     (semiconductor / leo_satellite).
//   - foundry → semiconductor: 晶圓代工.
//   - ground_equipment → telecom: authored tree parent leo_satellite.
//   - industrial → machinery, robotics → machinery, metal_processing → machinery:
//     資本財／機械. metal_processing follows its authored parent mining → steel.
//   - mining → steel, copper_industry → steel, precious_metals_recycling → steel,
//     rare_earth_specialty → steel: authored parent mining → steel; TWSE 鋼鐵類
//     covers mining and metal producers.
//   - etf_rotation → none: it is an asset-class rotation bucket, not an equity
//     industry, so it has no canonical L1 parent. ParentL1Of reports false.
var l2ParentL1 = map[string]string{
	"ai_supply_chain":           "electronics",
	"consumer":                  "retail",
	"cooling":                   "semiconductor",
	"copper_industry":           "steel",
	"foundry":                   "semiconductor",
	"ground_equipment":          "telecom",
	"industrial":                "machinery",
	"laser_communication":       "telecom",
	"leo_satellite":             "telecom",
	"metal_processing":          "steel",
	"mining":                    "steel",
	"precious_metals_recycling": "steel",
	"rare_earth_specialty":      "steel",
	"robotics":                  "machinery",
	"satellite_pcb":             "telecom",
	"satellite_rf_components":   "telecom",
	"server_assembly":           "semiconductor",
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
