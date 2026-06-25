package prompts

import "fmt"

// ReflectTemplate is the JSON structure the LLM must return for the
// reflect phase. The adapter parses this into orchestrator.Reflection.
const ReflectTemplate = `Return your decision as a JSON object with this exact structure:
{
  "continue": true,
  "final_conviction": 75,
  "reasoning": "<explanation>"
}

Fields:
- "continue": boolean. true = keep planning (more tool calls needed); false = commit final recommendation.
- "final_conviction": integer in [0, 100]. Only meaningful when continue=false; the agent's confidence.
- "reasoning": string. Brief explanation of the decision.

Return ONLY the JSON object, no other text.`

// ReflectPrompt returns the full prompt for the reflect phase,
// including the tool result and the expected response format.
func ReflectPrompt(skill, symbol, toolResult string) string {
	return fmt.Sprintf(
		`You are a sector agent for %q. You just received this tool result for %q:

---
%s
---

Decide whether to continue planning or commit a final recommendation.

%s`,
		skill, symbol, toolResult, ReflectTemplate,
	)
}
