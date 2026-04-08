package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// ─── FlexTime.UnmarshalJSON ──────────────────────────────────────────────────

func TestFlexTime_UnmarshalJSON_Formats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantY    int
		wantM    time.Month
		wantD    int
		wantH    int
		wantMin  int
		wantZero bool
	}{
		{
			name:  "RFC3339Nano",
			input: `"2026-04-08T09:30:00.123456789Z"`,
			wantY: 2026, wantM: time.April, wantD: 8, wantH: 9, wantMin: 30,
		},
		{
			name:  "RFC3339",
			input: `"2026-04-08T09:30:00Z"`,
			wantY: 2026, wantM: time.April, wantD: 8, wantH: 9, wantMin: 30,
		},
		{
			name:  "datetime without timezone",
			input: `"2026-04-08T09:30:00"`,
			wantY: 2026, wantM: time.April, wantD: 8, wantH: 9, wantMin: 30,
		},
		{
			name:  "date only",
			input: `"2026-04-08"`,
			wantY: 2026, wantM: time.April, wantD: 8,
		},
		{
			name:     "empty string → zero time",
			input:    `""`,
			wantZero: true,
		},
		{
			name:     "null → zero time",
			input:    `null`,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			if err := json.Unmarshal([]byte(tt.input), &ft); err != nil {
				t.Fatalf("UnmarshalJSON(%s) unexpected error: %v", tt.input, err)
			}
			if tt.wantZero {
				if !ft.IsZero() {
					t.Errorf("expected zero time, got %v", ft.Time)
				}
				return
			}
			if ft.Year() != tt.wantY {
				t.Errorf("Year = %d, want %d", ft.Year(), tt.wantY)
			}
			if ft.Month() != tt.wantM {
				t.Errorf("Month = %v, want %v", ft.Month(), tt.wantM)
			}
			if ft.Day() != tt.wantD {
				t.Errorf("Day = %d, want %d", ft.Day(), tt.wantD)
			}
			if tt.wantH != 0 && ft.Hour() != tt.wantH {
				t.Errorf("Hour = %d, want %d", ft.Hour(), tt.wantH)
			}
		})
	}
}

func TestFlexTime_UnmarshalJSON_InvalidFormat(t *testing.T) {
	var ft FlexTime
	if err := json.Unmarshal([]byte(`"not-a-date"`), &ft); err == nil {
		t.Fatal("expected error for unrecognised format, got nil")
	}
}

// ─── FlexTime.MarshalJSON ────────────────────────────────────────────────────

func TestFlexTime_MarshalJSON_NonZero(t *testing.T) {
	ts := time.Date(2026, time.April, 8, 9, 30, 0, 0, time.UTC)
	ft := FlexTime{Time: ts}

	data, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON unexpected error: %v", err)
	}
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		t.Fatalf("expected JSON string, got %s", data)
	}
	// Should round-trip
	var back FlexTime
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("round-trip UnmarshalJSON error: %v", err)
	}
	if !back.Equal(ts) {
		t.Errorf("round-trip mismatch: got %v, want %v", back.Time, ts)
	}
}

func TestFlexTime_MarshalJSON_Zero(t *testing.T) {
	ft := FlexTime{}
	data, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON unexpected error: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("MarshalJSON zero time = %s, want null", data)
	}
}

// ─── FlexTime round-trip via struct ──────────────────────────────────────────

func TestFlexTime_JSONRoundTrip_InStruct(t *testing.T) {
	type payload struct {
		T FlexTime `json:"t"`
	}

	original := payload{T: FlexTime{Time: time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.T.Equal(original.T.Time) {
		t.Errorf("round-trip mismatch: got %v, want %v", decoded.T.Time, original.T.Time)
	}
}
