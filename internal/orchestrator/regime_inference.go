package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type RegimeEvidence struct {
	Score      float64
	Confidence float64
	Source     string
	// LayerID identifies the constitutional causal chain layer (§二).
	LayerID string
}

type RegimeEvidenceSource interface {
	Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence
	// LayerID returns the constitutional layer this source represents (§二 因果傳導鏈).
	LayerID() string
}

type MacroEvidenceSource struct{}

func NewMacroEvidenceSource() *MacroEvidenceSource {
	return &MacroEvidenceSource{}
}

func (s *MacroEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	params := config.GetParametersConfig()
	volThreshold := params.Realtime.VolatilityThreshold.Value

	score := 0.0
	confidence := 0.3

	// VIX may appear under "VIX" or "^VIX" in the quotes map, depending
	// on the provider (Yahoo uses ^VIX, synthetic data uses VIX).
	// The map key is the Quote.Symbol field, not a prefixed format.
	vix, ok := quotes["VIX"]
	if !ok {
		vix, ok = quotes["^VIX"]
	}
	if ok {
		if vix.Last > volThreshold*1.5 {
			score = -0.8
			confidence = 0.7
		} else if vix.Last > volThreshold {
			score = -0.4
			confidence = 0.5
		} else if vix.Last < volThreshold*0.7 {
			score = 0.4
			confidence = 0.5
		}
	}

	return RegimeEvidence{Score: score, Confidence: confidence, Source: "macro", LayerID: "layer_0"}
}

func (s *MacroEvidenceSource) LayerID() string { return "layer_0" }

type TechnicalEvidenceSource struct{}

func NewTechnicalEvidenceSource() *TechnicalEvidenceSource {
	return &TechnicalEvidenceSource{}
}

func (s *TechnicalEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	score := 0.0
	confidence := 0.25

	for _, q := range quotes {
		if q.Volume > 0 && q.Last >= q.Open {
			score += 0.3
			confidence = 0.4
		} else if q.Volume > 0 {
			score -= 0.3
			confidence = 0.4
		}
	}

	return RegimeEvidence{Score: score, Confidence: confidence, Source: "technical", LayerID: "layer_4"}
}

func (s *TechnicalEvidenceSource) LayerID() string { return "layer_4" }

type NarrativeEvidenceSource struct{}

func NewNarrativeEvidenceSource() *NarrativeEvidenceSource {
	return &NarrativeEvidenceSource{}
}

func (s *NarrativeEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	if len(events) == 0 {
		return RegimeEvidence{Score: 0, Confidence: 0, Source: "narrative", LayerID: "layer_7"}
	}

	var totalScore float64
	var totalConfidence float64

	for _, e := range events {
		evScore := narrativeThemeScore(e.Theme)
		weight := e.Confidence * e.HitRate
		totalScore += evScore * weight
		totalConfidence += weight
	}

	if totalConfidence > 0 {
		totalScore /= totalConfidence
	}

	avgConfidence := totalConfidence / float64(len(events))
	return RegimeEvidence{Score: totalScore, Confidence: avgConfidence, Source: "narrative", LayerID: "layer_7"}
}

func (s *NarrativeEvidenceSource) LayerID() string { return "layer_7" }

// narrativeThemeScore maps a narrative theme to its regime evidence contribution.
// Negative = risk-off pressure, positive = risk-on support.
// D4 P1: expanded from 5 to all 24 detector themes.
func narrativeThemeScore(theme string) float64 {
	switch theme {
	// ── Risk-off themes (negative contribution) ──
	case "US_rates_up", "geopolitical_risk_spike", "oil_price_shock",
		"JPY_carry_unwind", "taiwan_political_risk", "semiconductor_downturn",
		"tariff_shock":
		return -0.5

	case "USD_TWD_volatility", "retail_institutional_divergence",
		"gold_rally", "dollar_surge", "inflation_spike",
		"shipping_rate_spike", "china_slowdown":
		return -0.3

	// ── Risk-on themes (positive contribution) ──
	case "AI_capex_surge", "US_rates_down", "taiwan_export_boom",
		"earnings_surprise":
		return +0.5

	// ── Seasonal themes — weight by period sensitivity ──
	case "spring_festival_season", "election_cycle", "earnings_blackout",
		"tech_peak_season", "year_end_window_dressing", "dividend_season":
		return +0.1 // muted: seasonal effects are time-boxed

	default:
		return 0
	}
}

type AgentSignalEvidenceSource struct {
	registry  domain.AgentRegistry
	plugins   *PluginRegistry
	overrides map[string]string
}

func NewAgentSignalEvidenceSource(registry domain.AgentRegistry, plugins *PluginRegistry, overrides map[string]string) *AgentSignalEvidenceSource {
	return &AgentSignalEvidenceSource{
		registry:  registry,
		plugins:   plugins,
		overrides: overrides,
	}
}

func (s *AgentSignalEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	score := 0
	for _, agent := range s.registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerContext {
			continue
		}
		prompt := s.plugins.ResolvePrompt(agent, s.overrides)
		score += s.plugins.RegimeScore(agent, quotes, prompt)
	}

	var regimeScore float64
	if score > 0 {
		regimeScore = 0.5
	} else if score < 0 {
		regimeScore = -0.5
	}

	return RegimeEvidence{Score: regimeScore, Confidence: 0.3, Source: "agent_signal", LayerID: "layer_root"}
}

func (s *AgentSignalEvidenceSource) LayerID() string { return "layer_root" }
