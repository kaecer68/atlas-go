package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestPosition_HasEntryDateField verifies that the canonical Position struct
// carries an EntryDate field with JSON tag `entry_date`. This contract is the
// foundation for MaxHoldingDays enforcement and T+2 settlement in the sim
// engine (Phase 2 P2-T1).
func TestPosition_HasEntryDateField(t *testing.T) {
	entry := time.Date(2026, 6, 17, 9, 30, 0, 0, time.UTC)
	pos := Position{
		Symbol:        "2330",
		Quantity:      100,
		AverageCost:   900.0,
		CurrentPrice:  910.0,
		MarketValue:   91000.0,
		UnrealizedPnL: 1000.0,
		EntryDate:     entry,
	}

	// Step 1: JSON marshal must include the entry_date key.
	data, err := json.Marshal(pos)
	if err != nil {
		t.Fatalf("marshal Position: %v", err)
	}
	if !strings.Contains(string(data), `"entry_date"`) {
		t.Errorf("expected JSON to contain entry_date, got: %s", string(data))
	}

	// Step 2: roundtrip must preserve EntryDate.
	var decoded Position
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal Position: %v", err)
	}
	if !decoded.EntryDate.Equal(entry) {
		t.Errorf("EntryDate roundtrip mismatch: want %v, got %v", entry, decoded.EntryDate)
	}
}

// TestPosition_ZeroEntryDateIsValid documents that an unset EntryDate is
// representable (zero time.Time) — used when loading legacy ledger data
// before this field existed.
func TestPosition_ZeroEntryDateIsValid(t *testing.T) {
	pos := Position{Symbol: "2330", Quantity: 100}
	if !pos.EntryDate.IsZero() {
		t.Errorf("expected zero EntryDate by default, got %v", pos.EntryDate)
	}
}
