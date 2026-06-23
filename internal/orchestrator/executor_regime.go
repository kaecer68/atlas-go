package orchestrator

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, events []narrative.NarrativeEvent, scratchpad *Scratchpad, sessionID string) domain.Regime {
	sources := []RegimeEvidenceSource{
		NewMacroEvidenceSource(),
		NewTechnicalEvidenceSource(),
		NewNarrativeEvidenceSource(),
		NewAgentSignalEvidenceSource(registry, plugins, overrides),
	}

	var totalScore, totalWeight float64
	for _, src := range sources {
		ev := src.Evidence(quotes, events)
		if ev.Confidence > 0 {
			totalScore += ev.Score * ev.Confidence
			totalWeight += ev.Confidence
		}
	}

	if totalWeight == 0 {
		if scratchpad != nil {
			scratchpad.Record(ReasoningTrace{
				SessionID:  sessionID,
				Timestamp:  time.Now().UTC(),
				Phase:      PhaseRegimeDetection,
				Step:       1,
				Component:  "regime_inference",
				Action:     "detect_regime",
				Reasoning:  "No evidence sources produced confidence; defaulting to neutral",
				Data:       map[string]any{"regime": domain.RegimeNeutral, "score": 0.0, "evidence_count": len(sources)},
				Confidence: 0.0,
			})
		}
		return domain.RegimeNeutral
	}

	normalized := totalScore / totalWeight

	var regime domain.Regime
	switch {
	case normalized > 0.15:
		regime = domain.RegimeRiskOn
	case normalized < -0.15:
		regime = domain.RegimeRiskOff
	default:
		regime = domain.RegimeNeutral
	}

	if scratchpad != nil {
		scratchpad.Record(ReasoningTrace{
			SessionID:  sessionID,
			Timestamp:  time.Now().UTC(),
			Phase:      PhaseRegimeDetection,
			Step:       1,
			Component:  "regime_inference",
			Action:     "detect_regime",
			Reasoning:  fmt.Sprintf("Regime detected: %s (normalized score: %.4f)", regime, normalized),
			Data:       map[string]any{"regime": regime, "score": normalized, "evidence_count": len(sources)},
			Confidence: totalWeight,
		})
	}

	return regime
}
