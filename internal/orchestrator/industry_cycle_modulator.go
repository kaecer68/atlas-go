package orchestrator

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// IndustryCycleModulator reads industry cycle positions and adjusts
// recommendation conviction based on the current business cycle phase.
// Expansion → confidence boost; recession → penalty; recovery/mature are neutral.
type IndustryCycleModulator struct {
	tracker          *industry.CycleTracker
	skillToIndustry  map[string]string
}

var skillToIndustry = map[string]string{
	"semiconductor_desk":   "semiconductor",
	"ai_supply_chain_desk": "ai_supply_chain",
	"financials_desk":      "financials",
	"shipping_desk":        "shipping",
	"etf_rotation_desk":    "etf_rotation",
	"value_yield":          "financials",
	"earnings_quality":     "electronics",
	"growth_momentum":      "electronics",
	"technical_breakout":   "electronics",
}

func NewIndustryCycleModulator(tracker *industry.CycleTracker) *IndustryCycleModulator {
	sti := skillToIndustry // hardcoded fallback
	if cfg := config.GetParametersConfig(); cfg != nil && cfg.Industry.SkillToIndustry.Value != nil {
		sti = cfg.Industry.SkillToIndustry.Value
	}
	return &IndustryCycleModulator{tracker: tracker, skillToIndustry: sti}
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

func (m *IndustryCycleModulator) ModulateRecommendations(
	recs []domain.Recommendation,
	registry domain.AgentRegistry,
) {
	if !m.IsAvailable() {
		return
	}

	skillLookup := make(map[string]string, len(registry.Agents))
	for _, agent := range registry.Agents {
		skillLookup[agent.ID] = agent.Skill
	}

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

		recs[i].Conviction += adj

		phaseName := map[industry.CyclePhase]string{
			industry.CycleExpansion: "擴張",
			industry.CycleRecovery:  "復甦",
			industry.CycleMature:    "成熟",
			industry.CycleRecession: "衰退",
		}[pos.BusinessCycle]

		step := domain.ConvictionStep{
			Rule:   "cycle_phase",
			Delta:  adj,
			Reason: fmt.Sprintf("產業%s處於%s期(信心度%.0f%%)", industryID, phaseName, pos.Confidence*100),
		}

		if recs[i].ConvictionBreakdown != nil {
			recs[i].ConvictionBreakdown.Steps = append(recs[i].ConvictionBreakdown.Steps, step)
			recs[i].ConvictionBreakdown.Final = recs[i].Conviction
		}
	}
}
