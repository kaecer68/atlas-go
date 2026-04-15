package domain

import (
	"strings"
	"testing"
)

func TestExtractPromptControl(t *testing.T) {
	prompt := `# Semiconductor Desk

<!-- control_block -->
{
  "volume_floor": 1500000,
  "volume_downgrade": 25,
  "close_strength_boost": 10
}
<!-- /control_block -->
`
	ctrl, ok := ExtractPromptControl(prompt)
	if !ok {
		t.Fatal("expected control block to be found")
	}
	if ctrl.VolumeFloor != 1500000 {
		t.Fatalf("volume_floor expected 1500000, got %d", ctrl.VolumeFloor)
	}
	if ctrl.VolumeDowngrade != 25 {
		t.Fatalf("volume_downgrade expected 25, got %d", ctrl.VolumeDowngrade)
	}
	if ctrl.CloseStrengthBoost != 10 {
		t.Fatalf("close_strength_boost expected 10, got %d", ctrl.CloseStrengthBoost)
	}
}

func TestExtractPromptControlMissing(t *testing.T) {
	_, ok := ExtractPromptControl("# Plain prompt without control block")
	if ok {
		t.Fatal("expected no control block")
	}
}

func TestRenderPromptControl(t *testing.T) {
	ctrl := PromptControl{VolumeFloor: 2000000, VolumeDowngrade: 30}
	rendered := RenderPromptControl(ctrl)
	if !strings.Contains(rendered, "<!-- control_block -->") {
		t.Fatal("missing control_block open tag")
	}
	if !strings.Contains(rendered, "<!-- /control_block -->") {
		t.Fatal("missing control_block close tag")
	}
	if !strings.Contains(rendered, "2000000") {
		t.Fatal("missing volume_floor value")
	}
}
