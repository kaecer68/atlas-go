package orchestrator

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// NarrativeConvictionModulator references active macro events and adjusts
// recommendation conviction based on which agents correlate with active themes.
// Agents whose skill maps to a currently-active event theme get a boost
// proportional to that theme's historical hit rate.
type NarrativeConvictionModulator struct {
	themeHitRates map[string]float64 // Theme → built-in hit rate
	skillToTheme  map[string]string  // Agent skill → NarrativeTheme
}

// defaultThemeHitRates are built-in hit rates documented in narrative/types.go.
var defaultThemeHitRates = map[string]float64{
	"AI_capex_surge":          0.81,
	"US_rates_up":             0.72,
	"JPY_carry_unwind":        0.68,
	"geopolitical_risk_spike": 0.65,
	"oil_price_shock":         0.58,
}

// defaultSkillToTheme maps agent skills to the narrative theme they correlate with.
var defaultSkillToTheme = map[string]string{
	"semiconductor_desk":   "AI_capex_surge",
	"ai_supply_chain_desk": "AI_capex_surge",
	"shipping_desk":        "oil_price_shock",
	"financials_desk":      "US_rates_up",
	"etf_rotation_desk":    "JPY_carry_unwind",
	"value_yield":          "US_rates_up",
	"earnings_quality":     "AI_capex_surge",
	"growth_momentum":      "AI_capex_surge",
	"technical_breakout":   "AI_capex_surge",
}

// NewNarrativeConvictionModulator creates a modulator with theme hit rates and
// skill-to-theme mappings. It reads from ParametersConfig first and falls back
// to hardcoded defaults when the configuration is nil or missing.
func NewNarrativeConvictionModulator() *NarrativeConvictionModulator {
	hitRates := defaultThemeHitRates
	skillToTheme := defaultSkillToTheme

	if cfg := config.GetParametersConfig(); cfg != nil {
		if cfg.NarrativeConviction.ThemeHitRates.Value != nil {
			hitRates = cfg.NarrativeConviction.ThemeHitRates.Value
		}
		if cfg.NarrativeConviction.SkillToTheme.Value != nil {
			skillToTheme = cfg.NarrativeConviction.SkillToTheme.Value
		}
	}

	return &NarrativeConvictionModulator{
		themeHitRates: hitRates,
		skillToTheme:  skillToTheme,
	}
}

// IsAvailable returns true when the modulator has its internal maps populated.
func (m *NarrativeConvictionModulator) IsAvailable() bool {
	return m != nil && m.themeHitRates != nil && m.skillToTheme != nil
}

// CollectModulationSteps returns the conviction steps this modulator would produce
// for the given recommendations, without modifying them in-place.
func (m *NarrativeConvictionModulator) CollectModulationSteps(
	recs []domain.Recommendation,
	registry domain.AgentRegistry,
	events []narrative.NarrativeEvent,
) []ModulationStep {
	if !m.IsAvailable() || len(events) == 0 {
		return nil
	}

	type activeInfo struct {
		hitRate    float64
		confidence float64
	}

	activeThemes := make(map[string]activeInfo)
	for _, ev := range events {
		if ev.Status != "active" {
			continue
		}
		hr := m.themeHitRates[ev.Theme]
		if hr == 0 {
			hr = ev.HitRate
		}
		activeThemes[ev.Theme] = activeInfo{hitRate: hr, confidence: ev.Confidence}
	}

	if len(activeThemes) == 0 {
		return nil
	}

	skillLookup := make(map[string]string, len(registry.Agents))
	for _, agent := range registry.Agents {
		skillLookup[agent.ID] = agent.Skill
	}

	var result []ModulationStep
	for i := range recs {
		skill := skillLookup[recs[i].Agent]
		theme, ok := m.skillToTheme[skill]
		if !ok {
			continue
		}

		info, ok := activeThemes[theme]
		if !ok {
			continue
		}

		adj := int(math.Round(10 * info.hitRate))
		if adj == 0 {
			continue
		}

		provenanceSource := "heuristic"
		provenanceRef := ""
		provenanceVal := ""
		if cfg := config.GetParametersConfig(); cfg != nil {
			if _, ok := cfg.NarrativeConviction.ThemeHitRates.Value[theme]; ok {
				provenanceSource = "config"
				provenanceRef = fmt.Sprintf("NarrativeConviction.ThemeHitRates.%s", theme)
				provenanceVal = fmt.Sprintf("%.2f", info.hitRate)
			}
		}

		result = append(result, ModulationStep{
			RecIndex: i,
			Steps: []domain.ConvictionStep{{
				Rule:        "modulator:narrative:narrative_boost",
				Delta:       adj,
				Reason:      fmt.Sprintf("%s (hit_rate: %.0f%%, confidence: %.0f%%)", theme, info.hitRate*100, info.confidence*100),
				Source:      provenanceSource,
				ParamRef:    provenanceRef,
				ParamValue:  provenanceVal,
				Sensitivity: paramSensitivity(provenanceVal),
			}},
		})
	}
	return result
}
