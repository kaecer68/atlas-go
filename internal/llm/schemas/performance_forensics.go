package schemas

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// PerformanceForensicsInput carries a risk snapshot and its data
// classification for performance forensics analysis.
type PerformanceForensicsInput struct {
	Snapshot  domain.RiskSnapshot `json:"snapshot"`
	DataClass llm.DataClass       `json:"data_class"`
}

// PerformanceForensicsResponse is the output of the performance
// forensics capability. It provides natural-language commentary
// and calibration context.
type PerformanceForensicsResponse struct {
	Commentary  string `json:"commentary"`
	Calibration string `json:"calibration"`
}
