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
// Skill-to-industry mapping is loaded from ParametersConfig. When config is
// missing, the modulator operates without industry assignments (no-op).
type IndustryCycleModulator struct {
	tracker         *industry.CycleTracker
	skillToIndustry map[string]string
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
