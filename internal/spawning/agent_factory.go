package spawning

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// AgentFactory creates new agent specifications based on knowledge gaps
type AgentFactory struct {
	defaultPromptTemplate string
	counter               int
}

// NewAgentFactory creates a new agent factory
func NewAgentFactory() *AgentFactory {
	return &AgentFactory{
		defaultPromptTemplate: defaultPromptTemplate(),
		counter:               0,
	}
}

// CreateAgentForGap generates an AgentSpec for a given knowledge gap
func (f *AgentFactory) CreateAgentForGap(
	gap *KnowledgeGap,
	parentAgentID string,
) (*domain.AgentSpec, string) {
	f.counter++

	// Generate agent ID based on gap type and counter
	agentID := f.generateAgentID(gap)

	// Determine layer based on gap type
	layer := f.determineLayer(gap)

	// Generate skill name
	skill := f.generateSkill(gap)

	// Generate prompt file path
	promptFile := fmt.Sprintf("prompts/agents/%s.md", agentID)

	// Create prompt content based on gap
	promptContent := f.generatePromptContent(gap, agentID)

	// Determine universe based on gap
	universe := f.determineUniverse(gap)

	// Set initial Darwinian weight (new agents start at neutral)
	initialWeight := 1.0

	spec := &domain.AgentSpec{
		ID:              agentID,
		Name:            f.generateName(gap),
		Layer:           layer,
		Skill:           skill,
		PromptFile:      promptFile,
		Enabled:         false, // Start disabled until training complete
		Universe:        universe,
		DarwinianWeight: initialWeight,
		PrimaryMetrics:  f.determineMetrics(gap),
		OperatingNotes: []string{
			fmt.Sprintf("Auto-spawned for gap: %s", gap.ID),
			fmt.Sprintf("Gap type: %s", gap.Type),
			fmt.Sprintf("Created: %s", time.Now().Format("2006-01-02")),
		},
	}

	return spec, promptContent
}

// generateAgentID creates a unique agent ID
func (f *AgentFactory) generateAgentID(gap *KnowledgeGap) string {
	timestamp := time.Now().Unix()

	switch gap.Type {
	case GapTypeSector:
		return fmt.Sprintf("spawn_%s_%d_%d", gap.Sector, f.counter, timestamp)
	case GapTypeStyle:
		return fmt.Sprintf("spawn_%s_%d_%d", gap.Style, f.counter, timestamp)
	case GapTypeRegime:
		return fmt.Sprintf("spawn_regime_%d_%d", f.counter, timestamp)
	case GapTypeCorrelation:
		return fmt.Sprintf("spawn_alt_%s_%d_%d", gap.Sector, f.counter, timestamp)
	default:
		return fmt.Sprintf("spawn_auto_%d_%d", f.counter, timestamp)
	}
}

// determineLayer assigns appropriate layer for the agent
func (f *AgentFactory) determineLayer(gap *KnowledgeGap) domain.AgentLayer {
	switch gap.Type {
	case GapTypeSector:
		return domain.LayerSector
	case GapTypeStyle:
		return domain.LayerStyle
	case GapTypeRegime:
		// Regime-specific agents could be context or style layer
		return domain.LayerStyle
	default:
		return domain.LayerStyle
	}
}

// generateSkill creates appropriate skill name
func (f *AgentFactory) generateSkill(gap *KnowledgeGap) string {
	switch gap.Type {
	case GapTypeSector:
		return fmt.Sprintf("sector_%s_specialist", gap.Sector)
	case GapTypeStyle:
		return fmt.Sprintf("style_%s", gap.Style)
	case GapTypeRegime:
		return "regime_specialist"
	case GapTypeCorrelation:
		return fmt.Sprintf("alternative_%s", gap.Sector)
	default:
		return "adaptive_specialist"
	}
}

// generateName creates human-readable agent name
func (f *AgentFactory) generateName(gap *KnowledgeGap) string {
	switch gap.Type {
	case GapTypeSector:
		return fmt.Sprintf("%s Specialist (Auto)", cases.Title(language.English).String(gap.Sector))
	case GapTypeStyle:
		return fmt.Sprintf("%s Style Agent (Auto)", cases.Title(language.English).String(gap.Style))
	case GapTypeRegime:
		return "Regime Adaptive Agent (Auto)"
	default:
		return fmt.Sprintf("Auto Agent %d", f.counter)
	}
}

// determineUniverse sets appropriate stock universe
func (f *AgentFactory) determineUniverse(gap *KnowledgeGap) []string {
	// Sector-specific universes
	sectorUniverses := map[string][]string{
		"semiconductor": {"2330.TW", "2454.TW", "2303.TW", "3034.TW", "2379.TW"},
		"electronics":   {"2317.TW", "2354.TW", "2382.TW", "3231.TW"},
		"financial":     {"2881.TW", "2882.TW", "2884.TW", "2885.TW", "2891.TW"},
		"shipping":      {"2603.TW", "2609.TW", "2615.TW", "2618.TW"},
		"biotech":       {"6456.TW", "8436.TW", "6589.TW"},
		"automotive":    {"2207.TW", "2227.TW", "2236.TW"},
	}

	if gap.Sector != "" {
		if universe, ok := sectorUniverses[gap.Sector]; ok {
			return universe
		}
	}

	// Default: empty means use system default
	return []string{}
}

// determineMetrics selects relevant metrics for the agent
func (f *AgentFactory) determineMetrics(gap *KnowledgeGap) []string {
	switch gap.Type {
	case GapTypeSector:
		return []string{"sector_momentum", "relative_strength", "earnings_growth"}
	case GapTypeStyle:
		switch gap.Style {
		case "value":
			return []string{"pe_ratio", "pb_ratio", "dividend_yield"}
		case "growth":
			return []string{"revenue_growth", "earnings_growth", "peg_ratio"}
		case "momentum":
			return []string{"rsi", "macd", "price_momentum"}
		default:
			return []string{"sharpe_ratio", "win_rate"}
		}
	case GapTypeRegime:
		return []string{"regime_indicator", "volatility", "correlation"}
	default:
		return []string{"sharpe_ratio", "win_rate", "max_drawdown"}
	}
}

// generatePromptContent creates prompt template for the new agent
func (f *AgentFactory) generatePromptContent(gap *KnowledgeGap, agentID string) string {
	// Base template
	base := f.defaultPromptTemplate

	// Add gap-specific specialization
	specialization := f.generateSpecialization(gap)

	// Combine
	content := fmt.Sprintf(
		`# Auto-Spawned Agent: %s

## Identity
You are an adaptive investment specialist auto-generated to address a specific knowledge gap in the system.

## Purpose
**Gap Addressed**: %s
**Gap Type**: %s
**Severity**: %s

## Specialization
%s

## Key Metrics to Track
%s

## Operating Guidelines
- Focus on your assigned specialization
- Coordinate with existing agents to avoid duplication
- Report unusual patterns that might indicate new gaps
- Maintain diversity of perspective from other agents

## Constraints
- Do not recommend outside your assigned universe without explicit reason
- Always provide conviction score (1-100)
- Include both bullish and bearish scenarios when relevant
- Cross-check with related agents before high-conviction recommendations

## Collaboration Notes
%s

---
*This agent was automatically generated by the Atlas spawning system.*
*Created: %s*
`,
		agentID,
		gap.Description,
		gap.Type,
		gap.Severity,
		specialization,
		strings.Join(f.determineMetrics(gap), ", "),
		f.generateCollaborationNotes(gap),
		time.Now().Format("2006-01-02"),
	)

	// Add base template content
	content += "\n## Base Guidelines\n" + base

	return content
}

// generateSpecialization creates specialization instructions
func (f *AgentFactory) generateSpecialization(gap *KnowledgeGap) string {
	switch gap.Type {
	case GapTypeSector:
		return fmt.Sprintf(`
This agent specializes in the **%s** sector.

### Sector Focus
- Deep expertise in %s industry dynamics
- Understanding of sector-specific valuation metrics
- Awareness of regulatory and competitive landscape
- Tracking of sector rotation patterns

### Responsibilities
1. Identify undervalued opportunities within %s
2. Flag sector-wide risks and headwinds
3. Provide relative strength analysis vs other sectors
4. Monitor earnings trends and guidance within sector
`, gap.Sector, gap.Sector, gap.Sector)

	case GapTypeStyle:
		return fmt.Sprintf(`
This agent specializes in **%s** investing style.

### Style Philosophy
- Strict adherence to %s principles
- Understanding of when %s outperforms/underperforms
- Risk management appropriate for %s approach

### Implementation
1. Apply %s criteria consistently
2. Document style-specific edge cases
3. Monitor style rotation cycles
4. Compare opportunities within %s framework
`, gap.Style, gap.Style, gap.Style, gap.Style, gap.Style, gap.Style)

	case GapTypeRegime:
		return `
This agent specializes in **regime-adaptive** investing.

### Adaptive Approach
- Detects current market regime (risk-on/risk-off, etc.)
- Adjusts recommendations based on macro conditions
- Provides regime-appropriate risk management

### Responsibilities
1. Identify prevailing market regime
2. Recommend regime-appropriate positioning
3. Flag regime transitions early
4. Adjust conviction based on regime fit
`

	default:
		return "This agent provides general adaptive coverage."
	}
}

// generateCollaborationNotes defines how this agent should work with others
func (f *AgentFactory) generateCollaborationNotes(gap *KnowledgeGap) string {
	notes := []string{
		"- Coordinate with Taiwan Macro agent for regime context",
		"- Leverage Darwinian Weights system for performance feedback",
	}

	if gap.Sector != "" {
		notes = append(notes, fmt.Sprintf("- Cross-check with other %s sector agents", gap.Sector))
	}

	if gap.Style != "" {
		notes = append(notes, fmt.Sprintf("- Validate against %s specialists", gap.Style))
	}

	notes = append(
		notes,
		"- Report conflicts with high-weight agents to system",
		"- Respect CIO portfolio synthesis final decisions",
	)

	return strings.Join(notes, "\n")
}

// defaultPromptTemplate returns base prompt template
func defaultPromptTemplate() string {
	return `
### Output Format

Always respond with:

RECOMMENDATION: [SYMBOL] | [BUY/SELL/HOLD] | [CONVICTION 1-100]
RATIONALE: [2-3 sentence clear explanation]
CATALYST: [Near-term trigger or None]
RISK: [Primary risk factor]

### Decision Framework

For BUY recommendations:
1. Minimum conviction 60
2. Clear positive catalyst within 90 days
3. Favorable risk/reward (>2:1)

For SELL recommendations:
1. Minimum conviction 70
2. Deteriorating fundamentals or technicals
3. Better opportunities elsewhere

For HOLD/No Action:
1. No clear edge or catalyst
2. Risk/reward unattractive
3. Conflicting signals from other agents

### Quality Standards

Before submitting recommendation:
- [ ] Conviction score justified by evidence
- [ ] Both bull and bear case considered
- [ ] Position sizing appropriate for conviction
- [ ] Exit criteria defined
- [ ] Cross-checked with related agents
`
}

// CloneAgentWithVariation creates a variation of an existing agent
func (f *AgentFactory) CloneAgentWithVariation(
	parent domain.AgentSpec,
	variationType string,
) (*domain.AgentSpec, string) {
	f.counter++

	agentID := fmt.Sprintf("%s_var_%s_%d", parent.ID, variationType, f.counter)

	spec := &domain.AgentSpec{
		ID:               agentID,
		Name:             fmt.Sprintf("%s (%s Variant)", parent.Name, cases.Title(language.English).String(variationType)),
		Layer:            parent.Layer,
		Skill:            parent.Skill + "_" + variationType,
		PromptFile:       fmt.Sprintf("prompts/agents/%s.md", agentID),
		Enabled:          false,
		Universe:         parent.Universe,
		DarwinianWeight:  1.0,
		PrimaryMetrics:   parent.PrimaryMetrics,
		RequiredSkills:   parent.RequiredSkills,
		ForbiddenActions: parent.ForbiddenActions,
		OperatingNotes: append(
			parent.OperatingNotes,
			fmt.Sprintf("Cloned from %s with %s variation", parent.ID, variationType),
			fmt.Sprintf("Created: %s", time.Now().Format("2006-01-02")),
		),
	}

	// Generate variation-specific prompt
	prompt := f.generateVariationPrompt(parent, variationType, agentID)

	return spec, prompt
}

// generateVariationPrompt creates prompt for agent variation
func (f *AgentFactory) generateVariationPrompt(parent domain.AgentSpec, variationType, agentID string) string {
	variationGuidelines := map[string]string{
		"conservative": "- Higher conviction thresholds (70+ for buy)\n- Stricter risk criteria\n- Prefer larger, established companies",
		"aggressive":   "- Lower conviction thresholds (50+ for buy)\n- Accept higher volatility\n- Focus on high-growth opportunities",
		"contrarian":   "- Seek opposite consensus\n- Fade crowded trades\n- Value disagreement as opportunity",
		"technical":    "- Prioritize technical indicators\n- Use chart patterns as primary input\n- Faster decision cycles",
		"fundamental":  "- Deep dive financials\n- Longer holding periods\n- Ignore short-term noise",
	}

	guidelines := variationGuidelines[variationType]
	if guidelines == "" {
		guidelines = "- Variant-specific behavior\n- Test alternative approach\n- Compare against parent agent"
	}

	return fmt.Sprintf(
		`# Agent Variant: %s

## Parent Agent
**Base**: %s
**Variation Type**: %s

## Additional Guidelines
%s

## Success Criteria
This variant will be evaluated against the parent agent on:
- Sharpe ratio comparison
- Hit rate in different regimes
- Maximum drawdown
- Correlation with parent (target < 0.8)

## Operating Notes
- Start with neutral Darwinian weight (1.0)
- Weight will adjust based on relative performance
- May be disabled if consistently underperforms parent

---
*Original parent prompt follows:*
[Parent agent prompt would be included here]
`,
		agentID,
		parent.ID,
		variationType,
		guidelines,
	)
}
