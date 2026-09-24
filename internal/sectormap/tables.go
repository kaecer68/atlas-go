package sectormap

// decl is the declared disposition of one foreign key. Exactly one of `self`
// (identity: the key is already canonical) or `targets` (explicit mapping) is
// set, or neither, which means the key is declared-unmapped.
//
// targets is a map so a 1:many mapping with explicit distribution weights is
// representable (Mapping.Targets sums to 1). No table uses that today: every
// declared mapping is 1:1, and compound vocabularies (GICS industrials/materials,
// TWSE 化學生技醫療類指數…) are declared unmapped with candidates instead of
// having weights invented for them.
type decl struct {
	self       bool
	targets    map[string]float64
	candidates []string
	reason     string
}

// ident declares key == canonical ID (identity mapping).
func ident() decl { return decl{self: true} }

// identReason declares an identity mapping that still needs a note.
func identReason(reason string) decl { return decl{self: true, reason: reason} }

// one declares a 1:1 mapping to a single canonical ID.
func one(target, reason string) decl {
	return decl{targets: map[string]float64{target: 1.0}, reason: reason}
}

// unmapped declares a key with no canonical target. candidates are canonical IDs
// an operator may choose from; they are never applied automatically.
func unmapped(reason string, candidates ...string) decl {
	return decl{candidates: candidates, reason: reason}
}

// forKeys builds a namespace table by selecting keys from a shared disposition
// table. The key list stays explicit per namespace so that a vocabulary change
// shows up as a drift-test failure for the affected namespace only.
func forKeys(base map[string]decl, keys ...string) map[string]decl {
	out := make(map[string]decl, len(keys))
	for _, k := range keys {
		d, ok := base[k]
		if !ok {
			panic("sectormap: key " + k + " has no declared disposition in the shared table")
		}
		out[k] = d
	}
	return out
}

// treeStyleKeys is the shared disposition table for the config-tree family of
// vocabularies (classification_tree, sector_symbols, default_metrics,
// cycle_thresholds). Keys not listed here cannot be used by those namespaces.
var treeStyleKeys = map[string]decl{
	// Canonical L1 sectors used verbatim by the config tree.
	"electronics": ident(),
	"energy":      ident(),
	"financials":  ident(),
	"semiconductor": identReason(
		"canonical L1 sector ID used verbatim by the config tree"),
	"shipping": ident(),
	"tourism":  identReason("canonical L1 sector ID; only sector_symbols.json declares it"),

	// Canonical L2 sub-industries that the config tree promotes to Level-1
	// segments. They are already canonical IDs, but they are L2, not L1 — that
	// layer mismatch is what broke SymbolL1Mapper and sectorallocation.
	"ai_supply_chain": identReason(
		"canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 electronics via ParentL1Of"),
	"consumer": identReason(
		"canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 retail via ParentL1Of"),
	"cooling":         ident(),
	"copper_industry": ident(),
	"etf_rotation": identReason(
		"canonical L2 ID, but it is an asset-class rotation bucket with no canonical L1 parent (ParentL1Of returns false)"),
	"foundry":                   ident(),
	"ground_equipment":          ident(),
	"industrial":                identReason("canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 machinery via ParentL1Of"),
	"laser_communication":       ident(),
	"leo_satellite":             identReason("canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 telecom via ParentL1Of"),
	"metal_processing":          ident(),
	"mining":                    identReason("canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 steel via ParentL1Of"),
	"precious_metals_recycling": ident(),
	"rare_earth_specialty":      ident(),
	"robotics":                  identReason("canonical L2 sub-industry promoted to a tree Level-1 segment; rolls up to L1 machinery via ParentL1Of"),
	"satellite_pcb":             ident(),
	"satellite_rf_components":   ident(),
	"server_assembly":           ident(),

	// Declared, but not equity industries. These must not enter an L1 weight
	// vector (docs/specs/sector-allocation-simulation-closure-spec.md §3.2).
	"defensive": unmapped(
		"strategy_bucket: 防禦性資產 is a style bucket, not an equity sector; spec §3.2 forbids it in equity_sector_l1",
		"cement", "food", "telecom", "energy"),
	"high_dividend": unmapped(
		"strategy_bucket: 高股息 is a yield screen, not an equity sector; spec §3.2 forbids it in equity_sector_l1"),
	"small_cap": unmapped(
		"strategy_bucket: 中小型股 is a size bucket, not an equity sector; spec §3.2 forbids it in equity_sector_l1",
		"retail", "textiles", "construction"),
	"tech": unmapped(
		"research_theme: 科技股 spans several canonical L1 sectors; spec §3.2 forbids research themes in equity_sector_l1",
		"electronics", "semiconductor", "other_electronics", "optoelectronics", "telecom"),
	"pcb": unmapped(
		"research_theme: canonical taxonomy has no PCB node; PCB makers sit inside L1 electronics (and satellite_pcb is satellite-specific)",
		"electronics", "other_electronics"),
	"thermal": unmapped(
		"research_theme: canonical taxonomy has no 散熱 node; cooling companies are electronics components (nearest canonical L2 is cooling)",
		"electronics", "other_electronics", "cooling"),
}

// gicsBaseWeightKeys is the disposition table for
// configs/parameters/sector_allocation.json → base_weights (GICS-style).
var gicsBaseWeightKeys = map[string]decl{
	"electronics":   ident(),
	"energy":        ident(),
	"financials":    ident(),
	"semiconductor": ident(),
	"telecom":       ident(),
	"consumer": one("consumer",
		"GICS broad Consumer (staples + discretionary) → canonical L2 consumer; the TWSE split across food/textiles/retail/auto is not recoverable from one GICS key"),
	"healthcare": one("biotech",
		"GICS Health Care ≈ TWSE 生技醫療類; canonical taxonomy has no separate healthcare node"),
	"industrials": unmapped(
		"GICS Industrials spans four canonical L1 sectors; a 1:1 or weighted mapping would be an operator decision, not a mechanical one",
		"machinery", "construction", "shipping", "steel"),
	"materials": unmapped(
		"GICS Materials spans four canonical L1 sectors with no defensible single target",
		"chemicals", "plastics", "cement", "steel"),
	"real_estate": unmapped(
		"canonical taxonomy has no real-estate L1; GICS Real Estate is mostly REITs/developers whereas canonical construction is 建材營造",
		"construction"),
	"utilities": unmapped(
		"canonical taxonomy has no utilities L1; TWSE 油電燃氣 maps to energy, but GICS Utilities also includes water/waste",
		"energy"),
	"_cash_reserve": unmapped(
		"asset_class namespace (cash reserve), not an equity sector; spec §3.2 forbids it in equity_sector_l1"),
}

// twseSectorIndexKeys maps the TWSE OpenAPI v1 Chinese index names
// (exchangeReport/MI_INDEX) to canonical L1 sectors.
//
// Evidence note (2026-09-24, live MI_INDEX response): the endpoint returns 37
// names ending in 類指數. This table maps the 20 that have a defensible single
// canonical L1 target — two pairs collapse (電腦及週邊設備 + 電子零組件 →
// electronics, 電機機械 + 電器電纜 → machinery) so all 20 canonical L1 sectors
// are reachable — and declares the remaining 15 as unmapped with reasons, so
// that marketdata.TWSESectorIndexProvider's silent drop becomes an auditable
// list. Two names previously declared here (化學工業類指數, 觀光類指數) do NOT
// exist in the live response; they are kept as historical aliases because old
// cached files may contain them.
var twseSectorIndexKeys = map[string]decl{
	// --- 20 canonical L1 sectors (mapped) ---
	"半導體類指數":     one("semiconductor", "TWSE industry index name"),
	"電腦及週邊設備類指數": one("electronics", "TWSE has no separate computer/peripheral L1; canonical electronics is the only home"),
	"電子零組件類指數":   one("electronics", "TWSE industry index name"),
	"其他電子類指數":    one("other_electronics", "TWSE industry index name"),
	"光電類指數":      one("optoelectronics", "TWSE industry index name"),
	"通信網路類指數":    one("telecom", "TWSE industry index name"),
	"航運類指數":      one("shipping", "TWSE industry index name"),
	"金融保險類指數":    one("financials", "TWSE industry index name"),
	"油電燃氣類指數":    one("energy", "TWSE industry index name"),
	"電機機械類指數":    one("machinery", "TWSE industry index name"),
	"電器電纜類指數":    one("machinery", "TWSE electric-wire/cable makers are grouped with 電機機械 in the canonical taxonomy"),
	"水泥類指數":      one("cement", "TWSE industry index name"),
	"食品類指數":      one("food", "TWSE industry index name"),
	"塑膠類指數":      one("plastics", "TWSE industry index name"),
	"紡織纖維類指數":    one("textiles", "TWSE industry index name"),
	"鋼鐵類指數":      one("steel", "TWSE industry index name"),
	"汽車類指數":      one("auto", "TWSE industry index name"),
	"化學類指數":      one("chemicals", "live TWSE name for the 化學 index; #1943 closed the chemicals gap with this key"),
	"生技醫療類指數":    one("biotech", "TWSE industry index name"),
	"建材營造類指數":    one("construction", "TWSE industry index name"),
	"觀光餐旅類指數":    one("tourism", "live TWSE name for the 觀光 index; #1943 closed the tourism gap with this key"),
	"貿易百貨類指數":    one("retail", "TWSE industry index name"),

	// --- historical aliases, absent from the live response ---
	"化學工業類指數": one("chemicals", "historical name, not present in the live MI_INDEX response (2026-09-24); kept so cached raw files still resolve"),
	"觀光類指數":   one("tourism", "historical name, not present in the live MI_INDEX response (2026-09-24); kept so cached raw files still resolve"),

	// --- live TWSE names with no defensible single canonical L1 target ---
	"電子工業類指數": unmapped("broad electronics-industry index overlapping 電子零組件/光電/其他電子; mapping it would double count",
		"electronics", "other_electronics", "optoelectronics"),
	"電子通路類指數": unmapped("electronics distribution channel, not a manufacturing sector",
		"electronics", "retail"),
	"資訊服務類指數": unmapped("no canonical IT-services node",
		"other_electronics", "telecom"),
	"數位雲端類指數": unmapped("no canonical digital/cloud node",
		"other_electronics"),
	"綠能環保類指數": unmapped("no canonical green-energy node",
		"energy"),
	"化學生技醫療類指數": unmapped("compound index spanning chemicals and biotech; weights would be invented",
		"chemicals", "biotech"),
	"塑膠化工類指數": unmapped("compound index spanning plastics and chemicals; weights would be invented",
		"plastics", "chemicals"),
	"玻璃陶瓷類指數": unmapped("no canonical glass/ceramics node",
		"cement", "chemicals"),
	"水泥窯製類指數": unmapped("overlaps 水泥類指數; mapping both to cement would double count the same group",
		"cement"),
	"造紙類指數": unmapped("no canonical paper node",
		"plastics", "chemicals"),
	"橡膠類指數": unmapped("no canonical rubber node",
		"plastics"),
	"機電類指數": unmapped("mechanical/electrical engineering group, distinct from 電機機械類指數",
		"machinery", "construction"),
	"運動休閒類指數": unmapped("consumer discretionary bucket, not an equity sector",
		"retail", "textiles", "auto"),
	"居家生活類指數": unmapped("consumer discretionary bucket, not an equity sector",
		"retail"),
	"其他類指數": unmapped("residual bucket of the TWSE taxonomy; has no canonical L1 target by construction"),
}

// twseSectorIndexLegacyKeys is the deprecated 8-name subset that
// TWSESectorIndexProvider.mapIndustryName emitted. It used two non-canonical
// IDs (ai_supply_chain, robotics); the table below states the canonical L1 each
// of those must now resolve to, so the two TWSE maps can no longer disagree.
var twseSectorIndexLegacyKeys = map[string]decl{
	"半導體類指數":     one("semiconductor", "legacy 8-index schema"),
	"電腦及週邊設備類指數": one("electronics", "legacy 8-index schema emitted ai_supply_chain (canonical L2); canonical L1 is electronics"),
	"電子零組件類指數":   one("electronics", "legacy 8-index schema"),
	"其他電子類指數":    one("other_electronics", "legacy 8-index schema"),
	"航運類指數":      one("shipping", "legacy 8-index schema"),
	"金融保險類指數":    one("financials", "legacy 8-index schema"),
	"油電燃氣類指數":    one("energy", "legacy 8-index schema"),
	"電機機械類指數":    one("machinery", "legacy 8-index schema emitted robotics (canonical L2); canonical L1 is machinery"),
}

// sectorIndexReaderKeys is the ID vocabulary accepted when reading
// sector_index files: the 20 canonical L1 IDs plus the two legacy IDs.
var sectorIndexReaderKeys = func() map[string]decl {
	out := make(map[string]decl, 22)
	for _, id := range canonicalL1 {
		out[id] = ident()
	}
	out["ai_supply_chain"] = one("electronics",
		"legacy 8-industry sector_index files emitted this canonical L2 ID; canonical L1 is electronics")
	out["robotics"] = one("machinery",
		"legacy 8-industry sector_index files emitted this canonical L2 ID; canonical L1 is machinery")
	return out
}()

// strategyTechniqueKeys maps the free-text `sectors` tags in
// data/seeds/strategy_techniques.json. That vocabulary mixes four namespaces,
// which spec §3.2 forbids in a single field.
var strategyTechniqueKeys = map[string]decl{
	"半導體":    one("semiconductor", "canonical L1 sector"),
	"電子":     one("electronics", "canonical L1 sector"),
	"塑化":     one("plastics", "canonical L1 sector (legacy short form of 塑膠)"),
	"AI 供應鏈": one("ai_supply_chain", "canonical L2 sub-industry"),
	"散熱":     one("cooling", "canonical L2 sub-industry (散熱)"),
	"PCB": unmapped(
		"research_theme: no canonical PCB node; PCB makers sit inside L1 electronics",
		"electronics", "other_electronics"),
	"電源": unmapped(
		"research_theme: no canonical power-supply node; switching-power makers are L1 electronics components",
		"electronics", "other_electronics"),
	"內需": unmapped(
		"strategy_bucket: domestic-demand screen, not an equity sector",
		"retail", "food", "construction"),
	"出口股":    unmapped("strategy_bucket: export-revenue screen, not an equity sector", "semiconductor", "electronics"),
	"出口導向股":  unmapped("strategy_bucket: export-revenue screen, not an equity sector", "semiconductor", "electronics"),
	"外銷股":    unmapped("strategy_bucket: export-revenue screen, not an equity sector", "semiconductor", "electronics"),
	"中小型股":   unmapped("strategy_bucket: size screen, not an equity sector", "retail", "textiles", "construction"),
	"權值股":    unmapped("strategy_bucket: index-weight screen, not an equity sector", "semiconductor", "financials"),
	"高股息":    unmapped("strategy_bucket: yield screen, not an equity sector"),
	"防禦性資產":  unmapped("asset_class: defensive asset bucket, not an equity sector", "cement", "food", "telecom"),
	"題材股":    unmapped("strategy_bucket: theme screen, not an equity sector"),
	"科技股":    unmapped("research_theme: spans several canonical L1 sectors", "electronics", "semiconductor", "other_electronics", "optoelectronics", "telecom"),
	"加權指數":   unmapped("asset_class: index level, not an equity sector"),
	"黃金":     unmapped("asset_class: commodity, not an equity sector"),
	"ETF 標的": unmapped("asset_class: ETF vehicle, not an equity sector", "etf_rotation"),
}

// finmindSectorSeriesKeys is the FinMind "twse" sector index series vocabulary
// (English series names) as consumed by marketdata.FinMindSectorIndexProvider.
// It carries 18 of the 20 canonical L1 sectors: 觀光 (tourism) exists upstream
// but is dropped by the provider table, and chemicals has no series. Both gaps
// are reported by the marketdata drift test rather than hidden.
var finmindSectorSeriesKeys = map[string]decl{
	"Automobile":                   one("auto", "FinMind twse series"),
	"BiotechnologyMedicalCare":     one("biotech", "FinMind twse series"),
	"Cement":                       one("cement", "FinMind twse series"),
	"BuildingMaterialConstruction": one("construction", "FinMind twse series"),
	"Electronic":                   one("electronics", "FinMind twse series"),
	"OilGasElectricity":            one("energy", "FinMind twse series"),
	"FinancialInsurance":           one("financials", "FinMind twse series"),
	"Food":                         one("food", "FinMind twse series"),
	"ElectricMachinery":            one("machinery", "FinMind twse series"),
	"Optoelectronic":               one("optoelectronics", "FinMind twse series"),
	"OtherElectronic":              one("other_electronics", "FinMind twse series"),
	"Plastics":                     one("plastics", "FinMind twse series"),
	"TradingConsumersGoods":        one("retail", "FinMind twse series"),
	"Semiconductor":                one("semiconductor", "FinMind twse series"),
	"ShippingTransportation":       one("shipping", "FinMind twse series"),
	"IronSteel":                    one("steel", "FinMind twse series"),
	"CommunicationsInternet":       one("telecom", "FinMind twse series"),
	"Textiles":                     one("textiles", "FinMind twse series"),
}

// tables is the authoritative registry: namespace → foreign key → disposition.
var tables = map[Namespace]map[string]decl{
	NamespaceCanonical: func() map[string]decl {
		out := make(map[string]decl, len(canonicalL1)+len(canonicalL2))
		for _, id := range canonicalL1 {
			out[id] = ident()
		}
		for _, id := range canonicalL2 {
			out[id] = ident()
		}
		return out
	}(),
	NamespaceRepresentativeStocks: func() map[string]decl {
		out := make(map[string]decl, len(canonicalL1))
		for _, id := range canonicalL1 {
			out[id] = ident()
		}
		return out
	}(),
	NamespaceClassificationTree: forKeys(treeStyleKeys,
		// L1 (16)
		"ai_supply_chain", "consumer", "defensive", "electronics", "energy",
		"etf_rotation", "financials", "high_dividend", "industrial",
		"leo_satellite", "mining", "robotics", "semiconductor", "shipping",
		"small_cap", "tech",
		// L2 (13)
		"cooling", "copper_industry", "foundry", "ground_equipment",
		"laser_communication", "metal_processing", "pcb",
		"precious_metals_recycling", "rare_earth_specialty", "satellite_pcb",
		"satellite_rf_components", "server_assembly", "thermal",
	),
	NamespaceSectorSymbols: forKeys(treeStyleKeys,
		"ai_supply_chain", "consumer", "cooling", "defensive", "electronics",
		"energy", "etf_rotation", "financials", "foundry", "high_dividend",
		"industrial", "leo_satellite", "mining", "pcb", "robotics",
		"semiconductor", "server_assembly", "shipping", "small_cap", "tech",
		"thermal", "tourism",
	),
	NamespaceDefaultMetrics: forKeys(treeStyleKeys,
		"ai_supply_chain", "consumer", "cooling", "copper_industry",
		"electronics", "energy", "etf_rotation", "financials", "foundry",
		"ground_equipment", "industrial", "laser_communication",
		"leo_satellite", "metal_processing", "mining",
		"precious_metals_recycling", "rare_earth_specialty", "robotics",
		"satellite_pcb", "satellite_rf_components", "semiconductor",
		"server_assembly", "shipping",
	),
	NamespaceCycleThresholds: forKeys(treeStyleKeys,
		"ai_supply_chain", "consumer", "electronics", "energy", "financials",
		"industrial", "mining", "robotics", "semiconductor", "shipping",
	),
	NamespaceGICSBaseWeights:          gicsBaseWeightKeys,
	NamespaceTWSESectorIndex:          twseSectorIndexKeys,
	NamespaceTWSESectorIndexLegacy:    twseSectorIndexLegacyKeys,
	NamespaceSectorIndexReader:        sectorIndexReaderKeys,
	NamespaceStrategyTechniqueSectors: strategyTechniqueKeys,
	NamespaceFinMindSectorSeries:      finmindSectorSeriesKeys,
	NamespaceTWSESIndustryCode:        twseIndustryCodeKeys,
	NamespaceETFRepresentatives:       etfRepresentativeKeys,
}
