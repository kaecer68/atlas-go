package schemas

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// RegimeExplanationInput carries a narrative event and its data
// classification for regime explanation.
type RegimeExplanationInput struct {
	Event     narrative.NarrativeEvent `json:"event"`
	DataClass llm.DataClass            `json:"data_class"`
}

// RegimeExplanationResponse is the output of the regime explanation
// capability. It provides a concise headline summarizing the regime
// shift implied by the event.
type RegimeExplanationResponse struct {
	Headline string `json:"headline"`
}
