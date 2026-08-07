package userstate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestUserSignalState_UnacknowledgedSerializesNilPointer guards the
// "未讀 vs 已讀" distinction: an unacknowledged state must serialize with
// acknowledged_at omitted (nil pointer), not a zero timestamp that would
// render as 0001-01-01 in the UI.
func TestUserSignalState_UnacknowledgedSerializesNilPointer(t *testing.T) {
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	s := UserSignalState{
		UserID:    42,
		SignalKey: "foreign-3day-inflow",
		Dismissed: false,
		UpdatedAt: now,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"acknowledged_at"`) {
		t.Fatalf("unacknowledged state must omit acknowledged_at, got %s", data)
	}
}

// TestUserSignalState_AcknowledgedRoundTrip guards that an acknowledged
// state survives a JSON round-trip with its timestamp intact.
func TestUserSignalState_AcknowledgedRoundTrip(t *testing.T) {
	ack := time.Date(2026, 8, 7, 9, 30, 0, 0, time.UTC)
	s := UserSignalState{
		UserID:         42,
		SignalKey:      "foreign-3day-inflow",
		AcknowledgedAt: &ack,
		Dismissed:      true,
		UpdatedAt:      ack,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back UserSignalState
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.AcknowledgedAt == nil || !back.AcknowledgedAt.Equal(ack) {
		t.Fatalf("acknowledged_at = %v, want %v", back.AcknowledgedAt, ack)
	}
	if !back.Dismissed {
		t.Fatal("dismissed = false, want true")
	}
}
