package orchestrator

import (
	"math"

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

// macroSubEvidence is one directional sub-signal for the macro layer.
type macroSubEvidence struct {
	score      float64
	confidence float64
	available  bool
}

// macroRateEvidence derives the US10Y directional sub-evidence (L1/L2:
// rate up = discount-rate compression, rate down = capital reflow).
// ChangePct is the intraday open-to-last move in percent; quote maps carry
// Last/Open only (spec v0.2 §6.2). Absent quote -> not available (#1785
// no-phantom-vote: the sub-evidence drops out instead of diluting).
func macroRateEvidence(quotes map[string]domain.Quote) macroSubEvidence {
	threshold := config.GetParametersConfig().Realtime.RateMoveThresholdPct.Value
	if threshold <= 0 {
		threshold = 0.5
	}
	for _, key := range []string{"US10Y", "^TNX"} {
		if q, ok := quotes[key]; ok && q.Open != 0 {
			chg := (q.Last - q.Open) / q.Open * 100
			switch {
			case chg > threshold:
				return macroSubEvidence{score: -0.4, confidence: 0.5, available: true}
			case chg < -threshold:
				return macroSubEvidence{score: +0.4, confidence: 0.5, available: true}
			default:
				return macroSubEvidence{score: 0, confidence: 0, available: true}
			}
		}
	}
	return macroSubEvidence{}
}

// macroDollarEvidence derives the DXY directional sub-evidence (L8:
// dollar surge = EM outflow pressure, dollar softening = inflow relief).
func macroDollarEvidence(quotes map[string]domain.Quote) macroSubEvidence {
	threshold := config.GetParametersConfig().Realtime.DXYMoveThresholdPct.Value
	if threshold <= 0 {
		threshold = 0.8
	}
	for _, key := range []string{"DXY", "^DXY"} {
		if q, ok := quotes[key]; ok && q.Open != 0 {
			chg := (q.Last - q.Open) / q.Open * 100
			switch {
			case chg > threshold:
				return macroSubEvidence{score: -0.3, confidence: 0.5, available: true}
			case chg < -threshold:
				return macroSubEvidence{score: +0.3, confidence: 0.5, available: true}
			default:
				return macroSubEvidence{score: 0, confidence: 0, available: true}
			}
		}
	}
	return macroSubEvidence{}
}

// composeMacroEvidence merges sub-evidences per spec v0.2 §6.2: same-direction
// (all nonzero same sign) -> max magnitude with confidence stacking; opposing
// signs -> confidence-weighted average with the minimum confidence. This
// guarantees confirming signals never dilute the strongest sub-signal.
func composeMacroEvidence(subs []macroSubEvidence) (score, confidence float64) {
	var signs []int
	for _, sub := range subs {
		if sub.available && sub.score != 0 {
			sign := 1
			if sub.score < 0 {
				sign = -1
			}
			signs = append(signs, sign)
		}
	}
	conflicted := false
	for i := 1; i < len(signs); i++ {
		if signs[i] != signs[0] {
			conflicted = true
			break
		}
	}

	if !conflicted {
		// Same direction: take max magnitude, stack a small confidence bonus
		// for each additional confirming sub-evidence.
		best := macroSubEvidence{}
		for _, sub := range subs {
			if sub.available && sub.score != 0 && math.Abs(sub.score) > math.Abs(best.score) {
				best = sub
			}
		}
		confirming := 0
		maxConf := 0.0
		for _, sub := range subs {
			if sub.available && sub.score != 0 {
				if sub.confidence > maxConf {
					maxConf = sub.confidence
				}
				confirming++
			}
		}
		if confirming == 0 {
			return 0, 0
		}
		return best.score, math.Min(1.0, maxConf+0.1*float64(confirming-1))
	}

	// Conflicted: weighted average by confidence, confidence = min.
	var num, den, minConf float64
	for _, sub := range subs {
		if sub.available && sub.score != 0 {
			num += sub.score * sub.confidence
			den += sub.confidence
			if minConf == 0 || sub.confidence < minConf {
				minConf = sub.confidence
			}
		}
	}
	if den == 0 {
		return 0, 0
	}
	return num / den, minConf
}

func (s *MacroEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	params := config.GetParametersConfig()
	volThreshold := params.Realtime.VolatilityThreshold.Value

	// VIX may appear under "VIX" or "^VIX" in the quotes map, depending
	// on the provider (Yahoo uses ^VIX, synthetic data uses VIX).
	// The map key is the Quote.Symbol field, not a prefixed format.
	vix, ok := quotes["VIX"]
	if !ok {
		vix, ok = quotes["^VIX"]
	}

	// #1785 baseline: no macro evidence at all -> confidence 0 (no phantom
	// vote). With the directional subs (spec v0.2 §6) a rates/dollar signal
	// alone is real evidence, so only the all-absent case stays at 0.
	var subs []macroSubEvidence

	if ok {
		sub := macroSubEvidence{available: true}
		switch {
		case vix.Last > volThreshold*1.5:
			sub.score, sub.confidence = -0.8, 0.7
		case vix.Last > volThreshold:
			sub.score, sub.confidence = -0.4, 0.5
		case vix.Last < volThreshold*0.7:
			sub.score, sub.confidence = 0.4, 0.5
		default:
			// VIX in neutral band: available but neutral (score 0).
			sub.confidence = 0
		}
		subs = append(subs, sub)
	} else {
		// #1785: no VIX in the quote map = no macro evidence at all. The old
		// code still voted with confidence 0.3 at score 0 — a phantom neutral
		// vote that diluted real evidence in small-sample simulation contexts.
		subs = append(subs, macroSubEvidence{})
	}

	subs = append(subs, macroRateEvidence(quotes), macroDollarEvidence(quotes))

	score, confidence := composeMacroEvidence(subs)
	return RegimeEvidence{Score: score, Confidence: confidence, Source: "macro", LayerID: "layer_0"}
}

func (s *MacroEvidenceSource) LayerID() string { return "layer_0" }

type TechnicalEvidenceSource struct{}

func NewTechnicalEvidenceSource() *TechnicalEvidenceSource {
	return &TechnicalEvidenceSource{}
}

func (s *TechnicalEvidenceSource) Evidence(quotes map[string]domain.Quote, events []narrative.NarrativeEvent) RegimeEvidence {
	// #1785: the old implementation accumulated ±0.3 PER QUOTE, so a tiny
	// quote map (daily-sim replay exposes only a handful of volumed symbols)
	// produced large raw scores with full confidence — e.g. 4 down-ticks →
	// score=-1.2 conf=0.4, enough to drag the whole session to RISK_OFF while
	// the authoritative stress index said RISK_ON. The layer now emits a
	// bounded vote ratio in [-1,1] and scales confidence by sample coverage:
	// a 4-symbol sample can no longer outvote the macro layer.
	up, down, total := 0, 0, 0
	for _, q := range quotes {
		if q.Volume <= 0 {
			continue
		}
		total++
		if q.Last >= q.Open {
			up++
		} else {
			down++
		}
	}
	if total == 0 {
		return RegimeEvidence{Score: 0, Confidence: 0, Source: "technical", LayerID: "layer_4"}
	}

	score := (float64(up) - float64(down)) / float64(total)
	confidence := 0.4 * min(1.0, float64(total)/20.0) // full weight at ≥20 symbols

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
	// Mirror-symmetry tiers (spec v0.2 §7 item 5): +0.5 mirrors the -0.5
	// strong risk-off tier; +0.3 mirrors the -0.3 mild tier (inflation_spike).
	case "AI_capex_surge", "US_rates_down", "taiwan_export_boom",
		"earnings_surprise", "conflict_deescalation", "us_earnings_boom",
		"dollar_softening":
		return +0.5

	case "inflation_cool", "inflation_moderate":
		return +0.3

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
	contributed := 0 // #1785: a registry with no enabled context agents must
	// not cast a phantom 0.3-confidence neutral vote — it diluted the other
	// layers and (combined with the old technical-layer math) produced false
	// RISK_OFF verdicts in small-sample simulation contexts.
	for _, agent := range s.registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerContext {
			continue
		}
		prompt := s.plugins.ResolvePrompt(agent, s.overrides)
		score += s.plugins.RegimeScore(agent, quotes, prompt)
		contributed++
	}

	if contributed == 0 {
		return RegimeEvidence{Score: 0, Confidence: 0, Source: "agent_signal", LayerID: "layer_root"}
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
