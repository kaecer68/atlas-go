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

// ─── SimulationConstraints JSON canonical snake_case ──────────────────────────

func TestSimulationConstraints_JSONKeys(t *testing.T) {
	orig := SimulationConstraints{
		StartingCash:                1_000_000.0,
		MaxPositionWeight:           0.05,
		MaxOpenPositions:            20,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 5,
		RequireCROPass:              true,
		TransactionCostBPS:          10.0,
		SlippageBPS:                 5.0,
		ReserveCashFraction:         0.10,
		StopLossPct:                 -0.05,
		TakeProfitPct:               0.15,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Parse as generic map to inspect keys
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// Assert exact canonical snake_case keys
	expectedKeys := []string{
		"starting_cash",
		"max_position_weight",
		"max_open_positions",
		"min_tradable_volume",
		"min_recommendation_conviction",
		"require_cro_pass",
		"transaction_cost_bps",
		"slippage_bps",
		"reserve_cash_fraction",
		"stop_loss_pct",
		"take_profit_pct",
	}

	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing expected snake_case key %q; got keys: %v", k, mapKeys(m))
		}
	}

	// Ensure no PascalCase keys leak through
	for k := range m {
		if k != "" && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("PascalCase key %q found — all keys must be snake_case", k)
		}
	}
}

func TestSimulationConstraints_RoundTrip(t *testing.T) {
	orig := SimulationConstraints{
		StartingCash:                1_000_000.0,
		MaxPositionWeight:           0.05,
		MaxOpenPositions:            20,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 5,
		RequireCROPass:              true,
		TransactionCostBPS:          10.0,
		SlippageBPS:                 5.0,
		ReserveCashFraction:         0.10,
		StopLossPct:                 -0.05,
		TakeProfitPct:               0.15,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SimulationConstraints
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.StartingCash != orig.StartingCash {
		t.Errorf("StartingCash = %v, want %v", decoded.StartingCash, orig.StartingCash)
	}
	if decoded.MaxPositionWeight != orig.MaxPositionWeight {
		t.Errorf("MaxPositionWeight = %v, want %v", decoded.MaxPositionWeight, orig.MaxPositionWeight)
	}
	if decoded.MaxOpenPositions != orig.MaxOpenPositions {
		t.Errorf("MaxOpenPositions = %v, want %v", decoded.MaxOpenPositions, orig.MaxOpenPositions)
	}
	if decoded.MinTradableVolume != orig.MinTradableVolume {
		t.Errorf("MinTradableVolume = %v, want %v", decoded.MinTradableVolume, orig.MinTradableVolume)
	}
	if decoded.MinRecommendationConviction != orig.MinRecommendationConviction {
		t.Errorf("MinRecommendationConviction = %v, want %v", decoded.MinRecommendationConviction, orig.MinRecommendationConviction)
	}
	if decoded.RequireCROPass != orig.RequireCROPass {
		t.Errorf("RequireCROPass = %v, want %v", decoded.RequireCROPass, orig.RequireCROPass)
	}
	if decoded.TransactionCostBPS != orig.TransactionCostBPS {
		t.Errorf("TransactionCostBPS = %v, want %v", decoded.TransactionCostBPS, orig.TransactionCostBPS)
	}
	if decoded.SlippageBPS != orig.SlippageBPS {
		t.Errorf("SlippageBPS = %v, want %v", decoded.SlippageBPS, orig.SlippageBPS)
	}
	if decoded.ReserveCashFraction != orig.ReserveCashFraction {
		t.Errorf("ReserveCashFraction = %v, want %v", decoded.ReserveCashFraction, orig.ReserveCashFraction)
	}
	if decoded.StopLossPct != orig.StopLossPct {
		t.Errorf("StopLossPct = %v, want %v", decoded.StopLossPct, orig.StopLossPct)
	}
	if decoded.TakeProfitPct != orig.TakeProfitPct {
		t.Errorf("TakeProfitPct = %v, want %v", decoded.TakeProfitPct, orig.TakeProfitPct)
	}
}

// ─── ExecutionPolicy JSON canonical snake_case ───────────────────────────────

func TestExecutionPolicy_JSONKeys(t *testing.T) {
	orig := ExecutionPolicy{
		ConvictionFloor:               3,
		RequireCROPass:                true,
		MomentumCrashProtection:      true,
		EnableConvictionNormalization: false,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	expectedKeys := []string{
		"conviction_floor",
		"require_cro_pass",
		"momentum_crash_protection",
		"enable_conviction_normalization",
	}

	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing expected snake_case key %q; got keys: %v", k, mapKeys(m))
		}
	}

	for k := range m {
		if k != "" && k[0] >= 'A' && k[0] <= 'Z' {
			t.Errorf("PascalCase key %q found — all keys must be snake_case", k)
		}
	}
}

func TestExecutionPolicy_RoundTrip(t *testing.T) {
	orig := ExecutionPolicy{
		ConvictionFloor:               3,
		RequireCROPass:                true,
		MomentumCrashProtection:      true,
		EnableConvictionNormalization: false,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ExecutionPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ConvictionFloor != orig.ConvictionFloor {
		t.Errorf("ConvictionFloor = %v, want %v", decoded.ConvictionFloor, orig.ConvictionFloor)
	}
	if decoded.RequireCROPass != orig.RequireCROPass {
		t.Errorf("RequireCROPass = %v, want %v", decoded.RequireCROPass, orig.RequireCROPass)
	}
	if decoded.MomentumCrashProtection != orig.MomentumCrashProtection {
		t.Errorf("MomentumCrashProtection = %v, want %v", decoded.MomentumCrashProtection, orig.MomentumCrashProtection)
	}
	if decoded.EnableConvictionNormalization != orig.EnableConvictionNormalization {
		t.Errorf("EnableConvictionNormalization = %v, want %v", decoded.EnableConvictionNormalization, orig.EnableConvictionNormalization)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
