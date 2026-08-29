package risk

import "context"

// ConfidenceCommentary is a package-level hook for LLM-based confidence
// commentary enrichment of a risk.RiskDecision. When non-nil, the
// EnrichDecision function calls this hook and returns the commentary.
//
// Set by main.go when config.LLMConfidenceCommentaryEnabled is true and
// an LLM router is available. Uses var indirection to avoid a risk→llm
// import cycle (risk is a leaf package that cannot import llm).
var ConfidenceCommentary func(ctx context.Context, decision any) (string, error)

// EnrichDecision runs the ConfidenceCommentary hook (when non-nil) against
// a risk.RiskDecision and returns the LLM-generated commentary. Returns
// an empty string when the hook is nil or returns an error so callers
// never fail on LLM unavailability.
func EnrichDecision(ctx context.Context, decision any) string {
	if ConfidenceCommentary == nil {
		return ""
	}
	commentary, err := ConfidenceCommentary(ctx, decision)
	if err != nil {
		return ""
	}
	return commentary
}
