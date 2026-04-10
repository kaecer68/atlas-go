// Package spawning implements automated agent creation based on detected knowledge gaps
// Inspired by Atlas-GIC autoresearch loop: identify gaps -> spawn agents -> validate -> integrate
package spawning

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// KnowledgeGap represents a detected gap in system coverage
type KnowledgeGap struct {
	ID          string
	Type        GapType
	Severity    GapSeverity
	Description string
	Sector      string
	Style       string
	MarketCap   string
	Evidence    []GapEvidence
	DetectedAt  time.Time
	Status      GapStatus
}

// GapType categorizes the type of knowledge gap
type GapType string

const (
	GapTypeSector      GapType = "sector"      // Missing sector coverage
	GapTypeStyle       GapType = "style"       // Missing investment style
	GapTypeMarketCap   GapType = "marketcap"   // Missing market cap segment
	GapTypeRegime      GapType = "regime"      // Poor performance in specific regime
	GapTypeSymbol      GapType = "symbol"      // Specific symbol undercoverage
	GapTypeCorrelation GapType = "correlation" // Excessive correlation between agents
)

// GapSeverity indicates the urgency of addressing the gap
type GapSeverity string

const (
	GapSeverityCritical GapSeverity = "critical" // >20% coverage loss
	GapSeverityHigh     GapSeverity = "high"     // 10-20% coverage loss
	GapSeverityMedium   GapSeverity = "medium"   // 5-10% coverage loss
	GapSeverityLow      GapSeverity = "low"      // <5% coverage loss
)

// GapStatus tracks the lifecycle of a gap
type GapStatus string

const (
	GapStatusOpen      GapStatus = "open"
	GapStatusSpawning  GapStatus = "spawning"
	GapStatusTesting   GapStatus = "testing"
	GapStatusResolved  GapStatus = "resolved"
	GapStatusDismissed GapStatus = "dismissed"
)

// GapEvidence provides supporting data for a gap
type GapEvidence struct {
	Metric    string
	Value     float64
	Threshold float64
	Context   string
}

// SpawnedAgent tracks an auto-created agent
type SpawnedAgent struct {
	AgentID           string
	ParentAgentID     string
	GapID             string
	CreatedAt         time.Time
	TrainingStart     time.Time
	TrainingEnd       time.Time
	Status            SpawnStatus
	TestScorecard     *domain.Scorecard
	ValidationWindows int
	CurrentWeight     float64
	PromptTemplate    string
}

// SpawnStatus tracks the lifecycle of a spawned agent
type SpawnStatus string

const (
	SpawnStatusTraining   SpawnStatus = "training"
	SpawnStatusValidating SpawnStatus = "validating"
	SpawnStatusCandidate  SpawnStatus = "candidate"
	SpawnStatusAccepted   SpawnStatus = "accepted"
	SpawnStatusRejected   SpawnStatus = "rejected"
	SpawnStatusDisabled   SpawnStatus = "disabled"
)

// GapDetector analyzes system performance to identify knowledge gaps
type GapDetector struct {
	minSignalsForAnalysis int
	coverageThreshold     float64
	correlationThreshold  float64
	mu                    sync.RWMutex
	gaps                  map[string]*KnowledgeGap
	historicalGaps        []string
}

// NewGapDetector creates a new gap detector with default settings
func NewGapDetector() *GapDetector {
	return &GapDetector{
		minSignalsForAnalysis: 30,   // Minimum signals before analyzing agent performance
		coverageThreshold:     0.8,  // 80% coverage target
		correlationThreshold:  0.85, // Flag agents with >85% correlation
		gaps:                  make(map[string]*KnowledgeGap),
		historicalGaps:        make([]string, 0),
	}
}

// DetectGaps analyzes registry and scorecards to find knowledge gaps
func (d *GapDetector) DetectGaps(
	registry domain.AgentRegistry,
	scorecards map[string]*domain.Scorecard,
	universe []string,
) []*KnowledgeGap {
	d.mu.Lock()
	defer d.mu.Unlock()

	newGaps := make([]*KnowledgeGap, 0)

	// 1. Detect sector coverage gaps
	sectorGaps := d.detectSectorGaps(registry, scorecards, universe)
	newGaps = append(newGaps, sectorGaps...)

	// 2. Detect style coverage gaps
	styleGaps := d.detectStyleGaps(registry, scorecards)
	newGaps = append(newGaps, styleGaps...)

	// 3. Detect market cap coverage gaps
	marketCapGaps := d.detectMarketCapGaps(registry, universe)
	newGaps = append(newGaps, marketCapGaps...)

	// 4. Detect regime-specific performance gaps
	regimeGaps := d.detectRegimeGaps(scorecards)
	newGaps = append(newGaps, regimeGaps...)

	// 5. Detect high-correlation agent pairs
	correlationGaps := d.detectCorrelationGaps(registry, scorecards)
	newGaps = append(newGaps, correlationGaps...)

	// Store new gaps
	for _, gap := range newGaps {
		if existing, ok := d.gaps[gap.ID]; !ok || existing.Status == GapStatusDismissed {
			d.gaps[gap.ID] = gap
		}
	}

	return newGaps
}

// detectSectorGaps identifies sectors with poor or no coverage
func (d *GapDetector) detectSectorGaps(
	registry domain.AgentRegistry,
	scorecards map[string]*domain.Scorecard,
	universe []string,
) []*KnowledgeGap {
	gaps := make([]*KnowledgeGap, 0)

	// Define standard sectors for Taiwan market
	sectors := []string{
		"semiconductor", "electronics", "financial", "shipping",
		"biotech", "automotive", "industrials", "consumer",
		"real_estate", "materials", "energy",
	}

	sectorCoverage := make(map[string]int)
	sectorPerformance := make(map[string][]float64)

	// Analyze existing agent coverage
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}

		// Infer sector from agent skill or universe
		sector := inferSectorFromAgent(agent)
		if sector == "" {
			continue
		}

		sectorCoverage[sector]++

		// Track performance if available
		if sc, ok := scorecards[agent.ID]; ok && sc.Observations >= d.minSignalsForAnalysis {
			sectorPerformance[sector] = append(sectorPerformance[sector], sc.SharpeLike)
		}
	}

	// Check for uncovered or poorly covered sectors
	for _, sector := range sectors {
		coverage := sectorCoverage[sector]
		performances := sectorPerformance[sector]

		if coverage == 0 {
			// No coverage at all
			gap := &KnowledgeGap{
				ID:          fmt.Sprintf("gap-sector-%s-%d", sector, time.Now().Unix()),
				Type:        GapTypeSector,
				Severity:    GapSeverityHigh,
				Description: fmt.Sprintf("No agent coverage for %s sector", sector),
				Sector:      sector,
				DetectedAt:  time.Now(),
				Status:      GapStatusOpen,
				Evidence: []GapEvidence{
					{Metric: "agent_coverage", Value: 0, Threshold: 1, Context: sector},
				},
			}
			gaps = append(gaps, gap)
		} else if coverage < 2 {
			// Minimal coverage - check performance
			avgSharpe := average(performances)
			if avgSharpe < 0.3 {
				gap := &KnowledgeGap{
					ID:          fmt.Sprintf("gap-sector-%s-weak-%d", sector, time.Now().Unix()),
					Type:        GapTypeSector,
					Severity:    GapSeverityMedium,
					Description: fmt.Sprintf("%s sector has poor performing coverage (Sharpe: %.2f)", sector, avgSharpe),
					Sector:      sector,
					DetectedAt:  time.Now(),
					Status:      GapStatusOpen,
					Evidence: []GapEvidence{
						{Metric: "avg_sharpe", Value: avgSharpe, Threshold: 0.5, Context: sector},
						{Metric: "agent_count", Value: float64(coverage), Threshold: 2, Context: sector},
					},
				}
				gaps = append(gaps, gap)
			}
		}
	}

	return gaps
}

// detectStyleGaps identifies missing investment style coverage
func (d *GapDetector) detectStyleGaps(
	registry domain.AgentRegistry,
	scorecards map[string]*domain.Scorecard,
) []*KnowledgeGap {
	gaps := make([]*KnowledgeGap, 0)

	// Define investment styles that should be covered
	styles := []string{
		"value", "growth", "momentum", "quality",
		"contrarian", "trend_following", "mean_reversion",
	}

	styleCoverage := make(map[string]int)

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		style := inferStyleFromAgent(agent)
		if style != "" {
			styleCoverage[style]++
		}
	}

	for _, style := range styles {
		if styleCoverage[style] == 0 {
			gap := &KnowledgeGap{
				ID:          fmt.Sprintf("gap-style-%s-%d", style, time.Now().Unix()),
				Type:        GapTypeStyle,
				Severity:    GapSeverityMedium,
				Description: fmt.Sprintf("No agent coverage for %s investing style", style),
				Style:       style,
				DetectedAt:  time.Now(),
				Status:      GapStatusOpen,
				Evidence: []GapEvidence{
					{Metric: "style_coverage", Value: 0, Threshold: 1, Context: style},
				},
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

// detectMarketCapGaps identifies market cap segments with poor coverage
func (d *GapDetector) detectMarketCapGaps(
	registry domain.AgentRegistry,
	universe []string,
) []*KnowledgeGap {
	gaps := make([]*KnowledgeGap, 0)

	// This would typically analyze the universe and detect gaps
	// For now, flag if we have no large/mid/small cap specialists

	return gaps
}

// detectRegimeGaps identifies agents performing poorly in specific market regimes
func (d *GapDetector) detectRegimeGaps(
	scorecards map[string]*domain.Scorecard,
) []*KnowledgeGap {
	gaps := make([]*KnowledgeGap, 0)

	// In a full implementation, this would analyze performance by regime
	// For now, detect agents with very poor overall performance

	for agentID, sc := range scorecards {
		if sc.Observations >= d.minSignalsForAnalysis && sc.SharpeLike < -0.5 {
			gap := &KnowledgeGap{
				ID:          fmt.Sprintf("gap-regime-poor-%s-%d", agentID, time.Now().Unix()),
				Type:        GapTypeRegime,
				Severity:    GapSeverityHigh,
				Description: fmt.Sprintf("Agent %s performing poorly across all regimes (Sharpe: %.2f)", agentID, sc.SharpeLike),
				DetectedAt:  time.Now(),
				Status:      GapStatusOpen,
				Evidence: []GapEvidence{
					{Metric: "sharpe_ratio", Value: sc.SharpeLike, Threshold: 0, Context: "all_regimes"},
					{Metric: "max_drawdown", Value: sc.MaxDrawdown, Threshold: -0.15, Context: agentID},
				},
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

// detectCorrelationGaps identifies agents that are too correlated
func (d *GapDetector) detectCorrelationGaps(
	registry domain.AgentRegistry,
	scorecards map[string]*domain.Scorecard,
) []*KnowledgeGap {
	gaps := make([]*KnowledgeGap, 0)

	// This would require historical recommendation correlation analysis
	// Simplified version: flag if too many agents in same sector/style

	sectorCount := make(map[string]int)
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		sector := inferSectorFromAgent(agent)
		if sector != "" {
			sectorCount[sector]++
		}
	}

	// Flag oversaturated sectors
	for sector, count := range sectorCount {
		if count > 5 {
			gap := &KnowledgeGap{
				ID:          fmt.Sprintf("gap-correlation-%s-%d", sector, time.Now().Unix()),
				Type:        GapTypeCorrelation,
				Severity:    GapSeverityLow,
				Description: fmt.Sprintf("%s sector has %d agents - potential correlation risk", sector, count),
				Sector:      sector,
				DetectedAt:  time.Now(),
				Status:      GapStatusOpen,
				Evidence: []GapEvidence{
					{Metric: "agent_count", Value: float64(count), Threshold: 5, Context: sector},
				},
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

// GetOpenGaps returns all currently open gaps
func (d *GapDetector) GetOpenGaps() []*KnowledgeGap {
	d.mu.RLock()
	defer d.mu.RUnlock()

	openGaps := make([]*KnowledgeGap, 0)
	for _, gap := range d.gaps {
		if gap.Status == GapStatusOpen {
			openGaps = append(openGaps, gap)
		}
	}

	// Sort by severity
	severityOrder := map[GapSeverity]int{
		GapSeverityCritical: 0,
		GapSeverityHigh:     1,
		GapSeverityMedium:   2,
		GapSeverityLow:      3,
	}

	sort.Slice(openGaps, func(i, j int) bool {
		return severityOrder[openGaps[i].Severity] < severityOrder[openGaps[j].Severity]
	})

	return openGaps
}

// UpdateGapStatus updates the status of a gap
func (d *GapDetector) UpdateGapStatus(gapID string, status GapStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if gap, ok := d.gaps[gapID]; ok {
		gap.Status = status
	}
}

// Helper functions

func inferSectorFromAgent(agent domain.AgentSpec) string {
	// Infer sector from agent ID or skill
	sectorKeywords := map[string]string{
		"semi":       "semiconductor",
		"chip":       "semiconductor",
		"financial":  "financial",
		"bank":       "financial",
		"shipping":   "shipping",
		"biotech":    "biotech",
		"pharma":     "biotech",
		"auto":       "automotive",
		"ev":         "automotive",
		"industrial": "industrials",
		"consumer":   "consumer",
		"retail":     "consumer",
		"reit":       "real_estate",
		"material":   "materials",
		"energy":     "energy",
		"ai":         "electronics",
		"tech":       "electronics",
	}

	searchText := strings.ToLower(agent.ID + " " + agent.Skill)
	for keyword, sector := range sectorKeywords {
		if strings.Contains(searchText, keyword) {
			return sector
		}
	}

	return ""
}

func inferStyleFromAgent(agent domain.AgentSpec) string {
	styleKeywords := map[string]string{
		"value":          "value",
		"yield":          "value",
		"growth":         "growth",
		"momentum":       "momentum",
		"quality":        "quality",
		"contrarian":     "contrarian",
		"trend":          "trend_following",
		"breakout":       "momentum",
		"mean_reversion": "mean_reversion",
		"reversal":       "mean_reversion",
	}

	searchText := strings.ToLower(agent.ID + " " + agent.Skill)
	for keyword, style := range styleKeywords {
		if strings.Contains(searchText, keyword) {
			return style
		}
	}

	return ""
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// CalculateGapPriorityScore computes a priority score for gap resolution
func CalculateGapPriorityScore(gap *KnowledgeGap) float64 {
	baseScore := 0.0

	// Severity weight
	switch gap.Severity {
	case GapSeverityCritical:
		baseScore += 100
	case GapSeverityHigh:
		baseScore += 70
	case GapSeverityMedium:
		baseScore += 40
	case GapSeverityLow:
		baseScore += 20
	}

	// Age factor (older gaps get slight priority boost)
	age := time.Since(gap.DetectedAt).Hours()
	ageBonus := math.Min(age/24, 10) // Cap at 10 points for gaps >10 days old

	return baseScore + ageBonus
}
