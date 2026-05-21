// Package industry provides granular industry classification, seasonality,
// cycle positioning, and supply-chain linkage for Taiwan stock market analysis.
package industry

import (
	"fmt"
	"time"
)

// IndustryLevel represents the granularity level of industry classification.
type IndustryLevel int

const (
	Level1 IndustryLevel = 1 // Broad sector (e.g., Technology)
	Level2 IndustryLevel = 2 // Industry group (e.g., Semiconductors)
	Level3 IndustryLevel = 3 // Sub-industry (e.g., DRAM)
)

// GeographicExposure indicates the primary market exposure of an industry.
type GeographicExposure string

const (
	ExposureDomestic GeographicExposure = "domestic" // Primarily Taiwan market
	ExposureExport   GeographicExposure = "export"   // Primarily overseas markets
	ExposureMixed    GeographicExposure = "mixed"    // Both domestic and export
)

// Cyclicality indicates the sensitivity to economic cycles.
type Cyclicality string

const (
	CyclicalityHigh   Cyclicality = "high"   // Highly cyclical (e.g., memory, shipping)
	CyclicalityMedium Cyclicality = "medium" // Moderately cyclical (e.g., foundry)
	CyclicalityLow    Cyclicality = "low"    // Defensive (e.g., utilities, consumer staples)
)

// TechnologyIntensity indicates the R&D intensity of an industry.
type TechnologyIntensity string

const (
	TechIntensityHigh   TechnologyIntensity = "high"   // Heavy R&D (e.g., advanced process)
	TechIntensityMedium TechnologyIntensity = "medium" // Moderate R&D (e.g., packaging)
	TechIntensityLow    TechnologyIntensity = "low"    // Low R&D (e.g., assembly)
)

// CapitalIntensity indicates the capital expenditure requirements.
type CapitalIntensity string

const (
	CapIntensityHigh   CapitalIntensity = "high"   // Heavy capex (e.g., foundry)
	CapIntensityMedium CapitalIntensity = "medium" // Moderate capex (e.g., IC design)
	CapIntensityLow    CapitalIntensity = "low"    // Light capex (e.g., fintech)
)

// IndustrySegment represents a single node in the industry classification tree.
type IndustrySegment struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	NameEN               string              `json:"name_en"`
	Level                IndustryLevel       `json:"level"`
	ParentID             string              `json:"parent_id,omitempty"`
	Weight               float64             `json:"weight,omitempty"`
	GeographicExposure   GeographicExposure  `json:"geographic_exposure"`
	Cyclicality          Cyclicality         `json:"cyclicality"`
	TechnologyIntensity  TechnologyIntensity `json:"technology_intensity"`
	CapitalIntensity     CapitalIntensity    `json:"capital_intensity"`
	RepresentativeStocks []string            `json:"representative_stocks,omitempty"`
	Description          string              `json:"description,omitempty"`
}

// IndustryClassification holds the complete classification for a stock.
type IndustryClassification struct {
	Symbol    string          `json:"symbol"`
	Level1    IndustrySegment `json:"level1"`
	Level2    IndustrySegment `json:"level2"`
	Level3    IndustrySegment `json:"level3"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// RiskProfile captures industry-specific risk characteristics.
type RiskProfile struct {
	CustomerConcentration float64  `json:"customer_concentration"`  // 0-1, higher means more concentrated
	NewsLatencyRisk       float64  `json:"news_latency_risk"`       // 0-1, higher means worse latency
	AsymmetricRisk        float64  `json:"asymmetric_risk"`         // Bad news impact multiplier
	SupplyChainDepth      int      `json:"supply_chain_depth"`      // Number of upstream tiers
	KeyCustomers          []string `json:"key_customers,omitempty"` // Major customer names
}

// IndustryMetrics holds real-time metrics for an industry.
type IndustryMetrics struct {
	IndustryID          string    `json:"industry_id"`
	PE                  float64   `json:"pe"`
	PB                  float64   `json:"pb"`
	DividendYield       float64   `json:"dividend_yield"`
	RevenueGrowthYoY    float64   `json:"revenue_growth_yoy"`
	ProfitGrowthYoY     float64   `json:"profit_growth_yoy"`
	InventoryTurnover   float64   `json:"inventory_turnover"`
	CapacityUtilization float64   `json:"capacity_utilization"`
	Timestamp           time.Time `json:"timestamp"`
}

// NarrativeAdjustment represents how active narrative events shift
// cycle phase detection for an industry. A negative RevenueBias pushes
// the effective growth downward, making recession/mature more likely.
type NarrativeAdjustment struct {
	RevenueBias float64 `json:"revenue_bias"`
	ProfitBias  float64 `json:"profit_bias"`
	Confidence  float64 `json:"confidence"` // 0-1 how reliable this bias is
	ActiveTheme string  `json:"active_theme,omitempty"`
}

// ClassificationTree provides hierarchical access to industry segments.
type ClassificationTree struct {
	segments map[string]*IndustrySegment
	children map[string][]string // parent_id -> []child_ids
}

// NewClassificationTree creates an empty classification tree.
func NewClassificationTree() *ClassificationTree {
	return &ClassificationTree{
		segments: make(map[string]*IndustrySegment),
		children: make(map[string][]string),
	}
}

// AddSegment registers a segment in the tree.
func (t *ClassificationTree) AddSegment(seg *IndustrySegment) {
	t.segments[seg.ID] = seg
	if seg.ParentID != "" {
		t.children[seg.ParentID] = append(t.children[seg.ParentID], seg.ID)
	}
}

// GetSegment retrieves a segment by ID.
func (t *ClassificationTree) GetSegment(id string) (*IndustrySegment, bool) {
	seg, ok := t.segments[id]
	return seg, ok
}

// GetChildren returns all direct children of a segment.
func (t *ClassificationTree) GetChildren(parentID string) []*IndustrySegment {
	var result []*IndustrySegment
	for _, childID := range t.children[parentID] {
		if seg, ok := t.segments[childID]; ok {
			result = append(result, seg)
		}
	}
	return result
}

// GetLevel1 returns all top-level industries.
func (t *ClassificationTree) GetLevel1() []*IndustrySegment {
	var result []*IndustrySegment
	for _, seg := range t.segments {
		if seg.Level == Level1 {
			result = append(result, seg)
		}
	}
	return result
}

func (t *ClassificationTree) GetAllSegments() []*IndustrySegment {
	var result []*IndustrySegment
	for _, seg := range t.segments {
		result = append(result, seg)
	}
	return result
}

// GetPath returns the full path from root to the given segment.
func (t *ClassificationTree) GetPath(segmentID string) []*IndustrySegment {
	var path []*IndustrySegment
	currentID := segmentID
	visited := make(map[string]bool)

	for currentID != "" {
		if visited[currentID] {
			break // Prevent infinite loop
		}
		visited[currentID] = true

		seg, ok := t.segments[currentID]
		if !ok {
			break
		}
		path = append([]*IndustrySegment{seg}, path...)
		currentID = seg.ParentID
	}
	return path
}

// Validate checks the integrity of the classification tree.
func (t *ClassificationTree) Validate() error {
	for id, seg := range t.segments {
		if seg.ID != id {
			return fmt.Errorf("segment ID mismatch: %s != %s", seg.ID, id)
		}
		if seg.Level < Level1 || seg.Level > Level3 {
			return fmt.Errorf("invalid level for %s: %d", id, seg.Level)
		}
		if seg.Level > Level1 && seg.ParentID == "" {
			return fmt.Errorf("level %d segment %s missing parent", seg.Level, id)
		}
		if seg.ParentID != "" {
			if _, ok := t.segments[seg.ParentID]; !ok {
				return fmt.Errorf("parent %s not found for %s", seg.ParentID, id)
			}
		}
	}
	return nil
}

// DefaultClassification returns the built-in Taiwan stock industry classification.
func DefaultClassification() *ClassificationTree {
	tree := NewClassificationTree()

	// Level 1: Broad sectors
	// 1. Semiconductor
	tree.AddSegment(&IndustrySegment{
		ID:                   "semiconductor",
		Name:                 "半導體",
		NameEN:               "Semiconductor",
		Level:                Level1,
		Weight:               0.23,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2330.TW", "2303.TW", "2454.TW"},
		Description:          "台灣核心產業，佔出口比重超過35%",
	})

	// 2. AI Supply Chain
	tree.AddSegment(&IndustrySegment{
		ID:                   "ai_supply_chain",
		Name:                 "AI供應鏈",
		NameEN:               "AI Supply Chain",
		Level:                Level1,
		Weight:               0.18,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2382.TW", "6669.TW", "2317.TW"},
		Description:          "AI伺服器、散熱、電源等AI基礎設施",
	})

	// 3. Robotics
	tree.AddSegment(&IndustrySegment{
		ID:                   "robotics",
		Name:                 "機器人",
		NameEN:               "Robotics",
		Level:                Level1,
		Weight:               0.07,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"2308.TW", "2395.TW", "6669.TW"},
		Description:          "工業自動化與智慧製造",
	})

	// 4. Financials
	tree.AddSegment(&IndustrySegment{
		ID:                   "financials",
		Name:                 "金融",
		NameEN:               "Financials",
		Level:                Level1,
		Weight:               0.14,
		GeographicExposure:   ExposureDomestic,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2881.TW", "2882.TW", "2886.TW"},
		Description:          "金融控股與銀行保險",
	})

	// 5. Shipping
	tree.AddSegment(&IndustrySegment{
		ID:                   "shipping",
		Name:                 "航運",
		NameEN:               "Shipping",
		Level:                Level1,
		Weight:               0.09,
		GeographicExposure:   ExposureMixed,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityLow,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2603.TW", "2609.TW", "2615.TW"},
		Description:          "國際海運與物流",
	})

	// 6. Energy
	tree.AddSegment(&IndustrySegment{
		ID:                   "energy",
		Name:                 "能源",
		NameEN:               "Energy",
		Level:                Level1,
		Weight:               0.05,
		GeographicExposure:   ExposureMixed,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"6505.TW", "1328.TW"},
		Description:          "石油天然氣、再生能源、公用事業",
	})

	// 7. Electronics Components
	tree.AddSegment(&IndustrySegment{
		ID:                   "electronics",
		Name:                 "電子零組件",
		NameEN:               "Electronics Components",
		Level:                Level1,
		Weight:               0.07,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"2327.TW", "3533.TW", "3324.TW"},
		Description:          "被動元件、連接器、散熱模組、機殼",
	})

	// 8. Consumer
	tree.AddSegment(&IndustrySegment{
		ID:                   "consumer",
		Name:                 "傳產/消費",
		NameEN:               "Consumer & Traditional",
		Level:                Level1,
		Weight:               0.05,
		GeographicExposure:   ExposureDomestic,
		Cyclicality:          CyclicalityLow,
		TechnologyIntensity:  TechIntensityLow,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"1216.TW", "1476.TW", "2903.TW"},
		Description:          "食品飲料、紡織成衣、百貨零售、觀光旅遊",
	})

	// 9. Industrial
	tree.AddSegment(&IndustrySegment{
		ID:                   "industrial",
		Name:                 "工業/製造",
		NameEN:               "Industrial & Manufacturing",
		Level:                Level1,
		Weight:               0.05,
		GeographicExposure:   ExposureMixed,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2002.TW", "1301.TW", "1101.TW"},
		Description:          "鋼鐵、塑化、水泥、工具機",
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "leo_satellite",
		Name:                 "低軌衛星",
		NameEN:               "LEO Satellite",
		Level:                Level1,
		Weight:               0.06,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"2308.TW", "2395.TW", "6669.TW"},
		Description:          "工業自動化與智慧製造",
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "leo_satellite",
		Name:                 "低軌衛星",
		NameEN:               "LEO Satellite",
		Level:                Level1,
		Weight:               0.06,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"3491.TW", "2313.TW", "6285.TW"},
		Description:          "低軌道衛星通訊與地面設備",
	})

	// Level 2 & 3: Semiconductor sub-industries
	addSemiconductorSubIndustries(tree)
	addAISupplyChainSubIndustries(tree)
	addRoboticsSubIndustries(tree)
	addFinancialsSubIndustries(tree)
	addShippingSubIndustries(tree)
	addEnergySubIndustries(tree)
	addElectronicsSubIndustries(tree)
	addConsumerSubIndustries(tree)
	addIndustrialSubIndustries(tree)
	addLEOSatelliteSubIndustries(tree)

	return tree
}

func addSemiconductorSubIndustries(tree *ClassificationTree) {
	// Level 2
	tree.AddSegment(&IndustrySegment{
		ID:                   "foundry",
		Name:                 "晶圓代工",
		NameEN:               "Foundry",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.40,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2330.TW", "2303.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "memory",
		Name:                 "記憶體",
		NameEN:               "Memory",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.20,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2408.TW", "2344.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "ic_design",
		Name:                 "IC設計",
		NameEN:               "IC Design",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.25,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityLow,
		RepresentativeStocks: []string{"2454.TW", "3034.TW", "3661.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "packaging",
		Name:                 "封裝測試",
		NameEN:               "Packaging & Testing",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.10,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"2311.TW", "2449.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "pcb",
		Name:                 "PCB",
		NameEN:               "Printed Circuit Board",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.03,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"3037.TW", "4958.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "semi_equipment",
		Name:                 "半導體設備",
		NameEN:               "Semiconductor Equipment",
		Level:                Level2,
		ParentID:             "semiconductor",
		Weight:               0.02,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"3658.TW"},
	})

	// Level 3: Foundry sub-categories
	tree.AddSegment(&IndustrySegment{
		ID:                   "advanced_process",
		Name:                 "先進製程",
		NameEN:               "Advanced Process",
		Level:                Level3,
		ParentID:             "foundry",
		RepresentativeStocks: []string{"2330.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "mature_process",
		Name:                 "成熟製程",
		NameEN:               "Mature Process",
		Level:                Level3,
		ParentID:             "foundry",
		RepresentativeStocks: []string{"2303.TW"},
	})

	// Level 3: Memory sub-categories
	tree.AddSegment(&IndustrySegment{
		ID:                   "dram",
		Name:                 "DRAM",
		NameEN:               "DRAM",
		Level:                Level3,
		ParentID:             "memory",
		RepresentativeStocks: []string{"2408.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "nand",
		Name:                 "NAND Flash",
		NameEN:               "NAND Flash",
		Level:                Level3,
		ParentID:             "memory",
		RepresentativeStocks: []string{"2344.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "hbm",
		Name:                 "HBM",
		NameEN:               "High Bandwidth Memory",
		Level:                Level3,
		ParentID:             "memory",
		RepresentativeStocks: []string{"3661.TW"},
	})

	// Level 3: IC Design sub-categories
	tree.AddSegment(&IndustrySegment{
		ID:                   "mobile_chip",
		Name:                 "手機晶片",
		NameEN:               "Mobile Chip",
		Level:                Level3,
		ParentID:             "ic_design",
		RepresentativeStocks: []string{"2454.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "ai_chip",
		Name:                 "AI晶片",
		NameEN:               "AI Chip",
		Level:                Level3,
		ParentID:             "ic_design",
		RepresentativeStocks: []string{"3661.TW", "3034.TW"},
	})
}

func addAISupplyChainSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "server_assembly",
		Name:                 "伺服器組裝",
		NameEN:               "Server Assembly",
		Level:                Level2,
		ParentID:             "ai_supply_chain",
		Weight:               0.50,
		RepresentativeStocks: []string{"2382.TW", "6669.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "cooling",
		Name:                 "散熱",
		NameEN:               "Cooling Solutions",
		Level:                Level2,
		ParentID:             "ai_supply_chain",
		Weight:               0.30,
		RepresentativeStocks: []string{"3017.TW", "3665.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "power_supply",
		Name:                 "電源",
		NameEN:               "Power Supply",
		Level:                Level2,
		ParentID:             "ai_supply_chain",
		Weight:               0.20,
		RepresentativeStocks: []string{"2308.TW"},
	})
}

func addRoboticsSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "reducer",
		Name:                 "減速機",
		NameEN:               "Reducer/Gearbox",
		Level:                Level2,
		ParentID:             "robotics",
		Weight:               0.25,
		RepresentativeStocks: []string{"4540.TW", "1598.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "servo_motor",
		Name:                 "伺服馬達",
		NameEN:               "Servo Motor",
		Level:                Level2,
		ParentID:             "robotics",
		Weight:               0.25,
		RepresentativeStocks: []string{"2049.TW", "4551.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "robot_controller",
		Name:                 "控制器",
		NameEN:               "Robot Controller",
		Level:                Level2,
		ParentID:             "robotics",
		Weight:               0.20,
		RepresentativeStocks: []string{"2395.TW", "2049.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "sensor_vision",
		Name:                 "感測器/視覺",
		NameEN:               "Sensor & Vision",
		Level:                Level2,
		ParentID:             "robotics",
		Weight:               0.15,
		RepresentativeStocks: []string{"3008.TW", "6732.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "robot_body",
		Name:                 "機器人本體",
		NameEN:               "Robot Body",
		Level:                Level2,
		ParentID:             "robotics",
		Weight:               0.15,
		RepresentativeStocks: []string{"2049.TW", "4552.TW"},
	})
}

func addFinancialsSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "banking",
		Name:                 "銀行",
		NameEN:               "Banking",
		Level:                Level2,
		ParentID:             "financials",
		Weight:               0.50,
		RepresentativeStocks: []string{"2881.TW", "2882.TW", "2886.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "insurance",
		Name:                 "保險",
		NameEN:               "Insurance",
		Level:                Level2,
		ParentID:             "financials",
		Weight:               0.25,
		RepresentativeStocks: []string{"2881.TW", "2882.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "securities",
		Name:                 "證券",
		NameEN:               "Securities",
		Level:                Level2,
		ParentID:             "financials",
		Weight:               0.15,
		RepresentativeStocks: []string{"2891.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "fintech",
		Name:                 "金融科技",
		NameEN:               "Fintech",
		Level:                Level2,
		ParentID:             "financials",
		Weight:               0.10,
		RepresentativeStocks: []string{},
	})
}

func addShippingSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "container_shipping",
		Name:                 "貨櫃航運",
		NameEN:               "Container Shipping",
		Level:                Level2,
		ParentID:             "shipping",
		Weight:               0.60,
		RepresentativeStocks: []string{"2603.TW", "2609.TW", "2615.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "bulk_shipping",
		Name:                 "散裝航運",
		NameEN:               "Bulk Shipping",
		Level:                Level2,
		ParentID:             "shipping",
		Weight:               0.25,
		RepresentativeStocks: []string{"2606.TW", "2637.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "port_logistics",
		Name:                 "港口物流",
		NameEN:               "Port & Logistics",
		Level:                Level2,
		ParentID:             "shipping",
		Weight:               0.15,
		RepresentativeStocks: []string{"5607.TW"},
	})
}

func addEnergySubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "oil_gas",
		Name:                 "石油天然氣",
		NameEN:               "Oil & Gas",
		Level:                Level2,
		ParentID:             "energy",
		Weight:               0.50,
		RepresentativeStocks: []string{"6505.TW", "1328.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "renewable",
		Name:                 "再生能源",
		NameEN:               "Renewable Energy",
		Level:                Level2,
		ParentID:             "energy",
		Weight:               0.30,
		RepresentativeStocks: []string{},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "utilities",
		Name:                 "公用事業",
		NameEN:               "Utilities",
		Level:                Level2,
		ParentID:             "energy",
		Weight:               0.20,
		RepresentativeStocks: []string{},
	})
}

func addElectronicsSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "passive_components",
		Name:                 "被動元件",
		NameEN:               "Passive Components",
		Level:                Level2,
		ParentID:             "electronics",
		Weight:               0.35,
		RepresentativeStocks: []string{"2327.TW", "2492.TW", "3026.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "connectors",
		Name:                 "連接器",
		NameEN:               "Connectors",
		Level:                Level2,
		ParentID:             "electronics",
		Weight:               0.25,
		RepresentativeStocks: []string{"3533.TW", "3665.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "thermal_modules",
		Name:                 "散熱模組",
		NameEN:               "Thermal Modules",
		Level:                Level2,
		ParentID:             "electronics",
		Weight:               0.25,
		RepresentativeStocks: []string{"3324.TW", "3017.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "chassis",
		Name:                 "機殼/機構件",
		NameEN:               "Chassis & Mechanical",
		Level:                Level2,
		ParentID:             "electronics",
		Weight:               0.15,
		RepresentativeStocks: []string{"8210.TW"},
	})
}

func addConsumerSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "food_beverage",
		Name:                 "食品飲料",
		NameEN:               "Food & Beverage",
		Level:                Level2,
		ParentID:             "consumer",
		Weight:               0.35,
		RepresentativeStocks: []string{"1216.TW", "1227.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "textile",
		Name:                 "紡織成衣",
		NameEN:               "Textile & Apparel",
		Level:                Level2,
		ParentID:             "consumer",
		Weight:               0.25,
		RepresentativeStocks: []string{"1476.TW", "4401.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "retail",
		Name:                 "百貨零售",
		NameEN:               "Retail",
		Level:                Level2,
		ParentID:             "consumer",
		Weight:               0.25,
		RepresentativeStocks: []string{"2903.TW", "2912.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "tourism",
		Name:                 "觀光旅遊",
		NameEN:               "Tourism",
		Level:                Level2,
		ParentID:             "consumer",
		Weight:               0.15,
		RepresentativeStocks: []string{"2731.TW"},
	})
}

func addLEOSatelliteSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "satellite_rf_components",
		Name:                 "衛星射頻元件",
		NameEN:               "Satellite RF Components",
		Level:                Level2,
		ParentID:             "leo_satellite",
		Weight:               0.30,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"3491.TW", "3105.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "satellite_pcb",
		Name:                 "衛星PCB",
		NameEN:               "Satellite PCB",
		Level:                Level2,
		ParentID:             "leo_satellite",
		Weight:               0.25,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityHigh,
		RepresentativeStocks: []string{"2313.TW", "2367.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "ground_equipment",
		Name:                 "地面設備",
		NameEN:               "Ground Equipment",
		Level:                Level2,
		ParentID:             "leo_satellite",
		Weight:               0.25,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityMedium,
		TechnologyIntensity:  TechIntensityMedium,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"6285.TW", "3022.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "laser_communication",
		Name:                 "雷射通訊",
		NameEN:               "Laser Communication",
		Level:                Level2,
		ParentID:             "leo_satellite",
		Weight:               0.20,
		GeographicExposure:   ExposureExport,
		Cyclicality:          CyclicalityHigh,
		TechnologyIntensity:  TechIntensityHigh,
		CapitalIntensity:     CapIntensityMedium,
		RepresentativeStocks: []string{"7717.TW", "3138.TW"},
	})
}

func addIndustrialSubIndustries(tree *ClassificationTree) {
	tree.AddSegment(&IndustrySegment{
		ID:                   "steel",
		Name:                 "鋼鐵",
		NameEN:               "Steel",
		Level:                Level2,
		ParentID:             "industrial",
		Weight:               0.30,
		RepresentativeStocks: []string{"2002.TW", "2006.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "petrochemicals",
		Name:                 "塑化",
		NameEN:               "Petrochemicals",
		Level:                Level2,
		ParentID:             "industrial",
		Weight:               0.30,
		RepresentativeStocks: []string{"1301.TW", "1303.TW", "1326.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "cement",
		Name:                 "水泥",
		NameEN:               "Cement",
		Level:                Level2,
		ParentID:             "industrial",
		Weight:               0.20,
		RepresentativeStocks: []string{"1101.TW", "1102.TW"},
	})

	tree.AddSegment(&IndustrySegment{
		ID:                   "machine_tools",
		Name:                 "工具機",
		NameEN:               "Machine Tools",
		Level:                Level2,
		ParentID:             "industrial",
		Weight:               0.20,
		RepresentativeStocks: []string{"1590.TW", "2049.TW"},
	})
}
