package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// codeReviewAnnotationSystemPrompt instructs the LLM to annotate code
// review findings from a CI workflow diff, identifying 3-5 issues with
// file location, severity, and Chinese message.
const codeReviewAnnotationSystemPrompt = `你是一個專業的程式碼審查員，請檢查以下 CI 工作流程的 diff 內容，找出 3-5 個程式碼審查發現，包含檔案位置、嚴重程度和中文說明。`

// codeReviewAnnotationUserPromptFmt is the user prompt template. The diff
// text and PR URL are interpolated.
const codeReviewAnnotationUserPromptFmt = `PR URL：%s

Diff 內容：
%s

請以 JSON 格式回覆，包含 annotations 陣列（每個元素有 file, line, severity, message）。`

// CodeReviewAnnotationHandler annotates CI workflow code review findings
// via the llm.Router.
type CodeReviewAnnotationHandler struct {
	router llm.Router
}

// NewCodeReviewAnnotationHandler creates a handler backed by the given Router.
func NewCodeReviewAnnotationHandler(router llm.Router) *CodeReviewAnnotationHandler {
	return &CodeReviewAnnotationHandler{router: router}
}

// Handle executes the code review annotation capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassPublic when unset.
//  3. Dispatches through the Router with CapabilityCodeReviewAnnotation.
//  4. Parses the response into a CodeReviewAnnotationResponse (JSON-first,
//     then raw string fallback as a single annotation).
func (h *CodeReviewAnnotationHandler) Handle(
	ctx context.Context,
	input schemas.CodeReviewAnnotationInput,
) (schemas.CodeReviewAnnotationResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityCodeReviewAnnotation,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 800},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.CodeReviewAnnotationResponse{}, nil
		}
		return schemas.CodeReviewAnnotationResponse{}, err
	}

	return parseCodeReviewAnnotationResponse(resp.Output)
}

// parseCodeReviewAnnotationResponse converts the Router's Response.Output into
// a CodeReviewAnnotationResponse. JSON-first, raw string fallback as a single
// annotation.
func parseCodeReviewAnnotationResponse(output string) (schemas.CodeReviewAnnotationResponse, error) {
	if output == "" {
		return schemas.CodeReviewAnnotationResponse{}, nil
	}

	var parsed schemas.CodeReviewAnnotationResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.CodeReviewAnnotationResponse{
		Annotations: []schemas.CodeAnnotation{
			{File: "", Line: 0, Severity: "info", Message: output},
		},
	}, nil
}
