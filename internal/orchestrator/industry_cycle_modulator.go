package orchestrator

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// IndustryCycleModulator reads industry cycle positions and adjusts
// recommendation conviction based on the current business cycle phase.
// Expansion → confidence boost; recession → penalty; recovery/mature are neutral.
type IndustryCycleModulator struct {
	tracker *industry.CycleTracker
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
	return &IndustryCycleModulator{tracker: tracker}
}

func (m *IndustryCycleModulator) IsAvailable() bool {
	return m != nil && m.tracker != nil
}

func phaseDelta(phase industry.CyclePhase) int {
	switch phase {
	case industry.CycleExpansion:
		return 20
	case industry.CycleRecovery:
		return 10
	case industry.CycleMature:
		return 0
	case industry.CycleRecession:
		return -20
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
		industryID, ok := skillToIndustry[skill]
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
