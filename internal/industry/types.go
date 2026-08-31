// Package industry provides granular industry classification, seasonality,
// cycle positioning, and supply-chain linkage for Taiwan stock market analysis.
package industry

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
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

// GetAggregatableSegments returns all segments (any level) that carry
// representative stocks, i.e. the industries the data aggregator can update.
// Order is stable: tree insertion order — deterministic report ordering for
// tests and logs.
func (t *ClassificationTree) GetAggregatableSegments() []*IndustrySegment {
	var result []*IndustrySegment
	for _, seg := range t.segments {
		if len(seg.RepresentativeStocks) > 0 {
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

// DefaultClassification returns the industry classification tree, loaded from
// ParametersConfig with a minimal safe fallback if config is unavailable.
func DefaultClassification() *ClassificationTree {
	tree := NewClassificationTree()

	cfg := config.GetParametersConfig()
	if cfg == nil || cfg.Industry.ClassificationTree.Value.Segments == nil {
		// Minimal safe fallback: single placeholder segment to avoid nil-tree panics.
		// This should never happen in production because config initializes on boot.
		tree.AddSegment(&IndustrySegment{
			ID:     "unknown",
			Name:   "未分類",
			NameEN: "Uncategorized",
			Level:  Level1,
		})
		return tree
	}

	for _, seg := range cfg.Industry.ClassificationTree.Value.Segments {
		tree.AddSegment(segmentFromConfig(seg))
	}

	return tree
}

func segmentFromConfig(seg config.IndustrySegmentConfig) *IndustrySegment {
	return &IndustrySegment{
		ID:                   seg.ID,
		Name:                 seg.Name,
		NameEN:               seg.NameEN,
		Level:                IndustryLevel(seg.Level),
		ParentID:             seg.ParentID,
		Weight:               seg.Weight,
		GeographicExposure:   parseGeographicExposure(seg.GeographicExposure),
		Cyclicality:          parseCyclicality(seg.Cyclicality),
		TechnologyIntensity:  parseTechnologyIntensity(seg.TechnologyIntensity),
		CapitalIntensity:     parseCapitalIntensity(seg.CapitalIntensity),
		RepresentativeStocks: seg.RepresentativeStocks,
		Description:          seg.Description,
	}
}

func parseGeographicExposure(s string) GeographicExposure {
	switch s {
	case "Domestic":
		return ExposureDomestic
	case "Export":
		return ExposureExport
	case "Global", "Mixed":
		return ExposureMixed
	default:
		return ExposureMixed
	}
}

func parseCyclicality(s string) Cyclicality {
	switch s {
	case "Cyclical":
		return CyclicalityHigh
	case "Defensive":
		return CyclicalityLow
	case "Hybrid":
		return CyclicalityMedium
	default:
		return CyclicalityMedium
	}
}

func parseTechnologyIntensity(s string) TechnologyIntensity {
	switch s {
	case "HighTech":
		return TechIntensityHigh
	case "MediumTech":
		return TechIntensityMedium
	case "LowTech":
		return TechIntensityLow
	default:
		return TechIntensityMedium
	}
}

func parseCapitalIntensity(s string) CapitalIntensity {
	switch s {
	case "HighCapital":
		return CapIntensityHigh
	case "MediumCapital":
		return CapIntensityMedium
	case "LowCapital":
		return CapIntensityLow
	default:
		return CapIntensityMedium
	}
}

// ClassificationTreeAccess is a simple interface for looking up industry segments.
// It allows the config package to remain decoupled from the full tree implementation.
type ClassificationTreeAccess interface {
	GetSegment(id string) (*IndustrySegment, bool)
	GetChildren(parentID string) []*IndustrySegment
	GetLevel1() []*IndustrySegment
	GetPath(segmentID string) []*IndustrySegment
}
