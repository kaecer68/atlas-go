package schemas

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// SentimentExplanationInput carries a narrative event and its data
// classification for sentiment analysis.
type SentimentExplanationInput struct {
	Event     narrative.NarrativeEvent `json:"event"`
	DataClass llm.DataClass            `json:"data_class"`
}

// SentimentExplanationResponse is the output of the sentiment
// explanation capability. It provides a narrative explanation
// and contributing factors.
type SentimentExplanationResponse struct {
	Explanation string   `json:"explanation"`
	Factors     []string `json:"factors"`
}
