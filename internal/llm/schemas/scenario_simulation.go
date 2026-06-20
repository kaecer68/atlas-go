package schemas

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/prism"
)

// ScenarioSimulationInput carries a training result with its regime
// type and data classification.
type ScenarioSimulationInput struct {
	Result    prism.TrainingResult `json:"result"`
	Regime    prism.RegimeType     `json:"regime"`
	DataClass llm.DataClass        `json:"data_class"`
}

// ScenarioSimulationResponse is the output of the scenario simulation
// capability. It provides an insight narrative and a cohort summary.
type ScenarioSimulationResponse struct {
	Insight       string `json:"insight"`
	CohortSummary string `json:"cohort_summary"`
}
