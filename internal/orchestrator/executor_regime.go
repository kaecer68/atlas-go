package orchestrator

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// inferRegime evaluates four evidence sources as a sequential pipeline
// per 憲章 §二 因果傳導鏈. Each layer's evidence is computed in order
// (layer_0 → layer_4 → layer_7 → layer_root), and when the prior layer
// strongly disagrees with the current layer, confidence is reduced.
// LayerID/LayerParentID traces are recorded in the scratchpad for
// causal chain provenance (B2 P0 + B5 P1).
// RegimeAuthorityFunc returns the authoritative market regime for the
// current moment (from regime_history, written continuously by macro_ingest
// from the Taiwan stress index), plus the stress score when available.
// ok=false means no authoritative value exists (cold start / store down) and
// the evidence-based inference is used as fallback (#1785).
type RegimeAuthorityFunc func() (regime domain.Regime, stressScore float64, ok bool)

func inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, events []narrative.NarrativeEvent, scratchpad *Scratchpad, sessionID string, authority RegimeAuthorityFunc) domain.Regime {
	sources := []RegimeEvidenceSource{
		NewMacroEvidenceSource(),                                   // layer_0: 全球資金總開關
		NewTechnicalEvidenceSource(),                               // layer_4: 台股大盤量能
		NewNarrativeEvidenceSource(),                               // layer_7: 事件層
		NewAgentSignalEvidenceSource(registry, plugins, overrides), // layer_root: LLM 綜合
	}

	type layerSnapshot struct {
		layerID string
		score   float64
		conf    float64
		source  string
	}
	var snapshots []layerSnapshot
	var prevScore float64

	var totalScore, totalConfidence float64

	for i, src := range sources {
		ev := src.Evidence(quotes, events)

		// B2: sequential constraint — when prior layer strongly disagrees,
		// reduce this layer's confidence per 憲章 "由上而下" principle.
		// Example: macro layer RISK_OFF + technical layer RISK_ON → halve technical confidence.
		if i > 0 && ev.Confidence > 0 {
			if (prevScore < -0.3 && ev.Score > 0.3) || (prevScore > 0.3 && ev.Score < -0.3) {
				ev.Confidence *= 0.5
			}
		}

		parentID := "layer_origin"
		if len(snapshots) > 0 {
			parentID = snapshots[len(snapshots)-1].layerID
		}

		if ev.Confidence <= 0 {
			if scratchpad != nil {
				scratchpad.Record(ReasoningTrace{
					SessionID:     sessionID,
					Timestamp:     time.Now().UTC(),
					Phase:         PhaseRegimeDetection,
					Step:          i + 1,
					Component:     "regime_inference",
					Action:        "layer_evidence",
					Reasoning:     fmt.Sprintf("%s: zero confidence, skipped", src.LayerID()),
					Data:          map[string]any{"layer": src.LayerID(), "score": ev.Score, "source": ev.Source},
					LayerID:       src.LayerID(),
					LayerParentID: parentID,
					Confidence:    0,
				})
			}
			continue
		}

		totalScore += ev.Score * ev.Confidence
		totalConfidence += ev.Confidence
		prevScore = ev.Score
		snapshots = append(snapshots, layerSnapshot{
			layerID: src.LayerID(),
			score:   ev.Score,
			conf:    ev.Confidence,
			source:  ev.Source,
		})

		if scratchpad != nil {
			scratchpad.Record(ReasoningTrace{
				SessionID:     sessionID,
				Timestamp:     time.Now().UTC(),
				Phase:         PhaseRegimeDetection,
				Step:          i + 1,
				Component:     "regime_inference",
				Action:        "layer_evidence",
				Reasoning:     fmt.Sprintf("%s: score=%.3f conf=%.3f source=%s", src.LayerID(), ev.Score, ev.Confidence, ev.Source),
				Data:          map[string]any{"layer": src.LayerID(), "score": ev.Score, "confidence": ev.Confidence, "source": ev.Source, "parent": parentID},
				Confidence:    ev.Confidence,
				LayerID:       src.LayerID(),
				LayerParentID: parentID,
			})
		}
	}

	// #1785: authoritative-first. regime_history (macro_ingest stress index)
	// is the designated truth (see monitoring/service/regime_consistency.go).
	// When an authoritative value exists, the 4-layer evidence runs only as
	// advisory context and the verdict comes from the authority — the two
	// dashboards can no longer disagree. Evidence inference remains the
	// fallback when no authority is available.
	if authority != nil {
		if authRegime, authScore, ok := authority(); ok {
			normalized := 0.0
			if totalConfidence > 0 {
				normalized = totalScore / totalConfidence
			}
			if scratchpad != nil {
				scratchpad.Record(ReasoningTrace{
					SessionID:  sessionID,
					Timestamp:  time.Now().UTC(),
					Phase:      PhaseRegimeDetection,
					Step:       len(sources) + 1,
					Component:  "regime_inference",
					Action:     "authoritative_regime_override",
					Reasoning:  fmt.Sprintf("Regime adopted from authoritative regime_history (stress score %.3f); 4-layer inference normalized %.4f (advisory, %d/%d layers active)", authScore, normalized, len(snapshots), len(sources)),
					Data:       map[string]any{"regime": authRegime, "score": authScore, "inferred_score": normalized, "evidence_count": len(sources), "source": "regime_history"},
					Confidence: 1.0,
				})
			}
			return authRegime
		}
	}

	if totalConfidence == 0 {
		if scratchpad != nil {
			scratchpad.Record(ReasoningTrace{
				SessionID:  sessionID,
				Timestamp:  time.Now().UTC(),
				Phase:      PhaseRegimeDetection,
				Step:       len(sources) + 1,
				Component:  "regime_inference",
				Action:     "detect_regime",
				Reasoning:  "No evidence sources produced confidence; defaulting to neutral",
				Data:       map[string]any{"regime": domain.RegimeNeutral, "score": 0.0, "evidence_count": len(sources)},
				Confidence: 0.0,
			})
		}
		return domain.RegimeNeutral
	}

	normalized := totalScore / totalConfidence

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
			Step:       len(sources) + 1,
			Component:  "regime_inference",
			Action:     "detect_regime",
			Reasoning:  fmt.Sprintf("Regime detected: %s (normalized score: %.4f, sequential layers: %d/%d)", regime, normalized, len(snapshots), len(sources)),
			Data:       map[string]any{"regime": regime, "score": normalized, "evidence_count": len(sources), "active_layers": len(snapshots)},
			Confidence: totalConfidence,
		})
	}

	return regime
}
