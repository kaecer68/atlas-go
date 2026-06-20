// Package schemas provides typed input/output data structures for each
// LLM capability defined in internal/llm. Each capability has its own
// file containing the Input and Response structs, which serve as the
// contract between capability handlers and the LLM Router layer.
//
// Design source: docs/llm-integration-strategy-framework.md §4 capability taxonomy
//
// Maturity: experimental
//
// Public API surface:
//
//   - RationaleGenerationInput  / RationaleGenerationResponse
//   - StrategySummaryInput      / StrategySummaryResponse
//   - PromptLintInput           / PromptLintResponse / LintIssue
//   - ScenarioSimulationInput   / ScenarioSimulationResponse
//   - RiskSurfaceExtractionInput / RiskSurfaceExtractionResponse
//   - RegimeExplanationInput    / RegimeExplanationResponse
//   - PerformanceForensicsInput / PerformanceForensicsResponse
//   - CodeReviewAnnotationInput / CodeReviewAnnotationResponse / CodeAnnotation
//   - SentimentExplanationInput / SentimentExplanationResponse
package schemas
