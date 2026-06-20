package schemas

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// StrategySummaryInput carries a strategy frame and its data
// classification for the strategy summary capability.
type StrategySummaryInput struct {
	Frame     strategy_techniques.StrategyFrame `json:"frame"`
	DataClass llm.DataClass                     `json:"data_class"`
}

// StrategySummaryResponse is the output of the strategy summary
// capability. It provides a human-readable summary and extracted
// key conditions from the strategy frame.
type StrategySummaryResponse struct {
	Summary       string   `json:"summary"`
	KeyConditions []string `json:"key_conditions"`
}
