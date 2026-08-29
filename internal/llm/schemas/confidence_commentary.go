package schemas

import "github.com/kaecer68/atlas-go/internal/llm"

// ConfidenceCommentaryInput carries a risk decision and its data
// classification for confidence commentary analysis.
//
// Decision uses interface{} to avoid an import cycle between the
// schemas and risk packages (risk ↔ schemas would form a cycle).
// Callers must pass a risk.RiskDecision value that the handler
// serializes to JSON for LLM consumption.
type ConfidenceCommentaryInput struct {
	Decision  any           `json:"decision"`
	DataClass llm.DataClass `json:"data_class"`
}

// ConfidenceCommentaryResponse is the output of the confidence
// commentary capability. It provides a natural-language explanation
// of why the decision's confidence level is appropriate or not.
type ConfidenceCommentaryResponse struct {
	Commentary string `json:"commentary"`
}
