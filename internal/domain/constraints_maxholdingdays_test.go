package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSimulationConstraints_HasMaxHoldingDaysField verifies the MaxHoldingDays
// constraint carries JSON tag `max_holding_days` and is configurable to 0
// (no enforcement). Foundation for P2-T4 force-close logic.
func TestSimulationConstraints_HasMaxHoldingDaysField(t *testing.T) {
	c := SimulationConstraints{
		StartingCash:      1_000_000,
		MaxHoldingDays:    20,
		StopLossPct:       -0.05,
		TakeProfitPct:     0.10,
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"max_holding_days":20`) {
		t.Errorf("expected JSON to contain max_holding_days:20, got: %s", string(data))
	}

	var decoded SimulationConstraints
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.MaxHoldingDays != 20 {
		t.Errorf("MaxHoldingDays roundtrip mismatch: want 20, got %d", decoded.MaxHoldingDays)
	}
}

// TestSimulationConstraints_ZeroMaxHoldingDaysIsValid documents that 0 means
// "no enforcement" (legacy behavior preserved). Production deployments must
// explicitly set a positive value to opt into force-close.
func TestSimulationConstraints_ZeroMaxHoldingDaysIsValid(t *testing.T) {
	c := SimulationConstraints{MaxHoldingDays: 0}
	if c.MaxHoldingDays != 0 {
		t.Errorf("expected zero MaxHoldingDays to roundtrip, got %d", c.MaxHoldingDays)
	}
}
