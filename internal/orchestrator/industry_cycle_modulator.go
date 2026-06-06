package orchestrator

import (
	"fmt"
	"math"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

func paramSensitivity(paramValue string) *float64 {
	if paramValue == "" {
		return nil
	}
	pv, err := strconv.ParseFloat(paramValue, 64)
	if err != nil {
		return nil
	}
	s := math.Abs(pv * 0.1)
	return &s
}

// IndustryCycleModulator reads industry cycle positions and adjusts
// recommendation conviction based on the current business cycle phase.
// Expansion → confidence boost; recession → penalty; recovery/mature are neutral.
//
// Skill-to-industry mapping is loaded from ParametersConfig (Industry.SkillToIndustry).
// When config is missing, skillToIndustry is nil and the modulator is a no-op —
// this is deliberate: mappings are strategy IP that should live in config,
// not hardcoded in the engine.
type IndustryCycleModulator struct {
	tracker         *industry.CycleTracker
	skillToIndustry map[string]string         // nil = no-op (config not loaded); not a bug
	cycleCard       *industry.CycleStatusCard // Wave 4: composite cycle sentiment card
}

func NewIndustryCycleModulator(tracker *industry.CycleTracker) *IndustryCycleModulator {
	m := &IndustryCycleModulator{tracker: tracker}
	if cfg := config.GetParametersConfig(); cfg != nil && cfg.Industry.SkillToIndustry.Value != nil {
		m.skillToIndustry = cfg.Industry.SkillToIndustry.Value
	}
	return m
}

func (m *IndustryCycleModulator) IsAvailable() bool {
	return m != nil && m.tracker != nil
}

// SetCycleCard stores a CycleStatusCard for use in position sizing and
// score modulation. Pass nil to clear the cached card.
func (m *IndustryCycleModulator) SetCycleCard(card *industry.CycleStatusCard) {
	if m == nil {
		return
	}
	m.cycleCard = card
}

// GetCycleCard returns the currently cached CycleStatusCard, or nil.
func (m *IndustryCycleModulator) GetCycleCard() *industry.CycleStatusCard {
	if m == nil {
		return nil
	}
	return m.cycleCard
}

// ModulatePosition adjusts a position size by the composite cycle sentiment.
// When cycle confidence is low (< 0.3), position is reduced by 20%.
// When cycle confidence is high (> 0.7), full position is allowed.
// When no card is available, the position is returned unchanged.
func (m *IndustryCycleModulator) ModulatePosition(size float64, card *industry.CycleStatusCard) float64 {
	if card == nil || size <= 0 {
		return size
	}
	conf := card.CycleConfidence
	switch {
	case conf < 0.3:
		return size * 0.80
	case conf > 0.7:
		return size
	default:
		return size * 0.90
	}
}

// ModulateScore adjusts a conviction score by the composite cycle coefficient.
// CompositeCoefficient ranges [0.8, 1.2]; scores are scaled proportionally.
// When no card is available, the score is returned unchanged.
func (m *IndustryCycleModulator) ModulateScore(score int, card *industry.CycleStatusCard) int {
	if card == nil || score <= 0 {
		return score
	}
	coef := card.CompositeCoefficient
	if coef == 0 {
		return score
	}
	multiplier := 1.0 + (coef-1.0)*2.0
	if multiplier < 0.6 {
		multiplier = 0.6
	}
	if multiplier > 1.4 {
		multiplier = 1.4
	}
	return int(float64(score) * multiplier)
}

// CycleConfidenceFromCard returns the cycle confidence from the cached card
// for the given industry. Falls back to CycleTracker if card is unavailable.
func (m *IndustryCycleModulator) CycleConfidenceFromCard(industryID string) float64 {
	card := m.GetCycleCard()
	if card != nil {
		return card.CycleConfidence
	}
	if m.tracker != nil {
		pos, ok := m.tracker.GetPosition(industryID)
		if ok {
			return pos.Confidence
		}
	}
	return 0.5
}

func phaseDelta(phase industry.CyclePhase) int {
	var exp, rec, mat, recs float64 = 20, 10, 0, -20 // hardcoded fallbacks
	if cfg := config.GetParametersConfig(); cfg != nil {
		exp = cfg.Industry.PhaseScores.Value.ScoreExpansion
		rec = cfg.Industry.PhaseScores.Value.ScoreRecovery
		mat = cfg.Industry.PhaseScores.Value.ScoreMature
		recs = cfg.Industry.PhaseScores.Value.ScoreRecession
	}
	switch phase {
	case industry.CycleExpansion:
		return int(math.Round(exp))
	case industry.CycleRecovery:
		return int(math.Round(rec))
	case industry.CycleMature:
		return int(math.Round(mat))
	case industry.CycleRecession:
		return int(math.Round(recs))
	default:
		return 0
	}
}

// ModulationStep groups provider-produced conviction steps with the index of the
// recommendation they apply to.
type ModulationStep struct {
	RecIndex int
	Steps    []domain.ConvictionStep
}

// CollectModulationSteps returns the conviction steps this modulator would produce
// for the given recommendations, without modifying them in-place.
func (m *IndustryCycleModulator) CollectModulationSteps(
	recs []domain.Recommendation,
	registry domain.AgentRegistry,
) []ModulationStep {
	if !m.IsAvailable() {
		return nil
	}

	skillLookup := make(map[string]string, len(registry.Agents))
	for _, agent := range registry.Agents {
		skillLookup[agent.ID] = agent.Skill
	}

	var result []ModulationStep
	for i := range recs {
		skill := skillLookup[recs[i].Agent]
		industryID, ok := m.skillToIndustry[skill]
		if !ok {
			continue
		}

		pos, ok := m.tracker.GetPosition(industryID)
		if !ok {
			continue
		}

		delta := phaseDelta(pos.BusinessCycle)
		if delta == 0 {
			continue
		}

		confidenceAdjust := math.Round(float64(delta) * pos.Confidence)
		adj := int(confidenceAdjust)

		phaseName := map[industry.CyclePhase]string{
			industry.CycleExpansion: "擴張",
			industry.CycleRecovery:  "復甦",
			industry.CycleMature:    "成熟",
			industry.CycleRecession: "衰退",
		}[pos.BusinessCycle]

		provenanceSource := "hardcoded"
		provenanceRef := ""
		provenanceVal := ""
		if cfg := config.GetParametersConfig(); cfg != nil {
			var phaseScore float64
			switch pos.BusinessCycle {
			case industry.CycleExpansion:
				phaseScore = cfg.Industry.PhaseScores.Value.ScoreExpansion
				provenanceRef = "Industry.PhaseScores.ScoreExpansion"
			case industry.CycleRecovery:
				phaseScore = cfg.Industry.PhaseScores.Value.ScoreRecovery
				provenanceRef = "Industry.PhaseScores.ScoreRecovery"
			case industry.CycleMature:
				phaseScore = cfg.Industry.PhaseScores.Value.ScoreMature
				provenanceRef = "Industry.PhaseScores.ScoreMature"
			case industry.CycleRecession:
				phaseScore = cfg.Industry.PhaseScores.Value.ScoreRecession
				provenanceRef = "Industry.PhaseScores.ScoreRecession"
			}
			if phaseScore != 0 {
				provenanceSource = "config"
				provenanceVal = fmt.Sprintf("%.0f", phaseScore)
			}
		}

		result = append(result, ModulationStep{
			RecIndex: i,
			Steps: []domain.ConvictionStep{{
				Rule:        "modulator:industry_cycle:cycle_phase",
				Delta:       adj,
				Reason:      fmt.Sprintf("產業%s處於%s期(信心度%.0f%%)", industryID, phaseName, pos.Confidence*100),
				Source:      provenanceSource,
				ParamRef:    provenanceRef,
				ParamValue:  provenanceVal,
				Sensitivity: paramSensitivity(provenanceVal),
			}},
		})
	}
	return result
}
