package schemas

import "github.com/kaecer68/atlas-go/internal/llm"

// PromptLintInput carries the prompt content to be linted, its file
// path for diagnostics, and its data classification.
type PromptLintInput struct {
	PromptContent string       `json:"prompt_content"`
	PromptPath    string       `json:"prompt_path"`
	DataClass     llm.DataClass `json:"data_class"`
}

// PromptLintResponse is the output of the prompt lint capability.
// It contains a list of detected issues and a pass/fail verdict.
type PromptLintResponse struct {
	Issues []LintIssue `json:"issues"`
	Pass   bool        `json:"pass"`
}

// LintIssue describes a single issue found during prompt linting.
type LintIssue struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
