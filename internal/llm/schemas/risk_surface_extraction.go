package schemas

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

// RiskSurfaceExtractionInput carries a knowledge gap and its data
// classification for risk surface analysis.
type RiskSurfaceExtractionInput struct {
	Gap       spawning.KnowledgeGap `json:"gap"`
	DataClass llm.DataClass          `json:"data_class"`
}

// RiskSurfaceExtractionResponse is the output of the risk surface
// extraction capability. It provides an enriched description of
// the gap and a coverage score.
type RiskSurfaceExtractionResponse struct {
	EnrichedDescription string  `json:"enriched_description"`
	Coverage            float64 `json:"coverage"`
}
