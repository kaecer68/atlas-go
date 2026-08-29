package narrative

import "context"

// RegimeExplainer is a package-level hook for LLM-based regime explanation
// of a NarrativeEvent. When non-nil, AnnotateEvent calls this function and
// stores the returned headline in event.Explanation.
//
// Set by main.go when config.LLMNarrativeExplainEnabled is true and an LLM
// router is available. Uses var indirection to avoid a narrative→llm
// import cycle (narrative is a leaf package that cannot import llm).
var RegimeExplainer func(ctx context.Context, event any) (string, error)

// SentimentExplainer is a package-level hook for LLM-based sentiment
// explanation of a NarrativeEvent. When non-nil, AnnotateEvent calls this
// function and stores the returned explanation in event.SentimentExplanation.
//
// Set by main.go when config.LLMNarrativeExplainEnabled is true and an LLM
// router is available. Uses the same var indirection pattern as RegimeExplainer.
var SentimentExplainer func(ctx context.Context, event any) (string, error)

// AnnotateEvent runs the RegimeExplainer and SentimentExplainer hooks (when
// non-nil) against a NarrativeEvent and mutates the event's Explanation and
// SentimentExplanation fields. Errors from either hook are silently ignored
// so callers never fail on LLM unavailability.
func AnnotateEvent(ctx context.Context, event *NarrativeEvent) {
	if RegimeExplainer != nil {
		explanation, err := RegimeExplainer(ctx, event)
		if err == nil && explanation != "" {
			event.Explanation = explanation
		}
	}
	if SentimentExplainer != nil {
		explanation, err := SentimentExplainer(ctx, event)
		if err == nil && explanation != "" {
			event.SentimentExplanation = explanation
		}
	}
}
