package shared

import (
	"strings"
	"testing"
)

func TestValidatePathComponent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantMsg string
	}{
		{"empty", "", true, "must not be empty"},
		{"slash", "foo/bar", true, "forbidden character '/'"},
		{"backslash", "foo\\bar", true, "forbidden character '\\'"},
		{"dotdot", "foo..bar", true, "forbidden sequence '..'"},
		{"null byte", "foo\x00bar", true, "null byte"},
		{"safe", "exp_2024-01-01_v1", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathComponent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateExperimentID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid alphanumeric", "exp_001", false},
		{"valid with hyphen dot colon", "exp-1.0:run", false},
		{"empty", "", true},
		{"slash", "exp/001", true},
		{"too long", strings.Repeat("a", 129), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExperimentID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateExperimentID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "session-20240101-daily", false},
		{"empty", "", true},
		{"space", "session 2024", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSessionID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDateParam(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "2024-01-15", false},
		{"invalid format", "2024/01/15", true},
		{"regex matches invalid month", "2024-13-15", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDateParam(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateDateParam(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
