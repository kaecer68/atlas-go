package shared

import (
	"strings"
	"testing"
)

func TestValidatePathComponent(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"safe", "session-20260614_daily.v1:ok", false},
		{"empty", "", true},
		{"slash", "../session", true},
		{"backslash", `bad\path`, true},
		{"dotdot", "bad..path", true},
		{"null", "bad\x00path", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePathComponent(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidatePathComponent(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateExperimentAndSessionID(t *testing.T) {
	longID := strings.Repeat("a", 129)
	cases := []struct {
		name    string
		fn      func(string) error
		input   string
		wantErr bool
	}{
		{"experiment safe", ValidateExperimentID, "exp-2026.06:run_1", false},
		{"experiment slash", ValidateExperimentID, "exp/bad", true},
		{"experiment too long", ValidateExperimentID, longID, true},
		{"session safe", ValidateSessionID, "session-20260614-daily", false},
		{"session space", ValidateSessionID, "session bad", true},
		{"session too long", ValidateSessionID, longID, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validator(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateDateParam(t *testing.T) {
	if err := ValidateDateParam("2026-06-14"); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
	for _, bad := range []string{"20260614", "2026-6-14", "not-a-date"} {
		if err := ValidateDateParam(bad); err == nil {
			t.Fatalf("invalid date %q accepted", bad)
		}
	}
}
