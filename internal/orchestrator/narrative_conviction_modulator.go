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
//
// Theme hit rates and skill-to-theme mappings are loaded from ParametersConfig
// (NarrativeConviction.ThemeHitRates + NarrativeConviction.SkillToTheme).
// When config is missing, both maps are nil and the modulator is a no-op —
// this is deliberate: mappings are strategy IP that should live in config,
// not hardcoded in the engine.
type NarrativeConvictionModulator struct {
	themeHitRates map[string]float64 // nil = no-op (config not loaded); not a bug
	skillToTheme  map[string]string  // nil = no-op (config not loaded); not a bug
}

func NewNarrativeConvictionModulator() *NarrativeConvictionModulator {
	m := &NarrativeConvictionModulator{}

	if cfg := config.GetParametersConfig(); cfg != nil {
		if cfg.NarrativeConviction.ThemeHitRates.Value != nil {
			m.themeHitRates = cfg.NarrativeConviction.ThemeHitRates.Value
		}
		if cfg.NarrativeConviction.SkillToTheme.Value != nil {
			m.skillToTheme = cfg.NarrativeConviction.SkillToTheme.Value
		}
	}

	return m
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
