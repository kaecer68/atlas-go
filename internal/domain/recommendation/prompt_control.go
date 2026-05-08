package recommendation

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// PromptControl defines structured, machine-readable controls embedded in prompts.
type PromptControl struct {
	VolumeFloor             int64  `json:"volume_floor,omitempty"`
	VolumeDowngrade         int    `json:"volume_downgrade,omitempty"`
	CloseStrengthBoost      int    `json:"close_strength_boost,omitempty"`
	HardRejectVolume        int64  `json:"hard_reject_volume,omitempty"`
	PriceCondition          string `json:"price_condition,omitempty"`
	ConvictionFloor         int    `json:"conviction_floor,omitempty"`
	VolumeBoost             int    `json:"volume_boost,omitempty"`
	RequireTrend            bool   `json:"require_trend,omitempty"`
	NeutralPenaltyReduction int    `json:"neutral_penalty_reduction,omitempty"`
}

var ControlBlockRe = regexp.MustCompile(`(?s)<!--\s*control_block\s*-->([\s\S]*?)<!--\s*/control_block\s*-->`)

// ExtractPromptControl parses a prompt string and returns the embedded control block.
func ExtractPromptControl(prompt string) (PromptControl, bool) {
	matches := ControlBlockRe.FindStringSubmatch(prompt)
	if len(matches) < 2 {
		return PromptControl{}, false
	}
	var ctrl PromptControl
	if err := json.Unmarshal([]byte(matches[1]), &ctrl); err != nil {
		return PromptControl{}, false
	}
	return ctrl, true
}

// RenderPromptControl serializes a control block into a prompt fragment.
func RenderPromptControl(ctrl PromptControl) string {
	b, _ := json.MarshalIndent(ctrl, "", "  ")
	return fmt.Sprintf("<!-- control_block -->\n%s\n<!-- /control_block -->", string(b))
}
