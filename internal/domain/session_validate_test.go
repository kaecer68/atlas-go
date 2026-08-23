package domain

import (
	"strings"
	"testing"
)

func validSummary() SessionSummary {
	return SessionSummary{
		SessionID:      "session-20260710-daily",
		Regime:         RegimeRiskOn,
		OrderCount:     3,
		PositionCount:  2,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
		OutcomeCount:   26,
	}
}

func TestSessionSummaryValidate_Valid(t *testing.T) {
	s := validSummary()
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	for _, r := range []Regime{RegimeRiskOn, RegimeRiskOff, RegimeNeutral} {
		s := validSummary()
		s.Regime = r
		if err := s.Validate(); err != nil {
			t.Errorf("Validate() regime %q: %v", r, err)
		}
	}
}

func TestSessionSummaryValidate_RejectsCorrupted(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SessionSummary)
		wantSub string
	}{
		{"missing session id", func(s *SessionSummary) { s.SessionID = "" }, "missing SessionID"},
		{"zero portfolio value", func(s *SessionSummary) { s.PortfolioValue = 0 }, "PortfolioValue must be > 0"},
		{"negative portfolio value", func(s *SessionSummary) { s.PortfolioValue = -1 }, "PortfolioValue must be > 0"},
		{"negative ending cash", func(s *SessionSummary) { s.EndingCash = -0.01 }, "EndingCash must be >= 0"},
		{"negative outcome count", func(s *SessionSummary) { s.OutcomeCount = -1 }, "OutcomeCount must be >= 0"},
		{"illegal regime", func(s *SessionSummary) { s.Regime = Regime("BULL") }, "illegal regime"},
		{"empty regime", func(s *SessionSummary) { s.Regime = "" }, "missing Regime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSummary()
			tt.mutate(&s)
			err := s.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("Validate() error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestSessionSummaryValidateLegacy_AcceptsLegalZero(t *testing.T) {
	// Legacy count-only row: PortfolioValue=0 + EndingCash=0 + empty regime.
	// Produced by cmd/backfill-summaries and legacy SQLite 5-column rows.
	legacy := SessionSummary{
		SessionID:    "session-20260101-daily",
		OutcomeCount: 42,
	}
	if err := legacy.ValidateLegacy(); err != nil {
		t.Fatalf("ValidateLegacy() rejected legal zero row: %v", err)
	}
	// Full rows also pass.
	full := validSummary()
	if err := full.ValidateLegacy(); err != nil {
		t.Fatalf("ValidateLegacy() rejected full row: %v", err)
	}
	// Zero portfolio with empty regime is fine for legacy…
	s := legacy
	s.Regime = ""
	if err := s.ValidateLegacy(); err != nil {
		t.Fatalf("ValidateLegacy() rejected empty regime legacy row: %v", err)
	}
}

func TestSessionSummaryValidateLegacy_RejectsCorruptedZero(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SessionSummary)
		wantSub string
	}{
		{"missing session id", func(s *SessionSummary) { s.SessionID = "" }, "missing SessionID"},
		{"zero portfolio with cash", func(s *SessionSummary) { s.PortfolioValue = 0; s.EndingCash = 50_000 }, "corrupted zero portfolio"},
		{"zero portfolio with orders", func(s *SessionSummary) { s.PortfolioValue = 0; s.OrderCount = 2 }, "corrupted zero portfolio"},
		{"negative portfolio", func(s *SessionSummary) { s.PortfolioValue = -100 }, "PortfolioValue must be >= 0"},
		{"negative ending cash", func(s *SessionSummary) { s.EndingCash = -1 }, "EndingCash must be >= 0"},
		{"negative outcome count", func(s *SessionSummary) { s.OutcomeCount = -3 }, "OutcomeCount must be >= 0"},
		{"illegal regime", func(s *SessionSummary) { s.Regime = Regime("UNKNOWN") }, "illegal regime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSummary()
			tt.mutate(&s)
			err := s.ValidateLegacy()
			if err == nil {
				t.Fatal("ValidateLegacy() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("ValidateLegacy() error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

// TestSessionSummaryValidate_RejectsCorruptedZero is the SSoT contract check:
// the strict path must NOT silently accept a zero portfolio value even though
// legacy data contains them — the write-time guard exists precisely so new
// corrupted rows cannot enter the pipeline.
func TestSessionSummaryValidate_RejectsZeroPortfolio(t *testing.T) {
	s := validSummary()
	s.PortfolioValue = 0
	if err := s.Validate(); err == nil {
		t.Fatal("Validate() accepted PortfolioValue=0, want error")
	}
}
