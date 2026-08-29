package risk

import "context"

// PerformanceForensics is a package-level hook for LLM-based performance
// forensics analysis of a domain.RiskSnapshot. When non-nil, the
// AnnotateSnapshot function calls this hook and returns the commentary.
//
// Set by main.go when config.LLMRiskForensicsEnabled is true and an LLM
// router is available. Uses var indirection to avoid a risk→llm import
// cycle (risk is a leaf package that cannot import llm).
var PerformanceForensics func(ctx context.Context, snapshot any) (string, error)

// AnnotateSnapshot runs the PerformanceForensics hook (when non-nil) against
// a domain.RiskSnapshot and returns the LLM-generated commentary. Returns
// an empty string when the hook is nil or returns an error so callers never
// fail on LLM unavailability.
//
// This function is not auto-wired into the existing VaR pipeline — callers
// invoke it explicitly when LLM-based forensics is desired.
func AnnotateSnapshot(ctx context.Context, snapshot any) string {
	if PerformanceForensics == nil {
		return ""
	}
	commentary, err := PerformanceForensics(ctx, snapshot)
	if err != nil {
		return ""
	}
	return commentary
}
