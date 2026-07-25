// Package industry — sector canonical resource (FU-7 Phase A).
//
// Single canonical taxonomy for Taiwan-equity sector identifiers.
// Additive: no existing string-based call sites change, every SectorID
// constant has a stable snake_case English name.
//
// Design rules (locked in by FU-7 master doc):
//
//   - Canonical form is the snake_case English SectorID constant
//     (e.g. "semiconductor"). Chinese names (e.g. "半導體") are display
//     labels only and live in DisplayZHTw.
//   - DisplayZHTw maps canonical IDs to their full Traditional-Chinese
//     display label, suitable for user-facing UI. Never use the value
//     as a map key, struct field, or JSON key — use the SectorID.
//   - DisplayZHAliases accepts legacy Chinese aliases (e.g. the
//     truncated "金融" used by frontend demo-data.js or the
//     TWSE index-style 金融保險類 suffix variants).
//   - SectorIDFromString resolves arbitrary inputs (canonical ID, full
//     Chinese label, legacy Chinese alias) to its canonical SectorID.
//
// This file is additive. representative_stocks.go and other canonical
// sources are NOT modified in Phase A. Module migration happens in
// follow-up Phase B-F.
package industry

import (
	"sort"
)

// SectorID is the canonical machine-readable identifier for a Taiwan stock
// sector. Snake_case English, stable across releases.
type SectorID string

// Canonical sector identifiers. Mirrors the 20-sector taxonomy of
// DefaultRepresentativeStocks() in representative_stocks.go.
const (
	SectorSemiconductor    SectorID = "semiconductor"     // 半導體
	SectorElectronics      SectorID = "electronics"       // 電子零組件
	SectorOptoelectronics  SectorID = "optoelectronics"   // 光電
	SectorFinancials       SectorID = "financials"        // 金融保險
	SectorCement           SectorID = "cement"            // 水泥
	SectorPlastics         SectorID = "plastics"          // 塑膠
	SectorTextiles         SectorID = "textiles"          // 紡織
	SectorSteel            SectorID = "steel"             // 鋼鐵
	SectorShipping         SectorID = "shipping"          // 航運
	SectorFood             SectorID = "food"              // 食品
	SectorAuto             SectorID = "auto"              // 汽車
	SectorTelecom          SectorID = "telecom"           // 通信網路
	SectorChemicals        SectorID = "chemicals"         // 化學
	SectorBiotech          SectorID = "biotech"           // 生技醫療
	SectorConstruction     SectorID = "construction"      // 營建
	SectorOtherElectronics SectorID = "other_electronics" // 其他電子
	SectorMachinery        SectorID = "machinery"         // 電機機械
	SectorTourism          SectorID = "tourism"           // 觀光
	SectorRetail           SectorID = "retail"            // 百貨
	SectorEnergy           SectorID = "energy"            // 油電燃氣
)

const (
	SubIndustryAISupplyChain      SectorID = "ai_supply_chain"
	SubIndustryRobotics           SectorID = "robotics"
	SubIndustryConsumer           SectorID = "consumer"
	SubIndustryIndustrial         SectorID = "industrial"
	SubIndustryFoundry            SectorID = "foundry"
	SubIndustryServerAssembly     SectorID = "server_assembly"
	SubIndustryCooling            SectorID = "cooling"
	SubIndustryLEOSatellite       SectorID = "leo_satellite"
	SubIndustrySatelliteRF        SectorID = "satellite_rf_components"
	SubIndustrySatellitePCB       SectorID = "satellite_pcb"
	SubIndustryGroundEquipment    SectorID = "ground_equipment"
	SubIndustryLaserCommunication SectorID = "laser_communication"
	SubIndustryMining             SectorID = "mining"
	SubIndustryPreciousMetals     SectorID = "precious_metals_recycling"
	SubIndustryCopper             SectorID = "copper_industry"
	SubIndustryRareEarth          SectorID = "rare_earth_specialty"
	SubIndustryMetalProcessing    SectorID = "metal_processing"
	SubIndustryETFRotation        SectorID = "etf_rotation"
)

// DisplayZHTw maps a canonical SectorID to its full Traditional-Chinese
// display label. The single source of Chinese labels for new code.
var DisplayZHTw = map[SectorID]string{
	SectorSemiconductor:    "半導體",
	SectorElectronics:      "電子零組件",
	SectorOptoelectronics:  "光電",
	SectorFinancials:       "金融保險",
	SectorCement:           "水泥",
	SectorPlastics:         "塑膠",
	SectorTextiles:         "紡織",
	SectorSteel:            "鋼鐵",
	SectorShipping:         "航運",
	SectorFood:             "食品",
	SectorAuto:             "汽車",
	SectorTelecom:          "通信網路",
	SectorChemicals:        "化學",
	SectorBiotech:          "生技醫療",
	SectorConstruction:     "營建",
	SectorOtherElectronics: "其他電子",
	SectorMachinery:        "電機機械",
	SectorTourism:          "觀光",
	SectorRetail:           "百貨",
	SectorEnergy:           "油電燃氣",
}

// Sub-industries (L2) — Phase F sector extension.
// P3-2 (2026-07-26): Chinese labels added so Hermes/OpenClaw agents display
// human-readable names instead of snake_case IDs.
var SubIndustryDisplayZHTw = map[SectorID]string{
	SubIndustryAISupplyChain:      "AI供應鏈",
	SubIndustryRobotics:           "機器人",
	SubIndustryConsumer:           "消費",
	SubIndustryIndustrial:         "工業",
	SubIndustryFoundry:            "晶圓代工",
	SubIndustryServerAssembly:     "伺服器組裝",
	SubIndustryCooling:            "散熱",
	SubIndustryLEOSatellite:       "低軌衛星",
	SubIndustrySatelliteRF:        "衛星射頻元件",
	SubIndustrySatellitePCB:       "衛星PCB",
	SubIndustryGroundEquipment:    "地面設備",
	SubIndustryLaserCommunication: "雷射通訊",
	SubIndustryMining:             "礦業",
	SubIndustryPreciousMetals:     "貴金屬回收",
	SubIndustryCopper:             "銅產業",
	SubIndustryRareEarth:          "稀土特化",
	SubIndustryMetalProcessing:    "金屬加工",
	SubIndustryETFRotation:        "ETF輪動",
}

// DisplayZHAliases maps legacy Chinese aliases to canonical SectorIDs.
// Used by SectorIDFromString to consume data sources that emit non-canonical
// Chinese labels (e.g. truncated "金融" or TWSE index-style "金融保險類"
// suffix variants) without throwing.
//
// Migration handbook: when you find a new non-canonical Chinese string
// in the wild, add it to DisplayZHAliases (and a test case) rather than
// renaming the canonical SectorID.
var DisplayZHAliases = map[string]SectorID{
	"半導體":    SectorSemiconductor,
	"半導體類":   SectorSemiconductor,
	"電子":     SectorElectronics,
	"電子零組件":  SectorElectronics,
	"電子零組件類": SectorElectronics,
	"光電":     SectorOptoelectronics,
	"金融":     SectorFinancials, // legacy truncated alias (demo-data.js)
	"金融保險":   SectorFinancials,
	"金融保險類":  SectorFinancials, // TWSE index style
	"金融類":    SectorFinancials,
	"水泥":     SectorCement,
	"塑膠":     SectorPlastics,
	"塑化":     SectorPlastics,
	"紡織":     SectorTextiles,
	"鋼鐵":     SectorSteel,
	"航運":     SectorShipping,
	"航運類":    SectorShipping,
	"食品":     SectorFood,
	"汽車":     SectorAuto,
	"通信網路":   SectorTelecom,
	"通信":     SectorTelecom,
	"化學":     SectorChemicals,
	"生技醫療":   SectorBiotech,
	"生技":     SectorBiotech,
	"營建":     SectorConstruction,
	"其他電子":   SectorOtherElectronics,
	"電機機械":   SectorMachinery,
	"機械":     SectorMachinery,
	"觀光":     SectorTourism,
	"觀光類":    SectorTourism,
	"百貨":     SectorRetail,
	"油電燃氣":   SectorEnergy,
	"能源":     SectorEnergy,
}

// subIndustryIDs is the canonical ID set used by IsValid / AllSectors for
// O(1) L2 lookups. Declared as a runtime set because Go does not have const
var subIndustryIDs = func() map[SectorID]struct{} {
	m := make(map[SectorID]struct{}, 18)
	for _, id := range []SectorID{
		SubIndustryAISupplyChain, SubIndustryRobotics, SubIndustryConsumer,
		SubIndustryIndustrial, SubIndustryFoundry, SubIndustryServerAssembly,
		SubIndustryCooling, SubIndustryLEOSatellite, SubIndustrySatelliteRF,
		SubIndustrySatellitePCB, SubIndustryGroundEquipment,
		SubIndustryLaserCommunication, SubIndustryMining,
		SubIndustryPreciousMetals, SubIndustryCopper, SubIndustryRareEarth,
		SubIndustryMetalProcessing, SubIndustryETFRotation,
	} {
		m[id] = struct{}{}
	}
	return m
}()

// String implements fmt.Stringer. Returns the canonical snake_case ID.
func (s SectorID) String() string { return string(s) }

// IsValid reports whether s is a known canonical SectorID (L1 or L2).
func (s SectorID) IsValid() bool {
	if _, ok := DisplayZHTw[s]; ok {
		return true
	}
	_, ok := subIndustryIDs[s]
	return ok
}

func (s SectorID) Layer() string {
	if _, ok := DisplayZHTw[s]; ok {
		return "L1"
	}
	if _, ok := subIndustryIDs[s]; ok {
		return "L2"
	}
	return "unknown"
}

func (s SectorID) IsL1() bool { return s.Layer() == "L1" }
func (s SectorID) IsL2() bool { return s.Layer() == "L2" }

// SectorIDFromString resolves an arbitrary sector string (canonical ID,
// full Chinese label, legacy Chinese alias) to a canonical SectorID.
// Returns ("", false) on miss; callers should treat empty as unknown.
//
// Resolution order:
//  1. Exact canonical match
//  2. DisplayZHAliases reverse lookup
func SectorIDFromString(s string) (SectorID, bool) {
	if s == "" {
		return "", false
	}
	if id := SectorID(s); id.IsValid() {
		return id, true
	}
	if id, ok := DisplayZHAliases[s]; ok {
		return id, true
	}
	return "", false
}

// AllSectors returns every canonical SectorID (L1 + L2), sorted ascending.
func AllSectors() []SectorID {
	out := make([]SectorID, 0, len(DisplayZHTw)+len(subIndustryIDs))
	seen := make(map[SectorID]struct{}, len(DisplayZHTw)+len(subIndustryIDs))
	for id := range DisplayZHTw {
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for id := range subIndustryIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func Layer(s SectorID) string {
	if _, ok := DisplayZHTw[s]; ok {
		return "L1"
	}
	if _, ok := subIndustryIDs[s]; ok {
		return "L2"
	}
	return "unknown"
}

func IsL1(s SectorID) bool { _, ok := DisplayZHTw[s]; return ok }

func IsL2(s SectorID) bool { _, ok := subIndustryIDs[s]; return ok }

// L1Sectors returns the 20 canonical L1 sector IDs in fixed sorted order.
func L1Sectors() []SectorID {
	ids := make([]SectorID, 0, len(DisplayZHTw))
	for id := range DisplayZHTw {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func DisplayZH(id SectorID) string {
	if label, ok := DisplayZHTw[id]; ok {
		return label
	}
	if label, ok := SubIndustryDisplayZHTw[id]; ok && label != "" {
		return label
	}
	if _, ok := subIndustryIDs[id]; ok {
		return string(id)
	}
	return ""
}
