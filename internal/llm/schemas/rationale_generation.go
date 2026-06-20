package schemas

import "github.com/kaecer68/atlas-go/internal/llm"

// RationaleGenerationInput carries the text to be translated and its
// data classification for routing decisions.
type RationaleGenerationInput struct {
	EnglishText string       `json:"english_text"`
	DataClass   llm.DataClass `json:"data_class"`
}

// RationaleGenerationResponse contains the translated text produced by
// the rationale generation capability.
type RationaleGenerationResponse struct {
	TranslatedText string `json:"translated_text"`
}
