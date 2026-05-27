package service

import (
	"strings"
	"testing"
	"time"
)

func TestCreateIntervention(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)

	type testCase struct {
		name             string
		interventionType string
		targetID         string
		reason           string
		operator         string
		value            float64

		wantType          string
		wantIDPrefix      string
		wantTargetAgentID string
		wantTargetSymbol  string
		wantTargetSector  string
		wantTargetModelID string
		wantValue         float64
		wantTTLHours      int
		wantExpiresAtSet  bool
	}

	tests := []testCase{
		{
			name:              "pause_agent",
			interventionType:  "pause_agent",
			targetID:          "agent-007",
			reason:            "underperforming in volatile regime",
			operator:          "admin",
			wantType:          "pause_agent",
			wantIDPrefix:      "int-pause-agent-007-",
			wantTargetAgentID: "agent-007",
			wantTTLHours:      24,
			wantExpiresAtSet:  true,
		},
		{
			name:              "resume_agent",
			interventionType:  "resume_agent",
			targetID:          "agent-007",
			reason:            "performance recovered",
			operator:          "admin",
			wantType:          "resume_agent",
			wantIDPrefix:      "int-resume-agent-007-",
			wantTargetAgentID: "agent-007",
			wantTTLHours:      0,
			wantExpiresAtSet:  false,
		},
		{
			name:              "set_model_weight",
			interventionType:  "set_model_weight",
			targetID:          "model-x",
			reason:            "calibration update",
			operator:          "admin",
			value:             1.5,
			wantType:          "set_model_weight",
			wantIDPrefix:      "int-model-model-x-",
			wantTargetModelID: "model-x",
			wantValue:         1.5,
			wantTTLHours:      72,
			wantExpiresAtSet:  true,
		},
		{
			name:             "sector_ban",
			interventionType: "sector_ban",
			targetID:         "semiconductor",
			reason:           "overconcentration risk",
			operator:         "admin",
			wantType:         "sector_ban",
			wantIDPrefix:     "int-sector-semiconductor-",
			wantTargetSector: "semiconductor",
			wantTTLHours:     24,
			wantExpiresAtSet: true,
		},
		{
			name:             "sector_unban",
			interventionType: "sector_unban",
			targetID:         "semiconductor",
			reason:           "risk normalized",
			operator:         "admin",
			wantType:         "sector_unban",
			wantIDPrefix:     "int-sector-semiconductor-",
			wantTargetSector: "semiconductor",
			wantTTLHours:     24,
			wantExpiresAtSet: true,
		},
		{
			name:              "approve_rec_with_colon_format",
			interventionType:  "approve_rec",
			targetID:          "agent-x:2330",
			reason:            "analyst override",
			operator:          "admin",
			wantType:          "approve_rec",
			wantIDPrefix:      "int-approve-agent-x:2330-",
			wantTargetAgentID: "agent-x",
			wantTargetSymbol:  "2330",
			wantTTLHours:      48,
			wantExpiresAtSet:  true,
		},
		{
			name:              "approve_rec_plain_symbol",
			interventionType:  "approve_rec",
			targetID:          "2330",
			reason:            "analyst override",
			operator:          "admin",
			wantType:          "approve_rec",
			wantIDPrefix:      "int-approve-2330-",
			wantTargetAgentID: "", // no agentID part
			wantTargetSymbol:  "2330",
			wantTTLHours:      48,
			wantExpiresAtSet:  true,
		},
		{
			name:              "reject_rec_with_colon_format",
			interventionType:  "reject_rec",
			targetID:          "agent-y:2317",
			reason:            "risk committee block",
			operator:          "admin",
			wantType:          "reject_rec",
			wantIDPrefix:      "int-reject-agent-y:2317-",
			wantTargetAgentID: "agent-y",
			wantTargetSymbol:  "2317",
			wantTTLHours:      48,
			wantExpiresAtSet:  true,
		},
		{
			name:              "reject_rec_plain_symbol",
			interventionType:  "reject_rec",
			targetID:          "2317",
			reason:            "risk committee block",
			operator:          "admin",
			wantType:          "reject_rec",
			wantIDPrefix:      "int-reject-2317-",
			wantTargetAgentID: "", // no agentID part
			wantTargetSymbol:  "2317",
			wantTTLHours:      48,
			wantExpiresAtSet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UTC()
			iv := svc.CreateIntervention(tt.interventionType, tt.targetID, tt.reason, tt.operator, tt.value)
			after := time.Now().UTC()

			// ID checks
			if iv.ID == "" {
				t.Error("ID is empty")
			}
			if !strings.HasPrefix(iv.ID, tt.wantIDPrefix) {
				t.Errorf("ID prefix: got %q, want prefix %q", iv.ID, tt.wantIDPrefix)
			}

			// Type checks
			if iv.Type != tt.wantType {
				t.Errorf("Type: got %q, want %q", iv.Type, tt.wantType)
			}

			// Reason / Operator preserved
			if iv.Reason != tt.reason {
				t.Errorf("Reason: got %q, want %q", iv.Reason, tt.reason)
			}
			if iv.Operator != tt.operator {
				t.Errorf("Operator: got %q, want %q", iv.Operator, tt.operator)
			}

			// Target field checks
			if iv.TargetAgentID != tt.wantTargetAgentID {
				t.Errorf("TargetAgentID: got %q, want %q", iv.TargetAgentID, tt.wantTargetAgentID)
			}
			if iv.TargetSymbol != tt.wantTargetSymbol {
				t.Errorf("TargetSymbol: got %q, want %q", iv.TargetSymbol, tt.wantTargetSymbol)
			}
			if iv.TargetSector != tt.wantTargetSector {
				t.Errorf("TargetSector: got %q, want %q", iv.TargetSector, tt.wantTargetSector)
			}
			if iv.TargetModelID != tt.wantTargetModelID {
				t.Errorf("TargetModelID: got %q, want %q", iv.TargetModelID, tt.wantTargetModelID)
			}

			// Value check
			if iv.Value != tt.wantValue {
				t.Errorf("Value: got %v, want %v", iv.Value, tt.wantValue)
			}

			// TTLHours check
			if iv.TTLHours != tt.wantTTLHours {
				t.Errorf("TTLHours: got %d, want %d", iv.TTLHours, tt.wantTTLHours)
			}

			// RecordedAt must be between before and after (within 1 second tolerance)
			tolerance := 1 * time.Second
			if iv.RecordedAt.Before(before.Add(-tolerance)) {
				t.Errorf("RecordedAt %v is too early (before %v)", iv.RecordedAt, before)
			}
			if iv.RecordedAt.After(after.Add(tolerance)) {
				t.Errorf("RecordedAt %v is too late (after %v)", iv.RecordedAt, after)
			}

			// ExpiresAt check
			if tt.wantExpiresAtSet {
				if iv.ExpiresAt.IsZero() {
					t.Error("ExpiresAt should be set but is zero")
				} else {
					expectedExpiry := iv.RecordedAt.Add(time.Duration(tt.wantTTLHours) * time.Hour)
					// Allow 1 second tolerance for clock resolution
					diff := iv.ExpiresAt.Sub(expectedExpiry)
					if diff < -1*time.Second || diff > 1*time.Second {
						t.Errorf("ExpiresAt: got %v, want ~%v (diff=%v)", iv.ExpiresAt, expectedExpiry, diff)
					}
				}
			} else {
				if !iv.ExpiresAt.IsZero() {
					t.Errorf("ExpiresAt should be zero but got %v", iv.ExpiresAt)
				}
			}
		})
	}
}

func TestMapKeys(t *testing.T) {
	t.Run("non-empty map returns keys", func(t *testing.T) {
		m := map[string]bool{
			"agent-a": true,
			"agent-b": true,
			"agent-c": true,
		}
		keys := mapKeys(m)
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d", len(keys))
		}
		// Check all expected keys are present (order not guaranteed)
		keySet := make(map[string]bool)
		for _, k := range keys {
			keySet[k] = true
		}
		for expectedKey := range m {
			if !keySet[expectedKey] {
				t.Errorf("missing key %q in result", expectedKey)
			}
		}
	})

	t.Run("empty map returns empty slice", func(t *testing.T) {
		keys := mapKeys(map[string]bool{})
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("single-entry map returns one key", func(t *testing.T) {
		m := map[string]bool{"only": true}
		keys := mapKeys(m)
		if len(keys) != 1 {
			t.Errorf("expected 1 key, got %d", len(keys))
		}
		if keys[0] != "only" {
			t.Errorf("expected key %q, got %q", "only", keys[0])
		}
	})
}
