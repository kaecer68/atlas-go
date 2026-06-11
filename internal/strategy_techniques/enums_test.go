package strategy_techniques

import "testing"

// TestLayer_IsValid covers all 5 layers + 6 invalid inputs (boundary cases).
func TestLayer_IsValid(t *testing.T) {
	validLayers := []Layer{
		LayerL1GlobalLiquidity,
		LayerL2ForeignBehavior,
		LayerL3IndustryCatalysts,
		LayerL4FXAndChips,
		LayerL5Geopolitics,
	}
	for _, l := range validLayers {
		if !l.IsValid() {
			t.Errorf("expected %q to be valid", l)
		}
	}

	invalidLayers := []Layer{"", "L0", "L6", "l1", "invalid", "L9"}
	for _, l := range invalidLayers {
		if l.IsValid() {
			t.Errorf("expected %q to be invalid", l)
		}
	}
}

func TestLayer_String(t *testing.T) {
	cases := map[Layer]string{
		LayerL1GlobalLiquidity:   "L1",
		LayerL2ForeignBehavior:   "L2",
		LayerL3IndustryCatalysts: "L3",
		LayerL4FXAndChips:        "L4",
		LayerL5Geopolitics:       "L5",
	}
	for layer, expected := range cases {
		if got := layer.String(); got != expected {
			t.Errorf("String() for %v: expected %q, got %q", layer, expected, got)
		}
	}
}

func TestLayer_String_invalidReturnsUnknown(t *testing.T) {
	if got := Layer("L99").String(); got != "unknown" {
		t.Errorf("expected \"unknown\" for invalid layer, got %q", got)
	}
}

func TestStatus_IsValid(t *testing.T) {
	validStatuses := []Status{StatusActive, StatusDegraded, StatusExpired}
	for _, s := range validStatuses {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalidStatuses := []Status{"", "pending", "deleted", "ACTIVE", "Degraded"}
	for _, s := range invalidStatuses {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid (case-sensitive)", s)
		}
	}
}

func TestStatus_String(t *testing.T) {
	cases := map[Status]string{
		StatusActive:   "active",
		StatusDegraded: "degraded",
		StatusExpired:  "expired",
	}
	for status, expected := range cases {
		if got := status.String(); got != expected {
			t.Errorf("String() for %v: expected %q, got %q", status, expected, got)
		}
	}
}

func TestAttributionMode_IsValid(t *testing.T) {
	validModes := []AttributionMode{
		AttributionModeRuleBased,
		AttributionModeLLMAnnotated,
	}
	for _, m := range validModes {
		if !m.IsValid() {
			t.Errorf("expected %q to be valid", m)
		}
	}

	invalidModes := []AttributionMode{"", "rule", "llm", "manual", "RULE_BASED"}
	for _, m := range invalidModes {
		if m.IsValid() {
			t.Errorf("expected %q to be invalid", m)
		}
	}
}

func TestAttributionMode_String(t *testing.T) {
	cases := map[AttributionMode]string{
		AttributionModeRuleBased:    "rule_based",
		AttributionModeLLMAnnotated: "llm_annotated",
	}
	for mode, expected := range cases {
		if got := mode.String(); got != expected {
			t.Errorf("String() for %v: expected %q, got %q", mode, expected, got)
		}
	}
}
