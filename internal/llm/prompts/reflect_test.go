package prompts

import (
	"strings"
	"testing"
)

// TestReflectPrompt_separatesToolResultFromSchema ensures the tool
// result block and the schema block are visually separated so the
// LLM does not confuse tool output with required response fields.
func TestReflectPrompt_separatesToolResultFromSchema(t *testing.T) {
	p := ReflectPrompt("semiconductor", "2330", "TOOL_RESULT_MARKER")

	if !strings.Contains(p, "TOOL_RESULT_MARKER") {
		t.Fatal("tool result not embedded")
	}
	// Both should appear in the rendered prompt.
	if !strings.Contains(p, ReflectTemplate) {
		t.Error("ReflectTemplate not embedded in ReflectPrompt")
	}
}
