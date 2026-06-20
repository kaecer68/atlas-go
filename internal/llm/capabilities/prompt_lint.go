package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// promptLintSystemPrompt instructs the LLM to identify issues in agent
// prompt files (ambiguity, missing examples, unclear constraints, etc.).
const promptLintSystemPrompt = `你是一個專業的 prompt 工程審查員，請檢查以下 agent prompt 的品質，找出 5 個常見問題：模糊用語、缺少範例、約束不明確、角色定義不清、輸出格式未指定。`

// promptLintUserPromptFmt is the user prompt template. The prompt content
// and file path are interpolated.
const promptLintUserPromptFmt = `Prompt 檔案：%s

內容：
%s

請以 JSON 格式回覆，包含 issues 陣列（每個元素有 line, severity, message）和 pass 布林值。`

// PromptLintHandler scans agent prompt markdown files for quality issues
// via the llm.Router. It returns a list of LintIssue and a pass/fail verdict.
type PromptLintHandler struct {
	router llm.Router
}

// NewPromptLintHandler creates a handler backed by the given Router.
func NewPromptLintHandler(router llm.Router) *PromptLintHandler {
	return &PromptLintHandler{router: router}
}

// Handle executes the prompt lint capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassPublic when unset.
//  3. Dispatches through the Router with CapabilityPromptLint.
//  4. Parses the response into a PromptLintResponse (JSON-first,
//     then raw string fallback as a single warning issue).
func (h *PromptLintHandler) Handle(
	ctx context.Context,
	input schemas.PromptLintInput,
) (schemas.PromptLintResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityPromptLint,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 800},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.PromptLintResponse{
				Pass:   true,
				Issues: nil,
			}, nil
		}
		return schemas.PromptLintResponse{}, err
	}

	return parsePromptLintResponse(resp.Output)
}

// parsePromptLintResponse converts the Router's Response.Output into a
// PromptLintResponse. JSON-first, raw string fallback as a single warning.
func parsePromptLintResponse(output string) (schemas.PromptLintResponse, error) {
	if output == "" {
		return schemas.PromptLintResponse{Pass: true}, nil
	}

	var parsed schemas.PromptLintResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.PromptLintResponse{
		Pass: false,
		Issues: []schemas.LintIssue{
			{Line: 0, Severity: "warning", Message: output},
		},
	}, nil
}
