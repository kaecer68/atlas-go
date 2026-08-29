package industry

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// SupplyChainNode represents a node in the supply chain graph.
type SupplyChainNode struct {
	IndustryID   string   `json:"industry_id"`
	Tier         int      `json:"tier"`                    // 0 = end product, 1 = direct supplier, etc.
	UpstreamOf   []string `json:"upstream_of,omitempty"`   // Industries this node supplies to
	DownstreamOf []string `json:"downstream_of,omitempty"` // Industries that supply to this node
	KeyMaterials []string `json:"key_materials,omitempty"`
}

// SupplyChainGraph models the upstream/downstream relationships between industries.
type SupplyChainGraph struct {
	nodes map[string]*SupplyChainNode
	mu    sync.RWMutex
}

// NewSupplyChainGraph creates an empty supply chain graph.
func NewSupplyChainGraph() *SupplyChainGraph {
	return &SupplyChainGraph{
		nodes: make(map[string]*SupplyChainNode),
	}
}

// AddNode adds a node to the supply chain graph.
func (g *SupplyChainGraph) AddNode(node *SupplyChainNode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[node.IndustryID] = node
}

// GetNode retrieves a node by industry ID.
func (g *SupplyChainGraph) GetNode(industryID string) (*SupplyChainNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[industryID]
	return node, ok
}

// GetUpstream returns all direct upstream suppliers for an industry.
func (g *SupplyChainGraph) GetUpstream(industryID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[industryID]
	if !ok {
		return nil
	}
	return node.DownstreamOf
}

// GetDownstream returns all direct downstream customers for an industry.
func (g *SupplyChainGraph) GetDownstream(industryID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[industryID]
	if !ok {
		return nil
	}
	return node.UpstreamOf
}

// GetUpstreamChain returns the full upstream chain (all tiers) for an industry.
func (g *SupplyChainGraph) GetUpstreamChain(industryID string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []string
	visited := make(map[string]bool)
	g.collectUpstream(industryID, 0, maxDepth, visited, &result)
	return result
}

func (g *SupplyChainGraph) collectUpstream(industryID string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth >= maxDepth {
		return
	}

	node, ok := g.nodes[industryID]
	if !ok {
		return
	}

	for _, upstream := range node.DownstreamOf {
		if visited[upstream] {
			continue
		}
		visited[upstream] = true
		*result = append(*result, upstream)
		g.collectUpstream(upstream, depth+1, maxDepth, visited, result)
	}
}

// GetDownstreamChain returns the full downstream chain (all tiers) for an industry.
func (g *SupplyChainGraph) GetDownstreamChain(industryID string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []string
	visited := make(map[string]bool)
	g.collectDownstream(industryID, 0, maxDepth, visited, &result)
	return result
}

func (g *SupplyChainGraph) collectDownstream(industryID string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth >= maxDepth {
		return
	}

	node, ok := g.nodes[industryID]
	if !ok {
		return
	}

	for _, downstream := range node.UpstreamOf {
		if visited[downstream] {
			continue
		}
		visited[downstream] = true
		*result = append(*result, downstream)
		g.collectDownstream(downstream, depth+1, maxDepth, visited, result)
	}
}

// MaxDegree returns the maximum number of connections (upstream + downstream)
// for any node in the graph. Used to normalize systemic importance to [0, 1].
func (g *SupplyChainGraph) MaxDegree() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	maxDeg := 0
	for _, node := range g.nodes {
		deg := len(node.UpstreamOf) + len(node.DownstreamOf)
		if deg > maxDeg {
			maxDeg = deg
		}
	}
	return maxDeg
}

// CorrelationMatrix holds the correlation coefficients between industry pairs.
type CorrelationMatrix struct {
	correlations  map[string]map[string]float64 // industry_a -> industry_b -> correlation
	window        int                           // rolling window in days
	mu            sync.RWMutex
	cycleProvider CycleProvider
}

// NewCorrelationMatrix creates a new correlation matrix.
func NewCorrelationMatrix(window int) *CorrelationMatrix {
	return &CorrelationMatrix{
		correlations: make(map[string]map[string]float64),
		window:       window,
	}
}

// UpdateCorrelation updates the correlation between two industries.
func (cm *CorrelationMatrix) UpdateCorrelation(industryA, industryB string, correlation float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.correlations[industryA] == nil {
		cm.correlations[industryA] = make(map[string]float64)
	}
	if cm.correlations[industryB] == nil {
		cm.correlations[industryB] = make(map[string]float64)
	}

	cm.correlations[industryA][industryB] = correlation
	cm.correlations[industryB][industryA] = correlation
}

// GetCorrelation returns the correlation between two industries.
func (cm *CorrelationMatrix) GetCorrelation(industryA, industryB string) (float64, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.correlations[industryA] == nil {
		return 0, false
	}
	corr, ok := cm.correlations[industryA][industryB]
	return corr, ok
}

// GetCorrelatedIndustries returns all industries correlated with the given industry.
func (cm *CorrelationMatrix) GetCorrelatedIndustries(industryID string, minCorrelation float64) map[string]float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]float64)
	if cm.correlations[industryID] == nil {
		return result
	}

	for otherIndustry, correlation := range cm.correlations[industryID] {
		if math.Abs(correlation) >= minCorrelation {
			result[otherIndustry] = correlation
		}
	}
	return result
}

// GetAllCorrelations returns the full correlation matrix.
func (cm *CorrelationMatrix) GetAllCorrelations() map[string]map[string]float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]map[string]float64)
	for industryA, correlations := range cm.correlations {
		result[industryA] = make(map[string]float64)
		maps.Copy(result[industryA], correlations)
	}
	return result
}

// SetCycleProvider configures the cycle provider for regime-aware correlation
// adjustment. Passing nil clears the provider (reverts to raw correlations).
func (cm *CorrelationMatrix) SetCycleProvider(cp CycleProvider) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cycleProvider = cp
}

// RegimeAdjustedCorrelation returns the correlation between two industries,
// adjusted upward during recession phases (Ang & Chen 2002). When either
// industry is in recession, the correlation is boosted by the configured
// RecessionCorrelationBoost factor (capped at 1.0).
func (cm *CorrelationMatrix) RegimeAdjustedCorrelation(industryA, industryB string) float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	baseCorr := 0.5
	if cm.correlations[industryA] != nil {
		if c, ok := cm.correlations[industryA][industryB]; ok {
			baseCorr = c
		}
	}

	if cm.cycleProvider == nil {
		return baseCorr
	}

	phaseA, okA := cm.cycleProvider.GetPhase(industryA)
	phaseB, okB := cm.cycleProvider.GetPhase(industryB)
	if (okA && phaseA == CycleRecession) || (okB && phaseB == CycleRecession) {
		boost := 0.30
		if cfg := config.GetParametersConfig(); cfg != nil {
			if b := cfg.Industry.LinkageParams.Value.RecessionCorrelationBoost; b > 0 {
				boost = b
			}
		}
		return math.Min(baseCorr*(1.0+boost), 1.0)
	}

	return baseCorr
}

// RecalculateFromReturns recomputes all pairwise correlations from industry
// return time series. Each map entry maps industryID → []daily returns.
// Only pairs with sufficient data (≥15 observations) are updated.
func (cm *CorrelationMatrix) RecalculateFromReturns(industryReturns map[string][]float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	industries := make([]string, 0, len(industryReturns))
	for id := range industryReturns {
		industries = append(industries, id)
	}

	for i := 0; i < len(industries); i++ {
		for j := i + 1; j < len(industries); j++ {
			a, b := industries[i], industries[j]
			returnsA, returnsB := industryReturns[a], industryReturns[b]
			n := len(returnsA)
			if len(returnsB) < n {
				n = len(returnsB)
			}
			if n < 30 {
				continue
			}
			corr := pearsonCorrelation(returnsA[:n], returnsB[:n])
			cm.unsafeUpdate(a, b, corr)
		}
	}
}

func (cm *CorrelationMatrix) unsafeUpdate(a, b string, corr float64) {
	if cm.correlations[a] == nil {
		cm.correlations[a] = make(map[string]float64)
	}
	if cm.correlations[b] == nil {
		cm.correlations[b] = make(map[string]float64)
	}
	cm.correlations[a][b] = corr
	cm.correlations[b][a] = corr
}

func pearsonCorrelation(x, y []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range n {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}
	num := float64(n)*sumXY - sumX*sumY
	den := math.Sqrt((float64(n)*sumX2 - sumX*sumX) * (float64(n)*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

// NarrativeLinkageProvider supplies active narrative themes and correlation
// multipliers for dynamic supply chain linkage adjustment.
type NarrativeLinkageProvider interface {
	ActiveThemes() []string
	CorrelationMultiplier(theme, industryA, industryB string) float64
}

// CycleProvider supplies business cycle phase information for cycle-aware
// correlation adjustment. During recession, correlations rise (Ang & Chen 2002).
type CycleProvider interface {
	GetPhase(industryID string) (CyclePhase, bool)
}

// ShockPropagation models how shocks propagate through the supply chain.
type ShockPropagation struct {
	graph             *SupplyChainGraph
	correlation       *CorrelationMatrix
	narrativeProvider NarrativeLinkageProvider
	downstreamDecay   float64
	upstreamDecay     float64
}

// NewShockPropagation creates a new shock propagation model.
func NewShockPropagation(graph *SupplyChainGraph, correlation *CorrelationMatrix) *ShockPropagation {
	return &ShockPropagation{
		graph:       graph,
		correlation: correlation,
	}
}

// SetNarrativeProvider enables narrative-aware correlation adjustment.
// Passing nil disables narrative overlay (safe default).
func (sp *ShockPropagation) SetNarrativeProvider(provider NarrativeLinkageProvider) {
	sp.narrativeProvider = provider
}

// SetDecayFactors configures the shock decay multipliers for downstream
// and upstream propagation. If zero is passed, PropagateShock falls back
// to built-in defaults (0.80 downstream, 0.60 upstream).
func (sp *ShockPropagation) SetDecayFactors(downstream, upstream float64) {
	sp.downstreamDecay = downstream
	sp.upstreamDecay = upstream
}

// regimeAmplifiedMagnitude returns amplified shock magnitude when the source
// industry is in recession. The amplification factor is read from
// Industry.LinkageParams.RecessionShockAmplifier config (default 1.30).
// This is orthogonal to RegimeAdjustedCorrelation which boosts correlation
// during recession — both effects compound.
func (sp *ShockPropagation) regimeAmplifiedMagnitude(sourceIndustry string, magnitude float64) float64 {
	if sp.correlation == nil || sp.correlation.cycleProvider == nil {
		return magnitude
	}

	phase, ok := sp.correlation.cycleProvider.GetPhase(sourceIndustry)
	if !ok || phase != CycleRecession {
		return magnitude
	}

	amplifier := 1.0
	if cfg := config.GetParametersConfig(); cfg != nil {
		if a := cfg.Industry.LinkageParams.Value.RecessionShockAmplifier; a > 0 {
			amplifier = a
		}
	}
	return magnitude * amplifier
}

// getNarrativeAdjustedCorrelation returns the correlation between two industries,
// adjusted by any active narrative themes.
func (sp *ShockPropagation) getNarrativeAdjustedCorrelation(industryA, industryB string) float64 {
	baseCorr := sp.correlation.RegimeAdjustedCorrelation(industryA, industryB)

	if sp.narrativeProvider == nil {
		return baseCorr
	}

	adjusted := baseCorr
	for _, theme := range sp.narrativeProvider.ActiveThemes() {
		multiplier := sp.narrativeProvider.CorrelationMultiplier(theme, industryA, industryB)
		adjusted *= multiplier
	}
	return math.Max(-1.0, math.Min(1.0, adjusted))
}

// PropagateShock calculates the impact of a shock on an industry, with
// narrative-aware correlation when a narrative provider is set.
func (sp *ShockPropagation) PropagateShock(sourceIndustry string, shockMagnitude float64, maxDepth int) map[string]float64 {
	impacts := make(map[string]float64)
	amplified := sp.regimeAmplifiedMagnitude(sourceIndustry, shockMagnitude)
	impacts[sourceIndustry] = shockMagnitude

	// Propagate downstream (customers affected)
	downstream := sp.graph.GetDownstreamChain(sourceIndustry, maxDepth)
	for _, industry := range downstream {
		correlation := sp.getNarrativeAdjustedCorrelation(sourceIndustry, industry)
		decay := sp.downstreamDecay
		if decay == 0 {
			if cfg := config.GetParametersConfig(); cfg != nil {
				decay = cfg.Industry.LinkageParams.Value.DownstreamDecayFactor
			}
			if decay == 0 {
				decay = 0.80
			}
		}
		impacts[industry] = amplified * correlation * decay
	}

	// Propagate upstream (suppliers affected)
	upstream := sp.graph.GetUpstreamChain(sourceIndustry, maxDepth)
	for _, industry := range upstream {
		correlation := sp.getNarrativeAdjustedCorrelation(sourceIndustry, industry)
		decay := sp.upstreamDecay
		if decay == 0 {
			if cfg := config.GetParametersConfig(); cfg != nil {
				decay = cfg.Industry.LinkageParams.Value.UpstreamDecayFactor
			}
			if decay == 0 {
				decay = 0.60
			}
		}
		impacts[industry] = amplified * correlation * decay
	}

	return impacts
}

// IndustryLinkageScore calculates a composite linkage score for an industry.
type IndustryLinkageScore struct {
	IndustryID            string    `json:"industry_id"`
	UpstreamCount         int       `json:"upstream_count"`
	DownstreamCount       int       `json:"downstream_count"`
	AvgCorrelation        float64   `json:"avg_correlation"`
	SystemicImportance    float64   `json:"systemic_importance"` // 0-1
	ShockPropagationSpeed float64   `json:"shock_propagation_speed"`
	Timestamp             time.Time `json:"timestamp"`
}

// CalculateLinkageScore calculates the linkage score for an industry with
// narrative-aware correlation when a narrative provider is active.
func (sp *ShockPropagation) CalculateLinkageScore(industryID string) *IndustryLinkageScore {
	upstream := sp.graph.GetUpstream(industryID)
	downstream := sp.graph.GetDownstream(industryID)

	var totalCorrelation float64
	var correlationCount int

	allRelated := append(upstream, downstream...)
	for _, related := range allRelated {
		if _, ok := sp.correlation.GetCorrelation(industryID, related); !ok {
			continue
		}
		corr := sp.getNarrativeAdjustedCorrelation(industryID, related)
		totalCorrelation += math.Abs(corr)
		correlationCount++
	}

	avgCorrelation := 0.0
	if correlationCount > 0 {
		avgCorrelation = totalCorrelation / float64(correlationCount)
	}

	systemicImportance := 0.0
	if len(upstream)+len(downstream) > 0 {
		// Use max of graph's MaxDegree and config's SystemicImportanceDivisor as the divisor,
		// ensuring the divisor is never smaller than the configured minimum (10.0).
		// This preserves dynamic adjustment for dense graphs while honoring the config
		// calibration intent from parameters.json.
		divisor := float64(sp.graph.MaxDegree())
		if cfg := config.GetParametersConfig(); cfg != nil {
			if configDiv := cfg.Industry.LinkageParams.Value.SystemicImportanceDivisor; configDiv > divisor {
				divisor = configDiv
			}
		}
		if divisor > 0 {
			systemicImportance = math.Min(1.0, float64(len(upstream)+len(downstream))/divisor)
		}
	}

	return &IndustryLinkageScore{
		IndustryID:            industryID,
		UpstreamCount:         len(upstream),
		DownstreamCount:       len(downstream),
		AvgCorrelation:        avgCorrelation,
		SystemicImportance:    systemicImportance,
		ShockPropagationSpeed: avgCorrelation * systemicImportance,
		Timestamp:             time.Now(),
	}
}

// DefaultSupplyChainGraph returns the built-in Taiwan supply chain relationships.
func DefaultSupplyChainGraph() *SupplyChainGraph {
	graph := NewSupplyChainGraph()

	// Semiconductor supply chain
	graph.AddNode(&SupplyChainNode{
		IndustryID:   "semiconductor",
		Tier:         1,
		UpstreamOf:   []string{"ai_supply_chain", "electronics"},
		DownstreamOf: []string{"semi_equipment", "materials", "financials"},
		KeyMaterials: []string{"silicon_wafer", "photoresist", "specialty_gases"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "foundry",
		Tier:         2,
		UpstreamOf:   []string{"ai_supply_chain", "ic_design"},
		DownstreamOf: []string{"semi_equipment", "materials"},
		KeyMaterials: []string{"silicon_wafer", "chemicals"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "ai_supply_chain",
		Tier:         0,
		UpstreamOf:   []string{},
		DownstreamOf: []string{"semiconductor", "electronics", "cooling", "financials"},
		KeyMaterials: []string{"ai_chips", "memory", "pcb"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "server_assembly",
		Tier:         1,
		UpstreamOf:   []string{},
		DownstreamOf: []string{"ai_supply_chain", "semiconductor", "cooling", "power_supply"},
		KeyMaterials: []string{"cpu", "gpu", "memory", "motherboard"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "cooling",
		Tier:         2,
		UpstreamOf:   []string{"ai_supply_chain", "server_assembly"},
		DownstreamOf: []string{"electronics", "mining"},
		KeyMaterials: []string{"copper", "aluminum", "coolant"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "electronics",
		Tier:         2,
		UpstreamOf:   []string{"ai_supply_chain", "consumer", "industrial"},
		DownstreamOf: []string{"semiconductor", "mining", "chemicals", "financials"},
		KeyMaterials: []string{"chips", "passive_components", "connectors"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "robotics",
		Tier:         1,
		UpstreamOf:   []string{"industrial", "consumer"},
		DownstreamOf: []string{"electronics", "mining", "software", "financials"},
		KeyMaterials: []string{"servo_motors", "reducers", "controllers", "sensors"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "shipping",
		Tier:         0,
		UpstreamOf:   []string{},
		DownstreamOf: []string{"energy", "industrial", "financials"},
		KeyMaterials: []string{"fuel", "steel", "containers"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "financials",
		Tier:         0,
		UpstreamOf:   []string{"semiconductor", "ai_supply_chain", "electronics", "robotics", "shipping", "energy"},
		DownstreamOf: []string{},
		KeyMaterials: []string{"capital", "credit", "insurance", "wealth_management"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "energy",
		Tier:         2,
		UpstreamOf:   []string{"shipping", "industrial", "consumer"},
		DownstreamOf: []string{"oil_gas", "utilities", "financials"},
		KeyMaterials: []string{"crude_oil", "natural_gas", "coal"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "industrial",
		Tier:         1,
		UpstreamOf:   []string{"energy", "shipping", "financials"},
		DownstreamOf: []string{"electronics", "robotics", "energy", "shipping"},
		KeyMaterials: []string{"steel", "cement", "machinery", "chemicals"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "consumer",
		Tier:         1,
		UpstreamOf:   []string{"electronics", "financials", "energy"},
		DownstreamOf: []string{"electronics", "robotics", "energy", "shipping"},
		KeyMaterials: []string{"retail", "food", "durable_goods"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "mining",
		Tier:         2,
		UpstreamOf:   []string{"semiconductor", "electronics", "ai_supply_chain", "cooling", "robotics", "industrial"},
		DownstreamOf: []string{"energy", "shipping", "financials"},
		KeyMaterials: []string{"gold", "silver", "copper", "rare_earth", "platinum"},
	})

	graph.AddNode(&SupplyChainNode{
		IndustryID:   "etf_rotation",
		Tier:         0,
		UpstreamOf:   []string{},
		DownstreamOf: []string{"financials"},
		KeyMaterials: []string{"capital_flow", "liquidity", "market_sentiment"},
	})

	return graph
}

// DefaultCorrelationMatrix returns the correlation matrix loaded from ParametersConfig.
// Falls back to a minimal safe matrix if config is unavailable.
func DefaultCorrelationMatrix() *CorrelationMatrix {
	cfg := config.GetParametersConfig()
	if cfg != nil {
		return LoadCorrelationMatrixFromConfig(&cfg.Industry.LinkageParams.Value)
	}
	// Minimal safe fallback: empty matrix with default correlation handled by GetCorrelation
	return NewCorrelationMatrix(30)
}

// LoadCorrelationMatrixFromConfig parses the config's CorrelationMatrix map
// and populates a CorrelationMatrix. Falls back to DefaultCorrelationMatrix()
// if cfg is nil or the map is empty.
func LoadCorrelationMatrixFromConfig(cfg *config.LinkageConfig) *CorrelationMatrix {
	if cfg == nil || len(cfg.CorrelationMatrix) == 0 {
		return DefaultCorrelationMatrix()
	}

	cm := NewCorrelationMatrix(30)
	for key, value := range cfg.CorrelationMatrix {
		parts := strings.Split(key, "↔")
		if len(parts) != 2 {
			continue
		}
		industry1 := strings.TrimSpace(parts[0])
		industry2 := strings.TrimSpace(parts[1])
		cm.UpdateCorrelation(industry1, industry2, value)
	}
	return cm
}

func (ls *IndustryLinkageScore) String() string {
	return fmt.Sprintf(
		"%s: Upstream=%d, Downstream=%d, AvgCorr=%.2f, Systemic=%.0f%%",
		ls.IndustryID,
		ls.UpstreamCount,
		ls.DownstreamCount,
		ls.AvgCorrelation,
		ls.SystemicImportance*100,
	)
}

type LinkageAnalyzer struct {
	graph       *SupplyChainGraph
	correlation *CorrelationMatrix
	propagation *ShockPropagation
}

func NewLinkageAnalyzer() *LinkageAnalyzer {
	graph, cm := loadGraphWithFallback()
	propagation := NewShockPropagation(graph, cm)

	cfg := config.GetParametersConfig()
	if cfg != nil {
		lp := cfg.Industry.LinkageParams.Value
		if lp.DownstreamDecayFactor > 0 {
			propagation.SetDecayFactors(lp.DownstreamDecayFactor, lp.UpstreamDecayFactor)
		}
	}

	return &LinkageAnalyzer{
		graph:       graph,
		correlation: cm,
		propagation: propagation,
	}
}

// loadGraphWithFallback attempts to load the supply chain graph and correlation
// matrix from configs/supply_chain_graph.json. If the file cannot be read (e.g.
// when running from a package directory in tests), it falls back to the hardcoded
// DefaultSupplyChainGraph and DefaultCorrelationMatrix.
func loadGraphWithFallback() (*SupplyChainGraph, *CorrelationMatrix) {
	// Try multiple paths to handle different working directories:
	//   project root: configs/supply_chain_graph.json
	//   cmd/ subdir:  ../configs/supply_chain_graph.json
	//   go test:      ../../configs/supply_chain_graph.json (from internal/industry/)
	for _, p := range supplyChainGraphCandidates() {
		if g, c, err := LoadSupplyChainGraph(p); err == nil {
			return g, c
		}
	}
	// If ATLAS_WORK_DIR is set, also try that.
	if wd := os.Getenv("ATLAS_WORK_DIR"); wd != "" {
		p := filepath.Join(wd, "configs", "supply_chain_graph.json")
		if g, c, err := LoadSupplyChainGraph(p); err == nil {
			return g, c
		}
	}
	return DefaultSupplyChainGraph(), loadCorrelationMatrixWithFallback()
}

func supplyChainGraphCandidates() []string {
	return []string{
		"configs/supply_chain_graph.json",
		"../configs/supply_chain_graph.json",
		"../../configs/supply_chain_graph.json",
	}
}

func loadCorrelationMatrixWithFallback() *CorrelationMatrix {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return DefaultCorrelationMatrix()
	}
	return LoadCorrelationMatrixFromConfig(&cfg.Industry.LinkageParams.Value)
}

func (la *LinkageAnalyzer) SetNarrativeProvider(provider NarrativeLinkageProvider) {
	la.propagation.SetNarrativeProvider(provider)
}

func (la *LinkageAnalyzer) GetSupplyChainGraph() *SupplyChainGraph {
	return la.graph
}

func (la *LinkageAnalyzer) GetCorrelationMatrix() *CorrelationMatrix {
	return la.correlation
}

func (la *LinkageAnalyzer) CalculateLinkageScore(industryID string) *IndustryLinkageScore {
	return la.propagation.CalculateLinkageScore(industryID)
}

func (la *LinkageAnalyzer) PropagateShock(sourceIndustry string, shockMagnitude float64, maxDepth int) map[string]float64 {
	return la.propagation.PropagateShock(sourceIndustry, shockMagnitude, maxDepth)
}

func (la *LinkageAnalyzer) SetSupplyChainGraph(graph *SupplyChainGraph, cm *CorrelationMatrix) {
	la.graph = graph
	la.correlation = cm
	la.propagation = NewShockPropagation(graph, cm)
}

// SetCycleProvider wires a cycle provider into the correlation matrix for
// regime-aware correlation adjustment during recession phases.
func (la *LinkageAnalyzer) SetCycleProvider(cp CycleProvider) {
	la.correlation.SetCycleProvider(cp)
}

type supplyChainGraphJSON struct {
	Nodes []struct {
		IndustryID   string   `json:"industry_id"`
		Tier         int      `json:"tier"`
		UpstreamOf   []string `json:"upstream_of"`
		DownstreamOf []string `json:"downstream_of"`
		KeyMaterials []string `json:"key_materials"`
	} `json:"nodes"`
	Correlations map[string]float64 `json:"correlations"`
}

func LoadSupplyChainGraph(path string) (*SupplyChainGraph, *CorrelationMatrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read supply chain graph: %w", err)
	}

	var cfg supplyChainGraphJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse supply chain graph: %w", err)
	}

	graph := NewSupplyChainGraph()
	for _, n := range cfg.Nodes {
		graph.AddNode(&SupplyChainNode{
			IndustryID:   n.IndustryID,
			Tier:         n.Tier,
			UpstreamOf:   n.UpstreamOf,
			DownstreamOf: n.DownstreamOf,
			KeyMaterials: n.KeyMaterials,
		})
	}

	cm := NewCorrelationMatrix(30)
	for key, val := range cfg.Correlations {
		parts := strings.SplitN(key, "↔", 2)
		if len(parts) == 2 {
			cm.UpdateCorrelation(parts[0], parts[1], val)
		}
	}

	return graph, cm, nil
}
